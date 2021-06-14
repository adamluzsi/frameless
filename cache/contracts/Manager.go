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
	FixtureFactory contracts.FixtureFactory
}

type Cache interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	frameless.CreatorPublisher
	frameless.UpdaterPublisher
	frameless.DeleterPublisher
}

func (spec Manager) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec Manager) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec Manager) Spec(tb testing.TB) {
	testcase.NewSpec(tb).Describe(`Manager`, func(s *testcase.Spec) {
		newManager := func(tb testing.TB) Cache {
			m, _, _ := spec.Subject(tb)
			return m
		}

		testcase.RunContract(s,
			contracts.Creator{T: spec.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newManager(tb)
				},
				FixtureFactory: spec.FixtureFactory,
			},
			contracts.Finder{T: spec.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newManager(tb)
				},
				FixtureFactory: spec.FixtureFactory,
			},
			contracts.Deleter{T: spec.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newManager(tb)
				},
				FixtureFactory: spec.FixtureFactory,
			},
			contracts.CreatorPublisher{T: spec.T,
				Subject: func(tb testing.TB) contracts.CreatorPublisherSubject {
					return newManager(tb)
				},
				FixtureFactory: spec.FixtureFactory,
			},
			contracts.DeleterPublisher{T: spec.T,
				Subject: func(tb testing.TB) contracts.DeleterPublisherSubject {
					return newManager(tb)
				},
				FixtureFactory: spec.FixtureFactory,
			},

			contracts.Updater{T: spec.T,
				Subject: func(tb testing.TB) contracts.UpdaterSubject {
					m, r, _ := spec.Subject(tb)
					if _, ok := r.(cache.ExtendedSource); !ok {
						tb.Skipf(`%T doesn't implement cache.UpdaterSource`, r)
					}
					return m
				},
				FixtureFactory: spec.FixtureFactory,
			},
			contracts.UpdaterPublisher{T: spec.T,
				Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject {
					m, r, _ := spec.Subject(tb)
					if _, ok := r.(cache.ExtendedSource); !ok {
						tb.Skipf(`%T doesn't implement cache.UpdaterSource`, r)
					}
					return m
				},
				FixtureFactory: spec.FixtureFactory,
			},

			contracts.OnePhaseCommitProtocol{
				T: spec.T,
				Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
					m, _, cpm := spec.Subject(tb)
					return cpm, m
				},
				FixtureFactory: spec.FixtureFactory,
			},
		)

		s.Context(``, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				manager, resource, cpm := spec.Subject(t)
				spec.manager().Set(t, manager)
				spec.source().Set(t, resource)
				spec.onePhaseCommitProtocolManager().Set(t, cpm)
			})

			spec.describeResultCaching(s)
			spec.describeCacheInvalidationByEventsThatMutatesAnEntity(s)
		})
	})
}

func (spec Manager) describeCacheInvalidationByEventsThatMutatesAnEntity(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(spec.T), func(s *testcase.Spec) {
		s.Let(`value`, func(t *testcase.T) interface{} {
			ptr := spec.createT()
			require.Nil(t, spec.sourceGet(t).Create(spec.context(), ptr))
			id, _ := extid.Lookup(ptr)
			t.Defer(spec.sourceGet(t).DeleteByID, spec.context(), id)
			return ptr
		})

		s.Test(`an update to the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = spec.managerGet(t).FindByID(spec.context(), spec.newT(), id) // should trigger caching
			_, _ = iterators.Count(spec.managerGet(t).FindAll(spec.context()))  // should trigger caching

			// mutate
			vUpdated := spec.createT()
			require.Nil(t, extid.Set(vUpdated, id))
			contracts.UpdateEntity(t, spec.managerGet(t), spec.context(), vUpdated)
			waiter.Wait()

			ptr := contracts.IsFindable(t, spec.T, spec.managerGet(t), spec.context(), id) // should trigger caching
			require.Equal(t, vUpdated, ptr)
		})

		s.Test(`a delete by id to the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = spec.managerGet(t).FindByID(spec.context(), spec.newT(), id) // should trigger caching
			_, _ = iterators.Count(spec.managerGet(t).FindAll(spec.context()))  // should trigger caching

			// delete
			require.Nil(t, spec.managerGet(t).DeleteByID(spec.context(), id))

			// assert
			contracts.IsAbsent(t, spec.T, spec.managerGet(t), spec.context(), id)
		})

		s.Test(`a delete all entity in the storage should invalidate the local cache unit entity state`, func(t *testcase.T) {
			v := t.I(`value`)
			id, _ := extid.Lookup(v)

			// cache
			_, _ = spec.managerGet(t).FindByID(spec.context(), spec.newT(), id) // should trigger caching
			_, _ = iterators.Count(spec.managerGet(t).FindAll(spec.context()))  // should trigger caching

			// delete
			require.Nil(t, spec.managerGet(t).DeleteAll(spec.context()))
			waiter.Wait()

			contracts.IsAbsent(t, spec.T, spec.managerGet(t), spec.context(), id) // should trigger caching for not found
		})
	})
}

