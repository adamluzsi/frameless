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

	subject.History().LogWith(t)
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

		fmt.Println(`log len:`, len(subject.Events()))
		require.Empty(t, subject.Events(),
			`after all the specs, the memory storage was expected to be empty.`+
				` If the storage has values, it means something is not cleaning up properly in the specs.`)
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
