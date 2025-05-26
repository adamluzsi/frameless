package cachecontracts

import (
	"context"
	"io"
	"iter"
	"reflect"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/internal/constant"
	cachepkg "go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/option"
	sh "go.llib.dev/frameless/spechelper"
	"go.llib.dev/frameless/spechelper/testent"
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

var _ io.Closer = &cachepkg.Cache[testent.Foo, testent.FooID]{}
var _ cachepkg.Interface[testent.Foo, testent.FooID] = &cachepkg.Cache[testent.Foo, testent.FooID]{}

func Cache[ENT any, ID comparable](repository cachepkg.Repository[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use(opts)

	source := testcase.Let(s, func(t *testcase.T) cacheSource[ENT, ID] {
		return &memory.Repository[ENT, ID]{}
	})

	cache := testcase.Let(s, func(t *testcase.T) *cachepkg.Cache[ENT, ID] {
		ch := &cachepkg.Cache[ENT, ID]{
			Source:     source.Get(t),
			Repository: repository,
		}
		t.Defer(ch.Close)
		return ch
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, cache.Get(t).DropCachedValues(c.CRUD.MakeContext(t)))
	})

	s.Describe("#InvalidateCachedQuery", func(s *testcase.Spec) {
		specInvalidateCachedQuery[ENT, ID](s, cache, source, repository)
	})

	s.Describe("#InvalidateByID", func(s *testcase.Spec) {
		specInvalidateByID[ENT, ID](s, cache, source, repository)
	})

	s.Describe("#CachedQueryMany", func(s *testcase.Spec) {
		specCachedQueryMany[ENT, ID](s, cache, source, repository)
	})

	s.Context("result caching behaviour", func(s *testcase.Spec) {
		describeResultCaching[ENT, ID](s, cache, source)
	})

	s.Context("invalidation by events that mutates an entity", func(s *testcase.Spec) {
		describeCacheInvalidationByEventsThatMutatesAnEntity[ENT, ID](s, cache, source)
	})

	s.Context("RefreshBehind option", func(s *testcase.Spec) {
		describeCacheRefreshBehind[ENT, ID](s, cache, source, repository)
	})

	s.Context("refresh", func(s *testcase.Spec) {
		describeCacheRefresh[ENT, ID](s, cache, source, repository)
	})

	// s.Context("TimeToLive option", func(s *testcase.Spec) {
	// 	describeCacheTimeToLive[ENT, ID](s, cache, source, repository)
	// })

	{
		ch := &cachepkg.Cache[ENT, ID]{
			Source:     &memory.Repository[ENT, ID]{},
			Repository: repository,
		}
		testcase.RunSuite(s,
			crudcontracts.ByIDDeleter[ENT, ID](ch, c.CRUD),
			crudcontracts.Creator[ENT, ID](ch, c.CRUD),
			crudcontracts.AllFinder[ENT, ID](ch, c.CRUD),
			crudcontracts.ByIDDeleter[ENT, ID](ch, c.CRUD),
			crudcontracts.AllDeleter[ENT, ID](ch, c.CRUD),
			crudcontracts.AllDeleter[ENT, ID](ch, c.CRUD),
			crudcontracts.Updater[ENT, ID](ch, c.CRUD),
			crudcontracts.Saver[ENT, ID](ch, c.CRUD),
			Repository[ENT, ID](repository, c),
		)
	}

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
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)
	s.Context(reflectkit.SymbolicName(*new(ENT)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) interface{} {
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.Get(t).DeleteByID, c.CRUD.MakeContext(t), id)
			return ptr
		})

		s.Before(func(t *testcase.T) {
			t.Must.NoError(cache.Get(t).DropCachedValues(c.CRUD.MakeContext(t)))
		})

		s.Test(`an update to the repository should refresh the by id look`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext(t)
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.Get(t).FindByID(ctx, entID)

			// should trigger caching
			iterkit.CollectErr(cache.Get(t).FindAll(ctx))

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[ENT, ID](t, cache.Get(t), ctx, vUpdated)
			waiter.Wait()

			ptr := c.CRUD.Helper().IsPresent(t, cache.Get(t), ctx, entID) // should trigger caching
			t.Must.Equal(vUpdated, ptr)
		})

		s.Test(`an update to the repository should refresh the QueryMany cache hits`, func(t *testcase.T) {
			ctx := c.CRUD.MakeContext(t)
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.Get(t).FindByID(ctx, entID)          // should trigger caching
			_, _ = iterkit.CollectErr(cache.Get(t).FindAll(ctx)) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(extid.Set(vUpdated, entID))
			crudtest.Update[ENT, ID](t, cache.Get(t), ctx, vUpdated)
			waiter.Wait()

			var (
				gotEnt ENT
				found  bool
			)
			for ent, err := range cache.Get(t).FindAll(ctx) {
				assert.NoError(t, err)
				id, ok := extid.Lookup[ID](ent)
				assert.True(t, ok, "lookup can't find the ID")
				if reflect.DeepEqual(entID, id) {
					found = true
					gotEnt = ent
					break
				}
			}

			t.Must.True(found, "it was expected to find the entity in the FindAll query result")
			t.Must.Equal(vUpdated, &gotEnt)
		})

		s.Test(`a delete by id to the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)             // should trigger caching
			_, _ = iterkit.CollectErr(cache.Get(t).FindAll(c.CRUD.MakeContext(t))) // should trigger caching

			// delete
			t.Must.NoError(cache.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))

			// assert
			c.CRUD.Helper().IsAbsent(t, cache.Get(t), c.CRUD.MakeContext(t), id)
		})

		s.Test(`a delete all entity in the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)             // should trigger caching
			_, _ = iterkit.CollectErr(cache.Get(t).FindAll(c.CRUD.MakeContext(t))) // should trigger caching

			// delete
			t.Must.NoError(cache.Get(t).DeleteAll(c.CRUD.MakeContext(t)))
			waiter.Wait()

			c.CRUD.Helper().IsAbsent(t, cache.Get(t), c.CRUD.MakeContext(t), id) // should trigger caching for not found
		})
	})
}

