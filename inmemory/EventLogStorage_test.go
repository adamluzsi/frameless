package inmemory_test

import (
	"context"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/stubs"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	cachecontracts "github.com/adamluzsi/frameless/cache/contracts"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/inmemory"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
)

var _ interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	frameless.CreatorPublisher
	frameless.UpdaterPublisher
	frameless.DeleterPublisher
	frameless.OnePhaseCommitProtocol
	cache.EntityStorage
} = &inmemory.EventLogStorage{}

func TestEventLogStorage_smoke(t *testing.T) {
	var (
		subject = inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		ctx     = context.Background()
		count   int
		err     error
	)

	require.Nil(t, subject.Create(ctx, &TestEntity{Data: `A`}))
	require.Nil(t, subject.Create(ctx, &TestEntity{Data: `B`}))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 2, count)

	require.Nil(t, subject.DeleteAll(ctx))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx1CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx1CTX, &TestEntity{Data: `C`}))
	count, err = iterators.Count(subject.FindAll(tx1CTX))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.RollbackTx(tx1CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx2CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx2CTX, &TestEntity{Data: `D`}))
	count, err = iterators.Count(subject.FindAll(tx2CTX))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.CommitTx(tx2CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func getStorageSpecsForT(subject *inmemory.EventLogStorage, T frameless.T, ff contracts.FixtureFactory) []testcase.Contract {
	return []testcase.Contract{
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return subject, subject }, FixtureFactory: ff},
		cachecontracts.EntityStorage{T: T, Subject: func(tb testing.TB) (storage cache.EntityStorage, cpm frameless.OnePhaseCommitProtocol) { return subject, subject.EventLog }, FixtureFactory: ff},
	}
}

func getStoragerySpecs(subject *inmemory.EventLogStorage, T interface{}) []testcase.Contract {
	return getStorageSpecsForT(subject, T, fixtures.FixtureFactory{})
}

func TestEventLogStorage(t *testing.T) {
	for _, spec := range getStoragerySpecs(inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog()), TestEntity{}) {
		spec.Test(t)
	}
}

func TestEventLogStorage_multipleInstanceTransactionOnTheSameContext(t *testing.T) {
	ff := fixtures.FixtureFactory{}

	t.Run(`with create in different tx`, func(t *testing.T) {
		subject1 := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		subject2 := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())

		ctx := context.Background()
		ctx, err := subject1.BeginTx(ctx)
		require.Nil(t, err)
		ctx, err = subject2.BeginTx(ctx)
		require.Nil(t, err)

		t.Log(`when in subject 1 store an entity`)
		entity := &TestEntity{Data: `42`}
		require.Nil(t, subject1.Create(ctx, entity))

		t.Log(`and subject 2 finish tx`)
		require.Nil(t, subject2.CommitTx(ctx))
		t.Log(`and subject 2 then try to find this entity`)
		found, err := subject2.FindByID(context.Background(), &TestEntity{}, entity.ID)
		require.Nil(t, err)
		require.False(t, found, `it should not see the uncommitted entity`)

		t.Log(`but after subject 1 commit the tx`)
		require.Nil(t, subject1.CommitTx(ctx))
		t.Log(`subject 1 can see the newT entity`)
		found, err = subject1.FindByID(context.Background(), &TestEntity{}, entity.ID)
		require.Nil(t, err)
		require.True(t, found)
	})

	t.Run(`deletes across tx instances in the same context`, func(t *testing.T) {
		subject1 := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		subject2 := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())

		ctx := ff.Context()
		e1 := ff.Create(TestEntity{}).(*TestEntity)
		e2 := ff.Create(TestEntity{}).(*TestEntity)

		require.Nil(t, subject1.Create(ctx, e1))
		id1, ok := extid.Lookup(e1)
		require.True(t, ok)
		require.NotEmpty(t, id1)
		t.Cleanup(func() { _ = subject1.DeleteByID(ff.Context(), id1) })

		require.Nil(t, subject2.Create(ctx, e2))
		id2, ok := extid.Lookup(e2)
		require.True(t, ok)
		require.NotEmpty(t, id2)
		t.Cleanup(func() { _ = subject2.DeleteByID(ff.Context(), id2) })

		ctx, err := subject1.BeginTx(ctx)
		require.Nil(t, err)
		ctx, err = subject2.BeginTx(ctx)
		require.Nil(t, err)

		found, err := subject1.FindByID(ctx, &TestEntity{}, id1)
		require.Nil(t, err)
		require.True(t, found)
		require.Nil(t, subject1.DeleteByID(ctx, id1))

		found, err = subject2.FindByID(ctx, &TestEntity{}, id2)
		require.True(t, found)
		require.Nil(t, subject2.DeleteByID(ctx, id2))

		found, err = subject1.FindByID(ctx, &TestEntity{}, id1)
		require.Nil(t, err)
		require.False(t, found)

		found, err = subject2.FindByID(ctx, &TestEntity{}, id2)
		require.Nil(t, err)
		require.False(t, found)

		found, err = subject1.FindByID(ff.Context(), &TestEntity{}, id1)
		require.Nil(t, err)
		require.True(t, found)

		require.Nil(t, subject1.CommitTx(ctx))
		require.Nil(t, subject2.CommitTx(ctx))

		found, err = subject1.FindByID(ff.Context(), &TestEntity{}, id1)
		require.Nil(t, err)
		require.False(t, found)

	})
}

