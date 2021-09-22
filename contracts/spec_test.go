package contracts_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/inmemory"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type ContractsSubject struct {
	frameless.OnePhaseCommitProtocol
	frameless.MetaAccessor
	CRUD interface {
		frameless.Creator
		frameless.Finder
		frameless.Updater
		frameless.Deleter
	}
	PublisherSubject interface {
		contracts.PublisherSubject
		contracts.UpdaterSubject
	}
}

func getContracts(T interface{}, ff func(tb testing.TB) frameless.FixtureFactory, c func(testing.TB) context.Context, newSubject func(tb testing.TB) ContractsSubject) []testcase.Contract {
	return []testcase.Contract{
		contracts.Creator{T: T,
			Subject:        func(tb testing.TB) contracts.CRD { return newSubject(tb).CRUD },
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.Publisher{T: T,
			Subject:        func(tb testing.TB) contracts.PublisherSubject { return newSubject(tb).PublisherSubject },
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.Updater{T: T,
			Subject:        func(tb testing.TB) contracts.UpdaterSubject { return newSubject(tb).PublisherSubject },
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.Deleter{T: T,
			Subject:        func(tb testing.TB) contracts.CRD { return newSubject(tb).CRUD },
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.Finder{T: T,
			Subject:        func(tb testing.TB) contracts.CRD { return newSubject(tb).CRUD },
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.OnePhaseCommitProtocol{T: T,
			Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
				s := newSubject(tb)
				return s.OnePhaseCommitProtocol, s.CRUD
			},
			FixtureFactory: ff,
			Context:        c,
		},
		contracts.MetaAccessor{T: T,
			V: "", // [string] but should work with other types as well
			Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
				s := newSubject(tb)
				return contracts.MetaAccessorSubject{
					MetaAccessor: s.MetaAccessor,
					Resource:     s.CRUD,
					Publisher:    s.PublisherSubject,
				}
			},
			FixtureFactory: ff,
			Context:        c,
		},
	}
}

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	T := Entity{}
	testcase.RunContract(t, getContracts(T, func(tb testing.TB) frameless.FixtureFactory {
		ff := fixtures.NewFactory(tb)
		require.NotNil(t, ff.Context())
		require.NotEmpty(t, ff.Create(T).(Entity))
		return ff
	}, func(tb testing.TB) context.Context {
		return context.Background()
	}, NewEventLogStorageContractSubject(T))...)
}

func NewEventLogStorageContractSubject(T interface{}) func(testing.TB) ContractsSubject {
	return func(tb testing.TB) ContractsSubject {
		eventLog := inmemory.NewEventLog()
		storage := inmemory.NewEventLogStorage(T, eventLog)
		return ContractsSubject{
			OnePhaseCommitProtocol: eventLog,
			MetaAccessor:           eventLog,
			CRUD:                   storage,
			PublisherSubject:       storage,
		}
	}
}

//--------------------------------------------------------------------------------------------------------------------//

func TestFixtureFactory(t *testing.T) {
	t.Run(`With ext:"ID" tag`, func(t *testing.T) {
		type T struct {
			ID   string `ext:"ID"`
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactory{
			T: T{},
			Subject: func(tb testing.TB) frameless.FixtureFactory {
				return fixtures.NewFactory(tb)
			},
			Context: func(tb testing.TB) context.Context {
				return context.Background()
			},
		})
	})

	t.Run(`without ext id`, func(t *testing.T) {
		type T struct {
			Text string
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactory{
			T: T{},
			Subject: func(tb testing.TB) frameless.FixtureFactory {
				return fixtures.NewFactory(tb)
			},
			Context: func(tb testing.TB) context.Context {
				return context.Background()
			},
		})
	})
}

//--------------------------------------------------------------------------------------------------------------------//

func TestEventuallyConsistentStorage(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}
	T := Entity{}
	testcase.RunContract(t, getContracts(T,
		func(tb testing.TB) frameless.FixtureFactory {
			ff := fixtures.NewFactory(tb)
			require.NotNil(t, ff.Context())
			require.NotEmpty(t, ff.Create(T).(Entity))
			return ff
		},
		func(tb testing.TB) context.Context {
			return context.Background()
		},
		func(tb testing.TB) ContractsSubject {
			eventLog := inmemory.NewEventLog()
			storage := NewEventuallyConsistentStorage(T, eventLog)
			tb.Cleanup(func() { _ = storage.Close() })
			return ContractsSubject{
				// EventuallyConsistentStorage must be used as commit manager
				// because the async go jobs requires waiting in the .CommitTx.
				OnePhaseCommitProtocol: storage,
				MetaAccessor:           eventLog,
				CRUD:                   storage,
				PublisherSubject:       storage,
			}
		})...)
}

func NewEventuallyConsistentStorage(T interface{}, eventLog *inmemory.EventLog) *EventuallyConsistentStorage {
	storage := &EventuallyConsistentStorage{EventLogStorage: inmemory.NewEventLogStorage(T, eventLog)}
	storage.jobs.queue = make(chan func(), 100)
	storage.Spawn()
	return storage
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

func (e *EventuallyConsistentStorage) Create(ctx context.Context, ptr interface{}) error {
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

func TestFixtureFactory_testcaseTNestingSupport(t *testing.T) {
	s := testcase.NewSpec(t)
	type Entity struct {
		ID      string `ext:"id"`
		X, Y, Z string
	}

	v := s.Let(`TestFixtureFactory_testcaseTNestingSupport#var`, func(t *testcase.T) interface{} { return 42 })
	vGet := func(t *testcase.T) int { return v.Get(t).(int) }
	varWithNoInit := testcase.Var{Name: "var_with_no_init"}
	varWithNoInit.LetValue(s, 42)

	spechelper.Contract{T: Entity{}, V: "string",
		Subject: func(tb testing.TB) spechelper.ContractSubject {
			t, ok := tb.(*testcase.T)
			require.True(t, ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			require.Equal(t, 42, vGet(t))
			require.Equal(t, 42, varWithNoInit.Get(t).(int))
			el := inmemory.NewEventLog()
			stg := inmemory.NewEventLogStorage(Entity{}, el)
			return spechelper.ContractSubject{
				MetaAccessor:           el,
				OnePhaseCommitProtocol: el,
				CRUD:                   stg,
			}
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			t, ok := tb.(*testcase.T)
			require.True(t, ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			require.Equal(t, 42, vGet(t))
			require.Equal(t, 42, varWithNoInit.Get(t).(int))
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)
}
