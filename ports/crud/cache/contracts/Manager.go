package cachecontracts

import (
	"context"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/cache"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

var (
	waiter = testcase.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      time.Second,
	}
	async = testcase.Eventually{RetryStrategy: &waiter}
)

type Manager[Entity, ID any] struct {
	MakeSubject func(testing.TB) ManagerSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type ManagerSubject[Entity, ID any] struct {
	Cache         Cache[Entity, ID]
	Source        cache.Source[Entity, ID]
	CommitManager comproto.OnePhaseCommitProtocol
}

type Cache[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.Finder[Entity, ID]
	crud.Updater[Entity]
	crud.Deleter[ID]
	pubsub.CreatorPublisher[Entity]
	pubsub.UpdaterPublisher[Entity]
	pubsub.DeleterPublisher[ID]
}

func (c Manager[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Manager[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Manager[Entity, ID]) ManagerSubject() testcase.Var[ManagerSubject[Entity, ID]] {
	return testcase.Var[ManagerSubject[Entity, ID]]{
		ID:   "ManagerSubject[Entity, ID]",
		Init: func(t *testcase.T) ManagerSubject[Entity, ID] { return c.MakeSubject(t) },
	}
}

func (c Manager[Entity, ID]) Spec(s *testcase.Spec) {
	newManager := func(tb testing.TB) Cache[Entity, ID] {
		return c.ManagerSubject().Get(tb.(*testcase.T)).Cache
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return newManager(tb)
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,

			SupportIDReuse: true,
		},
		crudcontracts.Finder[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
				return newManager(tb).(crudcontracts.FinderSubject[Entity, ID])
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,
		},
		crudcontracts.Deleter[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
				return newManager(tb)
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,
		},
		pubsubcontracts.Publisher[Entity, ID]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PublisherSubject[Entity, ID] {
				ms := c.MakeSubject(tb)
				if _, ok := ms.Source.(crud.Updater[Entity]); !ok {
					tb.Skip()
				}
				return ms.Cache
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,
		},
		crudcontracts.Updater[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
				ms := c.MakeSubject(tb)
				if _, ok := ms.Source.(crud.Updater[Entity]); !ok {
					tb.Skip()
				}
				return ms.Cache
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,
		},
		crudcontracts.OnePhaseCommitProtocol[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
				subject := c.MakeSubject(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
					Resource:      subject.Cache,
					CommitManager: subject.CommitManager,
				}
			},
			MakeEntity:  c.MakeEntity,
			MakeContext: c.MakeContext,
		},
	)

	s.Context(``, func(s *testcase.Spec) {
		c.describeResultCaching(s)
		c.describeCacheInvalidationByEventsThatMutatesAnEntity(s)
	})
}

func (c Manager[Entity, ID]) describeCacheInvalidationByEventsThatMutatesAnEntity(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) interface{} {
			ptr := c.createT(t)
			assert.Must(t).Nil(c.source().Get(t).Create(c.MakeContext(t), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(c.source().Get(t).DeleteByID, c.MakeContext(t), id)
			return ptr
		})

		s.Test(`an update to the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.manager().Get(t).FindByID(c.MakeContext(t), id)          // should trigger caching
			_, _ = iterators.Count(c.manager().Get(t).FindAll(c.MakeContext(t))) // should trigger caching

			// mutate
			vUpdated := c.createT(t)
			assert.Must(t).Nil(extid.Set(vUpdated, id))
			Update[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), vUpdated)
			waiter.Wait()

			ptr := IsFindable[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), id) // should trigger caching
			assert.Must(t).Equal(vUpdated, ptr)
		})

		s.Test(`a delete by id to the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.manager().Get(t).FindByID(c.MakeContext(t), id)          // should trigger caching
			_, _ = iterators.Count(c.manager().Get(t).FindAll(c.MakeContext(t))) // should trigger caching

			// delete
			assert.Must(t).Nil(c.manager().Get(t).DeleteByID(c.MakeContext(t), id))

			// assert
			IsAbsent[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), id)
		})

		s.Test(`a delete all entity in the repository should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := value.Get(t)
			id, _ := extid.Lookup[ID](v)

			// cache
			_, _, _ = c.manager().Get(t).FindByID(c.MakeContext(t), id)          // should trigger caching
			_, _ = iterators.Count(c.manager().Get(t).FindAll(c.MakeContext(t))) // should trigger caching

			// delete
			assert.Must(t).Nil(c.manager().Get(t).DeleteAll(c.MakeContext(t)))
			waiter.Wait()

			IsAbsent[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), id) // should trigger caching for not found
		})
	})
}

func (c Manager[Entity, ID]) manager() testcase.Var[Cache[Entity, ID]] {
	return testcase.Var[Cache[Entity, ID]]{
		ID: `cache`,
		Init: func(t *testcase.T) Cache[Entity, ID] {
			return c.ManagerSubject().Get(t).Cache
		},
	}
}

func (c Manager[Entity, ID]) source() testcase.Var[cache.Source[Entity, ID]] {
	// source resource where the cache manager retrieve the data in case cache hit is missing
	return testcase.Var[cache.Source[Entity, ID]]{
		ID: `cache manager's source of truth`,
		Init: func(t *testcase.T) cache.Source[Entity, ID] {
			return c.ManagerSubject().Get(t).Source
		},
	}
}

