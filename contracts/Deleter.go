package contracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Deleter struct {
	T
	Subject        func(testing.TB) CRD
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

func (c Deleter) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c Deleter) resourceGet(t *testcase.T) CRD {
	return c.resource().Get(t).(CRD)
}

func (c Deleter) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Deleter) Benchmark(b *testing.B) {
	b.Run(`DeleteByID`, c.benchmarkDeleteByID)
	b.Run(`DeleteAll`, c.benchmarkDeleteAll)
}

func (c Deleter) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	factoryLet(s, c.FixtureFactory)
	s.Describe(`DeleteByID`, c.specDeleteByID)
	s.Describe(`DeleteAll`, c.specDeleteAll)
}

func (c Deleter) specDeleteByID(s *testcase.Spec) {
	var (
		ctx = ctx.Let(s, func(t *testcase.T) interface{} {
			return c.Context(t)
		})
		id      = testcase.Var{Name: `id`}
		subject = func(t *testcase.T) error {
			return c.resourceGet(t).DeleteByID(ctx.Get(t).(context.Context), id.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		DeleteAllEntity(t, c.resourceGet(t), c.Context(t))
	})

	entity := s.Let(`entity`, func(t *testcase.T) interface{} {
		return CreatePTR(factoryGet(t), c.T)
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) interface{} {
			ent := entity.Get(t)
			CreateEntity(t, c.resourceGet(t), c.Context(t), ent)
			id, ok := extid.Lookup(ent)
			require.True(t, ok, ErrIDRequired.Error())
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(c.Context(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := s.Let(`oth-entity`, func(t *testcase.T) interface{} {
				ent := CreatePTR(factoryGet(t), c.T)
				CreateEntity(t, c.resourceGet(t), c.Context(t), ent)
				return ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				othID, _ := extid.Lookup(othEntity.Get(t))
				IsFindable(t, c.T, c.resourceGet(t), c.Context(t), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
				IsAbsent(t, c.T, c.resourceGet(t), ctx.Get(t).(context.Context), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})
	})
}

func (c Deleter) benchmarkDeleteByID(b *testing.B) {
	s := testcase.NewSpec(b)

	s.Around(func(t *testcase.T) func() {
		cleanup(b, c.Context(b), c.resourceGet(t))
		return func() { cleanup(b, c.Context(b), c.resourceGet(t)) }
	})

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newT(c.T)
		CreateEntity(t, c.resourceGet(t), c.Context(t), ptr)
		return ptr
	}).EagerLoading(s)

	id := s.Let(`id`, func(t *testcase.T) interface{} {
		return HasID(t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		require.Nil(b, c.resourceGet(t).DeleteByID(c.Context(t), id.Get(t)))
	})
}

func (c Deleter) specDeleteAll(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		return c.resourceGet(t).DeleteAll(t.I(`ctx`).(context.Context))
	}

	s.Let(`ctx`, func(t *testcase.T) interface{} { return c.Context(t) })

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		s.Let(`ctx`, func(t *testcase.T) interface{} {
			ctx, cancel := context.WithCancel(c.Context(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			require.Equal(t, context.Canceled, subject(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := CreatePTR(factoryGet(t), c.T)
		CreateEntity(t, c.resourceGet(t), c.Context(t), ent)
		eID := HasID(t, ent)
		IsFindable(t, c.T, c.resourceGet(t), c.Context(t), eID)
		require.Nil(t, subject(t))
		IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), eID)
	})
}

func (c Deleter) benchmarkDeleteAll(b *testing.B) {
	r := c.Subject(b)
	ctx := c.Context(b)
	cleanup(b, ctx, r)
	defer cleanup(b, ctx, r)
	// for some reason, doing setup with timer stop/start
	// makes this test unable to measure
	// the correct throughput, and hangs forever
	// so I just check empty delete all.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.Nil(b, r.DeleteAll(ctx))
	}
	b.StopTimer()
}
