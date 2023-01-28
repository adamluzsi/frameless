package crudcontracts

import (
	"context"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

type Deleter[Entity, ID any] struct {
	MakeSubject func(testing.TB) DeleterSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type DeleterSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.AllDeleter
}

func (c Deleter[Entity, ID]) resource() testcase.Var[DeleterSubject[Entity, ID]] {
	return testcase.Var[DeleterSubject[Entity, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) DeleterSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c Deleter[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Deleter[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Deleter[Entity, ID]) Spec(s *testcase.Spec) {
	ByIDDeleter[Entity, ID]{
		MakeSubject: func(tb testing.TB) ByIDDeleterSubject[Entity, ID] {
			return c.MakeSubject(tb)
		},
		MakeContext: c.MakeContext,
		MakeEntity:  c.MakeEntity,
	}.Spec(s)

	AllDeleter[Entity, ID]{
		MakeSubject: func(tb testing.TB) AllDeleterSubject[Entity, ID] {
			return c.MakeSubject(tb)
		},
		MakeContext: c.MakeContext,
		MakeEntity:  c.MakeEntity,
	}.Spec(s)
}

type ByIDDeleter[Entity, ID any] struct {
	MakeSubject func(testing.TB) ByIDDeleterSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type ByIDDeleterSubject[Entity, ID any] spechelper.CRD[Entity, ID]

func (c ByIDDeleter[Entity, ID]) resource() testcase.Var[ByIDDeleterSubject[Entity, ID]] {
	return testcase.Var[ByIDDeleterSubject[Entity, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) ByIDDeleterSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c ByIDDeleter[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ByIDDeleter[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c ByIDDeleter[Entity, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	s.Describe(`DeleteByID`, c.specDeleteByID)
}

func (c ByIDDeleter[Entity, ID]) specDeleteByID(s *testcase.Spec) {
	var (
		ctxVar = spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		id      = testcase.Var[ID]{ID: `id`}
		subject = func(t *testcase.T) error {
			return c.resource().Get(t).DeleteByID(ctxVar.Get(t).(context.Context), id.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), c.resource().Get(t))
	})

	entity := testcase.Let(s, func(t *testcase.T) *Entity {
		v := c.MakeEntity(t)
		return &v
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := entity.Get(t)
			Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), ent)
			id, ok := extid.Lookup[ID](ent)
			t.Must.True(ok, spechelper.ErrIDRequired.Error())
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctxVar.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				t.Must.Equal(context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := testcase.Let(s, func(t *testcase.T) *Entity {
				ent := c.MakeEntity(t)
				Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), &ent)
				return &ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				t.Must.Nil(subject(t))
				othID, _ := extid.Lookup[ID](othEntity.Get(t))
				IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(subject(t))
				IsAbsent[Entity, ID](t, c.resource().Get(t), ctxVar.Get(t).(context.Context), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				t.Must.ErrorIs(crud.ErrNotFound, subject(t))
			})
		})
	})
}

func (c ByIDDeleter[Entity, ID]) benchmarkDeleteByID(b *testing.B) {
	s := testcase.NewSpec(b)

	s.Around(func(t *testcase.T) func() {
		spechelper.TryCleanup(t, c.MakeContext(b), c.resource().Get(t))
		return func() { spechelper.TryCleanup(t, c.MakeContext(b), c.resource().Get(t)) }
	})

	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		e := c.MakeEntity(t)
		Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), &e)
		return &e
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Entity, ID](t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		t.Must.Nil(c.resource().Get(t).DeleteByID(c.MakeContext(t), id.Get(t)))
	})
}

type AllDeleter[Entity, ID any] struct {
	MakeSubject func(testing.TB) AllDeleterSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type AllDeleterSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.AllDeleter
}

func (c AllDeleter[Entity, ID]) resource() testcase.Var[AllDeleterSubject[Entity, ID]] {
	return testcase.Var[AllDeleterSubject[Entity, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) AllDeleterSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
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
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return c.MakeContext(t) })
	)
	act := func(t *testcase.T) error {
		return c.resource().Get(t).DeleteAll(ctx.Get(t))
	}

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.Equal(context.Canceled, act(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := c.MakeEntity(t)
		Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), &ent)
		entID := HasID[Entity, ID](t, &ent)
		IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), entID)
		t.Must.Nil(act(t))
		IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), entID)
	})
}

func (c AllDeleter[Entity, ID]) benchmarkDeleteAll(b *testing.B) {
	r := c.MakeSubject(b)
	ctx := c.MakeContext(b)
	spechelper.TryCleanup(b, ctx, r)
	defer spechelper.TryCleanup(b, ctx, r)
	// for some reason, doing setup with timer stop/start
	// makes this test unable to measure
	// the correct throughput, and hangs forever
	// so I just check empty delete all.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.DeleteAll(ctx)
	}
	b.StopTimer()
}
