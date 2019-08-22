package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type FinderSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec FinderSpec) Test(t *testing.T) {
	t.Run(`FinderSpec`, func(t *testing.T) {
		findByIDSpec{
			EntityType:     spec.EntityType,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Test(t)
		findAllSpec{
			EntityType:     spec.EntityType,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Test(t)
	})
}

func (spec FinderSpec) Benchmark(b *testing.B) {
	b.Run(`FinderSpec`, func(b *testing.B) {
		findByIDSpec{
			EntityType:     spec.EntityType,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Benchmark(b)
		findAllSpec{
			EntityType:     spec.EntityType,
			FixtureFactory: spec.FixtureFactory,
			Subject:        spec.Subject,
		}.Benchmark(b)
	})
}

type findByIDSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec findByIDSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`FindByID`, func(s *testcase.Spec) {

		subject := func(t *testcase.T) (bool, error) {
			return spec.Subject.FindByID(
				t.I(`ctx`).(context.Context),
				t.I(`ptr`),
				t.I(`id`).(string),
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.Let(`ptr`, func(t *testcase.T) interface{} {
			return reflects.New(spec.EntityType)
		})

		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
		})

		s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.EntityType)
		})

		s.When(`entity was saved in the Resource`, func(s *testcase.Spec) {

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`entity`)))
			})

			s.Let(`id`, func(t *testcase.T) interface{} {
				id, ok := resources.LookupID(t.I(`entity`))
				require.True(t, ok)
				return id
			})

			s.Then(`the entity will be returned`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.True(t, found)
				require.Equal(t, t.I(`entity`), t.I(`ptr`))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				s.Let(`ctx`, func(t *testcase.T) interface{} {
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

			s.And(`more similar entity is saved in the Resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					return spec.FixtureFactory.Create(spec.EntityType)
				})
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`oth-entity`)))
				})

				s.Then(`the entity`, func(t *testcase.T) {
					found, err := subject(t)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, t.I(`entity`), t.I(`ptr`))
				})
			})
		})

		s.When(`no entity saved before in the Resource`, func(s *testcase.Spec) {
			s.Let(`id`, func(t *testcase.T) interface{} { return `` })

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
			})

			s.Then(`the it will have no result`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.False(t, found)
			})
		})

	})

	s.Test(`E2E`, func(t *testcase.T) {
		var ids []string

		for i := 0; i < 12; i++ {

			entity := spec.FixtureFactory.Create(spec.EntityType)

			require.Nil(t, spec.Subject.Save(spec.Context(), entity))
			ID, ok := resources.LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)
			ids = append(ids, ID)

		}

		defer func() {
			for _, id := range ids {
				require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.EntityType, id))
			}
		}()

		t.Run("when no value stored that the query request", func(t *testing.T) {
			ptr := reflects.New(spec.EntityType)

			ok, err := spec.Subject.FindByID(spec.Context(), ptr, "not existing ID")

			require.Nil(t, err)
			require.False(t, ok)
		})

		t.Run("values returned", func(t *testing.T) {
			for _, ID := range ids {

				entityPtr := reflects.New(spec.EntityType)
				ok, err := spec.Subject.FindByID(spec.Context(), entityPtr, ID)

				require.Nil(t, err)
				require.True(t, ok)

				actualID, ok := resources.LookupID(entityPtr)

				if !ok {
					t.Fatal("can't find ID in the returned value")
				}

				require.Equal(t, ID, actualID)

			}
		})
	})

}

func (spec findByIDSpec) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)
	b.Run(`FindByID`, func(b *testing.B) {
		es := createEntities(benchmarkEntityVolumeCount, spec.FixtureFactory, spec.EntityType)
		ids := saveEntities(b, spec.Subject, spec.FixtureFactory, es...)
		defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)

		var executionTimes int
		b.ResetTimer()
	wrk:
		for {
			for _, id := range ids {
				ptr := reflects.New(spec.EntityType)
				found, err := spec.Subject.FindByID(spec.Context(), ptr, id)
				require.Nil(b, err)
				require.True(b, found)

				executionTimes++
				if b.N <= executionTimes {
					break wrk
				}
			}
		}
	})
}

// findAllSpec can return business entities from a given storage that implement it's test
// The "EntityType" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type findAllSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec findAllSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`FinderAll`, func(s *testcase.Spec) {

		subject := func(t *testcase.T) frameless.Iterator {
			return spec.Subject.FindAll(
				t.I(`ctx`).(context.Context),
				spec.EntityType,
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
		})

		s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.EntityType)
		})

		s.When(`entity was saved in the Resource`, func(s *testcase.Spec) {

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`entity`)))
			})

			s.Then(`the entity will returns the all the entity in volume`, func(t *testcase.T) {
				count, err := iterators.Count(subject(t))
				require.Nil(t, err)
				require.Equal(t, 1, count)
			})

			s.Then(`then the returned iterator includes the stored entity`, func(t *testcase.T) {
				all := subject(t)
				var entities []interface{}
				require.Nil(t, iterators.CollectAll(all, &entities))
				require.Equal(t, 1, len(entities))
				require.Contains(t, entities, reflects.BaseValueOf(t.I(`entity`)).Interface())
			})

			s.And(`more similar entity is saved in the Resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					return spec.FixtureFactory.Create(spec.EntityType)
				})
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`oth-entity`)))
				})

				s.Then(`all entity will be fetched`, func(t *testcase.T) {
					all := subject(t)
					var entities []interface{}
					require.Nil(t, iterators.CollectAll(all, &entities))
					require.Equal(t, 2, len(entities))
					require.Contains(t, entities, reflects.BaseValueOf(t.I(`entity`)).Interface())
					require.Contains(t, entities, reflects.BaseValueOf(t.I(`oth-entity`)).Interface())
				})
			})
		})

		s.When(`no entity saved before in the Resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
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
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)
	b.Run(`FindAll`, func(b *testing.B) {
		es := createEntities(benchmarkEntityVolumeCount, spec.FixtureFactory, spec.EntityType)
		saveEntities(b, spec.Subject, spec.FixtureFactory, es...)
		defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			i := spec.Subject.FindAll(spec.Context(), spec.EntityType)
			_, _ = iterators.Count(i)
		}
	})
}