func describeCacheRefreshBehind[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)

	spy := testcase.Let(s, func(t *testcase.T) *spySource[ENT, ID] {
		return &spySource[ENT, ID]{cacheSource: source.Get(t)}
	})

	RefreshBehind := testcase.Let[bool](s, func(t *testcase.T) bool {
		return t.Random.Bool()
	})

	cache.Let(s, func(t *testcase.T) *cachepkg.Cache[ENT, ID] {
		return &cachepkg.Cache[ENT, ID]{
			Source:     spy.Get(t),
			Repository: repository,

			RefreshBehind: RefreshBehind.Get(t),
		}
	})

	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext(t)
		})
	)
	act := func(t *testcase.T) iter.Seq2[ENT, error] {
		return cache.Get(t).FindAll(Context.Get(t))
	}

	var AfterAct = func(t *testcase.T) []ENT {
		t.Helper()
		vs, err := iterkit.CollectErr(act(t))
		assert.NoError(t, err)
		assert.NotEmpty(t, vs)
		return vs
	}

	value := testcase.Let(s, func(t *testcase.T) *ENT {
		ctx := c.CRUD.MakeContext(t)
		ptr := pointer.Of(c.CRUD.MakeEntity(t))
		c.CRUD.Helper().Create(t, source.Get(t), ctx, ptr)
		return ptr
	})

	s.Before(func(t *testcase.T) {
		id, found := extid.Lookup[ID](value.Get(t))
		assert.True(t, found)
		v := c.CRUD.Helper().IsPresent(t, source.Get(t), c.CRUD.MakeContext(t), id)
		assert.Equal(t, value.Get(t), v)
	})

	s.After(func(t *testcase.T) {
		t.Eventually(func(t *testcase.T) {
			// due to how asynchronous background jobs work, we need to wait till the cache is idle again,
			// else we risk that a refresh behind query leaks across tests.
			assert.True(t, cache.Get(t).Idle())
		})
	})

	s.When("a value is already cached", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			AfterAct(t)

			t.Eventually(func(t *testcase.T) { // wait until a potential
				assert.True(t, cache.Get(t).Idle())
			})
		}) // trigger caching

		s.And(`this value is being modified in the source`, func(s *testcase.Spec) {
			valueWithNewContent := testcase.Let(s, func(t *testcase.T) *ENT {
				ptr := value.Get(t)
				c.CRUD.ModifyEntity(t, ptr)
				return ptr
			})

			s.Before(func(t *testcase.T) {
				ptr := valueWithNewContent.Get(t)
				crudtest.Update[ENT, ID](t, source.Get(t), c.CRUD.MakeContext(t), ptr)
				waiter.Wait()
			})

			s.When("RefreshBehind is set to true", func(s *testcase.Spec) {
				RefreshBehind.LetValue(s, true)

				s.Then(`querying it continously will refresh the value behind the scenes`, func(t *testcase.T) {
					t.Eventually(func(t *testcase.T) {
						assert.Contain(t, AfterAct(t), *valueWithNewContent.Get(t))
					})
				})

				s.Then("source eventually accessed", func(t *testcase.T) {
					t.Log("given we query the cache multiple times")

					initial := spy.Get(t).count.Total
					t.Random.Repeat(3, 7, func() {
						AfterAct(t)
					})

					t.Log("then eventually the query is executed behind the scenes")
					t.Eventually(func(t *testcase.T) {
						total := spy.Get(t).count.Total
						assert.True(t, initial < total,
							assert.MessageF("%d < %d", initial, total))
					})
				})

				s.And("interacting with the source suddenly slows down and would take longer than before the iniator request finish", func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						spy.Get(t).sleepOn.FindAll = time.Second
						assert.Equal(t, spy.Get(t).sleepOn.FindAll, time.Second)
					})

					s.Then("refresh behind should still succeed", func(t *testcase.T) {
						t.Log("given we query the cache multiple times")

						initial := spy.Get(t).count.Total

						ctx, cancel := context.WithCancel(c.CRUD.MakeContext(t))
						for _, err := range cache.Get(t).FindAll(ctx) {
							assert.NoError(t, err)
						}
						cancel() // request life ended, cancelling is done

						t.Log("then eventually the query is executed behind the scenes")
						t.Eventually(func(t *testcase.T) {
							total := spy.Get(t).count.Total
							assert.True(t, initial < total,
								"expected that source was called behind the scene other than the initial caching act.",
								"It is possible that the query running begind the scene as part of refresh-behind got cancelled")
						})
					})
				})
			})

			s.When("RefreshBehind is set to false", func(s *testcase.Spec) {
				RefreshBehind.LetValue(s, false)

				s.Then(`querying it continously won't change the outcome of the currently cached value`, func(t *testcase.T) {
					assert.NotWithin(t, time.Second, func(ctx context.Context) {
						for ctx.Err() == nil {
							assert.NotContain(t, AfterAct(t), *valueWithNewContent.Get(t))
							// time.Sleep(10 * time.Millisecond)
						}
					}).Wait()
				})

				s.Then("source accessed only once to do the caching", func(t *testcase.T) {
					t.Log("given we query the cache multiple times")
					t.Random.Repeat(2, 7, func() { AfterAct(t) })

					t.Log("source only accessed once")
					assert.Equal(t, 1, spy.Get(t).count.Total)
				})
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		RefreshBehind.LetValue(s, true)

		s.Test("FindByID", func(t *testcase.T) {
			ctx := c.CRUD.MakeContext(t)
			id, ok := c.CRUD.IDA.Lookup(*value.Get(t))
			assert.True(t, ok)

			t.Log("a value is already cached")
			got, found, err := cache.Get(t).FindByID(ctx, id)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, got, *value.Get(t))

			t.Eventually(func(t *testcase.T) { // wait until a potential
				assert.True(t, cache.Get(t).Idle())
			})

			t.Log("this value is being modified in the source")
			valueWithNewContent := value.Get(t)
			c.CRUD.ModifyEntity(t, valueWithNewContent)
			crudtest.Update[ENT, ID](t, source.Get(t), c.CRUD.MakeContext(t), valueWithNewContent)
			waiter.Wait()

			t.Log("eventually the data refreshes")
			t.Eventually(func(t *testcase.T) {
				got, found, err := cache.Get(t).FindByID(ctx, id)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, got, *valueWithNewContent)
			})
		})
	})

}

