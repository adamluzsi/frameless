package storages_test

import (
	"context"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/specs"
	"github.com/adamluzsi/frameless/resources/storages"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestEventuallyConsistentStorage(t *testing.T) {
	if testing.Short() {
		original := specs.AsyncTester
		specs.AsyncTester = testcase.AsyncTester{
			WaitDuration: time.Microsecond,
			WaitTimeout:  5 * time.Second,
		}
		defer func() { specs.AsyncTester = original }()
	}

	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	storage := NewEventuallyConsistentStorage()
	storage.Spawn()
	defer storage.Close()

	ff := fixtures.FixtureFactory{}
	require.NotNil(t, ff.Context())
	require.NotNil(t, ff.Create(Entity{}).(*Entity))

	specs.Run(t,
		specs.Creator{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.CreatorPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.Updater{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.UpdaterPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.Deleter{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.DeleterPublisher{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.Finder{Subject: storage, T: Entity{}, FixtureFactory: ff},
		specs.OnePhaseCommitProtocol{Subject: storage, T: Entity{}, FixtureFactory: ff},
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
	jobs chan func()

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
	close(e.jobs)
	e.nullFn(e.workers.cancel)()
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

	e.jobs <- func() {
		const (
			max = int(time.Millisecond)
			min = int(time.Microsecond)
		)
		time.Sleep(time.Duration(fixtures.Random.IntBetween(min, max)))

		_ = e.Memory.CommitTx(tx)
	}

	return nil
}
