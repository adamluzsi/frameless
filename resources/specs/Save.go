package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Save interface {
	Save(ctx context.Context, ptr interface{}) error
}

type SaveSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec SaveSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
	})

	s.Test(`ext id field required`, func(t *testcase.T) {
		_, hasExtID := LookupID(spec.EntityType)
		require.True(t, hasExtID, frameless.ErrIDRequired.Error())
	})

	s.Describe(`Save`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) error {
			return spec.Subject.Save(
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

		s.When(`entity was not saved before`, func(s *testcase.Spec) {
			s.Then(`entity field that is marked as ext:ID will updated`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				id, _ := LookupID(t.I(`entity`))
				require.NotEmpty(t, id)
			})

			s.Then(`entity could be retrieved by ID`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				entity := t.I(`entity`)
				id, _ := LookupID(entity)
				ptr := reflects.New(spec.EntityType)
				found, err := spec.Subject.FindByID(spec.Context(), ptr, id)
				require.Nil(t, err)
				require.True(t, found)
				require.Equal(t, entity, ptr)
			})
		})

		s.When(`entity was already saved once`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) { require.Nil(t, subject(t)) })

			s.Then(`it will raise error because ext:ID field already points to an resource entry`, func(t *testcase.T) {
				t.Log(`this should not be misinterpreted as uniq value`)
				t.Log(`it is only about that the ext:ID field is already pointing to something`)
				require.Error(t, subject(t))
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.Test(`E2E`, func(t *testcase.T) {
			t.Run("persist an Save", func(t *testing.T) {

				if ID, _ := LookupID(spec.EntityType); ID != "" {
					t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
				}

				e := spec.FixtureFactory.Create(spec.EntityType)
				err := spec.Subject.Save(spec.Context(), e)

				require.Nil(t, err)

				ID, ok := LookupID(e)
				require.True(t, ok, "ID is not defined in the entity struct src definition")
				require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

				actual := reflects.New(spec.EntityType)

				ok, err = spec.Subject.FindByID(spec.Context(), actual, ID)
				require.Nil(t, err)
				require.True(t, ok)
				require.Equal(t, e, actual)

				require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.EntityType, ID))

			})

			t.Run("when entity already have an ID", func(t *testing.T) {
				newEntity := spec.FixtureFactory.Create(spec.EntityType)
				require.Nil(t, SetID(newEntity, "Hello world!"))
				require.Error(t, spec.Subject.Save(spec.Context(), newEntity))
			})
		})
	})
}

func TestSave(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	SaveSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
}
