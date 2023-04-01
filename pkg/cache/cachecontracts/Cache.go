package cachecontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/pointer"
	sh "github.com/adamluzsi/frameless/spechelper"
	"reflect"
	"testing"
	"time"

	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

var (
	waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      time.Second,
	}
	eventually = assert.Eventually{RetryStrategy: &waiter}
)

type Cache[Entity, ID any] func(testing.TB) CacheSubject[Entity, ID]

type CacheSubject[Entity, ID any] struct {
	Cache      cacheCache[Entity, ID]
	Source     cacheSource[Entity, ID]
	Repository cache.Repository[Entity, ID]

	MakeContext func() context.Context
	MakeEntity  func() Entity

	// ChangeEntity is an optional configuration field
	// to express what Entity fields are allowed to be changed by the user of the Updater.
	// For example, if the changed  Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(*Entity)
}

type cacheCache[Entity, ID any] interface {
	cache.Interface[Entity, ID]
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.AllFinder[Entity]
	crud.Updater[Entity]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
}

type cacheSource[Entity, ID any] interface {
	sh.CRUD[Entity, ID]
	cache.Source[Entity, ID]
}

func (c Cache[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Cache[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Cache[Entity, ID]) subject() testcase.Var[CacheSubject[Entity, ID]] {
	return testcase.Var[CacheSubject[Entity, ID]]{
		ID:   "ManagerSubject[Entity, ID]",
		Init: func(t *testcase.T) CacheSubject[Entity, ID] { return c(t) },
	}
}

func (c Cache[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID](func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
			ch := c(tb)
			if _, ok := ch.Source.(crud.Creator[Entity]); !ok {
				tb.Skip(cache.ErrNotImplementedBySource.Error())
			}
			return crudcontracts.CreatorSubject[Entity, ID]{
				Resource:       ch.Cache,
				MakeContext:    ch.MakeContext,
				MakeEntity:     ch.MakeEntity,
				SupportIDReuse: true,
			}
		}),
		crudcontracts.ByIDFinder[Entity, ID](func(tb testing.TB) crudcontracts.ByIDFinderSubject[Entity, ID] {
			ch := c.subject().Get(tb.(*testcase.T))
			var _ crud.ByIDFinder[Entity, ID] = ch.Source
			return crudcontracts.ByIDFinderSubject[Entity, ID]{
				Resource:    ch.Cache,
				MakeContext: ch.MakeContext,
				MakeEntity:  ch.MakeEntity,
			}
		}),
		crudcontracts.AllFinder[Entity, ID](func(tb testing.TB) crudcontracts.AllFinderSubject[Entity, ID] {
			ch := c.subject().Get(tb.(*testcase.T))
			if _, ok := ch.Source.(crud.Creator[Entity]); !ok {
				tb.Skip(cache.ErrNotImplementedBySource.Error())
			}
			return crudcontracts.AllFinderSubject[Entity, ID]{
				Resource:    ch.Cache,
				MakeContext: ch.MakeContext,
				MakeEntity:  ch.MakeEntity,
			}
		}),
		crudcontracts.ByIDDeleter[Entity, ID](func(tb testing.TB) crudcontracts.ByIDDeleterSubject[Entity, ID] {
			ch := c.subject().Get(tb.(*testcase.T))
			if _, ok := ch.Source.(crud.ByIDDeleter[Entity]); !ok {
				tb.Skip(cache.ErrNotImplementedBySource.Error())
			}
			return crudcontracts.ByIDDeleterSubject[Entity, ID]{
				Resource:    ch.Cache,
				MakeContext: ch.MakeContext,
				MakeEntity:  ch.MakeEntity,
			}
		}),
		crudcontracts.AllDeleter[Entity, ID](func(tb testing.TB) crudcontracts.AllDeleterSubject[Entity, ID] {
			ch := c.subject().Get(tb.(*testcase.T))
			if _, ok := ch.Source.(crud.AllDeleter); !ok {
				tb.Skip(cache.ErrNotImplementedBySource.Error())
			}
			return crudcontracts.AllDeleterSubject[Entity, ID]{
				Resource:    ch.Cache,
				MakeContext: ch.MakeContext,
				MakeEntity:  ch.MakeEntity,
			}
		}),
		crudcontracts.Updater[Entity, ID](func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
			ch := c.subject().Get(tb.(*testcase.T))
			if _, ok := ch.Source.(crud.Updater[Entity]); !ok {
				tb.Skip(cache.ErrNotImplementedBySource.Error())
			}
			return crudcontracts.UpdaterSubject[Entity, ID]{
				Resource:     ch.Cache,
				MakeContext:  ch.MakeContext,
				MakeEntity:   ch.MakeEntity,
				ChangeEntity: ch.ChangeEntity,
			}
		}),
		Repository[Entity, ID](func(tb testing.TB) RepositorySubject[Entity, ID] {
			sub := c(tb)
			return RepositorySubject[Entity, ID]{
				Repository:   sub.Repository,
				MakeContext:  sub.MakeContext,
				MakeEntity:   sub.MakeEntity,
				ChangeEntity: sub.ChangeEntity,
			}
		}),
		// TODO: support OnePhaseCommitProtocol with cache.Cache
	)

	s.Context(``, func(s *testcase.Spec) {
		c.describeResultCaching(s)
		c.describeCacheInvalidationByEventsThatMutatesAnEntity(s)
	})
}

