package cache_test

import (
	"context"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCache(t *testing.T) {
	m := memory.NewMemory()
	source := memory.NewRepository[testent.Foo, testent.FooID](m)
	cacheRepository := memory.NewCacheRepository[testent.Foo, testent.FooID](m)

	cachecontracts.Cache[testent.Foo, testent.FooID](
		cache.New[testent.Foo, testent.FooID](source, cacheRepository),
		source,
		cacheRepository,
		cachecontracts.Config[testent.Foo, testent.FooID]{
			MakeCache: func(src cache.Source[testent.Foo, testent.FooID], repo cache.Repository[testent.Foo, testent.FooID]) cachecontracts.CacheSubject[testent.Foo, testent.FooID] {
				return cache.New[testent.Foo, testent.FooID](src, repo)
			},
			CRUD: crudcontracts.Config[testent.Foo, testent.FooID]{
				MakeEntity: testent.MakeFoo,
			},
		},
	)
}

func TestCache_InvalidateByID_smoke(t *testing.T) {
	var (
		ctx    = context.Background()
		foo1   = testent.MakeFoo(t)
		foo2   = testent.MakeFoo(t)
		othFoo = testent.MakeFoo(t)
	)

	var (
		memmem     = memory.NewMemory()
		source     = memory.NewRepository[testent.Foo, testent.FooID](memmem)
		repository = memory.NewCacheRepository[testent.Foo, testent.FooID](memmem)
		cachei     = cache.New[testent.Foo, testent.FooID](source, repository)
	)

	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo1)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo2)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &othFoo)

	cachei.CachedQueryInvalidators = []cache.CachedQueryInvalidator[testent.Foo, testent.FooID]{
		{
			CheckHit: func(hit cache.Hit[testent.FooID]) bool {
				return strings.HasPrefix(hit.QueryID, "NOK-MANY-BAZ")
			},
		},
		{
			CheckEntity: func(ent testent.Foo) []cache.HitID {
				return []cache.HitID{foo1.Baz}
			},
		},
	}

	var expectedCachedQueryCount int

	_, _, err := cachei.FindByID(ctx, foo1.ID)
	assert.NoError(t, err)
	expectedCachedQueryCount++

	t.Log("queryID non referenced ID")
	_, _, err = cachei.CachedQueryOne(ctx, "NOK-ONE-1", func() (ent testent.Foo, found bool, err error) {
		return cachei.Source.FindByID(ctx, foo1.ID)
	})
	assert.NoError(t, err)
	expectedCachedQueryCount++

	t.Log("queryID with entity field")
	_, _, err = cachei.CachedQueryOne(ctx, foo1.Bar, func() (ent testent.Foo, found bool, err error) {
		return cachei.Source.FindByID(ctx, foo1.ID)
	})
	assert.NoError(t, err)
	expectedCachedQueryCount++

	t.Log("query many that doesn't have the value actively (excluded filter list)")
	_, err = iterators.Collect(cachei.CachedQueryMany(ctx, "NOK-MANY-BAZ", func() iterators.Iterator[testent.Foo] {
		return iterators.Filter[testent.Foo](source.FindAll(ctx), func(got testent.Foo) bool {
			return got.ID == foo2.ID
		})
	}))
	assert.NoError(t, err)
	expectedCachedQueryCount++

	t.Log("another query many that doesn't have the value actively (excluded filter list)")
	_, err = iterators.Collect(cachei.CachedQueryMany(ctx, foo1.Baz, func() iterators.Iterator[testent.Foo] {
		return iterators.Filter[testent.Foo](source.FindAll(ctx), func(got testent.Foo) bool {
			return got.Baz == foo2.Baz
		})
	}))
	assert.NoError(t, err)
	expectedCachedQueryCount++

	t.Log("unrelated cached query")
	_, _, err = cachei.CachedQueryOne(ctx, "notAffectedQueryKey", func() (ent testent.Foo, found bool, err error) {
		return cachei.Source.FindByID(ctx, othFoo.ID)
	})
	assert.NoError(t, err)
	expectedCachedQueryCount++

	hitCount, err := iterators.Count(cachei.Repository.Hits().FindAll(ctx))
	assert.NoError(t, err)
	assert.Equal(t, expectedCachedQueryCount, hitCount)

	assert.NoError(t, err)
	assert.NoError(t, cachei.InvalidateByID(ctx, foo1.ID))

	hits, err := iterators.Collect(cachei.Repository.Hits().FindAll(ctx))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(hits))
	assert.Equal(t, "notAffectedQueryKey", hits[0].QueryID)
}

