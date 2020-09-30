package storages_test

import (
	"context"
	"testing"

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

type fakeLogger struct {
	logs []interface{}
}

func (l *fakeLogger) Log(args ...interface{}) {
	l.logs = append(l.logs, args...)
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
