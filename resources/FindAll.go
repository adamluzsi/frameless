package resources

import (
	"context"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/adamluzsi/frameless"
)

type FindAll interface {
	FindAll(ctx context.Context, Type interface{}) frameless.Iterator
}

// FindAllSpec can return business entities from a given storage that implement it's test
// The "EntityType" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAllSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject iFindAll
}

type iFindAll interface {
	FindAll

	MinimumRequirements
}

func (spec FindAllSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`FindAll`, func(s *testcase.Spec) {

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

func TestFindAll(t *testing.T, r iFindAll, e interface{}, f FixtureFactory) {
	FindAllSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
}
