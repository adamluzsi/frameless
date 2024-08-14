package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

// Updater will request an update for a wrapped entity object in the Resource
func Updater[Entity, ID any](ssubject subjectUpdater[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(), ssubject)
	})

	var (
		requestContext = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext()
		})
		entityWithChanges = testcase.Var[*Entity]{ID: `entity-with-changes`}
	)
	subject := func(t *testcase.T) error {
		return ssubject.Update(
			requestContext.Get(t),
			entityWithChanges.Get(t),
		)
	}

	updaterBenchmark[Entity, ID](s, ssubject, c)

	s.When(`an entity already stored`, func(s *testcase.Spec) {
		entity := testcase.Let(s, func(t *testcase.T) *Entity {
			ent := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, ssubject, c.MakeContext(), ent)
			return ent
		}).EagerLoading(s)

		s.And(`the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) *Entity {
				id, ok := extid.Lookup[ID](entity.Get(t))
				t.Must.True(ok)
				var ent = entity.Get(t)
				if chEnt := c.ChangeEntity; chEnt != nil {
					chEnt(t, ent)
				} else {
					ent = pointer.Of(c.MakeEntity(t))
				}
				assert.Must(t).Nil(extid.Set(ent, id))
				return ent
			})

			s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				crudtest.HasEntity[Entity, ID](t, ssubject, c.MakeContext(), entityWithChanges.Get(t))
			})

			s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
				requestContext.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(c.MakeContext())
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					assert.Must(t).ErrorIs(context.Canceled, subject(t))
				})
			})
		})
	})

	s.When(`the received entity has ext.ID that is unknown in the repository`, func(s *testcase.Spec) {
		entityWithChanges.Let(s, func(t *testcase.T) *Entity {
			newEntity := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, ssubject, c.MakeContext(), newEntity)
			crudtest.Delete[Entity, ID](t, ssubject, c.MakeContext(), newEntity)
			return newEntity
		})

		s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
			t.Must.ErrorIs(crud.ErrNotFound, subject(t))
		})
	})

	return s.AsSuite("Updater")
}

type subjectUpdater[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.Updater[Entity]
}

func updaterBenchmark[Entity, ID any](s *testcase.Spec, subject subjectUpdater[Entity, ID], c Config[Entity, ID]) {
	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		ptr := pointer.Of(c.MakeEntity(t))
		crudtest.Create[Entity, ID](t, subject, c.MakeContext(), ptr)
		return ptr
	})

	s.Benchmark("", func(t *testcase.T) {
		t.Must.Nil(subject.Update(c.MakeContext(), ent.Get(t)))
	})
}