func TestCache_InvalidateByID_hasNoCascadeEffect(t *testing.T) {
	var (
		ctx  = context.Background()
		foo1 = testent.MakeFoo(t)
		foo2 = testent.MakeFoo(t)
		foo3 = testent.MakeFoo(t)
	)

	var (
		memmem     = memory.NewMemory()
		source     = memory.NewRepository[testent.Foo, testent.FooID](memmem)
		repository = memory.NewCacheRepository[testent.Foo, testent.FooID](memmem)
		cachei     = cache.New[testent.Foo, testent.FooID](source, repository)
	)

	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo1)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo2)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo3)

	_, _, err := cachei.FindByID(ctx, foo1.ID)
	assert.NoError(t, err)

	vs, err := iterators.Collect(cachei.FindAll(ctx))
	assert.NoError(t, err)
	assert.Contain(t, vs, []testent.Foo{foo1, foo2, foo3})

	vs, err = iterators.Collect(cachei.FindAll(ctx))
	assert.NoError(t, err)
	assert.Contain(t, vs, []testent.Foo{foo1, foo2, foo3})

	hvs, err := iterators.Collect(cachei.Repository.Hits().FindAll(ctx))
	assert.NoError(t, err)
	assert.Equal(t, 2, len(hvs))

	vs, err = iterators.Collect(cachei.Repository.Entities().FindAll(ctx))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(vs), assert.Message(pp.Format(vs)))
	assert.Contain(t, vs, []testent.Foo{foo1, foo2, foo3})
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
		subject = testcase.Let(s, func(t *testcase.T) *cache.Cache[testent.Foo, testent.FooID] {
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
		value, found, err := subject.Get(t).FindByID(context.Background(), foo.Get(t).ID)
		t.Must.NoError(err)
		t.Must.True(found)
		t.Must.Equal(foo.Get(t), value)
	})

	s.Test("FindAll works even with a faulty repo", func(t *testcase.T) {
		vs, err := iterators.Collect(subject.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t)}, vs)
	})

	s.Test("Create works even with a faulty repo", func(t *testcase.T) {
		foo2 := testent.MakeFoo(t)
		t.Must.NoError(subject.Get(t).Create(context.Background(), &foo2))

		vs, err := iterators.Collect(subject.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t), foo2}, vs)
	})

	s.Test("CachedQueryOne works even with a faulty repo", func(t *testcase.T) {
		value, found, err := subject.Get(t).CachedQueryOne(context.Background(), "query one test", func() (ent testent.Foo, found bool, err error) {
			return source.Get(t).FindByID(context.Background(), foo.Get(t).ID)
		})
		t.Must.NoError(err)
		t.Must.True(found)
		t.Must.Equal(foo.Get(t), value)
	})

	s.Test("CachedQueryMany works even with a faulty repo", func(t *testcase.T) {
		all := subject.Get(t).CachedQueryMany(context.Background(), "query many test", func() iterators.Iterator[testent.Foo] {
			return source.Get(t).FindAll(context.Background())
		})
		vs, err := iterators.Collect(all)
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t)}, vs)
	})

	s.Test("InvalidateByID will fail on an error", func(t *testcase.T) {
		cacheRepository.Get(t).FailurePercentage = 1

		t.Must.Error(subject.Get(t).InvalidateByID(context.Background(), foo.Get(t).ID))
	})

	s.Test("DropCachedValues will fail on an error", func(t *testcase.T) {
		cacheRepository.Get(t).FailurePercentage = 1

		t.Must.Error(subject.Get(t).DropCachedValues(context.Background()))
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
