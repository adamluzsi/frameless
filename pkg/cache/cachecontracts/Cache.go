package cachecontracts

import (
	"context"
	"fmt"
	"reflect"
	"time"

	cachepkg "go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/ports/option"
	sh "go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

var (
	waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      time.Second,
	}
	eventually = assert.Retry{Strategy: &waiter}
)

func Cache[Entity any, ID comparable](
	cache CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[Entity, ID]](opts)

	suites := []testcase.Suite{}
	suites = append(suites, crudcontracts.ByIDDeleter[Entity, ID](cache, c.CRUD))
	if _, ok := source.(crud.Creator[Entity]); ok {
		suites = append(suites, crudcontracts.Creator[Entity, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllFinder[Entity]); ok {
		suites = append(suites, crudcontracts.AllFinder[Entity, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.ByIDDeleter[Entity]); ok {
		suites = append(suites, crudcontracts.ByIDDeleter[Entity, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllDeleter); ok {
		suites = append(suites, crudcontracts.AllDeleter[Entity, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllDeleter); !ok {
		suites = append(suites, crudcontracts.AllDeleter[Entity, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.Updater[Entity]); ok {
		suites = append(suites, crudcontracts.Updater[Entity, ID](cache, c.CRUD))
	}

	suites = append(suites, Repository[Entity, ID](repository, c))

	// TODO: support OnePhaseCommitProtocol with cache.Cache

	s.Describe(".InvalidateCachedQuery", func(s *testcase.Spec) {
		specInvalidateCachedQuery[Entity, ID](s, cache, source, repository)
	})
	s.Describe(".InvalidateByID", func(s *testcase.Spec) {
		specInvalidateByID(s, cache, source, repository)
	})
	s.Describe(".CachedQueryMany", func(s *testcase.Spec) {
		specCachedQueryMany[Entity, ID](s, cache, source, repository)
	})

	s.Context(`cache behaviour`, func(s *testcase.Spec) {
		describeResultCaching[Entity, ID](s, cache, source, repository)
		describeCacheInvalidationByEventsThatMutatesAnEntity[Entity, ID](s, cache, source, repository)
	})

	return s.AsSuite("Cache")
}

type CacheSubject[Entity, ID any] interface {
	cachepkg.Interface[Entity, ID]
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.AllFinder[Entity]
	crud.Updater[Entity]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
}

type cacheSource[Entity, ID any] interface {
	sh.CRUD[Entity, ID]
	cachepkg.Source[Entity, ID]
}

func describeCacheInvalidationByEventsThatMutatesAnEntity[Entity any, ID comparable](
	s *testcase.Spec,
	cache CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) {
	c := option.Use[Config[Entity, ID]](opts)
	s.Context(reflectkit.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) interface{} {
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.DeleteByID, c.CRUD.MakeContext(), id)
			return ptr
		})

		s.Before(func(t *testcase.T) {
			t.Must.NoError(cache.DropCachedValues(c.CRUD.MakeContext()))
		})

		s.Test(`an update to the repository should refresh the by id look`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext()
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(ctx, entID)       // should trigger caching
			_, _ = iterators.Count(cache.FindAll(ctx)) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[Entity, ID](t, cache, ctx, vUpdated)
			waiter.Wait()

			ptr := crudtest.IsPresent[Entity, ID](t, cache, ctx, entID) // should trigger caching
			t.Must.Equal(vUpdated, ptr)
		})

		s.Test(`an update to the repository should refresh the QueryMany cache hits`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext()
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(ctx, entID)       // should trigger caching
			_, _ = iterators.Count(cache.FindAll(ctx)) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[Entity, ID](t, cache, ctx, vUpdated)
			waiter.Wait()

			var (
				gotEnt Entity
				found  bool
			)
			t.Must.NoError(iterators.ForEach(cache.FindAll(ctx), func(ent Entity) error {
				id, ok := extid.Lookup[ID](ent)
				if !ok {
					return fmt.Errorf("lookup can't find the ID")
				}
				if reflect.DeepEqual(entID, id) {
					found = true
					gotEnt = ent
					return iterators.Break
				}
				return nil
			}))

			t.Must.True(found, "it was expected to find the entity in the FindAll query result")
			t.Must.Equal(vUpdated, &gotEnt)
		})

		s.Test(`a delete by id to the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(c.CRUD.MakeContext(), id)          // should trigger caching
			_, _ = iterators.Count(cache.FindAll(c.CRUD.MakeContext())) // should trigger caching

			// delete
			t.Must.NoError(cache.DeleteByID(c.CRUD.MakeContext(), id))

			// assert
			crudtest.IsAbsent[Entity, ID](t, cache, c.CRUD.MakeContext(), id)
		})

		s.Test(`a delete all entity in the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(c.CRUD.MakeContext(), id)          // should trigger caching
			_, _ = iterators.Count(cache.FindAll(c.CRUD.MakeContext())) // should trigger caching

			// delete
			t.Must.NoError(cache.DeleteAll(c.CRUD.MakeContext()))
			waiter.Wait()

			crudtest.IsAbsent[Entity, ID](t, cache, c.CRUD.MakeContext(), id) // should trigger caching for not found
		})
	})
}

type SpySource[Entity, ID any] struct {
	cacheSource[Entity, ID]
	count struct {
		FindByID int
	}
}

func (stub *SpySource[Entity, ID]) FindByID(ctx context.Context, id ID) (_ent Entity, _found bool, _err error) {
	stub.count.FindByID++
	return stub.cacheSource.FindByID(ctx, id)
}

func describeResultCaching[Entity any, ID comparable](s *testcase.Spec,
	cache CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) {
	c := option.Use[Config[Entity, ID]](opts)
	s.Context(reflectkit.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) *Entity {
			ctx := c.CRUD.MakeContext()
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.DeleteByID, ctx, id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			id, found := extid.Lookup[ID](value.Get(t))
			assert.Must(t).True(found)
			v, found, err := cache.FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			assert.Must(t).True(found)
			assert.Must(t).Equal(*value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup[ID](value.Get(t))
				assert.Must(t).True(found)
				v := crudtest.IsPresent[Entity, ID](t, source, c.CRUD.MakeContext(), id)
				assert.Must(t).Equal(value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := testcase.Let(s, func(t *testcase.T) *Entity {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					nv := pointer.Of(c.CRUD.MakeEntity(t))
					t.Must.NoError(extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					crudtest.Update[Entity, ID](t, cache, c.CRUD.MakeContext(), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					t.Must.NotEmpty(id)
					crudtest.HasEntity[Entity, ID](t, cache, c.CRUD.MakeContext(), valueWithNewContent.Get(t))

					eventually.Assert(t, func(it assert.It) {
						v, found, err := cache.FindByID(c.CRUD.MakeContext(), id)
						it.Must.Nil(err)
						it.Must.True(found)
						it.Log(`actually`, v)
						it.Must.Equal(*valueWithNewContent.Get(t), v)
					})
				})
			})
		})

		s.And(`on multiple request`, func(s *testcase.Spec) {
			s.Then(`it will return it consistently`, func(t *testcase.T) {
				value := value.Get(t)
				id, found := extid.Lookup[ID](value)
				assert.Must(t).True(found)

				for i := 0; i < 42; i++ {
					v, found, err := cache.FindByID(c.CRUD.MakeContext(), id)
					t.Must.NoError(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*value, v)
				}
			})

			s.When(`the repository is sensitive to continuous requests`, func(s *testcase.Spec) {
				var cache = cache

				if c.MakeCache == nil {
					return
				}

				s.Sequential() // to avoid leaking of the local variable overriding.

				spy := testcase.Let(s, func(t *testcase.T) *SpySource[Entity, ID] {
					og := source
					v := &SpySource[Entity, ID]{cacheSource: og}
					source = v
					cache = c.MakeCache(source, repository)
					t.Defer(func() { source = og })
					return v
				}).EagerLoading(s)

				s.Then(`it will only bother the repository for the value once`, func(t *testcase.T) {
					var err error
					val := value.Get(t)
					id, found := extid.Lookup[ID](val)
					assert.Must(t).True(found)

					// trigger caching
					assert.Must(t).Equal(val, crudtest.IsPresent[Entity, ID](t, cache, c.CRUD.MakeContext(), id))
					numberOfFindByIDCallAfterEntityIsFound := spy.Get(t).count.FindByID
					waiter.Wait()

					nv, found, err := cache.FindByID(c.CRUD.MakeContext(), id) // should use cached val
					t.Must.NoError(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*val, nv)
					assert.Must(t).Equal(numberOfFindByIDCallAfterEntityIsFound, spy.Get(t).count.FindByID)
				})
			})
		})
	})
}

func specCachedQueryMany[Entity any, ID comparable](s *testcase.Spec,
	subject CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) {
	c := option.Use[Config[Entity, ID]](opts)
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext()
		})
		queryKey = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return t.Random.UUID()
		})
		query = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	)
	act := func(t *testcase.T) iterators.Iterator[Entity] {
		return subject.CachedQueryMany(Context.Get(t), queryKey.Get(t), query.Get(t))
	}

	s.When("query returns values", func(s *testcase.Spec) {
		var (
			ent1 = testcase.Let(s, func(t *testcase.T) *Entity {
				v := c.CRUD.MakeEntity(t)
				crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(), &v)
				return &v
			})
			ent2 = testcase.Let(s, func(t *testcase.T) *Entity {
				v := c.CRUD.MakeEntity(t)
				crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(), &v)
				return &v
			})
		)

		query.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[Entity] {
			return func() iterators.Iterator[Entity] {
				return iterators.Slice[Entity]([]Entity{*ent1.Get(t), *ent2.Get(t)})
			}
		})

		s.Then("it will return all the entities", func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
			t.Must.NoError(err)
			t.Must.ContainExactly([]Entity{*ent1.Get(t), *ent2.Get(t)}, vs)
		})

		s.Then("it will cache all returned entities", func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
			t.Must.NoError(err)

			cached, err := iterators.Collect(repository.Entities().FindAll(c.CRUD.MakeContext()))
			t.Must.NoError(err)
			t.Must.Contain(cached, vs)
		})

		s.Then("it will create a hit record", func(t *testcase.T) {
			_, err := iterators.Collect(act(t))
			t.Must.NoError(err)

			hits, err := iterators.Collect(repository.Hits().FindAll(c.CRUD.MakeContext()))
			t.Must.NoError(err)

			assert.OneOf(t, hits, func(it assert.It, got cachepkg.Hit[ID]) {
				it.Must.Equal(got.QueryID, queryKey.Get(t))
				it.Must.ContainExactly(got.EntityIDs, []ID{
					crudtest.HasID[Entity, ID](t, *ent1.Get(t)),
					crudtest.HasID[Entity, ID](t, *ent2.Get(t)),
				})
			})
		})
	})
}

