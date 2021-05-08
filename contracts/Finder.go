package contracts

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"reflect"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

type Finder struct {
	T
	Subject func(testing.TB) CRD
	FixtureFactory
}

func (spec Finder) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec Finder) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec Finder) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	const name = `Finder`
	s.Context(name, func(s *testcase.Spec) {
		testcase.RunContract(s,
			findByID{
				T:              spec.T,
				FixtureFactory: spec.FixtureFactory,
				Subject:        spec.Subject,
			},
			findAll{
				T:              spec.T,
				FixtureFactory: spec.FixtureFactory,
				Subject:        spec.Subject,
			})
	}, testcase.Group(name))
}

type findByID struct {
	T
	Subject func(testing.TB) CRD
	FixtureFactory
}

func (spec findByID) Test(t *testing.T) {
	FindOne{
		T:              spec.T,
		FixtureFactory: spec.FixtureFactory,
		Subject:        spec.Subject,
		MethodName:     ".FindByID",
		ToQuery: func(tb testing.TB, resource interface{}, ent T) QueryOne {
			id, ok := extid.Lookup(ent)
			if !ok { // if no id found create a dummy ID
				// since an id is required always to use FindByID
				// we generate a dummy id in case the received entity don't have one.
				// This helps to avoid error cases where ID is not set actually.
				// For those we have further specification later
				id = spec.createDummyID(tb, resource.(CRD))
			}

			return func(tb testing.TB, ctx context.Context, ptr T) (found bool, err error) {
				return resource.(frameless.Finder).FindByID(ctx, ptr, id)
			}
		},

		Specify: func(tb testing.TB) {
			testcase.NewSpec(tb).Test(`E2E`, func(t *testcase.T) {
				r := spec.Subject(t)

				var ids []interface{}
				for i := 0; i < 12; i++ {
					entity := spec.FixtureFactory.Create(spec.T)
					CreateEntity(t, r, spec.Context(), entity)
					id, ok := extid.Lookup(entity)
					require.True(t, ok, ErrIDRequired.Error())
					ids = append(ids, id)
				}

				t.Log("when no value stored that the query request")
				ptr := newEntity(spec.T)
				ok, err := r.FindByID(spec.Context(), ptr, "not existing ID")
				require.Nil(t, err)
				require.False(t, ok)

				t.Log("values returned")
				for _, ID := range ids {
					e := newEntity(spec.T)
					ok, err := r.FindByID(spec.Context(), e, ID)
					require.Nil(t, err)
					require.True(t, ok)

					actualID, ok := extid.Lookup(e)
					require.True(t, ok, "can't find ID in the returned value")
					require.Equal(t, ID, actualID)
				}
			})
		},
	}.Test(t)
}

func (spec findByID) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)
	r := spec.Subject(b)

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newEntity(spec.T)
		CreateEntity(t, r, spec.Context(), ptr)
		return ptr
	}).EagerLoading(s)

	id := s.Let(`id`, func(t *testcase.T) interface{} {
		return HasID(t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		_, err := r.FindByID(spec.Context(), newEntity(spec.T), id.Get(t))
		require.Nil(t, err)
	})
}

func (spec findByID) createDummyID(tb testing.TB, r CRD) interface{} {
	ent := spec.FixtureFactory.Create(spec.T)
	ctx := spec.FixtureFactory.Context()
	CreateEntity(tb, r, ctx, ent)
	id := HasID(tb, ent)
	DeleteEntity(tb, r, ctx, ent)
	return id
}

// findAll can return business entities from a given storage that implement it's test
// The "EntityTypeName" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type findAll struct {
	T
	Subject func(testing.TB) CRD
	FixtureFactory
}

func (spec findAll) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		resource = s.Let(`resource`, func(t *testcase.T) interface{} {
			return spec.Subject(t)
		})
		resourceGet = func(t *testcase.T) CRD {
			return resource.Get(t).(CRD)
		}
	)

	beforeAll := &sync.Once{}
	s.Before(func(t *testcase.T) {
		beforeAll.Do(func() {
			DeleteAllEntity(t, resourceGet(t), spec.Context(), spec.T)
		})
	})

	s.Describe(`FindAll`, func(s *testcase.Spec) {
		var (
			ctx = s.Let(`ctx`, func(t *testcase.T) interface{} {
				return spec.Context()
			})
			subject = func(t *testcase.T) iterators.Interface {
				return resourceGet(t).FindAll(ctx.Get(t).(context.Context))
			}
		)

		s.Before(func(t *testcase.T) {
			DeleteAllEntity(t, resourceGet(t), spec.Context(), spec.T)
		})

		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				CreateEntity(t, resourceGet(t), spec.Context(), entity.Get(t))
			})

			s.Then(`the entity will returns the all the entity in volume`, func(t *testcase.T) {
				AsyncTester.Assert(t, func(tb testing.TB) {
					count, err := iterators.Count(subject(t))
					require.Nil(tb, err)
					require.Equal(tb, 1, count)
				})
			})

			s.Then(`the returned iterator includes the stored entity`, func(t *testcase.T) {
				AsyncTester.Assert(t, func(tb testing.TB) {
					entities := spec.findAllN(t, subject, 1)
					contains(tb, entities, entity.Get(t))
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				othEntity := s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					ent := spec.FixtureFactory.Create(spec.T)
					CreateEntity(t, resourceGet(t), spec.Context(), ent)
					return ent
				}).EagerLoading(s)

				s.Then(`all entity will be fetched`, func(t *testcase.T) {
					AsyncTester.Assert(t, func(tb testing.TB) {
						entities := spec.findAllN(t, subject, 2)
						contains(tb, entities, entity.Get(t))
						contains(tb, entities, othEntity.Get(t))
					})
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				DeleteAllEntity(t, resourceGet(t), spec.Context(), spec.T)
			})

			s.Then(`the iterator will have no result`, func(t *testcase.T) {
				count, err := iterators.Count(subject(t))
				require.Nil(t, err)
				require.Equal(t, 0, count)
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				iter := subject(t)
				err := iter.Err()
				require.Error(t, err)
				require.Equal(t, context.Canceled, err)
			})
		})
	})
}

