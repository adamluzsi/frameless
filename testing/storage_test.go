package testing_test

import (
	"context"

	"github.com/stretchr/testify/require"

	flt "github.com/adamluzsi/frameless/testing"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/specs"

	"testing"
)

var _ interface {
	resources.Creator
	resources.Finder
	resources.Updater
	resources.Deleter
	resources.OnePhaseCommitProtocol
} = &flt.Storage{}

var (
	_ flt.StorageEventManager = &flt.Storage{}
	_ flt.StorageEventManager = &flt.StorageTransaction{}
)

func TestStorage_smoketest(t *testing.T) {
	var (
		subject = flt.NewStorage()
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

func TestStorage(t *testing.T) {
	subject := flt.NewStorage()
	specs.Creator{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Finder{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Updater{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.Deleter{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.OnePhaseCommitProtocol{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.CreatorPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterPublisher{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
}

func BenchmarkStorage(b *testing.B) {
	subject := flt.NewStorage()
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
