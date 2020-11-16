package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Creator struct {
	T interface{}
	FixtureFactory
	Subject minimumRequirements
}

func (spec Creator) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`Creator`, func(s *testcase.Spec) {
		getEntity := func(t *testcase.T) interface{} { return t.I(`entity`) }
		subject := func(t *testcase.T) error {
			ctx := t.I(`ctx`).(context.Context)
			err := spec.Subject.Create(ctx, getEntity(t))
			if err == nil {
				id, _ := resources.LookupID(getEntity(t))
				t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)
			}
			return err
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})

		s.When(`entity was not saved before`, func(s *testcase.Spec) {
			s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				id, _ := resources.LookupID(getEntity(t))
				require.NotEmpty(t, id)
			})

			s.Then(`entity could be retrieved by ID`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				entity := getEntity(t)
				id, _ := resources.LookupID(entity)
				ptr := newEntity(spec.T)
				found, err := spec.Subject.FindByID(spec.Context(), ptr, id)
				require.Nil(t, err)
				require.True(t, found)
				require.Equal(t, entity, ptr)
			})
		})

		s.When(`entity was already saved once`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) { require.Nil(t, subject(t)) })

			s.Then(`it will raise error because ext:ID field already points to an Resource entry`, func(t *testcase.T) {
				t.Log(`this should not be misinterpreted as uniq value`)
				t.Log(`it is only about that the ext:ID field is already pointing to something`)
				require.Error(t, subject(t))
			})
		})

		s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				require.Nil(t, subject(t))
			})

			s.Then(`persisted object can be found`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				ptr := newEntity(spec.T)
				id, _ := resources.LookupID(getEntity(t))
				found, err := spec.Subject.FindByID(spec.Context(), ptr, id)
				require.Nil(t, err)
				require.True(t, found)
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
			t.T.Run("persist on #Create", func(t *testing.T) {
				e := spec.FixtureFactory.Create(spec.T)
				err := spec.Subject.Create(spec.Context(), e)
				require.Nil(t, err)

				ID, ok := resources.LookupID(e)
				require.True(t, ok, "ID is not defined in the entity struct src definition")
				require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

				actual := newEntity(spec.T)
				ok, err = spec.Subject.FindByID(spec.Context(), actual, ID)
				require.Nil(t, err)
				require.True(t, ok)
				require.Equal(t, e, actual)

				require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.T, ID))
			})
		})
	})
}

func (spec Creator) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	b.Run(`Creator`, func(b *testing.B) {
		es := createEntities(spec.FixtureFactory, spec.T)
		defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)

		b.ResetTimer()
		for _, ptr := range es {
			require.Nil(b, spec.Subject.Create(spec.Context(), ptr))
		}
	})
}
