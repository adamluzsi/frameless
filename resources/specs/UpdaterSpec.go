package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

// Updater will request an update for a wrapped entity object in the Resource
type Updater struct {
	T interface{}
	FixtureFactory
	Subject interface {
		resources.Updater
		minimumRequirements
	}
}

func (spec Updater) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		DeleteAllEntity(t, spec.Subject, spec.Context(), spec.T)
	})

	s.Describe(`Updater`, func(s *testcase.Spec) {
		var (
			requestContext    = testcase.Var{Name: `request-context`}
			entityWithChanges = testcase.Var{Name: `entity-with-changes`}
			subject           = func(t *testcase.T) error {
				return spec.Subject.Update(
					requestContext.Get(t).(context.Context),
					entityWithChanges.Get(t),
				)
			}
		)

		ctx.Let(s, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		requestContext.Let(s, func(t *testcase.T) interface{} {
			return ctx.Get(t)
		})

		s.When(`an entity already stored`, func(s *testcase.Spec) {
			entity := s.Let(`entity`, func(t *testcase.T) interface{} {
				ent := spec.FixtureFactory.Create(spec.T)
				CreateEntity(t, spec.Subject, ctxGet(t), ent)
				return ent
			}).EagerLoading(s)

			s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
				entityWithChanges.Let(s, func(t *testcase.T) interface{} {
					newEntity := spec.FixtureFactory.Create(spec.T)
					id, _ := resources.LookupID(entity.Get(t))
					require.Nil(t, resources.SetID(newEntity, id))
					return newEntity
				})

				s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
					require.Nil(t, subject(t))

					HasEntity(t, spec.Subject, spec.Context(), entityWithChanges.Get(t))
				})

				s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
					requestContext.Let(s, func(t *testcase.T) interface{} {
						ctx, cancel := context.WithCancel(ctx.Get(t).(context.Context))
						cancel()
						return ctx
					})

					s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
						require.Equal(t, context.Canceled, subject(t))
					})
				})
			})
		})

		s.When(`the received entity has ext.ID that is unknown in the storage`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) interface{} {
				newEntity := spec.FixtureFactory.Create(spec.T)
				CreateEntity(t, spec.Subject, ctxGet(t), newEntity)
				DeleteEntity(t, spec.Subject, ctxGet(t), newEntity)
				return newEntity
			})

			s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

	})
}

func (spec Updater) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newEntity(spec.T)
		CreateEntity(t, spec.Subject, spec.Context(), ptr)
		return ptr
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		require.Nil(b, spec.Subject.Update(spec.Context(), ent.Get(t)))
	})
}
