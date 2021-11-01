package spechelper

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase/assert"
)

func ContractSubjectFnEventuallyConsistentStorage(T interface{}) func(testing.TB) ContractSubject {
	return func(tb testing.TB) ContractSubject {
		eventLog := inmemory.NewEventLog()
		storage := &EventuallyConsistentStorage{EventLogStorage: inmemory.NewEventLogStorage(T, eventLog)}
		storage.jobs.queue = make(chan func(), 100)
		storage.Spawn()
		tb.Cleanup(func() { assert.Must(tb).Nil(storage.Close()) })
		return ContractSubject{
			CRUD:                   storage,
			MetaAccessor:           eventLog,
			OnePhaseCommitProtocol: storage,
		}
	}
}

type EventuallyConsistentStorage struct {
	*inmemory.EventLogStorage
	jobs struct {
		queue chan func()
		wg    sync.WaitGroup
	}
	workers struct {
		cancel func()
	}
	closed bool
}

func (e *EventuallyConsistentStorage) Spawn() {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	currentCancel := e.nullFn(e.workers.cancel)
	e.workers.cancel = func() {
		currentCancel()
		cancel()
		wg.Wait()
	}

	wg.Add(1)
	go e.worker(ctx, &wg)
}

func (e *EventuallyConsistentStorage) nullFn(fn func()) func() {
	return func() {
		if fn != nil {
			fn()
		}
	}
}

func (e *EventuallyConsistentStorage) Close() error {
	e.jobs.wg.Wait()
	e.nullFn(e.workers.cancel)()
	close(e.jobs.queue)
	e.closed = true
	return nil
}

func (e *EventuallyConsistentStorage) Create(ctx context.Context, ptr interface{}, opts ...interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.Create(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) Update(ctx context.Context, ptr interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.Update(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) DeleteByID(ctx context.Context, id interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.DeleteByID(ctx, id)
	})
}

func (e *EventuallyConsistentStorage) DeleteAll(ctx context.Context) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.DeleteAll(ctx)
	})
}

type (
	eventuallyConsistentStorageTxKey   struct{}
	eventuallyConsistentStorageTxValue struct {
		sync.WaitGroup
		done bool
	}
)

func (e *EventuallyConsistentStorage) BeginTx(ctx context.Context) (context.Context, error) {
	if err := e.errOnDoneTx(ctx); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, eventuallyConsistentStorageTxKey{}, &eventuallyConsistentStorageTxValue{})
	return e.EventLog.BeginTx(ctx)
}

func (e *EventuallyConsistentStorage) errOnDoneTx(ctx context.Context) error {
	if v, ok := e.lookupTx(ctx); ok && v.done {
		return errors.New(`tx is already done`)
	}
	return nil
}

func (e *EventuallyConsistentStorage) lookupTx(ctx context.Context) (*eventuallyConsistentStorageTxValue, bool) {
	v, ok := ctx.Value(eventuallyConsistentStorageTxKey{}).(*eventuallyConsistentStorageTxValue)
	return v, ok
}

func (e *EventuallyConsistentStorage) CommitTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.CommitTx(tx)
}

func (e *EventuallyConsistentStorage) RollbackTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.RollbackTx(tx)
}

func (e *EventuallyConsistentStorage) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

wrk:
	for {
		select {
		case <-ctx.Done():
			break wrk
		case fn, ok := <-e.jobs.queue:
			if !ok {
				break wrk
			}
			fn()
		}
	}
}

func (e *EventuallyConsistentStorage) eventually(ctx context.Context, fn func(ctx context.Context) error) error {
	if e.closed {
		return errors.New(`closed`)
	}

	tx, err := e.EventLog.BeginTx(ctx)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = e.EventLog.RollbackTx(tx)
		return err
	}

	var txWG = &sync.WaitGroup{}
	if v, ok := e.lookupTx(tx); ok {
		txWG = &v.WaitGroup
	}

	txWG.Add(1)
	e.jobs.wg.Add(1)

	e.jobs.queue <- func() {
		defer e.jobs.wg.Done()
		defer txWG.Done()

		const (
			max = int(time.Millisecond)
			min = int(time.Microsecond)
		)
		time.Sleep(time.Duration(fixtures.Random.IntBetween(min, max)))
		_ = e.EventLog.CommitTx(tx)
	}

	return nil
}
