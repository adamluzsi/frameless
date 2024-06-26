package crudcontracts

import (
	"context"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/ports/contract"
	crudtest "go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/option"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
)

type deleterSubject[Entity, ID any] interface {
	crd[Entity, ID]
	allDeleterSubjectResource[Entity, ID]
}

func Deleter[Entity, ID any](subject deleterSubject[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe("DeleteByID", ByIDDeleter[Entity, ID](subject, opts...).Spec)
	s.Describe("DeleteAll", AllDeleter[Entity, ID](subject, opts...).Spec)
	return s.AsSuite("Deleter")
}

func ByIDDeleter[Entity, ID any](subject crd[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		Context = let.With[context.Context](s, c.MakeContext)
		id      = testcase.Var[ID]{ID: `id`}
	)
	act := func(t *testcase.T) error {
		return subject.DeleteByID(Context.Get(t), id.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(), act)
	})

	entity := testcase.Let(s, func(t *testcase.T) *Entity {
		return pointer.Of(c.MakeEntity(t))
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := entity.Get(t)
			crudtest.Create[Entity, ID](t, subject, c.MakeContext(), ent)
			id, ok := extid.Lookup[ID](ent)
			t.Must.True(ok, assert.Message(pp.Format(spechelper.ErrIDRequired.Error())))
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				t.Must.ErrorIs(context.Canceled, act(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := testcase.Let(s, func(t *testcase.T) *Entity {
				ent := c.MakeEntity(t)
				crudtest.Create[Entity, ID](t, subject, c.MakeContext(), &ent)
				return &ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				t.Must.Nil(act(t))
				othID, _ := extid.Lookup[ID](othEntity.Get(t))
				crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(act(t))
				crudtest.IsAbsent[Entity, ID](t, subject, Context.Get(t), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				t.Must.ErrorIs(crud.ErrNotFound, act(t))
			})
		})
	})

	return s.AsSuite("ByIDDeleter")
}

type allDeleterSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
}

func AllDeleter[Entity, ID any](subject allDeleterSubjectResource[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	conf := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return conf.MakeContext() })
	)
	act := func(t *testcase.T) error {
		return subject.DeleteAll(ctx.Get(t))
	}

	s.Benchmark("", func(t *testcase.T) {
		assert.NoError(t, subject.DeleteAll(conf.MakeContext()))
	})

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(conf.MakeContext())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := conf.MakeEntity(t)
		crudtest.Create[Entity, ID](t, subject, conf.MakeContext(), &ent)
		entID := crudtest.HasID[Entity, ID](t, ent)
		crudtest.IsPresent[Entity, ID](t, subject, conf.MakeContext(), entID)
		t.Must.Nil(act(t))
		crudtest.IsAbsent[Entity, ID](t, subject, conf.MakeContext(), entID)
	})

	return s.AsSuite("AllDeleter")
}