func TestEventLogStorage_Options_CompressEventLog(t *testing.T) {
	memory := inmemory.NewEventLog()
	subject := inmemory.NewEventLogStorage(TestEntity{}, memory)
	subject.Options.CompressEventLog = true

	for _, spec := range getStoragerySpecs(subject, TestEntity{}) {
		spec.Test(t)
	}

	for _, event := range memory.Events() {
		t.Logf("storageID:%s -> event:%#v", subject.StorageID(), event)
	}

	require.Empty(t, memory.Events(),
		`after all the specs, the memory storage was expected to be empty.`+
			` If the storage has values, it means something is not cleaning up properly in the specs.`)
}

func TestEventLogStorage_Options_AsyncSubscriptionHandling(t *testing.T) {
	s := testcase.NewSpec(t)

	var subscriber = func(t *testcase.T) *HangingSubscriber { return t.I(`HangingSubscriber`).(*HangingSubscriber) }
	s.Let(`HangingSubscriber`, func(t *testcase.T) interface{} {
		return NewHangingSubscriber()
	})

	var newStorage = func(t *testcase.T) *inmemory.EventLogStorage {
		s := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		ctx := context.Background()
		subscription, err := s.SubscribeToCreate(ctx, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToUpdate(ctx, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteAll(ctx, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteByID(ctx, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		return s
	}

	var subject = func(t *testcase.T) *inmemory.EventLogStorage {
		s := newStorage(t)
		s.EventLog.Options.DisableAsyncSubscriptionHandling = t.I(`DisableAsyncSubscriptionHandling`).(bool)
		return s
	}

	s.Before(func(t *testcase.T) {
		if testing.Short() {
			t.Skip()
		}
	})

	const hangingDuration = 500 * time.Millisecond

	thenCreateUpdateDeleteWill := func(s *testcase.Spec, willHang bool) {
		var desc string
		if willHang {
			desc = `event is blocking until subscriber finishes handling the event`
		} else {
			desc = `event should not hang while the subscriber is busy`
		}
		desc = ` ` + desc

		var assertion = func(t testing.TB, expected, actual time.Duration) {
			if willHang {
				require.LessOrEqual(t, int64(expected), int64(actual))
			} else {
				require.Greater(t, int64(expected), int64(actual))
			}
		}

		s.Then(`Create`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			require.Nil(t, memory.Create(context.Background(), &TestEntity{Data: `42`}))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`Update`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			ent := TestEntity{Data: `42`}
			require.Nil(t, memory.Create(context.Background(), &ent))
			ent.Data = `foo`

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			require.Nil(t, memory.Update(context.Background(), &ent))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`DeleteByID`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			ent := TestEntity{Data: `42`}
			require.Nil(t, memory.Create(context.Background(), &ent))

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			require.Nil(t, memory.DeleteByID(context.Background(), ent.ID))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`DeleteAll`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			require.Nil(t, memory.DeleteAll(context.Background()))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Test(`E2E`, func(t *testcase.T) {
			testcase.RunContract(t, getStoragerySpecs(subject(t), TestEntity{})...)
		})
	}

	s.When(`is enabled`, func(s *testcase.Spec) {
		s.LetValue(`DisableAsyncSubscriptionHandling`, false)

		thenCreateUpdateDeleteWill(s, false)
	}, testcase.SkipBenchmark())

	s.When(`is disabled`, func(s *testcase.Spec) {
		s.LetValue(`DisableAsyncSubscriptionHandling`, true)

		thenCreateUpdateDeleteWill(s, true)
	})
}

func NewHangingSubscriber() *HangingSubscriber {
	return &HangingSubscriber{}
}

type HangingSubscriber struct {
	m sync.RWMutex
}

func (h *HangingSubscriber) HangFor(d time.Duration) {
	h.m.Lock()
	go func() {
		defer h.m.Unlock()
		<-time.After(d)
	}()
}

func (h *HangingSubscriber) Handle(ctx context.Context, ent interface{}) error {
	h.m.RLock()
	defer h.m.RUnlock()
	return nil
}

func (h *HangingSubscriber) Error(ctx context.Context, err error) error {
	h.m.RLock()
	defer h.m.RUnlock()
	return nil
}

func TestEventLogStorage_NewIDFunc(t *testing.T) {
	t.Run(`when NewID is absent`, func(t *testing.T) {
		storage := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		storage.NewID = nil

		ptr := &TestEntity{Data: "42"}
		require.Nil(t, storage.Create(context.Background(), ptr))
		require.NotEmpty(t, ptr.ID)
	})

	t.Run(`when NewID is provided`, func(t *testing.T) {
		storage := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())
		expectedID := fixtures.Random.String()
		storage.NewID = func(ctx context.Context) (interface{}, error) {
			return expectedID, nil
		}

		ptr := &TestEntity{Data: "42"}
		require.Nil(t, storage.Create(context.Background(), ptr))
		require.Equal(t, expectedID, ptr.ID)
	})
}

func TestEventLogStorage_CompressEvents_smoke(t *testing.T) {
	el := inmemory.NewEventLog()

	type A struct {
		ID string `ext:"ID"`
		V  string
	}
	type B struct {
		ID string `ext:"ID"`
		V  string
	}

	ctx := context.Background()
	aS := inmemory.NewEventLogStorage(A{}, el)
	bS := inmemory.NewEventLogStorage(B{}, el)
	bS.Options.CompressEventLog = true

	a := &A{V: "42"}
	require.Nil(t, aS.Create(ctx, a))
	a.V = "24"
	require.Nil(t, aS.Update(ctx, a))
	require.Nil(t, aS.DeleteByID(ctx, a.ID))
	require.Len(t, el.Events(), 3)

	b := &B{V: "4242"}
	require.Nil(t, bS.Create(ctx, b))
	require.Len(t, el.Events(), 4)
	b.V = "2424"
	require.Nil(t, bS.Update(ctx, b))
	require.Len(t, el.Events(), 4)
	require.Nil(t, bS.DeleteByID(ctx, b.ID))
	require.Len(t, el.Events(), 3)

	aS.Compress()
	require.Len(t, el.Events(), 0, `both storage events are compressed, the event log should be empty`)
}

func TestEventLogStorage_LookupTx(t *testing.T) {
	s := inmemory.NewEventLogStorage(TestEntity{}, inmemory.NewEventLog())

	t.Run(`when outside of tx`, func(t *testing.T) {
		_, ok := s.EventLog.LookupTx(context.Background())
		require.False(t, ok)
	})

	t.Run(`when during tx`, func(t *testing.T) {
		ctx, err := s.BeginTx(context.Background())
		require.Nil(t, err)
		defer s.RollbackTx(ctx)

		e := TestEntity{Data: `42`}
		require.Nil(t, s.Create(ctx, &e))
		found, err := s.FindByID(ctx, &TestEntity{}, e.ID)
		require.Nil(t, err)
		require.True(t, found)
		found, err = s.FindByID(context.Background(), &TestEntity{}, e.ID)
		require.Nil(t, err)
		require.False(t, found)

		_, ok := s.EventLog.LookupTx(ctx)
		require.True(t, ok)
		_, ok = s.View(ctx)[e.ID]
		require.True(t, ok)
	})
}

func TestEventLogStorage_SaveEntityWithCustomKeyType(t *testing.T) {

	storage := inmemory.NewEventLogStorage(EntityWithStructID{}, inmemory.NewEventLog())
	var counter int
	storage.NewID = func(ctx context.Context) (interface{}, error) {
		counter++
		var e EntityWithStructID
		e.ID.V = counter
		return e.ID, nil
	}

	for _, spec := range getStorageSpecsForT(storage, EntityWithStructID{}, FFForEntityWithStructID{}) {
		spec.Test(t)
	}
}

type EntityWithStructID struct {
	ID   struct{ V int } `ext:"ID"`
	Data string
}

type FFForEntityWithStructID struct {
	fixtures.FixtureFactory
}

func (ff FFForEntityWithStructID) Create(T frameless.T) interface{} {
	switch T.(type) {
	case EntityWithStructID:
		ent := ff.FixtureFactory.Create(T).(*EntityWithStructID)
		ent.ID = struct{ V int }{V: fixtures.Random.Int()}
		return ent
	default:
		return ff.FixtureFactory.Create(T)
	}
}

func TestEventLogStorage_implementsCacheDataStorage(t *testing.T) {
	testcase.RunContract(t, cachecontracts.EntityStorage{
		T: TestEntity{},
		Subject: func(tb testing.TB) (cache.EntityStorage, frameless.OnePhaseCommitProtocol) {
			eventLog := inmemory.NewEventLog()
			storage := inmemory.NewEventLogStorage(TestEntity{}, eventLog)
			inmemory.LogHistoryOnFailure(tb, eventLog)
			return storage, eventLog
		},
		FixtureFactory: fixtures.FixtureFactory{},
	})
}

func TestEventLogStorage_Create_withAsyncSubscriptions(t *testing.T) {
	eventLog := inmemory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = false
	storage := inmemory.NewEventLogStorage(TestEntity{}, eventLog)
	ctx := context.Background()

	sub, err := storage.SubscribeToCreate(ctx, stubs.Subscriber{})
	require.Nil(t, err)
	t.Cleanup(func() { require.Nil(t, sub.Close()) })

	ent := TestEntity{Data: fixtures.Random.StringN(4)}
	require.Nil(t, storage.Create(ctx, &ent))
	contracts.IsFindable(t, TestEntity{}, storage, ctx, ent.ID)
}

func TestEventLogStorage_multipleStorageForSameEntity(t *testing.T) {
	ctx := context.Background()
	eventLog := inmemory.NewEventLog()
	s1 := inmemory.NewEventLogStorage(TestEntity{}, eventLog)
	s2 := inmemory.NewEventLogStorage(TestEntity{}, eventLog)
	ent := fixtures.FixtureFactory{}.Create(TestEntity{}).(*TestEntity)
	contracts.CreateEntity(t, s1, ctx, ent)
	contracts.IsAbsent(t, TestEntity{}, s2, ctx, contracts.HasID(t, ent))
}