func specInvalidateCachedQuery[Entity any, ID comparable](s *testcase.Spec,
	cache CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) {
	c := option.Use[Config[Entity, ID]](opts)
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext()
		})
		queryKey = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return t.Random.UUID()
		})
	)
	act := func(t *testcase.T) error {
		return cache.InvalidateCachedQuery(Context.Get(t), queryKey.Get(t))
	}

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[Entity]](s, nil)
	queryOne := func(t *testcase.T) (Entity, bool, error) {
		return cache.
			CachedQueryOne(c.CRUD.MakeContext(), queryKey.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	queryMany := func(t *testcase.T) iterators.Iterator[Entity] {
		return cache.
			CachedQueryMany(c.CRUD.MakeContext(), queryKey.Get(t), queryManyFunc.Get(t))
	}

	s.When("queryKey has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[Entity] {
			return func() (Entity, bool, error) {
				id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
				return source.FindByID(c.CRUD.MakeContext(), id)
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(), id))
			// we have hits
			n, err := iterators.Count(repository.Hits().FindAll(c.CRUD.MakeContext()))
			t.Must.NoError(err)
			t.Must.NotEqual(0, n)
			// we have cached entities
			n, err = iterators.Count(repository.Entities().FindAll(c.CRUD.MakeContext()))
			t.Must.NoError(err)
			t.Must.NotEqual(0, n)
			// cache still able to retrieve the invalid state
			ent, found, err = queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found, "it was not expected that the cached data got invalidated")
			t.Must.Equal(ent, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("queryKey has a cached data with CachedQueryMany", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		queryManyFunc.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[Entity] {
			return func() iterators.Iterator[Entity] {
				id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
				ent, found, err := source.FindByID(c.CRUD.MakeContext(), id)
				if err != nil {
					return iterators.Error[Entity](err)
				}
				if !found {
					return iterators.Empty[Entity]()
				}
				return iterators.SingleValue(ent)
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(), id))
			// cache has still the invalid state
			vs, err = iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("queryKey does not belong to any cached query hit", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
		})

		s.Then("nothing happens", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})
	})

	s.When("context is done", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(Context.Super(t))
			cancel()
			return ctx
		})

		s.Then("context error is propagated back", func(t *testcase.T) {
			t.Must.ErrorIs(Context.Get(t).Err(), act(t))
		})
	})
}

