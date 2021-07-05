package contracts

import (
	"context"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/extid"
	"reflect"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var (
	waiter = testcase.Waiter{
		WaitDuration: time.Millisecond,
		WaitTimeout:  time.Second,
	}
	async = testcase.Retry{Strategy: &waiter}
)

type Manager struct {
	T              frameless.T
	Subject        func(testing.TB) (Cache, cache.Source, frameless.OnePhaseCommitProtocol)
	FixtureFactory func(testing.TB) contracts.FixtureFactory
}

type Cache interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	frameless.Publisher
}

func (c Manager) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Manager) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Manager) Spec(s *testcase.Spec) {
	newManager := func(tb testing.TB) Cache {
		m, _, _ := c.Subject(tb)
		return m
	}
	factoryLet(s, c.FixtureFactory)

	testcase.RunContract(s,
		contracts.Creator{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return newManager(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Finder{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return newManager(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Deleter{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return newManager(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Publisher{T: c.T,
			Subject: func(tb testing.TB) contracts.PublisherSubject {
				manager, source, _ := c.Subject(tb)
				if _, ok := source.(frameless.Updater); !ok {
					tb.Skip()
				}
				return manager
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Updater{T: c.T,
			Subject: func(tb testing.TB) contracts.UpdaterSubject {
				m, r, _ := c.Subject(tb)
				if _, ok := r.(frameless.Updater); !ok {
					tb.Skip()
				}
				return m
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.OnePhaseCommitProtocol{
			T: c.T,
			Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
				m, _, cpm := c.Subject(tb)
				return cpm, m
			},
			FixtureFactory: c.FixtureFactory,
		},
	)

	s.Context(``, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			manager, resource, cpm := c.Subject(t)
			c.manager().Set(t, manager)
			c.source().Set(t, resource)
			c.onePhaseCommitProtocolManager().Set(t, cpm)
		})

		c.describeResultCaching(s)
		c.describeCacheInvalidationByEventsThatMutatesAnEntity(s)
	})
}

func (c Manager) describeCacheInvalidationByEventsThatMutatesAnEntity(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(c.T), func(s *testcase.Spec) {
		s.Let(`value`, func(t *testcase.T) interface{} {
			ptr := c.createT(t)
			require.Nil(t, c.sourceGet(t).Create(c.context(t), ptr))
			id, _ := extid.Lookup(ptr)
			t.Defer(c.sourceGet(t).DeleteByID, c.context(t), id)
			return ptr
		})

		s.Test(`an update to the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = c.managerGet(t).FindByID(c.context(t), c.newT(), id)   // should trigger caching
			_, _ = iterators.Count(c.managerGet(t).FindAll(c.context(t))) // should trigger caching

			// mutate
			vUpdated := c.createT(t)
			require.Nil(t, extid.Set(vUpdated, id))
			contracts.UpdateEntity(t, c.managerGet(t), c.context(t), vUpdated)
			waiter.Wait()

			ptr := contracts.IsFindable(t, c.T, c.managerGet(t), c.context(t), id) // should trigger caching
			require.Equal(t, vUpdated, ptr)
		})

		s.Test(`a delete by id to the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = c.managerGet(t).FindByID(c.context(t), c.newT(), id)   // should trigger caching
			_, _ = iterators.Count(c.managerGet(t).FindAll(c.context(t))) // should trigger caching

			// delete
			require.Nil(t, c.managerGet(t).DeleteByID(c.context(t), id))

			// assert
			contracts.IsAbsent(t, c.T, c.managerGet(t), c.context(t), id)
		})

		s.Test(`a delete all entity in the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = c.managerGet(t).FindByID(c.context(t), c.newT(), id)   // should trigger caching
			_, _ = iterators.Count(c.managerGet(t).FindAll(c.context(t))) // should trigger caching

			// delete
			require.Nil(t, c.managerGet(t).DeleteAll(c.context(t)))
			waiter.Wait()

			contracts.IsAbsent(t, c.T, c.managerGet(t), c.context(t), id) // should trigger caching for not found
		})
	})
}

func (c Manager) manager() testcase.Var {
	return testcase.Var{Name: `cache`}
}

func (c Manager) managerGet(t *testcase.T) Cache {
	return c.manager().Get(t).(Cache)
}

func (c Manager) onePhaseCommitProtocolManager() testcase.Var {
	return testcase.Var{Name: `one phase commit protocol manager`}
}

func (c Manager) onePhaseCommitProtocolManagerGet(t *testcase.T) frameless.OnePhaseCommitProtocol {
	return c.onePhaseCommitProtocolManager().Get(t).(frameless.OnePhaseCommitProtocol)
}

func (c Manager) source() testcase.Var {
	// source resource where the cache manager retrieve the data in case cache hit is missing
	return testcase.Var{Name: `cache manager's source of truth`}
}

func (c Manager) sourceGet(t *testcase.T) cache.Source {
	return c.source().Get(t).(cache.Source)
}

type StubSource struct {
	cache.Source
	count struct {
		FindByID int
	}
}

func (stub *StubSource) FindByID(ctx context.Context, ptr, id interface{}) (_found bool, _err error) {
	stub.count.FindByID++
	return stub.Source.FindByID(ctx, ptr, id)
}

func (c Manager) describeResultCaching(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(c.T), func(s *testcase.Spec) {
		value := s.Let(`stored value`, func(t *testcase.T) interface{} {
			ptr := c.createT(t)
			storage := c.sourceGet(t)
			require.Nil(t, storage.Create(c.context(t), ptr))
			id, _ := extid.Lookup(ptr)
			t.Defer(storage.DeleteByID, c.context(t), id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			v := c.newT()
			id, found := extid.Lookup(value.Get(t))
			require.True(t, found)
			found, err := c.managerGet(t).FindByID(c.context(t), v, id)
			require.Nil(t, err)
			require.True(t, found)
			require.Equal(t, value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup(value.Get(t))
				require.True(t, found)
				v := contracts.IsFindable(t, c.T, c.sourceGet(t), c.context(t), id)
				require.Equal(t, value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := s.Let(`value-with-new-content`, func(t *testcase.T) interface{} {
					id, found := extid.Lookup(value.Get(t))
					require.True(t, found)
					nv := c.createT(t)
					require.Nil(t, extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					contracts.UpdateEntity(t, c.managerGet(t), c.context(t), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup(value.Get(t))
					require.True(t, found)
					require.NotEmpty(t, id)
					contracts.HasEntity(t, c.managerGet(t), c.context(t), valueWithNewContent.Get(t))

					async.Assert(t, func(tb testing.TB) {
						v := c.newT()
						found, err := c.managerGet(t).FindByID(c.context(t), v, id)
						require.Nil(tb, err)
						require.True(tb, found)
						tb.Log(`actually`, v)
						require.Equal(tb, valueWithNewContent.Get(t), v)
					})
				})
			})
		})

		s.And(`on multiple request`, func(s *testcase.Spec) {
			s.Then(`it will return it consistently`, func(t *testcase.T) {
				value := value.Get(t)
				id, found := extid.Lookup(value)
				require.True(t, found)

				for i := 0; i < 42; i++ {
					v := c.newT()
					found, err := c.managerGet(t).FindByID(c.context(t), v, id)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, value, v)
				}
			})

			s.When(`the storage is sensitive to continuous requests`, func(s *testcase.Spec) {
				stub := s.Let(`stub`, func(t *testcase.T) interface{} {
					return &StubSource{Source: c.sourceGet(t)}
				})
				stubGet := func(t *testcase.T) *StubSource {
					return stub.Get(t).(*StubSource)
				}
				s.Before(func(t *testcase.T) {
					c.source().Set(t, stubGet(t))
				})

				s.Then(`it will only bother the storage for the value once`, func(t *testcase.T) {
					var (
						nv  interface{}
						err error
					)
					value := value.Get(t)
					id, found := extid.Lookup(value)
					require.True(t, found)

					// trigger caching
					nv = contracts.IsFindable(t, c.T, c.managerGet(t), c.context(t), id)
					require.Equal(t, value, nv)
					numberOfFindByIDCallAfterEntityIsFound := stubGet(t).count.FindByID
					waiter.Wait()

					nv = c.newT()
					found, err = c.managerGet(t).FindByID(c.context(t), nv, id) // should use cached value
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, value, nv)
					require.Equal(t, numberOfFindByIDCallAfterEntityIsFound, stubGet(t).count.FindByID)
				})
			})
		})
	}, testcase.Flaky(time.Minute))
}

func (c Manager) context(t *testcase.T) context.Context {
	return factoryGet(t).Context()
}

func (c Manager) newT() interface{} {
	return reflect.New(reflect.TypeOf(c.T)).Interface()
}

func (c Manager) createT(t *testcase.T) interface{} {
	return contracts.CreatePTR(factoryGet(t), c.T)
}
