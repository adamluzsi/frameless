package cache_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCache(t *testing.T) {
	cachecontracts.Cache[testent.Foo, testent.FooID](func(tb testing.TB) cachecontracts.CacheSubject[testent.Foo, testent.FooID] {
		m := memory.NewMemory()
		source := memory.NewRepository[testent.Foo, testent.FooID](m)
		cacheRepository := memory.NewCacheRepository[testent.Foo, testent.FooID](m)
		return cachecontracts.CacheSubject[testent.Foo, testent.FooID]{
			Cache:        cache.New[testent.Foo, testent.FooID](source, cacheRepository),
			Source:       source,
			Repository:   cacheRepository,
			MakeContext:  testent.MakeContextFunc(tb),
			MakeEntity:   testent.MakeFooFunc(tb),
			ChangeEntity: nil,
		}
	}).Test(t)
}

func TestCache_withFaultyCacheRepository(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		source = testcase.Let(s, func(t *testcase.T) *memory.Repository[testent.Foo, testent.FooID] {
			return memory.NewRepository[testent.Foo, testent.FooID](memory.NewMemory())
		})
		cacheRepository = testcase.Let(s, func(t *testcase.T) *FaultyCacheRepository[testent.Foo, testent.FooID] {
			return NewFaultyCacheRepository[testent.Foo, testent.FooID](0.3)
		})
		cache = testcase.Let(s, func(t *testcase.T) *cache.Cache[testent.Foo, testent.FooID] {
			return cache.New[testent.Foo, testent.FooID](source.Get(t), cacheRepository.Get(t))
		})
	)

	// given we have a foo
	foo := testcase.Let(s, func(t *testcase.T) testent.Foo {
		foo := testent.MakeFoo(t)
		crudtest.Create[testent.Foo, testent.FooID](t, source.Get(t), context.Background(), &foo)
		return foo
	}).EagerLoading(s)

	s.Test("FindByID works even with a faulty repo", func(t *testcase.T) {
		value, found, err := cache.Get(t).FindByID(context.Background(), foo.Get(t).ID)
		t.Must.NoError(err)
		t.Must.True(found)
		t.Must.Equal(foo.Get(t), value)
	})

	s.Test("FindAll works even with a faulty repo", func(t *testcase.T) {
		vs, err := iterators.Collect(cache.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t)}, vs)
	})

	s.Test("Create works even with a faulty repo", func(t *testcase.T) {
		foo2 := testent.MakeFoo(t)
		t.Must.NoError(cache.Get(t).Create(context.Background(), &foo2))

		vs, err := iterators.Collect(cache.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t), foo2}, vs)
	})

	s.Test("CachedQueryOne works even with a faulty repo", func(t *testcase.T) {
		value, found, err := cache.Get(t).CachedQueryOne(context.Background(), "query one test", func() (ent testent.Foo, found bool, err error) {
			return source.Get(t).FindByID(context.Background(), foo.Get(t).ID)
		})
		t.Must.NoError(err)
		t.Must.True(found)
		t.Must.Equal(foo.Get(t), value)
	})

	s.Test("CachedQueryMany works even with a faulty repo", func(t *testcase.T) {
		all := cache.Get(t).CachedQueryMany(context.Background(), "query many test", func() iterators.Iterator[testent.Foo] {
			return source.Get(t).FindAll(context.Background())
		})
		vs, err := iterators.Collect(all)
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t)}, vs)
	})

	s.Test("InvalidateByID will fail on an error", func(t *testcase.T) {
		cacheRepository.Get(t).FailurePercentage = 1

		t.Must.Error(cache.Get(t).InvalidateByID(context.Background(), foo.Get(t).ID))
	})

	s.Test("DropCachedValues will fail on an error", func(t *testcase.T) {
		cacheRepository.Get(t).FailurePercentage = 1

		t.Must.Error(cache.Get(t).DropCachedValues(context.Background()))
	})

	// TODO: Update, Delete
}

func NewFaultyCacheRepository[Entity, ID any](FailurePercentage float64) *FaultyCacheRepository[Entity, ID] {
	m := memory.NewMemory()
	return &FaultyCacheRepository[Entity, ID]{
		FailurePercentage:      FailurePercentage,
		Random:                 random.New(random.CryptoSeed{}),
		CacheRepo:              memory.NewCacheRepository[Entity, ID](m),
		OnePhaseCommitProtocol: m,
	}
}

