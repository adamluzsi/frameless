package resources

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"
	"testing"

	"github.com/stretchr/testify/require"
)

type Delete interface {
	Delete(ctx context.Context, Entity interface{}) error
}

// DeleteSpec request a destroy of a specific entity that is wrapped in the query use case object
type DeleteSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject iDelete
}

type iDelete interface {
	Delete

	MinimumRequirements
}

// Test will test that an DeleteSpec is implemented by a generic specification
func (spec DeleteSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	extIDFieldRequired(s, spec.EntityType)

	s.Describe(`Delete`, func(s *testcase.Spec) {

		subject := func(t *testcase.T) error {
			return spec.Subject.Delete(
				t.I(`ctx`).(context.Context),
				t.I(`entity`),
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.EntityType)
		})

		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
		})

		s.When(`ID is set in the entity object`, func(s *testcase.Spec) {
			s.Around(func(t *testcase.T) func() {
				entity := t.I(`entity`)
				require.Nil(t, spec.Subject.Save(spec.Context(), entity))
				id, ok := LookupID(entity)
				require.True(t, ok, frameless.ErrIDRequired.Error())

				return func() {
					_ = spec.Subject.DeleteByID(spec.Context(), spec.EntityType, id)
				}
			})

			s.Then(`it is expected to delete the object in the Resource`, func(t *testcase.T) {
				entity := t.I(`entity`)
				ID, _ := LookupID(entity)

				err := subject(t)
				require.Nil(t, err)

				e := spec.FixtureFactory.Create(spec.EntityType)
				ok, err := spec.Subject.FindByID(spec.Context(), e, ID)
				require.Nil(t, err)
				require.False(t, ok)
			})
		})

		s.When(`ID is has no value or empty in the entity`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, ok := LookupID(t.I(`entity`))
				require.True(t, ok)
				require.Empty(t, id)
			})

			s.Test(`it is expected to return with error`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

		s.When(`context arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it will respond with ctx canceled error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})
	})
}

func TestDelete(t *testing.T, r iDelete, e interface{}, f FixtureFactory) {
	DeleteSpec{EntityType: e, FixtureFactory: f, Subject: r}.Test(t)
}
