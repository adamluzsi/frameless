package cache_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/internal/constant"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/crudkit"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCache(t *testing.T) {
	cacheRepository := &memory.CacheRepository[testent.Foo, testent.FooID]{}
	cachecontracts.Cache(cacheRepository).Test(t)
}

func TestCache_InvalidateByID_smoke(t *testing.T) { // flaky: go test -count 1024 -failfast -run TestCache_InvalidateByID_smoke
	var ctx = context.Background()

	var (
		foo1 = testent.Foo{
			Foo: "foo1",
			Bar: "bar1",
			Baz: "baz1",
		}
		foo2 = testent.Foo{
			Foo: "foo2",
			Bar: "bar2",
			Baz: "baz2",
		}
		foo3 = testent.Foo{
			Foo: "oth",
			Bar: "oth",
			Baz: "oth",
		}
	)

	var (
		memmem     = memory.NewMemory()
		source     = memory.NewRepository[testent.Foo, testent.FooID](memmem)
		repository = memory.NewCacheRepository[testent.Foo, testent.FooID](memmem)
		cachei     = cache.New[testent.Foo, testent.FooID](source, repository)
	)

	var getHits = func() []cache.Hit[testent.FooID] {
		iter, err := cachei.Repository.Hits().FindAll(context.Background())
		assert.NoError(t, err)
		vs, err := iterkit.CollectErrIter(iter)
		assert.NoError(t, err)
		return vs
	}

	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo1)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo2)
	crudtest.Create[testent.Foo, testent.FooID](t, source, context.Background(), &foo3)

	{
		t.Log("when we use FindByID with foo1.ID")
		_, _, err := cachei.FindByID(ctx, foo1.ID)
		assert.NoError(t, err)

		t.Log("then it will be cached")
		expID := cache.Query{Name: "FindByID", ARGS: cache.QueryARGS{"id": foo1.ID}}.HitID()
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Contain(t, got.ID, expID)
		})
	}
	{
		t.Log(`when CachedQueryOne used with OP:"NOK-ONE-1" with result relating to foo1`)
		qid := cache.Query{Name: "NOK-ONE-1"}
		_, _, err := cachei.CachedQueryOne(ctx, qid.HitID(), func(ctx context.Context) (ent testent.Foo, found bool, err error) {
			return cachei.Source.FindByID(ctx, foo1.ID)
		})
		assert.NoError(t, err)

		t.Log("then it will be cached")
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Equal(t, got.ID, qid.HitID())
			assert.Contain(t, got.EntityIDs, foo1.ID)
		})
	}
	{
		t.Log("given we have an invalidator that returns with all possible operation that has dynamic name content using the entity")
		cachei.Invalidators = append(cachei.Invalidators, cache.Invalidator[testent.Foo, testent.FooID]{
			CheckEntity: func(ent testent.Foo) []cache.HitID {
				return []cache.HitID{cache.Query{Name: constant.String(ent.Bar)}.HitID()}
			},
		})

		t.Log("when a custom query is used that also referencing to the foo1 entity (by Bar field)")
		assert.NotEqual(t, foo1.Bar, foo2.Bar)
		assert.NotEqual(t, foo1.Bar, foo3.Bar)
		qid := cache.Query{Name: "FindByBarID", ARGS: map[string]any{"bar": foo1.Bar}}
		_, _, err := cachei.CachedQueryOne(ctx, qid.HitID(), func(ctx context.Context) (ent testent.Foo, found bool, err error) {
			itr, err := cachei.FindAll(ctx)
			if err != nil {
				return ent, found, err
			}
			itr = iterkit.OnErrIterValue(itr, func(itr iter.Seq[testent.Foo]) iter.Seq[testent.Foo] {
				return iterkit.Filter(itr, func(f testent.Foo) bool {
					return f.Bar == foo1.Bar
				})
			})
			return crudkit.First(itr)
		})
		assert.NoError(t, err)

		t.Log("then FindByBarID will be cached")
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Equal(t, got.ID, qid.HitID())
			assert.Contain(t, got.EntityIDs, foo1.ID)
		})
	}
	{
		t.Log("given we have an invalidator that looks for queries that had operation name starting with NOK-MANY-BAZ")
		cachei.Invalidators = append(cachei.Invalidators, cache.Invalidator[testent.Foo, testent.FooID]{
			CheckHit: func(hit cache.Hit[testent.FooID]) bool {
				return strings.Contains(string(hit.ID), "NOK-MANY-BAZ")
			},
		})
		t.Log("when we have a custom query that has no arguments but only returns foo2")
		qid := cache.Query{Name: "NOK-MANY-BAZ"}
		iter, err := cachei.CachedQueryMany(ctx, qid.HitID(), func(ctx context.Context) (iter.Seq2[testent.Foo, error], error) {
			return iterkit.ToErrIter(iterkit.Slice([]testent.Foo{foo2})), nil
		})
		assert.NoError(t, err)
		_, err = iterkit.CollectErrIter(iter) // drain iterator
		assert.NoError(t, err)

		t.Log("then we expect that the new NOK-MANY-BAZ will be filtered")
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Equal(t, got.ID, qid.HitID())
			assert.Contain(t, got.EntityIDs, foo2.ID)
		})
	}

	{
		t.Log("when we also have a cached query that will be unrelated to foo1, (only relates to foo3)")
		qid := cache.Query{Name: "notAffectedQueryKey"}
		_, _, err := cachei.CachedQueryOne(ctx, qid.HitID(), func(ctx context.Context) (ent testent.Foo, found bool, err error) {
			return cachei.Source.FindByID(ctx, foo3.ID)
		})
		assert.NoError(t, err)

		t.Log("then we expect that the query which will be intentionally unrelated to foo1 is also cached")
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Equal(t, got.ID, qid.HitID())
			assert.Contain(t, got.EntityIDs, foo3.ID)
		})
	}

	{
		t.Log("and when we invalidate the cache for foo1")
		assert.NoError(t, cachei.InvalidateByID(ctx, foo1.ID))

		t.Log(`then OP:"NOK-ONE-1" should be invalidated as it was also related to foo1`)
		assert.NoneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Contain(t, got.ID, "NOK-ONE-1")
		})

		t.Log("then FindByBarID query that has an invalidator telling that FindByBarID for foo1 should be invalidated, is actually invalidated")
		FindByBarIDForFoo1 := cache.Query{Name: "FindByBarID", ARGS: map[string]any{"bar": foo1.Bar}}
		assert.NoneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			assert.Equal(t, got.ID, FindByBarIDForFoo1.HitID())
		})

		t.Log("then NOK-MANY-BAZ will be invalidated as well as its result because the cahed query invalidator will match it")
		assert.NoneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			qid := cache.Query{Name: "NOK-MANY-BAZ"}
			assert.Equal(t, got.ID, qid.HitID())
		})

		t.Log("then the cached query that was not related to foo1 should be left there")
		assert.OneOf(t, getHits(), func(t assert.It, got cache.Hit[testent.FooID]) {
			qid := cache.Query{Name: "notAffectedQueryKey"}
			assert.Equal(t, got.ID, qid.HitID())
		})
	}
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

	vs, err := crudkit.CollectQueryMany(cachei.FindAll(ctx))
	assert.NoError(t, err)
	assert.Contain(t, vs, []testent.Foo{foo1, foo2, foo3})

	vs, err = crudkit.CollectQueryMany(cachei.FindAll(ctx))
	assert.NoError(t, err)
	assert.Contain(t, vs, []testent.Foo{foo1, foo2, foo3})

	hvs, err := crudkit.CollectQueryMany(cachei.Repository.Hits().FindAll(ctx))
	assert.NoError(t, err)
	assert.Equal(t, 2, len(hvs))

	vs, err = crudkit.CollectQueryMany(cachei.Repository.Entities().FindAll(ctx))
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
		vs, err := crudkit.CollectQueryMany(subject.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t)}, vs)
	})

	s.Test("Create works even with a faulty repo", func(t *testcase.T) {
		foo2 := testent.MakeFoo(t)
		t.Must.NoError(subject.Get(t).Create(context.Background(), &foo2))

		vs, err := crudkit.CollectQueryMany(subject.Get(t).FindAll(context.Background()))
		t.Must.NoError(err)
		t.Must.ContainExactly([]testent.Foo{foo.Get(t), foo2}, vs)
	})

	s.Test("CachedQueryOne works even with a faulty repo", func(t *testcase.T) {
		value, found, err := subject.Get(t).CachedQueryOne(context.Background(), cache.Query{Name: "query one test"}.HitID(),
			func(ctx context.Context) (ent testent.Foo, found bool, err error) {
				return source.Get(t).FindByID(context.Background(), foo.Get(t).ID)
			})
		t.Must.NoError(err)
		t.Must.True(found)
		t.Must.Equal(foo.Get(t), value)
	})

	s.Test("CachedQueryMany works even with a faulty repo", func(t *testcase.T) {
		all, err := subject.Get(t).CachedQueryMany(context.Background(), cache.Query{Name: "query many test"}.HitID(),
			func(ctx context.Context) (iter.Seq2[testent.Foo, error], error) {
				return source.Get(t).FindAll(context.Background())
			})
		assert.NoError(t, err)
		vs, err := iterkit.CollectErrIter(all)
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

func (fer *faultyEntityRepo[Entity, ID]) FindAll(ctx context.Context) (iter.Seq2[Entity, error], error) {
	if fer.fcr.shouldFail() {
		return nil, fer.fcr.Random.Error()
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

func (fer *faultyEntityRepo[Entity, ID]) FindByIDs(ctx context.Context, ids ...ID) (iter.Seq2[Entity, error], error) {
	if fer.fcr.shouldFail() {
		return nil, fer.fcr.Random.Error()
	}
	return fer.EntityRepository.FindByIDs(ctx, ids...)
}

func (fer *faultyEntityRepo[Entity, ID]) Save(ctx context.Context, ptr *Entity) error {
	if fer.fcr.shouldFail() {
		return fer.fcr.Random.Error()
	}
	return fer.EntityRepository.Save(ctx, ptr)
}

type faultyHitRepo[Entity, ID any] struct {
	fcr *FaultyCacheRepository[Entity, ID]
	cache.HitRepository[ID]
}

func (fhr *faultyHitRepo[Entity, ID]) Save(ctx context.Context, ptr *cache.Hit[ID]) error {
	if fhr.fcr.shouldFail() {
		return fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().Save(ctx, ptr)
}

func (fhr *faultyHitRepo[Entity, ID]) FindByID(ctx context.Context, id cache.HitID) (ent cache.Hit[ID], found bool, err error) {
	if fhr.fcr.shouldFail() {
		return ent, false, fhr.fcr.Random.Error()
	}
	return fhr.fcr.CacheRepo.Hits().FindByID(ctx, id)
}

func (fhr *faultyHitRepo[Entity, ID]) FindAll(ctx context.Context) (iter.Seq2[cache.Hit[ID], error], error) {
	if fhr.fcr.shouldFail() {
		return nil, fhr.fcr.Random.Error()
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
