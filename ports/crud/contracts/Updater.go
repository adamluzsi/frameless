package crudcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

// Updater will request an update for a wrapped entity object in the Resource
type Updater[Entity, ID any] struct {
	MakeSubject func(testing.TB) UpdaterSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity

	// ChangeEntity is an optional configuration field
	// to express what Entity fields are allowed to be changed by the user of the Updater.
	// For example, if the changed  Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(testing.TB, *Entity)
}

type UpdaterSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.Updater[Entity]
}

func (c Updater[Entity, ID]) resource() testcase.Var[UpdaterSubject[Entity, ID]] {
	return testcase.Var[UpdaterSubject[Entity, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) UpdaterSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c Updater[Entity, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), c.resource().Get(t))
	})

	var (
		requestContext    = testcase.Var[context.Context]{ID: `request-Context`}
		entityWithChanges = testcase.Var[*Entity]{ID: `entity-with-changes`}
		subject           = func(t *testcase.T) error {
			return c.resource().Get(t).Update(
				requestContext.Get(t),
				entityWithChanges.Get(t),
			)
		}
	)

	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeContext(t)
	})

	requestContext.Let(s, func(t *testcase.T) context.Context {
		return spechelper.ContextVar.Get(t)
	})

	s.When(`an entity already stored`, func(s *testcase.Spec) {
		entity := testcase.Let(s, func(t *testcase.T) *Entity {
			ent := spechelper.ToPtr(c.MakeEntity(t))
			Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ent)
			return ent
		}).EagerLoading(s)

		s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) *Entity {
				id, ok := extid.Lookup[ID](entity.Get(t))
				t.Must.True(ok)
				var ent = entity.Get(t)
				if c.ChangeEntity != nil {
					c.ChangeEntity(t, ent)
				} else {
					ent = spechelper.ToPtr(c.MakeEntity(t))
				}
				assert.Must(t).Nil(extid.Set(ent, id))
				return ent
			})

			s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				HasEntity[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), entityWithChanges.Get(t))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				requestContext.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(spechelper.ContextVar.Get(t))
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					assert.Must(t).Equal(context.Canceled, subject(t))
				})
			})
		})
	})

	s.When(`the received entity has ext.ID that is unknown in the repository`, func(s *testcase.Spec) {
		entityWithChanges.Let(s, func(t *testcase.T) *Entity {
			newEntity := spechelper.ToPtr(c.MakeEntity(t))
			Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), newEntity)
			Delete[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), newEntity)
			return newEntity
		})

		s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
			t.Must.ErrorIs(crud.ErrNotFound, subject(t))
		})
	})
}

func (c Updater[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Updater[Entity, ID]) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)

	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		ptr := spechelper.ToPtr(c.MakeEntity(t))
		Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), ptr)
		return ptr
	})

	s.Test(``, func(t *testcase.T) {
		assert.Must(b).Nil(c.resource().Get(t).Update(c.MakeContext(t), ent.Get(t)))
	})
}
