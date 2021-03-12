package storages_test

import (
	"context"
	"errors"
	"runtime/debug"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/contracts"
	"github.com/adamluzsi/frameless/resources/storages/inmemory"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func TestEventuallyConsistentStorage(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	newStorage := func(tb testing.TB) *EventuallyConsistentStorage {
		storage := NewEventuallyConsistentStorage()
		tb.Cleanup(func() { _ = storage.Close() })
		return storage
	}

	ff := fixtures.FixtureFactory{}
	require.NotNil(t, ff.Context())
	require.NotNil(t, ff.Create(Entity{}).(*Entity))

	testcase.RunContract(t,
		contracts.Creator{Subject: func(tb testing.TB) contracts.CRD { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.CreatorPublisher{Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.Updater{Subject: func(tb testing.TB) contracts.UpdaterSubject { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.UpdaterPublisher{Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.Deleter{Subject: func(tb testing.TB) contracts.CRD { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.DeleterPublisher{Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.Finder{Subject: func(tb testing.TB) contracts.CRD { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject { return newStorage(tb) }, T: Entity{}, FixtureFactory: ff},
	)
}

func NewEventuallyConsistentStorage() *EventuallyConsistentStorage {
	e := &EventuallyConsistentStorage{Storage: inmemory.New()}
	e.jobs.queue = make(chan func(), 100)
	e.Spawn()
	return e
}

type EventuallyConsistentStorage struct {
	*inmemory.Storage
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

func (e *EventuallyConsistentStorage) Create(ctx context.Context, ptr interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.Create(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) Update(ctx context.Context, ptr interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.Update(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) DeleteByID(ctx context.Context, T resources.T, id interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.DeleteByID(ctx, T, id)
	})
}

func (e *EventuallyConsistentStorage) DeleteAll(ctx context.Context, T resources.T) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.DeleteAll(ctx, T)
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
	return e.Storage.BeginTx(ctx)
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
	return e.Storage.CommitTx(tx)
}

func (e *EventuallyConsistentStorage) RollbackTx(tx context.Context) error {
	if v, ok := e.lookupTx(tx); ok {
		v.WaitGroup.Wait()
		v.done = true
	}
	return e.Storage.RollbackTx(tx)
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
		debug.PrintStack()
		return errors.New(`closed`)
	}

	tx, err := e.Storage.BeginTx(ctx)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = e.Storage.RollbackTx(tx)
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
		_ = e.Storage.CommitTx(tx)
	}

	return nil
}
