package contracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Creator struct {
	T
	Subject        func(testing.TB) CRD
	FixtureFactory func(testing.TB) FixtureFactory
}

func (c Creator) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Creator) Benchmark(b *testing.B) {
	resource := c.Subject(b)
	ff := c.FixtureFactory(b)
	cleanup(b, resource, ff, c.T)
	b.Run(`Creator`, func(b *testing.B) {
		es := createEntities(ff, c.T)
		defer cleanup(b, resource, ff, c.T)

		b.ResetTimer()
		for _, ptr := range es {
			require.Nil(b, resource.Create(ff.Context(), ptr))
		}
	})
}

func (c Creator) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)
	resource := s.Let(`resource`, func(t *testcase.T) interface{} {
		return c.Subject(t)
	})
	resourceGet := func(t *testcase.T) CRD {
		return resource.Get(t).(CRD)
	}
	var (
		ctx = s.Let(`ctx`, func(t *testcase.T) interface{} {
			return factoryGet(t).Context()
		})
		ptr = s.Let(`entity`, func(t *testcase.T) interface{} {
			return CreatePTR(factoryGet(t), c.T)
		})
		getID = func(t *testcase.T) interface{} {
			id, _ := extid.Lookup(ptr.Get(t))
			return id
		}
	)
	subject := func(t *testcase.T) error {
		ctx := ctx.Get(t).(context.Context)
		err := resourceGet(t).Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := extid.Lookup(ptr.Get(t))
			t.Defer(resourceGet(t).DeleteByID, ctx, id)
			IsFindable(t, c.T, resourceGet(t), ctx, id)
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
			require.Equal(t, ptr.Get(t), IsFindable(t, c.T, resourceGet(t), factoryGet(t).Context(), getID(t)))
		})
	})

	s.When(`entity was already saved once`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, subject(t))
			IsFindable(t, c.T, resourceGet(t), factoryGet(t).Context(), getID(t))
		})

		s.Then(`it will raise error because ext:ID field already points to a existing record`, func(t *testcase.T) {
			require.Error(t, subject(t))
		})
	})

	s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, subject(t))
			IsFindable(t, c.T, resourceGet(t), factoryGet(t).Context(), getID(t))
			require.Nil(t, resourceGet(t).DeleteByID(factoryGet(t).Context(), getID(t)))
			IsAbsent(t, c.T, resourceGet(t), factoryGet(t).Context(), getID(t))
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			require.Nil(t, subject(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			IsFindable(t, c.T, resourceGet(t), factoryGet(t).Context(), getID(t))
		})
	})

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) interface{} {
			ctx, cancel := context.WithCancel(factoryGet(t).Context())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			require.Equal(t, context.Canceled, subject(t))
		})
	})

	s.Test(`persist on #Create`, func(t *testcase.T) {
		e := CreatePTR(factoryGet(t), c.T)
		err := resourceGet(t).Create(factoryGet(t).Context(), e)
		require.Nil(t, err)

		ID, ok := extid.Lookup(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

		require.Equal(t, e, IsFindable(t, c.T, resourceGet(t), factoryGet(t).Context(), ID))
		require.Nil(t, resourceGet(t).DeleteByID(factoryGet(t).Context(), ID))
	})
}