type SpySource[Entity, ID any] struct {
	cache.Source[Entity, ID]
	count struct {
		FindByID int
	}
}

func (stub *SpySource[Entity, ID]) FindByID(ctx context.Context, id ID) (_ent Entity, _found bool, _err error) {
	stub.count.FindByID++
	return stub.Source.FindByID(ctx, id)
}

func (c Manager[Entity, ID]) describeResultCaching(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(*new(Entity)), func(s *testcase.Spec) {
		value := testcase.Let(s, func(t *testcase.T) *Entity {
			ptr := c.createT(t)
			repository := c.source().Get(t)
			assert.Must(t).Nil(repository.Create(c.MakeContext(t), ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Defer(repository.DeleteByID, c.MakeContext(t), id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			id, found := extid.Lookup[ID](value.Get(t))
			assert.Must(t).True(found)
			v, found, err := c.manager().Get(t).FindByID(c.MakeContext(t), id)
			assert.Must(t).Nil(err)
			assert.Must(t).True(found)
			assert.Must(t).Equal(*value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup[ID](value.Get(t))
				assert.Must(t).True(found)
				v := IsFindable[Entity, ID](t, c.source().Get(t), c.MakeContext(t), id)
				assert.Must(t).Equal(value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := testcase.Let(s, func(t *testcase.T) *Entity {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					nv := c.createT(t)
					assert.Must(t).Nil(extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					Update[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup[ID](value.Get(t))
					assert.Must(t).True(found)
					t.Must.NotEmpty(id)
					HasEntity[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), valueWithNewContent.Get(t))

					async.Assert(t, func(it assert.It) {
						v, found, err := c.manager().Get(t).FindByID(c.MakeContext(t), id)
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
					v, found, err := c.manager().Get(t).FindByID(c.MakeContext(t), id)
					assert.Must(t).Nil(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*value, v)
				}
			})

			s.When(`the repository is sensitive to continuous requests`, func(s *testcase.Spec) {
				spy := testcase.Let(s, func(t *testcase.T) *SpySource[Entity, ID] {
					return &SpySource[Entity, ID]{Source: c.source().Get(t)}
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
					assert.Must(t).Equal(val, IsFindable[Entity, ID](t, c.manager().Get(t), c.MakeContext(t), id))
					numberOfFindByIDCallAfterEntityIsFound := spy.Get(t).count.FindByID
					waiter.Wait()

					nv, found, err := c.manager().Get(t).FindByID(c.MakeContext(t), id) // should use cached val
					assert.Must(t).Nil(err)
					assert.Must(t).True(found)
					assert.Must(t).Equal(*val, nv)
					assert.Must(t).Equal(numberOfFindByIDCallAfterEntityIsFound, spy.Get(t).count.FindByID)
				})
			})
		})
	}, testcase.Flaky(time.Minute))
}

func (c Manager[Entity, ID]) createT(t *testcase.T) *Entity {
	ent := c.MakeEntity(t)
	return &ent
}