type FaultyCacheRepository[Entity, ID any] struct {
	FailurePercentage float64
	Random            *random.Random
	CacheRepo         *memory.CacheRepository[Entity, ID]
	comproto.OnePhaseCommitProtocol
}

func (fcr *FaultyCacheRepository[Entity, ID]) shouldFail() bool {
	return float64(fcr.Random.IntBetween(0, 100))/100.0 < fcr.FailurePercentage
}

func (fcr *FaultyCacheRepository[Entity, ID]) Entities() cache.EntityRepository[Entity, ID] {
	return &faultyEntityRepo[Entity, ID]{
		fcr:              fcr,
		EntityRepository: fcr.CacheRepo.Entities(),
	}
}

func (fcr *FaultyCacheRepository[Entity, ID]) Hits() cache.HitRepository[ID] {
	return &faultyHitRepo[Entity, ID]{
		fcr:           fcr,
		HitRepository: fcr.CacheRepo.Hits(),
	}
}

type faultyEntityRepo[Entity, ID any] struct {
	fcr *FaultyCacheRepository[Entity, ID]
	cache.EntityRepository[Entity, ID]
}

func (fer *faultyEntityRepo[Entity, ID]) FindByID(ctx context.Context, id ID) (ent Entity, found bool, err error) {
	if fer.fcr.shouldFail() {
		return ent, false, fer.fcr.Random.Error()
	}
	return fer.EntityRepository.FindByID(ctx, id)
}

func (fer *faultyEntityRepo[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.Create(ctx, ptr)
}

func (fer *faultyEntityRepo[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.Update(ctx, ptr)
}

func (fer *faultyEntityRepo[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	if fer.fcr.shouldFail() {
		return iterators.Error[Entity](fer.fcr.Random.Error())
	}
	return fer.EntityRepository.FindAll(ctx)
}

func (fer *faultyEntityRepo[Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.DeleteByID(ctx, id)
}

func (fer *faultyEntityRepo[Entity, ID]) DeleteAll(ctx context.Context) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.DeleteAll(ctx)
}

func (fer *faultyEntityRepo[Entity, ID]) FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Entity] {
	if fer.fcr.shouldFail() {
		return iterators.Error[Entity](fer.fcr.Random.Error())
	}
	return fer.EntityRepository.FindByIDs(ctx, ids...)
}

func (fer *faultyEntityRepo[Entity, ID]) Upsert(ctx context.Context, ptrs ...*Entity) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.Upsert(ctx, ptrs...)
}

type faultyHitRepo[Entity, ID any] struct {
	fcr *FaultyCacheRepository[Entity, ID]
	cache.HitRepository[ID]
}

func (fhr *faultyHitRepo[Entity, ID]) Create(ctx context.Context, ptr *cache.Hit[ID]) error {
	if fhr.fcr.shouldFail() {
		return fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().Create(ctx, ptr)
}

func (fhr *faultyHitRepo[Entity, ID]) Update(ctx context.Context, ptr *cache.Hit[ID]) error {
	if fhr.fcr.shouldFail() {
		return fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().Update(ctx, ptr)
}

func (fhr *faultyHitRepo[Entity, ID]) FindByID(ctx context.Context, id cache.HitID) (ent cache.Hit[ID], found bool, err error) {
	if fhr.fcr.shouldFail() {
		return ent, false, fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().FindByID(ctx, id)
}

func (fhr *faultyHitRepo[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[cache.Hit[ID]] {
	if fhr.fcr.shouldFail() {
		return iterators.Error[cache.Hit[ID]](fhr.fcr.Random.Error())
	}
	return fhr.fcr.CacheRepo.Hits().FindAll(ctx)
}

func (fhr *faultyHitRepo[Entity, ID]) DeleteByID(ctx context.Context, id cache.HitID) error {
	if fhr.fcr.shouldFail() {
		return fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().DeleteByID(ctx, id)
}

func (fhr *faultyHitRepo[Entity, ID]) DeleteAll(ctx context.Context) error {
	if fhr.fcr.shouldFail() {
		return fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().DeleteAll(ctx)
}
