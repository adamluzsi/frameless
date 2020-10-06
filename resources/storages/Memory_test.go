package storages_test

import (
	"context"
	"github.com/adamluzsi/testcase"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/specs"
	"github.com/adamluzsi/frameless/resources/storages"
)

var _ interface {
	resources.Creator
	resources.Finder
	resources.Updater
	resources.Deleter
	resources.OnePhaseCommitProtocol
} = &storages.Memory{}

var (
	_ storages.MemoryEventManager = &storages.Memory{}
	_ storages.MemoryEventManager = &storages.MemoryTransaction{}
)

func TestStorage_smokeTest(t *testing.T) {
	var (
		subject = storages.NewMemory()
		ctx     = context.Background()
		count   int
		err     error
	)

	require.Nil(t, subject.Create(ctx, &Entity{Data: `A`}))
	require.Nil(t, subject.Create(ctx, &Entity{Data: `B`}))
	count, err = iterators.Count(subject.FindAll(ctx, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 2, count)

	require.Nil(t, subject.DeleteAll(ctx, Entity{}))
	count, err = iterators.Count(subject.FindAll(ctx, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx1CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx1CTX, &Entity{Data: `C`}))
	count, err = iterators.Count(subject.FindAll(tx1CTX, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.RollbackTx(tx1CTX))
	count, err = iterators.Count(subject.FindAll(ctx, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 0, count)

	tx2CTX, err := subject.BeginTx(ctx)
	require.Nil(t, err)
	require.Nil(t, subject.Create(tx2CTX, &Entity{Data: `D`}))
	count, err = iterators.Count(subject.FindAll(tx2CTX, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 1, count)
	require.Nil(t, subject.CommitTx(tx2CTX))
	count, err = iterators.Count(subject.FindAll(ctx, Entity{}))
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func getMemorySpecs(subject *storages.Memory) []specs.Interface {
	return []specs.Interface{
		specs.Creator{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}},
		specs.Finder{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}},
		specs.Updater{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}},
		specs.Deleter{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}},
		specs.OnePhaseCommitProtocol{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}},
		specs.CreatorPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}},
		specs.UpdaterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}},
		specs.DeleterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}},
	}
}

func TestMemory(t *testing.T) {
	for _, spec := range getMemorySpecs(storages.NewMemory()) {
		spec.Test(t)
	}

	t.Run(`#DisableEventLogging`, func(t *testing.T) {
		subject := storages.NewMemory()
		subject.DisableEventLogging()

		for _, spec := range getMemorySpecs(subject) {
			spec.Test(t)
		}

		require.Empty(t, subject.Events(),
			`after all the specs, the memory storage was expected to be empty.`+
				` If the storage has values, it means something is not cleaning up properly in the specs.`)
	})
}

