package storages_test

import (
	"context"
	"fmt"
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

func TestStorage_smoketest(t *testing.T) {
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

	subject.History().LogWith(t)
}

func TestMemory(t *testing.T) {
	subject := storages.NewMemory()
	specs.Creator{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Finder{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Updater{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Deleter{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.OnePhaseCommitProtocol{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.CreatorPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
}

func TestMemory_DisableEventLogging(t *testing.T) {
	subject := storages.NewMemory()
	subject.DisableEventLogging()
	specs.Creator{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Finder{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Updater{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Deleter{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.OnePhaseCommitProtocol{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.CreatorPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)

	fmt.Println(`log len:`, len(subject.Events()))
	require.Empty(t, subject.Events(),
		`after all the specs, the memory storage was expected to be empty.`+
			` If the storage has values, it means something is not cleaning up properly in the specs.`)
}

func BenchmarkMemory(b *testing.B) {
	subject := storages.NewMemory()
	specs.Creator{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.Finder{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.Updater{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.Deleter{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.OnePhaseCommitProtocol{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.CreatorPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.UpdaterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.DeleterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
}

type Entity struct {
	ID   string `ext:"ID"`
	Data string
}
