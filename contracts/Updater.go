package contracts

import (
	"context"
	"testing"

	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"
)

// Updater will request an update for a wrapped entity object in the Resource
type Updater[Ent any, ID any] struct {
	Subject func(testing.TB) UpdaterSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type UpdaterSubject[Ent any, ID any] interface {
	CRD[Ent, ID]
	frameless.Updater[Ent]
}

func (c Updater[Ent, ID]) resource() testcase.Var[UpdaterSubject[Ent, ID]] {
	return testcase.Var[UpdaterSubject[Ent, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) UpdaterSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c Updater[Ent, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)

	s.Before(func(t *testcase.T) {
		DeleteAll[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t))
	})

	var (
		requestContext    = testcase.Var[context.Context]{ID: `request-Context`}
		entityWithChanges = testcase.Var[*Ent]{ID: `entity-with-changes`}
		subject           = func(t *testcase.T) error {
			return c.resource().Get(t).Update(
				requestContext.Get(t),
				entityWithChanges.Get(t),
			)
		}
	)

	ctxVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeCtx(t)
	})

	requestContext.Let(s, func(t *testcase.T) context.Context {
		return ctxVar.Get(t)
	})

	s.When(`an entity already stored`, func(s *testcase.Spec) {
		entity := testcase.Let(s, func(t *testcase.T) *Ent {
			ent := toPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), ctxVar.Get(t), ent)
			return ent
		}).EagerLoading(s)

		s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) *Ent {
				newEntity := toPtr(c.MakeEnt(t))
				id, _ := extid.Lookup[ID](entity.Get(t))
				assert.Must(t).Nil(extid.Set(newEntity, id))
				return newEntity
			})

			s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				HasEntity[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), entityWithChanges.Get(t))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				requestContext.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(ctxVar.Get(t))
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					assert.Must(t).Equal(context.Canceled, subject(t))
				})
			})
		})
	})

	s.When(`the received entity has ext.ID that is unknown in the storage`, func(s *testcase.Spec) {
		entityWithChanges.Let(s, func(t *testcase.T) *Ent {
			newEntity := toPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), ctxVar.Get(t), newEntity)
			Delete[Ent, ID](t, c.resource().Get(t), ctxVar.Get(t), newEntity)
			return newEntity
		})

		s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
			assert.Must(t).NotNil(subject(t))
		})
	})
}

func (c Updater[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Updater[Ent, ID]) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)

	ent := testcase.Let(s, func(t *testcase.T) *Ent {
		ptr := toPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), ptr)
		return ptr
	})

	s.Test(``, func(t *testcase.T) {
		assert.Must(b).Nil(c.resource().Get(t).Update(c.MakeCtx(t), ent.Get(t)))
	})
}