func (spec Manager) manager() testcase.Var {
	return testcase.Var{Name: `cache`}
}

func (spec Manager) managerGet(t *testcase.T) Cache {
	return spec.manager().Get(t).(Cache)
}

func (spec Manager) onePhaseCommitProtocolManager() testcase.Var {
	return testcase.Var{Name: `one phase commit protocol manager`}
}

func (spec Manager) onePhaseCommitProtocolManagerGet(t *testcase.T) frameless.OnePhaseCommitProtocol {
	return spec.onePhaseCommitProtocolManager().Get(t).(frameless.OnePhaseCommitProtocol)
}

func (spec Manager) source() testcase.Var {
	// source resource where the cache manager retrieve the data in case cache hit is missing
	return testcase.Var{Name: `cache manager's source of truth`}
}

func (spec Manager) sourceGet(t *testcase.T) cache.Source {
	return spec.source().Get(t).(cache.Source)
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

func (spec Manager) describeResultCaching(s *testcase.Spec) {
	s.Context(reflects.SymbolicName(spec.T), func(s *testcase.Spec) {
		value := s.Let(`stored value`, func(t *testcase.T) interface{} {
			ptr := spec.createT()
			storage := spec.sourceGet(t)
			require.Nil(t, storage.Create(spec.context(), ptr))
			id, _ := extid.Lookup(ptr)
			t.Defer(storage.DeleteByID, spec.context(), id)
			return ptr
		})

		s.Then(`it will return the value`, func(t *testcase.T) {
			v := spec.newT()
			id, found := extid.Lookup(value.Get(t))
			require.True(t, found)
			found, err := spec.managerGet(t).FindByID(spec.context(), v, id)
			require.Nil(t, err)
			require.True(t, found)
			require.Equal(t, value.Get(t), v)
		})

		s.And(`after value already cached`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, found := extid.Lookup(value.Get(t))
				require.True(t, found)
				v := contracts.IsFindable(t, spec.T, spec.sourceGet(t), spec.context(), id)
				require.Equal(t, value.Get(t), v)
			})

			s.And(`value is suddenly updated `, func(s *testcase.Spec) {
				valueWithNewContent := s.Let(`value-with-new-content`, func(t *testcase.T) interface{} {
					id, found := extid.Lookup(value.Get(t))
					require.True(t, found)
					nv := spec.createT()
					require.Nil(t, extid.Set(nv, id))
					return nv
				})

				s.Before(func(t *testcase.T) {
					ptr := valueWithNewContent.Get(t)
					contracts.UpdateEntity(t, spec.managerGet(t), spec.context(), ptr)
					waiter.Wait()
				})

				s.Then(`it will return the new value instead the old one`, func(t *testcase.T) {
					id, found := extid.Lookup(value.Get(t))
					require.True(t, found)
					require.NotEmpty(t, id)
					contracts.HasEntity(t, spec.managerGet(t), spec.context(), valueWithNewContent.Get(t))

					async.Assert(t, func(tb testing.TB) {
						v := spec.newT()
						found, err := spec.managerGet(t).FindByID(spec.context(), v, id)
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
					v := spec.newT()
					found, err := spec.managerGet(t).FindByID(spec.context(), v, id)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, value, v)
				}
			})

			s.When(`the storage is sensitive to continuous requests`, func(s *testcase.Spec) {
				stub := s.Let(`stub`, func(t *testcase.T) interface{} {
					return &StubSource{Source: spec.sourceGet(t)}
				})
				stubGet := func(t *testcase.T) *StubSource {
					return stub.Get(t).(*StubSource)
				}
				s.Before(func(t *testcase.T) {
					spec.source().Set(t, stubGet(t))
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
					nv = contracts.IsFindable(t, spec.T, spec.managerGet(t), spec.context(), id)
					require.Equal(t, value, nv)
					numberOfFindByIDCallAfterEntityIsFound := stubGet(t).count.FindByID
					waiter.Wait()

					nv = spec.newT()
					found, err = spec.managerGet(t).FindByID(spec.context(), nv, id) // should use cached value
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, value, nv)
					require.Equal(t, numberOfFindByIDCallAfterEntityIsFound, stubGet(t).count.FindByID)
				})
			})
		})
	}, testcase.Flaky(time.Minute))
}

func (spec Manager) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec Manager) newT() interface{} {
	return reflect.New(reflect.TypeOf(spec.T)).Interface()
}

func (spec Manager) createT() interface{} {
	return spec.FixtureFactory.Create(spec.T)
}
