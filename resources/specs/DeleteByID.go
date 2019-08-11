package specs

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// DeleteByID request to destroy a business entity in the resource that implement it's test.
type DeleteByID interface {
	DeleteByID(ctx context.Context, Type interface{}, ID string) error
}

type DeleteByIDSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec DeleteByIDSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`DeleteByID`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) error {
			return spec.Subject.DeleteByID(
				t.I(`ctx`).(context.Context),
				spec.EntityType,
				t.I(`id`).(string),
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

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`entity`)))
			})

			s.Let(`id`, func(t *testcase.T) interface{} {
				id, ok := LookupID(t.I(`entity`))
				require.True(t, ok, frameless.ErrIDRequired.Error())
				require.NotEmpty(t, id)
				return id
			})

			s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				e := reflects.New(spec.EntityType)
				found, err := spec.Subject.FindByID(spec.Context(), e, t.I(`id`).(string))
				require.Nil(t, err)
				require.False(t, found)
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				s.Let(`ctx`, func(t *testcase.T) interface{} {
					ctx, cancel := context.WithCancel(spec.Context())
					cancel()
					return ctx
				})

				s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
					require.Equal(t, context.Canceled, subject(t))
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					return spec.FixtureFactory.Create(spec.EntityType)
				})
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`oth-entity`)))
				})

				s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
					require.Nil(t, subject(t))

					othID, ok := LookupID(t.I(`oth-entity`))
					require.True(t, ok, frameless.ErrIDRequired.Error())

					e := reflects.New(spec.EntityType)
					found, err := spec.Subject.FindByID(spec.Context(), e, othID)
					require.Nil(t, err)
					require.True(t, found)
				})
			})

			s.And(`the entity was deleted then`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, subject(t))
				})

				s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
					require.Equal(t, frameless.ErrNotFound, subject(t))
				})
			})
		})

		s.When(`entity never saved before in the resource`, func(s *testcase.Spec) {
			s.Let(`id`, func(t *testcase.T) interface{} {
				id, _ := LookupID(t.I(`entity`))
				return id
			})

			s.Before(func(t *testcase.T) {
				require.Empty(t, t.I(`id`).(string))
			})

			s.Then(`it will return with error, because you cannot delete something that does not exist`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

	})
}

func TestDeleteByID(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	DeleteByIDSpec{Subject: r, EntityType: e, FixtureFactory: f}.Test(t)
}
