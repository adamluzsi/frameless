package crudcontracts

import (
	"context"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/pp"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"

	. "go.llib.dev/frameless/ports/crud/crudtest"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

type Deleter[Entity, ID any] func(testing.TB) DeleterSubject[Entity, ID]

type DeleterSubject[Entity, ID any] struct {
	Resource interface {
		spechelper.CRD[Entity, ID]
		crud.AllDeleter
	}
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

func (c Deleter[Entity, ID]) Name() string { return "Deleter" }

func (c Deleter[Entity, ID]) resource() testcase.Var[DeleterSubject[Entity, ID]] {
	return testcase.Var[DeleterSubject[Entity, ID]]{
		ID:   "DeleterSubject[Entity, ID]",
		Init: func(t *testcase.T) DeleterSubject[Entity, ID] { return c(t) },
	}
}

func (c Deleter[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Deleter[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Deleter[Entity, ID]) Spec(s *testcase.Spec) {
	ByIDDeleter[Entity, ID](func(tb testing.TB) ByIDDeleterSubject[Entity, ID] {
		s := c(tb)
		return ByIDDeleterSubject[Entity, ID]{
			Resource:    s.Resource,
			MakeContext: s.MakeContext,
			MakeEntity:  s.MakeEntity,
		}
	}).Spec(s)
	AllDeleter[Entity, ID](func(tb testing.TB) AllDeleterSubject[Entity, ID] {
		sub := c(tb)
		return AllDeleterSubject[Entity, ID]{
			Resource:    sub.Resource,
			MakeContext: sub.MakeContext,
			MakeEntity:  sub.MakeEntity,
		}
	}).Spec(s)
}

type ByIDDeleter[Entity, ID any] func(testing.TB) ByIDDeleterSubject[Entity, ID]

type ByIDDeleterSubject[Entity, ID any] struct {
	Resource    spechelper.CRD[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

func (c ByIDDeleter[Entity, ID]) Name() string { return "ByIDDeleter" }

func (c ByIDDeleter[Entity, ID]) subject() testcase.Var[ByIDDeleterSubject[Entity, ID]] {
	return testcase.Var[ByIDDeleterSubject[Entity, ID]]{
		ID:   "ByIDDeleterSubject[Entity, ID]",
		Init: func(t *testcase.T) ByIDDeleterSubject[Entity, ID] { return c(t) },
	}
}

func (c ByIDDeleter[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ByIDDeleter[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c ByIDDeleter[Entity, ID]) Spec(s *testcase.Spec) {
	c.subject().Bind(s)
	s.Describe(`DeleteByID`, c.specDeleteByID)
}

func (c ByIDDeleter[Entity, ID]) specDeleteByID(s *testcase.Spec) {
	var (
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.subject().Get(t).MakeContext()
		})
		id      = testcase.Var[ID]{ID: `id`}
		subject = func(t *testcase.T) error {
			return c.subject().Get(t).Resource.DeleteByID(Context.Get(t).(context.Context), id.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(), c.subject().Get(t))
	})

	entity := testcase.Let(s, func(t *testcase.T) *Entity {
		return pointer.Of(c.subject().Get(t).MakeEntity())
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := entity.Get(t)
			Create[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), ent)
			id, ok := extid.Lookup[ID](ent)
			t.Must.True(ok, assert.Message(pp.Format(spechelper.ErrIDRequired.Error())))
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsAbsent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.subject().Get(t).MakeContext())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				t.Must.ErrorIs(context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := testcase.Let(s, func(t *testcase.T) *Entity {
				ent := c.subject().Get(t).MakeEntity()
				Create[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), &ent)
				return &ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				t.Must.Nil(subject(t))
				othID, _ := extid.Lookup[ID](othEntity.Get(t))
				IsPresent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(subject(t))
				IsAbsent[Entity, ID](t, c.subject().Get(t).Resource, Context.Get(t).(context.Context), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				t.Must.ErrorIs(crud.ErrNotFound, subject(t))
			})
		})
	})
}

func (c ByIDDeleter[Entity, ID]) benchmarkDeleteByID(b *testing.B) {
	s := testcase.NewSpec(b)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(), c.subject().Get(t))
		t.Defer(spechelper.TryCleanup, t, c.subject().Get(t).MakeContext(), c.subject().Get(t))
	})

	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		e := c.subject().Get(t).MakeEntity()
		Create[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), &e)
		return &e
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Entity, ID](t, pointer.Deref(ent.Get(t)))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		t.Must.Nil(c.subject().Get(t).Resource.DeleteByID(c.subject().Get(t).MakeContext(), id.Get(t)))
	})
}

type AllDeleter[Entity, ID any] func(testing.TB) AllDeleterSubject[Entity, ID]

type AllDeleterSubject[Entity, ID any] struct {
	Resource    allDeleterSubjectResource[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

type allDeleterSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
}

func (c AllDeleter[Entity, ID]) Name() string { return "AllDeleter" }

func (c AllDeleter[Entity, ID]) subject() testcase.Var[AllDeleterSubject[Entity, ID]] {
	return testcase.Var[AllDeleterSubject[Entity, ID]]{
		ID:   "AllDeleterSubject[Entity, ID]",
		Init: func(t *testcase.T) AllDeleterSubject[Entity, ID] { return c(t) },
	}
}

func (c AllDeleter[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c AllDeleter[Entity, ID]) Benchmark(b *testing.B) {
	b.Run(`DeleteAll`, c.benchmarkDeleteAll)
}

func (c AllDeleter[Entity, ID]) Spec(s *testcase.Spec) {
	s.Describe(`DeleteAll`, c.specDeleteAll)
}

func (c AllDeleter[Entity, ID]) specDeleteAll(s *testcase.Spec) {
	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return c.subject().Get(t).MakeContext() })
	)
	act := func(t *testcase.T) error {
		return c.subject().Get(t).Resource.DeleteAll(ctx.Get(t))
	}

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.subject().Get(t).MakeContext())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := c.subject().Get(t).MakeEntity()
		Create[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), &ent)
		entID := HasID[Entity, ID](t, ent)
		IsPresent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), entID)
		t.Must.Nil(act(t))
		IsAbsent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), entID)
	})
}

func (c AllDeleter[Entity, ID]) benchmarkDeleteAll(b *testing.B) {
	sub := c(testcase.ToT(b))
	ctx := sub.MakeContext()
	spechelper.TryCleanup(b, ctx, sub)
	defer spechelper.TryCleanup(b, ctx, sub)
	// for some reason, doing setup with timer stop/start
	// makes this test unable to measure
	// the correct throughput, and hangs forever,
	// so I just check empty delete all.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sub.Resource.DeleteAll(ctx)
	}
	b.StopTimer()
}
