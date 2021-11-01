package contracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/contracts/assert"
	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

// Updater will request an update for a wrapped entity object in the Resource
type Updater struct {
	T              T
	Subject        func(testing.TB) UpdaterSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type UpdaterSubject interface {
	CRD
	frameless.Updater
}

func (c Updater) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c Updater) resourceGet(t *testcase.T) UpdaterSubject {
	return c.resource().Get(t).(UpdaterSubject)
}

func (c Updater) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	factoryLet(s, c.FixtureFactory)

	s.Before(func(t *testcase.T) {
		assert.DeleteAllEntity(t, c.resourceGet(t), c.Context(t))
	})

	var (
		requestContext    = testcase.Var{Name: `request-Context`}
		entityWithChanges = testcase.Var{Name: `entity-with-changes`}
		subject           = func(t *testcase.T) error {
			return c.resourceGet(t).Update(
				requestContext.Get(t).(context.Context),
				entityWithChanges.Get(t),
			)
		}
	)

	ctx.Let(s, func(t *testcase.T) interface{} {
		return c.Context(t)
	})

	requestContext.Let(s, func(t *testcase.T) interface{} {
		return ctx.Get(t)
	})

	s.When(`an entity already stored`, func(s *testcase.Spec) {
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			ent := CreatePTR(factoryGet(t), c.T)
			assert.CreateEntity(t, c.resourceGet(t), ctxGet(t), ent)
			return ent
		}).EagerLoading(s)

		s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) interface{} {
				newEntity := CreatePTR(factoryGet(t), c.T)
				id, _ := extid.Lookup(entity.Get(t))
				require.Nil(t, extid.Set(newEntity, id))
				return newEntity
			})

			s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				assert.HasEntity(t, c.resourceGet(t), c.Context(t), entityWithChanges.Get(t))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				requestContext.Let(s, func(t *testcase.T) interface{} {
					ctx, cancel := context.WithCancel(ctx.Get(t).(context.Context))
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					require.Equal(t, context.Canceled, subject(t))
				})
			})
		})
	})

	s.When(`the received entity has ext.ID that is unknown in the storage`, func(s *testcase.Spec) {
		entityWithChanges.Let(s, func(t *testcase.T) interface{} {
			newEntity := CreatePTR(factoryGet(t), c.T)
			assert.CreateEntity(t, c.resourceGet(t), ctxGet(t), newEntity)
			assert.DeleteEntity(t, c.resourceGet(t), ctxGet(t), newEntity)
			return newEntity
		})

		s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
			require.Error(t, subject(t))
		})
	})
}

func (c Updater) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Updater) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)
	factoryLet(s, c.FixtureFactory)

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newT(c.T)
		assert.CreateEntity(t, c.resourceGet(t), c.Context(t), ptr)
		return ptr
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		require.Nil(b, c.resourceGet(t).Update(c.Context(t), ent.Get(t)))
	})
}
