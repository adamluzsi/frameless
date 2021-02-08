package storages_test

import (
	"context"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/contracts"
	"github.com/adamluzsi/frameless/resources/storages"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestEventuallyConsistentStorage(t *testing.T) {
	if testing.Short() {
		original := contracts.AsyncTester
		contracts.AsyncTester = testcase.Retry{Strategy: testcase.Waiter{
			WaitDuration: time.Microsecond,
			WaitTimeout:  5 * time.Second,
		}}
		defer func() { contracts.AsyncTester = original }()
	}

	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	storage := NewEventuallyConsistentStorage()
	storage.Spawn()
	t.Cleanup(func() { storage.Close() })
	//defer storage.Close()

	ff := fixtures.FixtureFactory{}
	require.NotNil(t, ff.Context())
	require.NotNil(t, ff.Create(Entity{}).(*Entity))

	testcase.RunContracts(t,
		contracts.Creator{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.CreatorPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.Updater{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.UpdaterPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.Deleter{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.DeleterPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.Finder{Subject: storage, T: Entity{}, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{Subject: storage, T: Entity{}, FixtureFactory: ff},
	)
}

func NewEventuallyConsistentStorage() *EventuallyConsistentStorage {
	return &EventuallyConsistentStorage{
		Memory: storages.NewMemory(),
		jobs:   make(chan func(), 100),
	}
}

type EventuallyConsistentStorage struct {
	*storages.Memory
	jobs    chan func()
	workers struct {
		cancel func()
	}
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
	e.nullFn(e.workers.cancel)()
	close(e.jobs)
	return nil
}

func (e *EventuallyConsistentStorage) Create(ctx context.Context, ptr interface{}) error {
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Memory.Create(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) Update(ctx context.Context, ptr interface{}) error {
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Memory.Update(ctx, ptr)
	})
}

func (e *EventuallyConsistentStorage) DeleteByID(ctx context.Context, T resources.T, id interface{}) error {
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Memory.DeleteByID(ctx, T, id)
	})
}

func (e *EventuallyConsistentStorage) DeleteAll(ctx context.Context, T resources.T) error {
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Memory.DeleteAll(ctx, T)
	})
}

type eventuallyConsistentStorageTxKey struct{} //--> *sync.WaitGroup

func (e *EventuallyConsistentStorage) BeginTx(ctx context.Context) (context.Context, error) {
	ctx = context.WithValue(ctx, eventuallyConsistentStorageTxKey{}, &sync.WaitGroup{})
	return e.Memory.BeginTx(ctx)
}

func (e *EventuallyConsistentStorage) txlock(tx context.Context) *sync.WaitGroup {
	txWG, ok := tx.Value(eventuallyConsistentStorageTxKey{}).(*sync.WaitGroup)
	if !ok {
		return &sync.WaitGroup{}
	}
	return txWG
}

func (e *EventuallyConsistentStorage) CommitTx(tx context.Context) error {
	e.txlock(tx).Wait()
	return e.Memory.CommitTx(tx)
}

func (e *EventuallyConsistentStorage) RollbackTx(tx context.Context) error {
	e.txlock(tx).Wait()
	return e.Memory.RollbackTx(tx)
}

func (e *EventuallyConsistentStorage) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

wrk:
	for {
		select {
		case <-ctx.Done():
			break wrk
		case fn, ok := <-e.jobs:
			if !ok {
				break wrk
			}
			fn()
		}
	}
}

func (e *EventuallyConsistentStorage) eventually(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := e.Memory.BeginTx(ctx)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = e.Memory.RollbackTx(tx)
		return err
	}

	wg := e.txlock(tx)
	wg.Add(1)
	e.jobs <- func() {
		defer wg.Done()
		const (
			max = int(time.Millisecond)
			min = int(time.Microsecond)
		)
		time.Sleep(time.Duration(fixtures.Random.IntBetween(min, max)))
		_ = e.Memory.CommitTx(tx)
	}

	return nil
}