func specInvalidateByID[Entity any, ID comparable](s *testcase.Spec,
	cache CacheSubject[Entity, ID],
	source cacheSource[Entity, ID],
	repository cachepkg.Repository[Entity, ID],
	opts ...Option[Entity, ID],
) {
	c := option.Use[Config[Entity, ID]](opts)
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext()
		})
		id = testcase.Let[ID](s, nil)
	)
	act := func(t *testcase.T) error {
		return cache.InvalidateByID(Context.Get(t), id.Get(t))
	}

	queryKey := testcase.LetValue[cachepkg.HitID](s, "query-key")

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[Entity]](s, nil)
	queryOne := func(t *testcase.T) (Entity, bool, error) {
		return cache.
			CachedQueryOne(c.CRUD.MakeContext(), queryKey.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	queryMany := func(t *testcase.T) iterators.Iterator[Entity] {
		return cache.
			CachedQueryMany(c.CRUD.MakeContext(), queryKey.Get(t), queryManyFunc.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.Cleanup(func() {
			t.Must.NoError(cache.
				InvalidateCachedQuery(c.CRUD.MakeContext(), queryKey.Get(t)))
		})
	})

	s.When("entity id has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		id.Let(s, func(t *testcase.T) ID {
			return crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[Entity] {
			return func() (Entity, bool, error) {
				return source.FindByID(c.CRUD.MakeContext(), id.Get(t))
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(), id))
			// cache has still the invalid state
			ent, found, err = queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("entity id has a cached data with FindByID", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})
		id.Let(s, func(t *testcase.T) ID {
			return crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := cache.FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(), id))
			// cache has still the invalid state
			ent, found, err = cache.FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			ent, found, err := cache.FindByID(c.CRUD.MakeContext(), id.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("entity id has a cached data with CachedQueryMany", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		id.Let(s, func(t *testcase.T) ID {
			return crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
		})

		queryManyFunc.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[Entity] {
			return func() iterators.Iterator[Entity] {
				id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
				ent, found, err := source.FindByID(c.CRUD.MakeContext(), id)
				if err != nil {
					return iterators.Error[Entity](err)
				}
				if !found {
					return iterators.Empty[Entity]()
				}
				return iterators.SingleValue(ent)
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(), id))
			// cache has still the invalid state
			vs, err = iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterators.Collect(queryMany(t))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, *entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(), queryKey.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("entity id does not belong to any cached query hit", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := c.CRUD.MakeEntity(t)
			crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(), &ent)
			v := crudtest.HasID[Entity, ID](t, ent)
			crudtest.Delete[Entity, ID](t, source, c.CRUD.MakeContext(), &ent)
			return v
		})

		s.Before(func(t *testcase.T) {
			_, found, err := source.FindByID(c.CRUD.MakeContext(), id.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
		})

		s.Then("nothing happens", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})
	})

	s.When("context is done", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			var id ID
			return id
		})

		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(Context.Super(t))
			cancel()
			return ctx
		})

		s.Then("context error is propagated back", func(t *testcase.T) {
			t.Must.ErrorIs(Context.Get(t).Err(), act(t))
		})
	})
}