func describeCacheRefresh[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)

	spy := testcase.Let(s, func(t *testcase.T) *spySource[ENT, ID] {
		return &spySource[ENT, ID]{cacheSource: source.Get(t)}
	})

	cache.Let(s, func(t *testcase.T) *cachepkg.Cache[ENT, ID] {
		return &cachepkg.Cache[ENT, ID]{
			Source:     spy.Get(t),
			Repository: repository,

			RefreshBehind: false,
		}
	})

	var Context = testcase.Let(s, func(t *testcase.T) context.Context {
		return c.CRUD.MakeContext(t)
	})

	s.Test("RefreshQueryMany", func(t *testcase.T) {
		hitID := cachepkg.HitID(t.Random.String())

		var res []ENT
		t.Random.Repeat(3, 7, func() {
			v := c.CRUD.MakeEntity(t)
			id := c.MakeID(t)
			assert.NoError(t, c.CRUD.IDA.Set(&v, id))
			res = append(res, v)
		})

		var query = func(t *testcase.T) []ENT {
			vs, err := iterkit.CollectErr(cache.Get(t).CachedQueryMany(c.CRUD.MakeContext(t),
				hitID,
				func(ctx context.Context) iter.Seq2[ENT, error] {
					return iterkit.ToErrSeq(iterkit.Slice(res))
				}))
			assert.NoError(t, err)
			return vs
		}

		var refreshQuery = func(t *testcase.T) error {
			return cache.Get(t).RefreshQueryMany(c.CRUD.MakeContext(t),
				hitID,
				func(ctx context.Context) iter.Seq2[ENT, error] {
					return iterkit.ToErrSeq(iterkit.Slice(res))
				})
		}

		t.Log("a value is already cached")
		assert.ContainExactly(t, query(t), res)

		t.Eventually(func(t *testcase.T) {
			assert.True(t, cache.Get(t).Idle())
		})

		t.Log("this value is being modified in the source")
		t.Random.Repeat(1, 3, func() {
			v := c.CRUD.MakeEntity(t)
			id := c.MakeID(t)
			assert.NoError(t, c.CRUD.IDA.Set(&v, id))
			res = append(res, v)
		})

		assert.NotContain(t, query(t), res)

		t.Log("then the data refreshes when refresh many is called")
		assert.NoError(t, refreshQuery(t))

		t.Eventually(func(t *testcase.T) {
			assert.ContainExactly(t, query(t), res)
		})
	})

	s.Test("RefreshQueryOne", func(t *testcase.T) {
		value := c.CRUD.MakeEntity(t)
		id := c.MakeID(t)
		assert.NoError(t, c.CRUD.IDA.Set(&value, id))
		hitID := cachepkg.HitID(t.Random.String())

		refresh := func(t *testcase.T) error {
			return cache.Get(t).RefreshQueryOne(c.CRUD.MakeContext(t),
				hitID,
				func(ctx context.Context) (_ ENT, found bool, _ error) {
					return value, true, nil
				})
		}

		query := func(t *testcase.T) (ENT, bool) {
			v, found, err := cache.Get(t).CachedQueryOne(c.CRUD.MakeContext(t),
				hitID,
				func(ctx context.Context) (_ ENT, found bool, _ error) {
					return value, true, nil
				})
			assert.NoError(t, err)
			return v, found
		}

		t.Log("a value is already cached")
		got, found := query(t)
		assert.True(t, found)
		assert.Equal(t, got, value)

		t.Eventually(func(t *testcase.T) {
			assert.True(t, cache.Get(t).Idle())
		})

		t.Log("this value is being modified in the source")
		c.CRUD.ModifyEntity(t, &value)

		t.Log("the cached data differes from what it is in the source")
		got, found = query(t)
		assert.True(t, found)
		assert.NotEqual(t, got, value)

		t.Log("then the data refreshes when refresh is called")
		assert.NoError(t, refresh(t))

		t.Log("eventually the new state of the value can be retrieved")
		t.Eventually(func(t *testcase.T) {
			got, found := query(t)
			assert.True(t, found)
			assert.Equal(t, got, value)
		})
	})

	s.Test("Refresh + FindAll", func(t *testcase.T) {
		if _, ok := source.Get(t).(crud.AllFinder[ENT]); !ok {
			t.Skipf("%T doesn't implement crud.AllFinder", source.Get(t))
		}

		refreshFindAll := func(t *testcase.T) error {
			return cache.Get(t).Refresh(Context.Get(t))
		}

		ctx := c.CRUD.MakeContext(t)
		value := c.CRUD.MakeEntity(t)
		t.Must.NoError(source.Get(t).Create(ctx, &value))
		id, _ := extid.Lookup[ID](value)
		t.Defer(source.Get(t).DeleteByID, ctx, id)

		id, ok := c.CRUD.IDA.Lookup(value)
		assert.True(t, ok)

		t.Log("a value is already cached")
		got, found, err := cache.Get(t).FindByID(ctx, id)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, got, value)

		t.Eventually(func(t *testcase.T) {
			assert.True(t, cache.Get(t).Idle())
		})

		t.Log("this value is being modified in the source")
		valueWithNewContent := value // pass by value copy
		c.CRUD.ModifyEntity(t, &valueWithNewContent)
		crudtest.Update[ENT, ID](t, source.Get(t), c.CRUD.MakeContext(t), &valueWithNewContent)
		waiter.Wait()

		t.Log("then the data refreshes when refresh many is called")
		assert.NoError(t, refreshFindAll(t))

		t.Eventually(func(t *testcase.T) {
			got, found, err := cache.Get(t).FindByID(ctx, id)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, got, valueWithNewContent)
		})
	})

	s.Test("RefreshByID + FindByID", func(t *testcase.T) {
		ctx := c.CRUD.MakeContext(t)
		value := c.CRUD.MakeEntity(t)
		t.Must.NoError(source.Get(t).Create(ctx, &value))
		id, _ := extid.Lookup[ID](value)
		t.Defer(source.Get(t).DeleteByID, ctx, id)
		id, ok := c.CRUD.IDA.Lookup(value)
		assert.True(t, ok)

		refresh := func(t *testcase.T) error {
			return cache.Get(t).RefreshByID(c.CRUD.MakeContext(t), id)
		}

		query := func(t *testcase.T) (ENT, bool) {
			v, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
			assert.NoError(t, err)
			return v, found
		}

		t.Log("a value is already cached")
		got, found := query(t)
		assert.True(t, found)
		assert.Equal(t, got, value)

		t.Eventually(func(t *testcase.T) {
			assert.True(t, cache.Get(t).Idle())
		})

		t.Log("this value is being modified in the source")
		valueWithNewContent := value // pass by value copy
		c.CRUD.ModifyEntity(t, &valueWithNewContent)
		crudtest.Update[ENT, ID](t, source.Get(t), c.CRUD.MakeContext(t), &valueWithNewContent)
		waiter.Wait()

		t.Log("then the data refreshes when refresh many is called")
		assert.NoError(t, refresh(t))

		t.Eventually(func(t *testcase.T) {
			got, found := query(t)
			assert.True(t, found)
			assert.Equal(t, got, valueWithNewContent)
		})
	})

	s.When("source doesn't support crudAllFinder", func(s *testcase.Spec) {
		cache.Let(s, func(t *testcase.T) *cachepkg.Cache[ENT, ID] {
			type CacheSourceWOAF struct {
				crud.Creator[ENT]
				crud.ByIDFinder[ENT, ID]
				crud.Updater[ENT]
				crud.ByIDDeleter[ID]
			}
			ch := cache.Super(t)
			ch.Source = CacheSourceWOAF{
				ByIDFinder:  source.Get(t),
				ByIDDeleter: source.Get(t),
				Creator:     source.Get(t),
				Updater:     source.Get(t),
			}
			return ch
		})

		s.Test("Refresh still refresh stored entities", func(t *testcase.T) {
			refreshFindAll := func(t *testcase.T) error {
				return cache.Get(t).Refresh(Context.Get(t))
			}

			ctx := c.CRUD.MakeContext(t)
			value := c.CRUD.MakeEntity(t)
			t.Must.NoError(source.Get(t).Create(ctx, &value))
			id, _ := extid.Lookup[ID](value)
			t.Defer(source.Get(t).DeleteByID, ctx, id)

			id, ok := c.CRUD.IDA.Lookup(value)
			assert.True(t, ok)

			t.Log("a value is already cached")
			got, found, err := cache.Get(t).FindByID(ctx, id)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, got, value)

			t.Eventually(func(t *testcase.T) {
				assert.True(t, cache.Get(t).Idle())
			})

			t.Log("this value is being modified in the source")
			valueWithNewContent := value // pass by value copy
			c.CRUD.ModifyEntity(t, &valueWithNewContent)
			crudtest.Update[ENT, ID](t, source.Get(t), c.CRUD.MakeContext(t), &valueWithNewContent)
			waiter.Wait()

			t.Log("then the data refreshes when refresh many is called")
			assert.NoError(t, refreshFindAll(t))

			t.Eventually(func(t *testcase.T) {
				got, found, err := cache.Get(t).FindByID(ctx, id)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, got, valueWithNewContent)
			})
		})
	})
}

