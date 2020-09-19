package dev_test

import (
	"context"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/dev"
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
} = &dev.Storage{}

var (
	_ dev.StorageEventManager = &dev.Storage{}
	_ dev.StorageEventManager = &dev.StorageTransaction{}
)

func TestStorage_smoketest(t *testing.T) {
	var (
		subject = dev.NewStorage()
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
	subject := dev.NewStorage()
	specs.CreatorSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.FinderSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.OnePhaseCommitProtocolSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.CreatorPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
}

func BenchmarkStorage(b *testing.B) {
	subject := dev.NewStorage()
	specs.CreatorSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.FinderSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.UpdaterSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.DeleterSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.OnePhaseCommitProtocolSpec{EntityType: Entity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.CreatorPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.UpdaterPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.DeleterPublisherSpec{Subject: subject, EntityType: Entity{}, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
}

type Entity struct {
	ID   string `ext:"ID"`
	Data string
}
