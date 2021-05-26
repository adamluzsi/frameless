package contracts_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/inmemory"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type getContractsSubject interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	frameless.OnePhaseCommitProtocol
	contracts.UpdaterSubject
	contracts.CreatorPublisherSubject
	contracts.UpdaterPublisherSubject
	contracts.DeleterPublisherSubject
}

func getContracts(T interface{}, ff contracts.FixtureFactory, newSubject func(tb testing.TB) getContractsSubject) []testcase.Contract {
	return []testcase.Contract{
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return newSubject(tb) }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return newSubject(tb) }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return newSubject(tb) }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return newSubject(tb) }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return newSubject(tb) }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return newSubject(tb) }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return newSubject(tb) }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
			s := newSubject(tb)
			return s, s
		}, FixtureFactory: ff},
	}
}

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	ff := fixtures.FixtureFactory{}
	require.NotNil(t, ff.Context())
	require.NotNil(t, ff.Create(Entity{}).(*Entity))

	testcase.RunContract(t, getContracts(Entity{}, ff, func(tb testing.TB) getContractsSubject {
		return inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
	})...)
}

//--------------------------------------------------------------------------------------------------------------------//

func TestFixtureFactory(t *testing.T) {
	t.Run(`With ext:"ID" tag`, func(t *testing.T) {
		type T struct {
			ID   string `ext:"ID"`
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactorySpec{
			Type:           T{},
			FixtureFactory: fixtures.FixtureFactory{},
		})
	})

	t.Run(`without ext id`, func(t *testing.T) {
		type T struct {
			Text string
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactorySpec{
			Type:           T{},
			FixtureFactory: fixtures.FixtureFactory{},
		})
	})
}

//--------------------------------------------------------------------------------------------------------------------//

func TestEventuallyConsistentStorage(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	ff := fixtures.FixtureFactory{}
	require.NotNil(t, ff.Context())
	require.NotNil(t, ff.Create(Entity{}).(*Entity))

	testcase.RunContract(t, getContracts(Entity{}, ff, func(tb testing.TB) getContractsSubject {
		storage := NewEventuallyConsistentStorage(Entity{})
		tb.Cleanup(func() { _ = storage.Close() })
		return storage
	})...)
}

func NewEventuallyConsistentStorage(T interface{}) *EventuallyConsistentStorage {
	e := &EventuallyConsistentStorage{Storage: inmemory.NewStorage(T, inmemory.NewEventLog())}
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

func (e *EventuallyConsistentStorage) DeleteByID(ctx context.Context, id interface{}) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.DeleteByID(ctx, id)
	})
}

func (e *EventuallyConsistentStorage) DeleteAll(ctx context.Context) error {
	if err := e.errOnDoneTx(ctx); err != nil {
		return err
	}
	return e.eventually(ctx, func(ctx context.Context) error {
		return e.Storage.DeleteAll(ctx)
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