func TestMemory_multipleInstanceTransactionOnTheSameContext(t *testing.T) {
	ff := fixtures.FixtureFactory{}

	t.Run(`with create in different tx`, func(t *testing.T) {
		subject1 := storages.NewMemory()
		subject2 := storages.NewMemory()

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
		t.Log(`subject 1 can see the new entity`)
		found, err = subject1.FindByID(context.Background(), &Entity{}, entity.ID)
		require.Nil(t, err)
		require.True(t, found)
	})

	t.Run(`deletes across tx instances in the same context`, func(t *testing.T) {
		subject1 := storages.NewMemory()
		subject2 := storages.NewMemory()

		ctx := ff.Context()
		e1 := ff.Create(Entity{}).(*Entity)
		e2 := ff.Create(Entity{}).(*Entity)

		require.Nil(t, subject1.Create(ctx, e1))
		id1, ok := resources.LookupID(e1)
		require.True(t, ok)
		require.NotEmpty(t, id1)
		t.Cleanup(func() { _ = subject1.DeleteByID(ff.Context(), Entity{}, id1) })

		require.Nil(t, subject2.Create(ctx, e2))
		id2, ok := resources.LookupID(e2)
		require.True(t, ok)
		require.NotEmpty(t, id2)
		t.Cleanup(func() { _ = subject2.DeleteByID(ff.Context(), Entity{}, id2) })

		ctx, err := subject1.BeginTx(ctx)
		require.Nil(t, err)
		ctx, err = subject2.BeginTx(ctx)
		require.Nil(t, err)

		found, err := subject1.FindByID(ctx, &Entity{}, id1)
		require.Nil(t, err)
		require.True(t, found)
		require.Nil(t, subject1.DeleteByID(ctx, Entity{}, id1))

		found, err = subject2.FindByID(ctx, &Entity{}, id2)
		require.True(t, found)
		require.Nil(t, subject2.DeleteByID(ctx, Entity{}, id2))

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

func TestMemory_Options_AsyncSubscriptionHandling(t *testing.T) {
	s := testcase.NewSpec(t)

	var subscriber = func(t *testcase.T) *HangingSubscriber { return t.I(`HangingSubscriber`).(*HangingSubscriber) }
	s.Let(`HangingSubscriber`, func(t *testcase.T) interface{} {
		return NewHangingSubscriber()
	})

	var newMemory = func(t *testcase.T) *storages.Memory {
		s := storages.NewMemory()
		ctx := context.Background()
		subscription, err := s.SubscribeToCreate(ctx, Entity{}, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToUpdate(ctx, Entity{}, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteAll(ctx, Entity{}, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteByID(ctx, Entity{}, subscriber(t))
		require.Nil(t, err)
		t.Defer(subscription.Close)
		return s
	}

	var subject = func(t *testcase.T) *storages.Memory {
		s := newMemory(t)
		s.Options.AsyncSubscriptionHandling = t.I(`AsyncSubscriptionHandling`).(bool)
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
			require.Nil(t, memory.DeleteByID(context.Background(), Entity{}, ent.ID))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`DeleteAll`+desc, func(t *testcase.T) {
			memory := subject(t)
			sub := subscriber(t)

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			require.Nil(t, memory.DeleteAll(context.Background(), Entity{}))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})
	}

	s.When(`is enabled`, func(s *testcase.Spec) {
		s.LetValue(`AsyncSubscriptionHandling`, true)

		thenCreateUpdateDeleteWill(s, false)
	})

	s.When(`is disabled`, func(s *testcase.Spec) {
		s.LetValue(`AsyncSubscriptionHandling`, false)

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

func TestMemory_LogHistory(t *testing.T) {
	t.Run(`asking storage history directly`, func(t *testing.T) {
		s := storages.NewMemory()

		e := Entity{Data: `42`}
		require.Nil(t, s.Create(context.Background(), &e))
		require.Nil(t, s.DeleteByID(context.Background(), Entity{}, e.ID))
		require.Nil(t, s.DeleteAll(context.Background(), Entity{}))

		l := &fakeLogger{}
		s.LogHistory(l)
		require.Len(t, l.logs, 3)
		require.Contains(t, l.logs[0], `Create`)
		require.Contains(t, l.logs[1], `DeleteByID`)
		require.Contains(t, l.logs[2], `DeleteAll`)
	})

	t.Run(`storage history when used with tx`, func(t *testing.T) {
		s := storages.NewMemory()

		ctx, err := s.BeginTx(context.Background())
		require.Nil(t, err)

		e := Entity{Data: `42`}
		require.Nil(t, s.Create(ctx, &e))
		require.Nil(t, s.DeleteByID(ctx, Entity{}, e.ID))
		require.Nil(t, s.DeleteAll(ctx, Entity{}))

		l := &fakeLogger{}
		s.LogHistory(l)
		require.Len(t, l.logs, 0)

		require.Nil(t, s.CommitTx(ctx))

		l = &fakeLogger{}
		s.LogHistory(l)
		require.Len(t, l.logs, 3)
		require.Contains(t, l.logs[0], `Create`)
		require.Contains(t, l.logs[1], `DeleteByID`)
		require.Contains(t, l.logs[2], `DeleteAll`)
	})

	t.Run(`storage transaction history from context`, func(t *testing.T) {
		s := storages.NewMemory()

		ctx, err := s.BeginTx(context.Background())
		require.Nil(t, err)

		e := Entity{Data: `42`}
		require.Nil(t, s.Create(ctx, &e))
		require.Nil(t, s.DeleteByID(ctx, Entity{}, e.ID))
		require.Nil(t, s.DeleteAll(ctx, Entity{}))

		l := &fakeLogger{}
		s.LogContextHistory(l, ctx)
		require.Len(t, l.logs, 3)
		require.Contains(t, l.logs[0], `Create`)
		require.Contains(t, l.logs[1], `DeleteByID`)
		require.Contains(t, l.logs[2], `DeleteAll`)

		require.Nil(t, s.RollbackTx(ctx))
	})
}

type fakeLogger struct {
	logs []interface{}
}

func (l *fakeLogger) Log(args ...interface{}) {
	l.logs = append(l.logs, args...)
}

func TestMemory_LookupTxEvent(t *testing.T) {
	s := storages.NewMemory()

	t.Run(`when outside of tx`, func(t *testing.T) {
		_, ok := s.LookupTx(context.Background())
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

		tx, ok := s.LookupTx(ctx)
		require.True(t, ok)
		_, ok = tx.View()[s.EntityTypeNameFor(Entity{})][e.ID]
		require.True(t, ok)
	})
}

func BenchmarkMemory(b *testing.B) {
	b.Run(`with event log`, func(b *testing.B) {
		for _, spec := range getMemorySpecs(storages.NewMemory()) {
			spec.Benchmark(b)
		}
	})

	b.Run(`without event log`, func(b *testing.B) {
		subject := storages.NewMemory()
		subject.DisableEventLogging()
		for _, spec := range getMemorySpecs(subject) {
			spec.Benchmark(b)
		}
	})
}

type Entity struct {
	ID   string `ext:"ID"`
	Data string
}
