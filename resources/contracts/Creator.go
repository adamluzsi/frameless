package contracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Creator struct {
	T
	Subject func(testing.TB) CRD
	FixtureFactory
}

func (spec Creator) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	resource := s.Let(`resource`, func(t *testcase.T) interface{} {
		return spec.Subject(t)
	})
	resourceGet := func(t *testcase.T) CRD {
		return resource.Get(t).(CRD)
	}

	s.Describe(`Creator`, func(s *testcase.Spec) {
		var (
			ctx = s.Let(`ctx`, func(t *testcase.T) interface{} {
				return spec.Context()
			})
			ptr = s.Let(`entity`, func(t *testcase.T) interface{} {
				return spec.FixtureFactory.Create(spec.T)
			})
			getID = func(t *testcase.T) interface{} {
				id, _ := resources.LookupID(ptr.Get(t))
				return id
			}
		)
		subject := func(t *testcase.T) error {
			ctx := ctx.Get(t).(context.Context)
			err := resourceGet(t).Create(ctx, ptr.Get(t))
			if err == nil {
				id, _ := resources.LookupID(ptr.Get(t))
				t.Defer(resourceGet(t).DeleteByID, ctx, spec.T, id)
				IsFindable(t, spec.T, resourceGet(t), ctx, id)
			}
			return err
		}

		s.When(`entity was not saved before`, func(s *testcase.Spec) {
			s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.NotEmpty(t, getID(t))
			})

			s.Then(`entity could be retrieved by ID`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.Equal(t, ptr.Get(t), IsFindable(t, spec.T, resourceGet(t), spec.Context(), getID(t)))
			})
		})

		s.When(`entity was already saved once`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
				IsFindable(t, spec.T, resourceGet(t), spec.Context(), getID(t))
			})

			s.Then(`it will raise error because ext:ID field already points to a existing record`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

		s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
				IsFindable(t, spec.T, resourceGet(t), spec.Context(), getID(t))
				require.Nil(t, resourceGet(t).DeleteByID(spec.Context(), spec.T, getID(t)))
				IsAbsent(t, spec.T, resourceGet(t), spec.Context(), getID(t))
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				require.Nil(t, subject(t))
			})

			s.Then(`persisted object can be found`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				IsFindable(t, spec.T, resourceGet(t), spec.Context(), getID(t))
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.Test(`persist on #Create`, func(t *testcase.T) {
			e := spec.FixtureFactory.Create(spec.T)
			err := resourceGet(t).Create(spec.Context(), e)
			require.Nil(t, err)

			ID, ok := resources.LookupID(e)
			require.True(t, ok, "ID is not defined in the entity struct src definition")
			require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

			require.Equal(t, e, IsFindable(t, spec.T, resourceGet(t), spec.Context(), ID))
			require.Nil(t, resourceGet(t).DeleteByID(spec.Context(), spec.T, ID))
		})
	})
}

func (spec Creator) Benchmark(b *testing.B) {
	resource := spec.Subject(b)
	cleanup(b, resource, spec.FixtureFactory, spec.T)
	b.Run(`Creator`, func(b *testing.B) {
		es := createEntities(spec.FixtureFactory, spec.T)
		defer cleanup(b, resource, spec.FixtureFactory, spec.T)

		b.ResetTimer()
		for _, ptr := range es {
			require.Nil(b, resource.Create(spec.Context(), ptr))
		}
	})
}