// func describeCacheTimeToLive[ENT any, ID comparable](s *testcase.Spec,
// 	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
// 	source testcase.Var[cacheSource[ENT, ID]],
// 	repository cachepkg.Repository[ENT, ID],
// 	opts ...Option[ENT, ID],
// ) {
// 	c := option.Use(opts)
//
// 	spy := testcase.Let(s, func(t *testcase.T) *spySource[ENT, ID] {
// 		return &spySource[ENT, ID]{cacheSource: source.Get(t)}
// 	})
//
// 	ttl := testcase.LetValue[time.Duration](s, 0)
//
// 	cache.Let(s, func(t *testcase.T) *cachepkg.Cache[ENT, ID] {
// 		return &cachepkg.Cache[ENT, ID]{
// 			Source:     spy.Get(t),
// 			Repository: repository,
// 			TimeToLive: ttl.Get(t),
// 		}
// 	})
//
// 	var (
// 		Context = testcase.Let(s, func(t *testcase.T) context.Context {
// 			return c.CRUD.MakeContext(t)
// 		})
// 		hitID = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
// 			return cachepkg.Query{Name: constant.String(t.Random.UUID())}.HitID()
// 		})
// 		query = testcase.Let[cachepkg.QueryManyFunc[ENT]](s, nil)
// 	)
// 	act := func(t *testcase.T) (iter.Seq2[ENT, error], error) {
// 		return cache.Get(t).CachedQueryMany(Context.Get(t), hitID.Get(t), query.Get(t))
// 	}
//
// 	_ = act
// }

