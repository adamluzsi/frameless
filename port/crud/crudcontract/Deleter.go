package crudcontract

import (
	"context"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/port/crud"
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

func ByIDDeleter[ENT, ID any](subject crud.ByIDDeleter[ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		Context = let.With[context.Context](s, c.MakeContext)
		id      = testcase.Var[ID]{ID: `id`}
	)
	act := func(t *testcase.T) error {
		return subject.DeleteByID(Context.Get(t), id.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), act)
	})

	ptr := testcase.Let(s, func(t *testcase.T) *ENT {
		return pointer.Of(c.MakeEntity(t))
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			p := ptr.Get(t)
			shouldStore[ENT, ID](t, c, subject, p)
			// id, ok := lookupID[ID](c, *p)
			// t.Must.True(ok, assert.Message(pp.Format(spechelper.ErrIDRequired.Error())))
			return c.IDA.Get(*p)
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			t.Must.NoError(act(t))

			shouldAbsent(t, c, subject, c.MakeContext(t), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				t.Must.ErrorIs(context.Canceled, act(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntPtr := testcase.Let(s, func(t *testcase.T) *ENT {
				ent := c.MakeEntity(t)
				shouldStore(t, c, subject, &ent)

				return &ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				t.Must.NoError(act(t))
				othID, _ := lookupID[ID](c, *othEntPtr.Get(t))
				shouldPresent(t, c, subject, c.MakeContext(t), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.NoError(act(t))
				shouldAbsent(t, c, subject, Context.Get(t), id.Get(t))
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

func AllDeleter[ENT, ID any](subject allDeleterSubjectResource[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return c.MakeContext(t) })
	)
	act := func(t *testcase.T) error {
		return subject.DeleteAll(ctx.Get(t))
	}

	s.Benchmark("", func(t *testcase.T) {
		assert.NoError(t, subject.DeleteAll(c.MakeContext(t)))
	})

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := c.MakeEntity(t)
		c.Helper().Create(t, subject, c.MakeContext(t), &ent)
		entID := c.Helper().HasID(t, &ent)
		crudtest.IsPresent[ENT, ID](t, subject, c.MakeContext(t), entID)
		t.Must.NoError(act(t))
		crudtest.IsAbsent[ENT, ID](t, subject, c.MakeContext(t), entID)
	})

	return s.AsSuite("AllDeleter")
}
