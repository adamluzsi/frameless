package crudcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/testcase"
)

type Deleter[Ent, ID any] struct {
	Subject func(testing.TB) DeleterSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type DeleterSubject[Ent, ID any] interface {
	AllDeleterSubject[Ent, ID]
	ByIDDeleterSubject[Ent, ID]
}

func (c Deleter[Ent, ID]) resource() testcase.Var[DeleterSubject[Ent, ID]] {
	return testcase.Var[DeleterSubject[Ent, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) DeleterSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c Deleter[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Deleter[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Deleter[Ent, ID]) Spec(s *testcase.Spec) {
	ByIDDeleter[Ent, ID]{
		Subject: func(tb testing.TB) ByIDDeleterSubject[Ent, ID] {
			return c.Subject(tb)
		},
		MakeCtx: c.MakeCtx,
		MakeEnt: c.MakeEnt,
	}.Spec(s)
	
	AllDeleter[Ent, ID]{
		Subject: func(tb testing.TB) AllDeleterSubject[Ent, ID] {
			return c.Subject(tb)
		},
		MakeCtx: c.MakeCtx,
		MakeEnt: c.MakeEnt,
	}.Spec(s)
}

type ByIDDeleter[Ent, ID any] struct {
	Subject func(testing.TB) ByIDDeleterSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type ByIDDeleterSubject[Ent, ID any] spechelper.CRD[Ent, ID]

func (c ByIDDeleter[Ent, ID]) resource() testcase.Var[ByIDDeleterSubject[Ent, ID]] {
	return testcase.Var[ByIDDeleterSubject[Ent, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) ByIDDeleterSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c ByIDDeleter[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ByIDDeleter[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c ByIDDeleter[Ent, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	s.Describe(`DeleteByID`, c.specDeleteByID)
}

func (c ByIDDeleter[Ent, ID]) specDeleteByID(s *testcase.Spec) {
	var (
		ctxVar = spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			return c.MakeCtx(t)
		})
		id      = testcase.Var[ID]{ID: `id`}
		subject = func(t *testcase.T) error {
			return c.resource().Get(t).DeleteByID(ctxVar.Get(t).(context.Context), id.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeCtx(t), c.resource().Get(t))
	})

	entity := testcase.Let(s, func(t *testcase.T) *Ent {
		v := c.MakeEnt(t)
		return &v
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) ID {
			ent := entity.Get(t)
			Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), ent)
			id, ok := extid.Lookup[ID](ent)
			t.Must.True(ok, spechelper.ErrIDRequired.Error())
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctxVar.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeCtx(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				t.Must.Equal(context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := testcase.Let(s, func(t *testcase.T) *Ent {
				ent := c.MakeEnt(t)
				Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), &ent)
				return &ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				t.Must.Nil(subject(t))
				othID, _ := extid.Lookup[ID](othEntity.Get(t))
				IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), othID)
			})
		})

		s.And(`the entity was deleted`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(subject(t))
				IsAbsent[Ent, ID](t, c.resource().Get(t), ctxVar.Get(t).(context.Context), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				t.Must.NotNil(subject(t))
			})
		})
	})
}

func (c ByIDDeleter[Ent, ID]) benchmarkDeleteByID(b *testing.B) {
	s := testcase.NewSpec(b)

	s.Around(func(t *testcase.T) func() {
		spechelper.TryCleanup(t, c.MakeCtx(b), c.resource().Get(t))
		return func() { spechelper.TryCleanup(t, c.MakeCtx(b), c.resource().Get(t)) }
	})

	ent := testcase.Let(s, func(t *testcase.T) *Ent {
		e := c.MakeEnt(t)
		Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), &e)
		return &e
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Ent, ID](t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		t.Must.Nil(c.resource().Get(t).DeleteByID(c.MakeCtx(t), id.Get(t)))
	})
}

type AllDeleter[Ent, ID any] struct {
	Subject func(testing.TB) AllDeleterSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type AllDeleterSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	crud.AllDeleter
}

func (c AllDeleter[Ent, ID]) resource() testcase.Var[AllDeleterSubject[Ent, ID]] {
	return testcase.Var[AllDeleterSubject[Ent, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) AllDeleterSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c AllDeleter[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c AllDeleter[Ent, ID]) Benchmark(b *testing.B) {
	b.Run(`DeleteAll`, c.benchmarkDeleteAll)
}

func (c AllDeleter[Ent, ID]) Spec(s *testcase.Spec) {
	s.Describe(`DeleteAll`, c.specDeleteAll)
}

func (c AllDeleter[Ent, ID]) specDeleteAll(s *testcase.Spec) {
	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return c.MakeCtx(t) })
	)
	act := func(t *testcase.T) error {
		return c.resource().Get(t).DeleteAll(ctx.Get(t))
	}

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeCtx(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.Equal(context.Canceled, act(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := c.MakeEnt(t)
		Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), &ent)
		entID := HasID[Ent, ID](t, &ent)
		IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), entID)
		t.Must.Nil(act(t))
		IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), entID)
	})
}

func (c AllDeleter[Ent, ID]) benchmarkDeleteAll(b *testing.B) {
	r := c.Subject(b)
	ctx := c.MakeCtx(b)
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
