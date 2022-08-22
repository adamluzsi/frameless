package contracts_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/adapters"
	inmemory2 "github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func TestEventuallyConsistentStorage(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	testcase.RunSuite(t, adapters.Contract[Entity, string, string]{
		Subject: ContractSubjectFnEventuallyConsistentStorage[Entity, string](),
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeEnt: func(tb testing.TB) Entity {
			t := tb.(*testcase.T)
			return Entity{Data: t.Random.String()}
		},
		MakeV: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	})
}

func ContractSubjectFnEventuallyConsistentStorage[Ent, ID any]() func(testing.TB) adapters.ContractSubject[Ent, ID] {
	return func(tb testing.TB) adapters.ContractSubject[Ent, ID] {
		eventLog := inmemory2.NewEventLog()
		storage := &EventuallyConsistentStorage[Ent, ID]{EventLogStorage: inmemory2.NewEventLogStorage[Ent, ID](eventLog)}
		storage.jobs.queue = make(chan func(), 100)
		storage.Spawn()
		tb.Cleanup(func() { assert.Must(tb).Nil(storage.Close()) })
		return adapters.ContractSubject[Ent, ID]{
			Resource:      storage,
			MetaAccessor:  eventLog,
			CommitManager: storage,
		}
	}
}

type EventuallyConsistentStorage[Ent, ID any] struct {
	*inmemory2.EventLogStorage[Ent, ID]
	jobs struct {
		queue chan func()
		wg    sync.WaitGroup
	}
	workers struct {
		cancel func()
	}
	closed bool
}

func (e *EventuallyConsistentStorage[Ent, ID]) Spawn() {
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

func (e *EventuallyConsistentStorage[Ent, ID]) nullFn(fn func()) func() {
	return func() {
		if fn != nil {
			fn()
		}
	}
}

func (e *EventuallyConsistentStorage[Ent, ID]) Close() error {
	e.jobs.wg.Wait()
	e.nullFn(e.workers.cancel)()
	close(e.jobs.queue)
	e.closed = true
	return nil
}

func (e *EventuallyConsistentStorage[Ent, ID]) Create(ctx context.Context, ptr *Ent) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.Create(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage[Ent, ID]) Update(ctx context.Context, ptr *Ent) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.Update(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage[Ent, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogStorage.DeleteByID(ctx, id)
	})
}

func (e *EventuallyConsistentStorage[Ent, ID]) DeleteAll(ctx context.Context) error {
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

func (e *EventuallyConsistentStorage[Ent, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	if err := e.errOnDoneTx(ctx); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, eventuallyConsistentStorageTxKey{}, &eventuallyConsistentStorageTxValue{})
	return e.EventLog.BeginTx(ctx)
}

func (e *EventuallyConsistentStorage[Ent, ID]) errOnDoneTx(ctx context.Context) error {
	if v, ok := e.lookupTx(ctx); ok && v.done {
		return errors.New(`tx is already done`)
	}
	return nil
}

func (e *EventuallyConsistentStorage[Ent, ID]) lookupTx(ctx context.Context) (*eventuallyConsistentStorageTxValue, bool) {
	v, ok := ctx.Value(eventuallyConsistentStorageTxKey{}).(*eventuallyConsistentStorageTxValue)
	return v, ok
}

func (e *EventuallyConsistentStorage[Ent, ID]) CommitTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.CommitTx(tx)
}

func (e *EventuallyConsistentStorage[Ent, ID]) RollbackTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.RollbackTx(tx)
}

func (e *EventuallyConsistentStorage[Ent, ID]) worker(ctx context.Context, wg *sync.WaitGroup) {
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

func (e *EventuallyConsistentStorage[Ent, ID]) eventually(ctx context.Context, fn func(ctx context.Context) error) error {
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
		time.Sleep(time.Duration(random.New(random.CryptoSeed{}).IntBetween(min, max)))
		_ = e.EventLog.CommitTx(tx)
	}

	return nil
}
