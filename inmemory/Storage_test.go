package inmemory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
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
} = &inmemory.Storage{}

func TestStorage_smokeTest(t *testing.T) {
	var (
		subject = inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
		ctx     = context.Background()
		count   int
		err     error
	)

	require.Nil(t, subject.Create(ctx, &Entity{Data: `A`}))
	require.Nil(t, subject.Create(ctx, &Entity{Data: `B`}))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 2, count)

	require.Nil(t, subject.DeleteAll(ctx))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx1CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx1CTX, &Entity{Data: `C`}))
	count, err = iterators.Count(subject.FindAll(tx1CTX))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.RollbackTx(tx1CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx2CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx2CTX, &Entity{Data: `D`}))
	count, err = iterators.Count(subject.FindAll(tx2CTX))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.CommitTx(tx2CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func getStorageSpecsForT(subject *inmemory.Storage, T frameless.T, ff contracts.FixtureFactory) []testcase.Contract {
	return []testcase.Contract{
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return subject, subject }, FixtureFactory: ff},
	}
}

func getStoragerySpecs(subject *inmemory.Storage, T interface{}) []testcase.Contract {
	return getStorageSpecsForT(subject, T, fixtures.FixtureFactory{})
}

func TestStorage(t *testing.T) {
	for _, spec := range getStoragerySpecs(inmemory.NewStorage(Entity{}, inmemory.NewEventLog()), Entity{}) {
		spec.Test(t)
	}
}

func TestStorage_multipleInstanceTransactionOnTheSameContext(t *testing.T) {
	ff := fixtures.FixtureFactory{}

	t.Run(`with create in different tx`, func(t *testing.T) {
		subject1 := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
		subject2 := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())

		ctx := context.Background()
		ctx, err := subject1.BeginTx(ctx)
		require.Nil(t, err)
		ctx, err = subject2.BeginTx(ctx)
		require.Nil(t, err)

		t.Log(`when in subject 1 store an entity`)
		entity := &Entity{Data: `42`}
		require.Nil(t, subject1.Create(ctx, entity))

		t.Log(`and subject 2 finish tx`)
		require.Nil(t, subject2.CommitTx(ctx))
		t.Log(`and subject 2 then try to find this entity`)
		found, err := subject2.FindByID(context.Background(), &Entity{}, entity.ID)
		require.Nil(t, err)
		require.False(t, found, `it should not see the uncommitted entity`)

		t.Log(`but after subject 1 commit the tx`)
		require.Nil(t, subject1.CommitTx(ctx))
		t.Log(`subject 1 can see the newT entity`)
		found, err = subject1.FindByID(context.Background(), &Entity{}, entity.ID)
		require.Nil(t, err)
		require.True(t, found)
	})

	t.Run(`deletes across tx instances in the same context`, func(t *testing.T) {
		subject1 := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
		subject2 := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())

		ctx := ff.Context()
		e1 := ff.Create(Entity{}).(*Entity)
		e2 := ff.Create(Entity{}).(*Entity)

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

		found, err := subject1.FindByID(ctx, &Entity{}, id1)
		require.Nil(t, err)
		require.True(t, found)
		require.Nil(t, subject1.DeleteByID(ctx, id1))

		found, err = subject2.FindByID(ctx, &Entity{}, id2)
		require.True(t, found)
		require.Nil(t, subject2.DeleteByID(ctx, id2))

		found, err = subject1.FindByID(ctx, &Entity{}, id1)
		require.Nil(t, err)
		require.False(t, found)

		found, err = subject2.FindByID(ctx, &Entity{}, id2)
		require.Nil(t, err)
		require.False(t, found)

		found, err = subject1.FindByID(ff.Context(), &Entity{}, id1)
		require.Nil(t, err)
		require.True(t, found)

		require.Nil(t, subject1.CommitTx(ctx))
		require.Nil(t, subject2.CommitTx(ctx))

		found, err = subject1.FindByID(ff.Context(), &Entity{}, id1)
		require.Nil(t, err)
		require.False(t, found)

	})
}

