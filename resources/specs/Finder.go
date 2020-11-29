package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

type Finder struct {
	T interface{}
	FixtureFactory
	Subject minimumRequirements
}

func (spec Finder) Test(t *testing.T) {
	t.Run(`Finder`, func(t *testing.T) {
		findByIDSpec{
			T:              spec.T,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Test(t)

		findAllSpec{
			T:              spec.T,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Test(t)
	})
}

func (spec Finder) Benchmark(b *testing.B) {
	b.Run(`Finder`, func(b *testing.B) {
		findByIDSpec{
			T:              spec.T,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Benchmark(b)
		findAllSpec{
			T:              spec.T,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Benchmark(b)
	})
}

type findByIDSpec struct {
	T interface{}
	FixtureFactory
	Subject minimumRequirements
}

func (spec findByIDSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
	})

	s.Describe(`FindByID`, func(s *testcase.Spec) {

		var (
			ctx = s.Let(`ctx`, func(t *testcase.T) interface{} {
				return spec.Context()
			})
			ptr = s.Let(`ptr`, func(t *testcase.T) interface{} {
				return newEntity(spec.T)
			})
			id      = testcase.Var{Name: `id`}
			subject = func(t *testcase.T) (bool, error) {
				return spec.Subject.FindByID(
					ctx.Get(t).(context.Context),
					ptr.Get(t),
					id.Get(t),
				)
			}
		)

		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				CreateEntity(t, spec.Subject, spec.Context(), entity.Get(t))
				newID, ok := resources.LookupID(entity.Get(t))
				require.True(t, ok)
				id.Set(t, newID)
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

				s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
					found, err := subject(t)
					require.Equal(t, context.Canceled, err)
					require.False(t, found)
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					return spec.FixtureFactory.Create(spec.T)
				})
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.Create(spec.Context(), t.I(`oth-entity`)))
				})

				s.Then(`the entity`, func(t *testcase.T) {
					found, err := subject(t)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, t.I(`entity`), t.I(`ptr`))
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Let(`id`, func(t *testcase.T) interface{} { return `` })

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
			})

			s.Then(`it will have no result`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.False(t, found)
			})
		})

	})

	s.Test(`E2E`, func(t *testcase.T) {
		var ids []interface{}

		for i := 0; i < 12; i++ {
			entity := spec.FixtureFactory.Create(spec.T)
			CreateEntity(t, spec.Subject, spec.Context(), entity)
			id, ok := resources.LookupID(entity)
			require.True(t, ok, ErrIDRequired.Error())
			ids = append(ids, id)
		}

		t.Log("when no value stored that the query request")
		ptr := newEntity(spec.T)
		ok, err := spec.Subject.FindByID(spec.Context(), ptr, "not existing ID")
		require.Nil(t, err)
		require.False(t, ok)

		t.Log("values returned")
		for _, ID := range ids {
			e := newEntity(spec.T)
			ok, err := spec.Subject.FindByID(spec.Context(), e, ID)
			require.Nil(t, err)
			require.True(t, ok)

			actualID, ok := resources.LookupID(e)
			require.True(t, ok, "can't find ID in the returned value")
			require.Equal(t, ID, actualID)
		}
	})

}

func (spec findByIDSpec) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newEntity(spec.T)
		CreateEntity(t, spec.Subject, spec.Context(), ptr)
		return ptr
	}).EagerLoading(s)

	id := s.Let(`id`, func(t *testcase.T) interface{} {
		return HasID(t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		_, err := spec.Subject.FindByID(spec.Context(), spec.T, id.Get(t))
		require.Nil(t, err)
	})
}

// findAllSpec can return business entities from a given storage that implement it's test
// The "EntityTypeName" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type findAllSpec struct {
	T interface{}
	FixtureFactory
	Subject minimumRequirements
}

func (spec findAllSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
	})

	s.Describe(`FindAll`, func(s *testcase.Spec) {
		var (
			ctx = s.Let(`ctx`, func(t *testcase.T) interface{} {
				return spec.Context()
			})
			subject = func(t *testcase.T) iterators.Interface {
				return spec.Subject.FindAll(ctx.Get(t).(context.Context), spec.T)
			}
		)

		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
		})

		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				CreateEntity(t, spec.Subject, spec.Context(), entity.Get(t))
			})

			s.Then(`the entity will returns the all the entity in volume`, func(t *testcase.T) {
				count, err := iterators.Count(subject(t))
				require.Nil(t, err)
				require.Equal(t, 1, count)
			})

			s.Then(`the returned iterator includes the stored entity`, func(t *testcase.T) {
				all := subject(t)
				var entities []interface{}
				require.Nil(t, iterators.Collect(all, &entities))
				require.Equal(t, 1, len(entities))
				contains(t, entities, entity.Get(t))
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				othEntity := s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					ent := spec.FixtureFactory.Create(spec.T)
					CreateEntity(t, spec.Subject, spec.Context(), ent)
					return ent
				}).EagerLoading(s)

				s.Then(`all entity will be fetched`, func(t *testcase.T) {
					all := subject(t)
					var entities []interface{}
					require.Nil(t, iterators.Collect(all, &entities))
					require.Equal(t, 2, len(entities))
					contains(t, entities, entity.Get(t))
					contains(t, entities, othEntity.Get(t))
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				DeleteAllEntity(t, spec.Subject, spec.Context(), spec.T)
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

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				iter := subject(t)
				err := iter.Err()
				require.Error(t, err)
				require.Equal(t, context.Canceled, err)
			})
		})
	})
}

func (spec findAllSpec) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)

	s := testcase.NewSpec(b)

	s.Before(func(t *testcase.T) {
		saveEntities(t, spec.Subject, spec.FixtureFactory,
			createEntities(spec.FixtureFactory, spec.T)...)
	})

	s.Test(``, func(t *testcase.T) {
		_, err := iterators.Count(spec.Subject.FindAll(spec.Context(), spec.T))
		require.Nil(t, err)
	})
}