type spySource[ENT, ID any] struct {
	cacheSource[ENT, ID]
	count struct {
		Total    int
		FindAll  int
		FindByID int
	}
	sleepOn struct {
		FindAll  time.Duration
		FindByID time.Duration
	}
}

func (spy *spySource[ENT, ID]) FindAll(ctx context.Context) iter.Seq2[ENT, error] {
	spy.count.Total++
	spy.count.FindAll++
	time.Sleep(spy.sleepOn.FindAll)
	return spy.cacheSource.(crud.AllFinder[ENT]).FindAll(ctx)
}

func (spy *spySource[ENT, ID]) FindByID(ctx context.Context, id ID) (_ent ENT, _found bool, _err error) {
	spy.count.Total++
	spy.count.FindByID++
	time.Sleep(spy.sleepOn.FindByID)
	return spy.cacheSource.FindByID(ctx, id)
}

func describeResultCaching[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)
	s.Context(reflectkit.SymbolicName(reflectkit.TypeOf[ENT]()), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) *ENT {
			ctx := c.CRUD.MakeContext(t)
			ptr := pointer.Of(c.CRUD.MakeEntity(t))
			t.Must.NoError(source.Get(t).Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(source.Get(t).DeleteByID, ctx, id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			id, found := extid.Lookup[ID](value.Get(t))
			assert.True(t, found)
			v, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			assert.True(t, found)
			assert.Equal(t, *value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup[ID](value.Get(t))
				assert.True(t, found)
				v := c.CRUD.Helper().IsPresent(t, source.Get(t), c.CRUD.MakeContext(t), id)
				assert.Equal(t, value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := testcase.Let(s, func(t *testcase.T) *ENT {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.True(t, found)
					nv := pointer.Of(c.CRUD.MakeEntity(t))
					t.Must.NoError(extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					crudtest.Update[ENT, ID](t, cache.Get(t), c.CRUD.MakeContext(t), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.True(t, found)
					t.Must.NotEmpty(id)
					c.CRUD.Helper().HasEntity(t, cache.Get(t), c.CRUD.MakeContext(t), valueWithNewContent.Get(t))

					eventually.Assert(t, func(it assert.It) {
						v, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
						it.Must.NoError(err)
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
				assert.True(t, found)

				for i := 0; i < 42; i++ {
					v, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
					t.Must.NoError(err)
					assert.True(t, found)
					assert.Equal(t, *value, v)
				}
			})

			s.When(`the repository is sensitive to continuous requests`, func(s *testcase.Spec) {
				spy := testcase.Let[*spySource[ENT, ID]](s, nil)

				source.Let(s, func(t *testcase.T) cacheSource[ENT, ID] {
					og := source.Super(t)
					spysrc := &spySource[ENT, ID]{cacheSource: og}
					spy.Set(t, spysrc)
					return spysrc
				}).EagerLoading(s)

				s.Then(`it will only bother the repository for the value once`, func(t *testcase.T) {
					var err error
					val := value.Get(t)
					id, found := extid.Lookup[ID](val)
					assert.True(t, found)

					// trigger caching
					assert.Equal(t, val, c.CRUD.Helper().IsPresent(t, cache.Get(t), c.CRUD.MakeContext(t), id))
					numberOfFindByIDCallAfterEntityIsFound := spy.Get(t).count.FindByID
					waiter.Wait()

					nv, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id) // should use cached val
					t.Must.NoError(err)
					assert.True(t, found)
					assert.Equal(t, *val, nv)
					assert.Equal(t, numberOfFindByIDCallAfterEntityIsFound, spy.Get(t).count.FindByID)
				})
			})
		})
	})
}

func specCachedQueryMany[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext(t)
		})
		hitID = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return cachepkg.Query{Name: constant.String(t.Random.UUID())}.HitID()
		})
		query = testcase.Let[cachepkg.QueryManyFunc[ENT]](s, nil)
	)
	act := func(t *testcase.T) iter.Seq2[ENT, error] {
		return cache.Get(t).CachedQueryMany(Context.Get(t), hitID.Get(t), query.Get(t))
	}

	s.When("query returns values", func(s *testcase.Spec) {
		var (
			ent1 = testcase.Let(s, func(t *testcase.T) *ENT {
				v := c.CRUD.MakeEntity(t)
				c.CRUD.Helper().Create(t, source.Get(t), c.CRUD.MakeContext(t), &v)
				return &v
			})
			ent2 = testcase.Let(s, func(t *testcase.T) *ENT {
				v := c.CRUD.MakeEntity(t)
				c.CRUD.Helper().Create(t, source.Get(t), c.CRUD.MakeContext(t), &v)
				return &v
			})
		)

		query.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[ENT] {
			return func(ctx context.Context) iter.Seq2[ENT, error] {
				return iterkit.ToErrSeq(iterkit.Slice[ENT]([]ENT{*ent1.Get(t), *ent2.Get(t)}))
			}
		})

		s.Then("it will return all the entities", func(t *testcase.T) {
			vs, err := iterkit.CollectErr(act(t))
			t.Must.NoError(err)
			t.Must.ContainExactly([]ENT{*ent1.Get(t), *ent2.Get(t)}, vs)
		})

		s.Then("it will cache all returned entities", func(t *testcase.T) {
			vs, err := iterkit.CollectErr(act(t))
			t.Must.NoError(err)

			cached, err := iterkit.CollectErr(repository.Entities().FindAll(c.CRUD.MakeContext(t)))
			t.Must.NoError(err)
			t.Must.Contain(cached, vs)
		})

		s.Then("it will create a hit record", func(t *testcase.T) {
			_, err := iterkit.CollectErr(act(t))
			t.Must.NoError(err)

			hits, err := iterkit.CollectErr(repository.Hits().FindAll(c.CRUD.MakeContext(t)))
			t.Must.NoError(err)

			assert.OneOf(t, hits, func(it assert.It, got cachepkg.Hit[ID]) {
				it.Must.Equal(got.ID, hitID.Get(t))
				it.Must.ContainExactly(got.EntityIDs, []ID{
					c.CRUD.Helper().HasID(t, ent1.Get(t)),
					c.CRUD.Helper().HasID(t, ent2.Get(t)),
				})
			})
		})
	})
}

