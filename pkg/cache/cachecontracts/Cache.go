package cachecontracts

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.llib.dev/frameless/internal/constant"
	cachepkg "go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/option"
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

func Cache[ENT any, ID comparable](
	cache CacheSubject[ENT, ID],
	source cacheSource[ENT, ID],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[ENT, ID]](opts)

	suites := []testcase.Suite{}
	suites = append(suites, crudcontracts.ByIDDeleter[ENT, ID](cache, c.CRUD))
	if _, ok := source.(crud.Creator[ENT]); ok {
		suites = append(suites, crudcontracts.Creator[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllFinder[ENT]); ok {
		suites = append(suites, crudcontracts.AllFinder[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.ByIDDeleter[ENT]); ok {
		suites = append(suites, crudcontracts.ByIDDeleter[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllDeleter); ok {
		suites = append(suites, crudcontracts.AllDeleter[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.AllDeleter); !ok {
		suites = append(suites, crudcontracts.AllDeleter[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.Updater[ENT]); ok {
		suites = append(suites, crudcontracts.Updater[ENT, ID](cache, c.CRUD))
	}
	if _, ok := source.(crud.Saver[ENT]); ok {
		suites = append(suites, crudcontracts.Saver[ENT, ID](cache, c.CRUD))
	}

	suites = append(suites, Repository[ENT, ID](repository, c))

	// TODO: support OnePhaseCommitProtocol with cache.Cache

	s.Describe(".InvalidateCachedQuery", func(s *testcase.Spec) {
		specInvalidateCachedQuery[ENT, ID](s, cache, source, repository)
	})
	s.Describe(".InvalidateByID", func(s *testcase.Spec) {
		specInvalidateByID(s, cache, source, repository)
	})
	s.Describe(".CachedQueryMany", func(s *testcase.Spec) {
		specCachedQueryMany[ENT, ID](s, cache, source, repository)
	})

	s.Context(`cache behaviour`, func(s *testcase.Spec) {
		describeResultCaching[ENT, ID](s, cache, source, repository)
		describeCacheInvalidationByEventsThatMutatesAnEntity[ENT, ID](s, cache, source, repository)
	})

	return s.AsSuite("Cache")
}

type CacheSubject[ENT, ID any] interface {
	cachepkg.Interface[ENT, ID]
	crud.Creator[ENT]
	crud.Saver[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.AllFinder[ENT]
	crud.Updater[ENT]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
}

type cacheSource[ENT, ID any] interface {
	sh.CRUD[ENT, ID]
	cachepkg.Source[ENT, ID]
}

func describeCacheInvalidationByEventsThatMutatesAnEntity[ENT any, ID comparable](
	s *testcase.Spec,
	cache CacheSubject[ENT, ID],
	source cacheSource[ENT, ID],
	repository cachepkg.Repository[ENT, ID], // TODO: remove this if this is not used, else check if something wrong with the setup
	opts ...Option[ENT, ID],
) {
	c := option.Use[Config[ENT, ID]](opts)
	s.Context(reflectkit.SymbolicName(*new(ENT)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) interface{} {
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.DeleteByID, c.CRUD.MakeContext(t), id)
			return ptr
		})

		s.Before(func(t *testcase.T) {
			t.Must.NoError(cache.DropCachedValues(c.CRUD.MakeContext(t)))
		})

		s.Test(`an update to the repository should refresh the by id look`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext(t)
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(ctx, entID)                          // should trigger caching
			_, _ = iterators.Count(iterators.WithErr(cache.FindAll(ctx))) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[ENT, ID](t, cache, ctx, vUpdated)
			waiter.Wait()

			ptr := crudtest.IsPresent[ENT, ID](t, cache, ctx, entID) // should trigger caching
			t.Must.Equal(vUpdated, ptr)
		})

		s.Test(`an update to the repository should refresh the QueryMany cache hits`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext(t)
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(ctx, entID)                          // should trigger caching
			_, _ = iterators.Count(iterators.WithErr(cache.FindAll(ctx))) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[ENT, ID](t, cache, ctx, vUpdated)
			waiter.Wait()

			var (
				gotEnt ENT
				found  bool
			)
			t.Must.NoError(iterators.ForEach(iterators.WithErr(cache.FindAll(ctx)), func(ent ENT) error {
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
			_, _, _ = cache.FindByID(c.CRUD.MakeContext(t), id)                             // should trigger caching
			_, _ = iterators.Count(iterators.WithErr(cache.FindAll(c.CRUD.MakeContext(t)))) // should trigger caching

			// delete
			t.Must.NoError(cache.DeleteByID(c.CRUD.MakeContext(t), id))

			// assert
			crudtest.IsAbsent[ENT, ID](t, cache, c.CRUD.MakeContext(t), id)
		})

		s.Test(`a delete all entity in the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.FindByID(c.CRUD.MakeContext(t), id)                             // should trigger caching
			_, _ = iterators.Count(iterators.WithErr(cache.FindAll(c.CRUD.MakeContext(t)))) // should trigger caching

			// delete
			t.Must.NoError(cache.DeleteAll(c.CRUD.MakeContext(t)))
			waiter.Wait()

			crudtest.IsAbsent[ENT, ID](t, cache, c.CRUD.MakeContext(t), id) // should trigger caching for not found
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
			ctx := c.CRUD.MakeContext(t)
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.DeleteByID, ctx, id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			id, found := extid.Lookup[ID](value.Get(t))
			assert.Must(t).True(found)
			v, found, err := cache.FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			assert.Must(t).True(found)
			assert.Must(t).Equal(*value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup[ID](value.Get(t))
				assert.Must(t).True(found)
				v := crudtest.IsPresent[Entity, ID](t, source, c.CRUD.MakeContext(t), id)
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
					crudtest.Update[Entity, ID](t, cache, c.CRUD.MakeContext(t), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					t.Must.NotEmpty(id)
					crudtest.HasEntity[Entity, ID](t, cache, c.CRUD.MakeContext(t), valueWithNewContent.Get(t))

					eventually.Assert(t, func(it assert.It) {
						v, found, err := cache.FindByID(c.CRUD.MakeContext(t), id)
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
					v, found, err := cache.FindByID(c.CRUD.MakeContext(t), id)
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
					assert.Must(t).Equal(val, crudtest.IsPresent[Entity, ID](t, cache, c.CRUD.MakeContext(t), id))
					numberOfFindByIDCallAfterEntityIsFound := spy.Get(t).count.FindByID
					waiter.Wait()

					nv, found, err := cache.FindByID(c.CRUD.MakeContext(t), id) // should use cached val
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
			return c.CRUD.MakeContext(t)
		})
		hitID = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return cachepkg.Query{Name: constant.String(t.Random.UUID())}.HitID()
		})
		query = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	)
	act := func(t *testcase.T) (iterators.Iterator[Entity], error) {
		return subject.CachedQueryMany(Context.Get(t), hitID.Get(t), query.Get(t))
	}

	s.When("query returns values", func(s *testcase.Spec) {
		var (
			ent1 = testcase.Let(s, func(t *testcase.T) *Entity {
				v := c.CRUD.MakeEntity(t)
				crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(t), &v)
				return &v
			})
			ent2 = testcase.Let(s, func(t *testcase.T) *Entity {
				v := c.CRUD.MakeEntity(t)
				crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(t), &v)
				return &v
			})
		)

		query.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[Entity] {
			return func() (iterators.Iterator[Entity], error) {
				return iterators.Slice[Entity]([]Entity{*ent1.Get(t), *ent2.Get(t)}), nil
			}
		})

		s.Then("it will return all the entities", func(t *testcase.T) {
			iter, err := act(t)
			assert.NoError(t, err)
			vs, err := iterators.Collect(iter)
			t.Must.NoError(err)
			t.Must.ContainExactly([]Entity{*ent1.Get(t), *ent2.Get(t)}, vs)
		})

		s.Then("it will cache all returned entities", func(t *testcase.T) {
			iter, err := act(t)
			assert.NoError(t, err)
			vs, err := iterators.Collect(iter)
			t.Must.NoError(err)

			cached, err := iterators.Collect(iterators.WithErr(repository.Entities().FindAll(c.CRUD.MakeContext(t))))
			t.Must.NoError(err)
			t.Must.Contain(cached, vs)
		})

		s.Then("it will create a hit record", func(t *testcase.T) {
			iter, err := act(t)
			assert.NoError(t, err)
			_, err = iterators.Collect(iter)
			t.Must.NoError(err)

			hits, err := iterators.Collect(iterators.WithErr(repository.Hits().FindAll(c.CRUD.MakeContext(t))))
			t.Must.NoError(err)

			assert.OneOf(t, hits, func(it assert.It, got cachepkg.Hit[ID]) {
				it.Must.Equal(got.ID, hitID.Get(t))
				it.Must.ContainExactly(got.EntityIDs, []ID{
					crudtest.HasID[Entity, ID](t, ent1.Get(t)),
					crudtest.HasID[Entity, ID](t, ent2.Get(t)),
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
			return c.CRUD.MakeContext(t)
		})
		hitID = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return cachepkg.Query{Name: constant.String(t.Random.UUID())}.HitID()
		})
	)
	act := func(t *testcase.T) error {
		return cache.InvalidateCachedQuery(Context.Get(t), hitID.Get(t))
	}

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[Entity]](s, nil)
	queryOne := func(t *testcase.T) (Entity, bool, error) {
		return cache.
			CachedQueryOne(c.CRUD.MakeContext(t), hitID.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	queryMany := func(t *testcase.T) (iterators.Iterator[Entity], error) {
		return cache.CachedQueryMany(c.CRUD.MakeContext(t), hitID.Get(t), queryManyFunc.Get(t))
	}

	s.When("queryKey has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[Entity] {
			return func() (Entity, bool, error) {
				id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
				return source.FindByID(c.CRUD.MakeContext(t), id)
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(t), id))
			// we have hits
			n, err := iterators.Count(iterators.WithErr(repository.Hits().FindAll(c.CRUD.MakeContext(t))))
			t.Must.NoError(err)
			t.Must.NotEqual(0, n)
			// we have cached entities
			n, err = iterators.Count(iterators.WithErr(repository.Entities().FindAll(c.CRUD.MakeContext(t))))
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
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
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
			return func() (iterators.Iterator[Entity], error) {
				id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
				ent, found, err := source.FindByID(c.CRUD.MakeContext(t), id)
				if err != nil {
					return nil, err
				}
				if !found {
					return iterators.Empty[Entity](), nil
				}
				return iterators.SingleValue(ent), nil
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			vs, err = iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("queryKey does not belong to any cached query hit", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
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
			return c.CRUD.MakeContext(t)
		})
		id = testcase.Let[ID](s, nil)
	)
	act := func(t *testcase.T) error {
		return cache.InvalidateByID(Context.Get(t), id.Get(t))
	}

	hitID := testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
		return cachepkg.Query{Name: "operation-name"}.HitID()
	})

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[Entity]](s, nil)
	queryOne := func(t *testcase.T) (Entity, bool, error) {
		return cache.
			CachedQueryOne(c.CRUD.MakeContext(t), hitID.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[Entity]](s, nil)
	queryMany := func(t *testcase.T) (iterators.Iterator[Entity], error) {
		return cache.
			CachedQueryMany(c.CRUD.MakeContext(t), hitID.Get(t), queryManyFunc.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.Cleanup(func() {
			t.Must.NoError(cache.
				InvalidateCachedQuery(c.CRUD.MakeContext(t), hitID.Get(t)))
		})
	})

	s.When("entity id has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		id.Let(s, func(t *testcase.T) ID {
			return crudtest.HasID[Entity, ID](t, entPtr.Get(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[Entity] {
			return func() (Entity, bool, error) {
				return source.FindByID(c.CRUD.MakeContext(t), id.Get(t))
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(t), id))
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
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
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
			return crudtest.HasID[Entity, ID](t, entPtr.Get(t))
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := cache.FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			ent, found, err = cache.FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			ent, found, err := cache.FindByID(c.CRUD.MakeContext(t), id.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
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
			return crudtest.HasID[Entity, ID](t, entPtr.Get(t))
		})

		queryManyFunc.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[Entity] {
			return func() (iterators.Iterator[Entity], error) {
				id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
				ent, found, err := source.FindByID(c.CRUD.MakeContext(t), id)
				if err != nil {
					return nil, err
				}
				if !found {
					return iterators.Empty[Entity](), nil
				}
				return iterators.SingleValue(ent), nil
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			vs, err = iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterators.Collect(iterators.WithErr(queryMany(t)))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := crudtest.HasID[Entity, ID](t, entPtr.Get(t))

			ent, found, err := repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Entities().FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("hit for the query key is flushed", func(t *testcase.T) {
			ent, found, err := repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.NotEmpty(ent)

			t.Must.NoError(act(t))

			ent, found, err = repository.Hits().FindByID(c.CRUD.MakeContext(t), hitID.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})
	})

	s.When("entity id does not belong to any cached query hit", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := c.CRUD.MakeEntity(t)
			crudtest.Create[Entity, ID](t, source, c.CRUD.MakeContext(t), &ent)
			v := crudtest.HasID[Entity, ID](t, &ent)
			crudtest.Delete[Entity, ID](t, source, c.CRUD.MakeContext(t), &ent)
			return v
		})

		s.Before(func(t *testcase.T) {
			_, found, err := source.FindByID(c.CRUD.MakeContext(t), id.Get(t))
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