func TestStorage_Options_EventLogging_disable(t *testing.T) {
	memory := inmemory.NewEventLog()
	subject := inmemory.NewStorage(Entity{}, memory)
	subject.Options.DisableEventLogging = true

	for _, spec := range getStoragerySpecs(subject, Entity{}) {
		spec.Test(t)
	}

	require.Empty(t, memory.Events(),
		`after all the specs, the memory storage was expected to be empty.`+
			` If the storage has values, it means something is not cleaning up properly in the specs.`)
}

func TestStorage_Options_AsyncSubscriptionHandling(t *testing.T) {
	s := testcase.NewSpec(t)

	var subscriber = func(t *testcase.T) *HangingSubscriber { return t.I(`HangingSubscriber`).(*HangingSubscriber) }
	s.Let(`HangingSubscriber`, func(t *testcase.T) interface{} {
		return NewHangingSubscriber()
	})

	var newStorage = func(t *testcase.T) *inmemory.Storage {
		s := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
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

	var subject = func(t *testcase.T) *inmemory.Storage {
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
			require.Nil(t, memory.Create(context.Background(), &Entity{Data: `42`}))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`Update`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			ent := Entity{Data: `42`}
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

			ent := Entity{Data: `42`}
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
			testcase.RunContract(t, getStoragerySpecs(subject(t), Entity{})...)
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

func TestStorage_NewIDFunc(t *testing.T) {
	t.Run(`when NewID is absent`, func(t *testing.T) {
		storage := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
		storage.NewID = nil

		ptr := &Entity{Data: "42"}
		require.Nil(t, storage.Create(context.Background(), ptr))
		require.NotEmpty(t, ptr.ID)
	})

	t.Run(`when NewID is provided`, func(t *testing.T) {
		storage := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())
		expectedID := fixtures.Random.String()
		storage.NewID = func(ctx context.Context) (interface{}, error) {
			return expectedID, nil
		}

		ptr := &Entity{Data: "42"}
		require.Nil(t, storage.Create(context.Background(), ptr))
		require.Equal(t, expectedID, ptr.ID)
	})
}

func TestStorage_CompressEvents_smokeTest(t *testing.T) {
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
	aS := inmemory.NewStorage(A{}, el)
	bS := inmemory.NewStorage(B{}, el)
	bS.Options.DisableEventLogging = true

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

	aS.CompressEvents()
	require.Len(t, el.Events(), 0, `both storage events are compressed, the event log should be empty`)
}

func TestStorage_LookupTx(t *testing.T) {
	s := inmemory.NewStorage(Entity{}, inmemory.NewEventLog())

	t.Run(`when outside of tx`, func(t *testing.T) {
		_, ok := s.EventLog.LookupTx(context.Background())
		require.False(t, ok)
	})

	t.Run(`when during tx`, func(t *testing.T) {
		ctx, err := s.BeginTx(context.Background())
		require.Nil(t, err)
		defer s.RollbackTx(ctx)

		e := Entity{Data: `42`}
		require.Nil(t, s.Create(ctx, &e))
		found, err := s.FindByID(ctx, &Entity{}, e.ID)
		require.Nil(t, err)
		require.True(t, found)
		found, err = s.FindByID(context.Background(), &Entity{}, e.ID)
		require.Nil(t, err)
		require.False(t, found)

		_, ok := s.EventLog.LookupTx(ctx)
		require.True(t, ok)
		_, ok = s.View(ctx)[e.ID]
		require.True(t, ok)
	})
}

type Entity struct {
	ID   string `ext:"Namespace"`
	Data string
}

func TestStorage_SaveEntityWithCustomKeyType(t *testing.T) {
	for _, spec := range getStorageSpecsForT(inmemory.NewStorage(EntityWithStructID{}, inmemory.NewEventLog()), EntityWithStructID{}, FFForEntityWithStructID{}) {
		spec.Test(t)
	}
}

type EntityWithStructID struct {
	ID   struct{ V int } `ext:"Namespace"`
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