func (c Cache[Entity, ID]) describeCacheInvalidationByEventsThatMutatesAnEntity(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) interface{} {
			ptr := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.NoError(c.source().Get(t).Create(c.subject().Get(t).MakeContext(), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(c.source().Get(t).DeleteByID, c.subject().Get(t).MakeContext(), id)
			return ptr
		})

		s.Test(`an update to the repository should refresh the by id look`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.cache().Get(t).FindByID(ctx, entID)       // should trigger caching
			_, _ = iterators.Count(c.cache().Get(t).FindAll(ctx)) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.NoError(extid.Set(vUpdated, entID))
			Update[Entity, ID](t, c.cache().Get(t), ctx, vUpdated)
			waiter.Wait()

			ptr := IsFindable[Entity, ID](t, c.cache().Get(t), ctx, entID) // should trigger caching
			t.Must.Equal(vUpdated, ptr)
		})

		s.Test(`an update to the repository should refresh the QueryMany cache hits`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			v := value.Get(t)
			entID, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.cache().Get(t).FindByID(ctx, entID)       // should trigger caching
			_, _ = iterators.Count(c.cache().Get(t).FindAll(ctx)) // should trigger caching

			// mutate
			vUpdated := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.NoError(extid.Set(vUpdated, entID))
			Update[Entity, ID](t, c.cache().Get(t), ctx, vUpdated)
			waiter.Wait()

			var (
				gotEnt Entity
				found  bool
			)
			t.Must.NoError(iterators.ForEach(c.cache().Get(t).FindAll(ctx), func(ent Entity) error {
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
			_, _, _ = c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id)          // should trigger caching
			_, _ = iterators.Count(c.cache().Get(t).FindAll(c.subject().Get(t).MakeContext())) // should trigger caching

			// delete
			t.Must.NoError(c.cache().Get(t).DeleteByID(c.subject().Get(t).MakeContext(), id))

			// assert
			IsAbsent[Entity, ID](t, c.cache().Get(t), c.subject().Get(t).MakeContext(), id)
		})

		s.Test(`a delete all entity in the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id)          // should trigger caching
			_, _ = iterators.Count(c.cache().Get(t).FindAll(c.subject().Get(t).MakeContext())) // should trigger caching

			// delete
			t.Must.NoError(c.cache().Get(t).DeleteAll(c.subject().Get(t).MakeContext()))
			waiter.Wait()

			IsAbsent[Entity, ID](t, c.cache().Get(t), c.subject().Get(t).MakeContext(), id) // should trigger caching for not found
		})
	})
}

func (c Cache[Entity, ID]) cache() testcase.Var[*cache.Cache[Entity, ID]] {
	return testcase.Var[*cache.Cache[Entity, ID]]{
		ID: `*cache.Cache`,
		Init: func(t *testcase.T) *cache.Cache[Entity, ID] {
			subject := c.subject().Get(t)
			return cache.New[Entity, ID](subject.Source, subject.Repository)
		},
	}
}

func (c Cache[Entity, ID]) source() testcase.Var[cacheSource[Entity, ID]] {
	// source resource where the cache manager retrieve the data in case cache hit is missing
	return testcase.Var[cacheSource[Entity, ID]]{
		ID: `cache manager's source of truth`,
		Init: func(t *testcase.T) cacheSource[Entity, ID] {
			return c.subject().Get(t).Source
		},
	}
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

func (c Cache[Entity, ID]) describeResultCaching(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) *Entity {
			ctx := c.subject().Get(t).MakeContext()
			ptr := pointer.Of(c.subject().Get(t).MakeEntity())
			repository := c.source().Get(t)
			t.Must.NoError(repository.Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(repository.DeleteByID, ctx, id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			id, found := extid.Lookup[ID](value.Get(t))
			assert.Must(t).True(found)
			v, found, err := c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id)
			t.Must.NoError(err)
			assert.Must(t).True(found)
			assert.Must(t).Equal(*value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup[ID](value.Get(t))
				assert.Must(t).True(found)
				v := IsFindable[Entity, ID](t, c.source().Get(t), c.subject().Get(t).MakeContext(), id)
				assert.Must(t).Equal(value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := testcase.Let(s, func(t *testcase.T) *Entity {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					nv := pointer.Of(c.subject().Get(t).MakeEntity())
					t.Must.NoError(extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					Update[Entity, ID](t, c.cache().Get(t), c.subject().Get(t).MakeContext(), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					t.Must.NotEmpty(id)
					HasEntity[Entity, ID](t, c.cache().Get(t), c.subject().Get(t).MakeContext(), valueWithNewContent.Get(t))

					eventually.Assert(t, func(it assert.It) {
						v, found, err := c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id)
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
					v, found, err := c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id)
					t.Must.NoError(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*value, v)
				}
			})

			s.When(`the repository is sensitive to continuous requests`, func(s *testcase.Spec) {
				spy := testcase.Let(s, func(t *testcase.T) *SpySource[Entity, ID] {
					return &SpySource[Entity, ID]{cacheSource: c.source().Get(t)}
				})
				s.Before(func(t *testcase.T) {
					c.source().Set(t, spy.Get(t))
				})

				s.Then(`it will only bother the repository for the value once`, func(t *testcase.T) {
					var err error
					val := value.Get(t)
					id, found := extid.Lookup[ID](val)
					assert.Must(t).True(found)

					// trigger caching
					assert.Must(t).Equal(val, IsFindable[Entity, ID](t, c.cache().Get(t), c.subject().Get(t).MakeContext(), id))
					numberOfFindByIDCallAfterEntityIsFound := spy.Get(t).count.FindByID
					waiter.Wait()

					nv, found, err := c.cache().Get(t).FindByID(c.subject().Get(t).MakeContext(), id) // should use cached val
					t.Must.NoError(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*val, nv)
					assert.Must(t).Equal(numberOfFindByIDCallAfterEntityIsFound, spy.Get(t).count.FindByID)
				})
			})
		})
	})
}