func specInvalidateCachedQuery[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)

	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext(t)
		})
		hitID = testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
			return cachepkg.Query{Name: constant.String(t.Random.UUID())}.HitID()
		})
	)
	act := func(t *testcase.T) error {
		return cache.Get(t).
			InvalidateCachedQuery(Context.Get(t), hitID.Get(t))
	}

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[ENT]](s, nil)
	queryOne := func(t *testcase.T) (ENT, bool, error) {
		return cache.Get(t).
			CachedQueryOne(c.CRUD.MakeContext(t), hitID.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[ENT]](s, nil)
	queryMany := func(t *testcase.T) iter.Seq2[ENT, error] {
		return cache.Get(t).CachedQueryMany(c.CRUD.MakeContext(t), hitID.Get(t), queryManyFunc.Get(t))
	}

	s.When("queryKey has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *ENT {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[ENT] {
			return func(ctx context.Context) (ENT, bool, error) {
				id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
				return source.Get(t).FindByID(ctx, id)
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))
			// we have hits
			hvs, err := iterkit.CollectErr(repository.Hits().FindAll(c.CRUD.MakeContext(t)))
			assert.NoError(t, err)

			assert.OneOf(t, hvs, func(t assert.It, got cachepkg.Hit[ID]) {
				assert.Contain(t, got.EntityIDs, id)
			}, "expected that there is at least one hit that points to our ID")

			// we have cached entities
			evs, err := iterkit.CollectErr(repository.Entities().FindAll(c.CRUD.MakeContext(t)))
			assert.NoError(t, err)
			assert.NotEmpty(t, evs, "expected that we have cached entities")

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
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))

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
		entPtr := testcase.Let(s, func(t *testcase.T) *ENT {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		queryManyFunc.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[ENT] {
			return func(ctx context.Context) iter.Seq2[ENT, error] {
				return iterkit.From(func(yield func(ENT) bool) error {
					id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
					ent, found, err := source.Get(t).FindByID(ctx, id)
					if err != nil {
						return err
					}
					if !found {
						return nil
					}
					yield(ent)
					return nil
				})
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			vs, err = iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))

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

func specInvalidateByID[ENT any, ID comparable](s *testcase.Spec,
	cache testcase.Var[*cachepkg.Cache[ENT, ID]],
	source testcase.Var[cacheSource[ENT, ID]],
	repository cachepkg.Repository[ENT, ID],
	opts ...Option[ENT, ID],
) {
	c := option.Use(opts)
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.CRUD.MakeContext(t)
		})
		id = testcase.Let[ID](s, nil)
	)
	act := func(t *testcase.T) error {
		return cache.Get(t).InvalidateByID(Context.Get(t), id.Get(t))
	}

	hitID := testcase.Let(s, func(t *testcase.T) cachepkg.HitID {
		return cachepkg.Query{Name: "operation-name"}.HitID()
	})

	var queryOneFunc = testcase.Let[cachepkg.QueryOneFunc[ENT]](s, nil)
	queryOne := func(t *testcase.T) (ENT, bool, error) {
		return cache.Get(t).
			CachedQueryOne(c.CRUD.MakeContext(t), hitID.Get(t), queryOneFunc.Get(t))
	}

	var queryManyFunc = testcase.Let[cachepkg.QueryManyFunc[ENT]](s, nil)
	queryMany := func(t *testcase.T) iter.Seq2[ENT, error] {
		return cache.Get(t).
			CachedQueryMany(c.CRUD.MakeContext(t), hitID.Get(t), queryManyFunc.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.Cleanup(func() {
			t.Must.NoError(cache.Get(t).
				InvalidateCachedQuery(c.CRUD.MakeContext(t), hitID.Get(t)))
		})
	})

	s.When("entity id has a cached data with CachedQueryOne", func(s *testcase.Spec) {
		entPtr := testcase.Let(s, func(t *testcase.T) *ENT {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		id.Let(s, func(t *testcase.T) ID {
			return c.CRUD.Helper().HasID(t, entPtr.Get(t))
		})

		queryOneFunc.Let(s, func(t *testcase.T) cachepkg.QueryOneFunc[ENT] {
			return func(ctx context.Context) (ENT, bool, error) {
				return source.Get(t).FindByID(ctx, id.Get(t))
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := queryOne(t)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))
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
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))

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
		entPtr := testcase.Let(s, func(t *testcase.T) *ENT {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})
		id.Let(s, func(t *testcase.T) ID {
			return c.CRUD.Helper().HasID(t, entPtr.Get(t))
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			ent, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			ent, found, err = cache.Get(t).FindByID(c.CRUD.MakeContext(t), id)
			t.Must.NoError(err)
			t.Must.True(found)
			t.Must.Equal(ent, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			ent, found, err := cache.Get(t).FindByID(c.CRUD.MakeContext(t), id.Get(t))
			t.Must.NoError(err)
			t.Must.False(found)
			t.Must.Empty(ent)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))

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
		entPtr := testcase.Let(s, func(t *testcase.T) *ENT {
			return pointer.Of(c.CRUD.MakeEntity(t))
		})

		id.Let(s, func(t *testcase.T) ID {
			return c.CRUD.Helper().HasID(t, entPtr.Get(t))
		})

		queryManyFunc.Let(s, func(t *testcase.T) cachepkg.QueryManyFunc[ENT] {
			return func(ctx context.Context) iter.Seq2[ENT, error] {
				return iterkit.From(func(yield func(ENT) bool) error {
					id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
					ent, found, err := source.Get(t).FindByID(ctx, id)
					if err != nil {
						return err
					}
					if !found {
						return nil
					}
					yield(ent)
					return nil
				})
			}
		})

		s.Before(func(t *testcase.T) {
			// create ent in source
			t.Must.NoError(source.Get(t).Create(c.CRUD.MakeContext(t), entPtr.Get(t)))
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))
			// warm up the cache before making the data invalidated
			vs, err := iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
			// make ent state differ in source from the cached one
			t.Must.NoError(source.Get(t).DeleteByID(c.CRUD.MakeContext(t), id))
			// cache has still the invalid state
			vs, err = iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Contain(vs, *entPtr.Get(t))
		})

		s.Then("cached data is invalidated", func(t *testcase.T) {
			t.Must.NoError(act(t))

			vs, err := iterkit.CollectErr(queryMany(t))
			t.Must.NoError(err)
			t.Must.Empty(vs)
		})

		s.Then("related data in entity repository is gone", func(t *testcase.T) {
			id := c.CRUD.Helper().HasID(t, entPtr.Get(t))

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
			c.CRUD.Helper().Create(t, source.Get(t), c.CRUD.MakeContext(t), &ent)
			v := c.CRUD.Helper().HasID(t, &ent)
			c.CRUD.Helper().Delete(t, source.Get(t), c.CRUD.MakeContext(t), &ent)
			return v
		})

		s.Before(func(t *testcase.T) {
			_, found, err := source.Get(t).FindByID(c.CRUD.MakeContext(t), id.Get(t))
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
