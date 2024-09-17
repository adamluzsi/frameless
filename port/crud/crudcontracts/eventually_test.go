package crudcontracts_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestEventuallyConsistentResource(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	eventLog := memory.NewEventLog()
	repo := &EventuallyConsistentResource[Entity, string]{EventLogRepository: memory.NewEventLogRepository[Entity, string](eventLog)}
	repo.jobs.queue = make(chan func(), 100)
	repo.Spawn()
	t.Cleanup(func() { assert.Must(t).Nil(repo.Close()) })

	testcase.RunSuite(t, resource.Contract[Entity, string](repo, resource.Config[Entity, string]{
		MetaAccessor:  eventLog,
		CommitManager: repo,
	}))
}

type EventuallyConsistentResource[Entity, ID any] struct {
	*memory.EventLogRepository[Entity, ID]
	jobs struct {
		queue chan func()
		wg    sync.WaitGroup
	}
	workers struct {
		cancel func()
	}
	closed bool
}

func (e *EventuallyConsistentResource[Entity, ID]) Spawn() {
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

func (e *EventuallyConsistentResource[Entity, ID]) nullFn(fn func()) func() {
	return func() {
		if fn != nil {
			fn()
		}
	}
}

func (e *EventuallyConsistentResource[Entity, ID]) Close() error {
	e.jobs.wg.Wait()
	e.nullFn(e.workers.cancel)()
	close(e.jobs.queue)
	e.closed = true
	return nil
}

func (e *EventuallyConsistentResource[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogRepository.Create(ctx, ptr)
	})
}

func (e *EventuallyConsistentResource[Entity, ID]) Save(ctx context.Context, ptr *Entity) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogRepository.Save(ctx, ptr)
	})
}

func (e *EventuallyConsistentResource[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogRepository.Update(ctx, ptr)
	})
}

func (e *EventuallyConsistentResource[Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogRepository.DeleteByID(ctx, id)
	})
}

func (e *EventuallyConsistentResource[Entity, ID]) DeleteAll(ctx context.Context) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.EventLogRepository.DeleteAll(ctx)
	})
}

type (
	eventuallyConsistentResourceTxKey   struct{}
	eventuallyConsistentResourceTxValue struct {
		sync.WaitGroup
		done bool
	}
)

func (e *EventuallyConsistentResource[Entity, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	if err := e.errOnDoneTx(ctx); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, eventuallyConsistentResourceTxKey{}, &eventuallyConsistentResourceTxValue{})
	return e.EventLog.BeginTx(ctx)
}

func (e *EventuallyConsistentResource[Entity, ID]) errOnDoneTx(ctx context.Context) error {
	if v, ok := e.lookupTx(ctx); ok && v.done {
		return errors.New(`comproto is already done`)
	}
	return nil
}

func (e *EventuallyConsistentResource[Entity, ID]) lookupTx(ctx context.Context) (*eventuallyConsistentResourceTxValue, bool) {
	v, ok := ctx.Value(eventuallyConsistentResourceTxKey{}).(*eventuallyConsistentResourceTxValue)
	return v, ok
}

func (e *EventuallyConsistentResource[Entity, ID]) CommitTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.CommitTx(tx)
}

func (e *EventuallyConsistentResource[Entity, ID]) RollbackTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.EventLog.RollbackTx(tx)
}

func (e *EventuallyConsistentResource[Entity, ID]) worker(ctx context.Context, wg *sync.WaitGroup) {
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

func (e *EventuallyConsistentResource[Entity, ID]) eventually(ctx context.Context, fn func(ctx context.Context) error) error {
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