func (spec findAll) findAllN(t *testcase.T, subject func(t *testcase.T) iterators.Interface, n int) []interface{} {
	sliceRType := reflect.SliceOf(reflect.TypeOf(spec.T))
	var entities interface{}
	AsyncTester.Assert(t, func(tb testing.TB) {
		all := subject(t)
		entities = reflect.MakeSlice(sliceRType, 0, 0).Interface()
		require.Nil(t, iterators.Collect(all, &entities))
		require.Len(t, entities, n)
	})

	var out = []interface{}{}
	rslice := reflect.ValueOf(entities)
	for i := 0; i < rslice.Len(); i++ {
		out = append(out, rslice.Index(i).Interface())
	}
	return out
}

func (spec findAll) Benchmark(b *testing.B) {
	r := spec.Subject(b)
	cleanup(b, r, spec.FixtureFactory, spec.T)

	s := testcase.NewSpec(b)

	s.Before(func(t *testcase.T) {
		saveEntities(t, r, spec.FixtureFactory,
			createEntities(spec.FixtureFactory, spec.T)...)
	})

	s.Test(``, func(t *testcase.T) {
		_, err := iterators.Count(r.FindAll(spec.Context()))
		require.Nil(t, err)
	})
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// QueryOne is the generic representation of a query that meant to find one result.
// It is really similar to resources.Finder#FindByID,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOne is generated through ToQuery factory function in FindOne resource contract specification.
type QueryOne func(tb testing.TB, ctx context.Context, ptr T) (found bool, err error)

type FindOne struct {
	T
	Subject func(testing.TB) CRD
	FixtureFactory
	// MethodName is the name of the test subject QueryOne method of this contract specification.
	MethodName string
	// ToQuery takes an entity ptr and returns with a closure that has the knowledge about how to query on the Subject resource to find the entity.
	//
	// By convention, any preparation action that affect the Storage must take place prior to returning the closure.
	// The QueryOne closure should only have the Method call with the already mapped values.
	// ToQuery will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	ToQuery func(tb testing.TB, resource /* CRD */ interface{}, ent T) QueryOne
	// Specify allow further specification describing for a given FindOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

func (spec FindOne) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec FindOne) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec FindOne) Spec(tb testing.TB) {
	testcase.NewSpec(tb).Describe(spec.MethodName, func(s *testcase.Spec) {
		var (
			ctx = s.Let(ctx.Name, func(t *testcase.T) interface{} {
				return spec.Context()
			})
			entity = s.Let(`entity`, func(t *testcase.T) interface{} {
				return spec.FixtureFactory.Create(spec.T)
			})
			ptr = s.Let(`ptr`, func(t *testcase.T) interface{} {
				return newEntity(spec.T)
			})
			resource = s.Let(`resource`, func(t *testcase.T) interface{} {
				return spec.Subject(t)
			})
			resourceGet = func(t *testcase.T) CRD {
				return resource.Get(t).(CRD)
			}
			query = s.Let(`query`, func(t *testcase.T) interface{} {
				t.Log(entity.Get(t))
				return spec.ToQuery(t, resourceGet(t), entity.Get(t))
			})
			subject = func(t *testcase.T) (bool, error) {
				return query.Get(t).(QueryOne)(t, ctx.Get(t).(context.Context), ptr.Get(t))
			}
		)

		s.Before(func(t *testcase.T) {
			DeleteAllEntity(t, resourceGet(t), spec.Context(), spec.T)
		})

		s.When(`entity was present in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				CreateEntity(t, resourceGet(t), spec.Context(), entity.Get(t))
				HasID(t, entity.Get(t))
			})

			s.Then(`the entity will be returned`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.True(t, found)
				require.Equal(t, entity.Get(t), ptr.Get(t))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) interface{} {
					ctx, cancel := context.WithCancel(spec.Context())
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					found, err := subject(t)
					require.Equal(t, context.Canceled, err)
					require.False(t, found)
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					ent := spec.FixtureFactory.Create(spec.T)
					CreateEntity(t, resourceGet(t), spec.Context(), ent)
					return ent
				}).EagerLoading(s)

				s.Then(`still the correct entity is returned`, func(t *testcase.T) {
					found, err := subject(t)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, t.I(`entity`), t.I(`ptr`))
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				DeleteAllEntity(t, resourceGet(t), spec.Context(), spec.T)
			})

			s.Then(`it will have no result`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.False(t, found)
			})
		})

		if spec.Specify != nil {
			s.Test(`Specify`, func(t *testcase.T) {
				spec.Specify(t)
			})
		}
	})
}

func (spec FindOne) copyPtrValue(ptr interface{}) interface{} {
	rv := reflect.ValueOf(ptr)
	copyRV := reflect.New(rv.Elem().Type())
	copyRV.Elem().Set(rv.Elem()) // copy with pass by value
	return copyRV.Interface()
}
