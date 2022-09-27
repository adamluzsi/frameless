package crudcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/testcase"
)

type Creator[Ent any, ID any] struct {
	Subject func(testing.TB) CreatorSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type CreatorSubject[Ent, ID any] spechelper.CRD[Ent, ID]

func (c Creator[T, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Creator[Ent, ID]) Benchmark(b *testing.B) {
	resource := c.Subject(b)
	spechelper.TryCleanup(b, c.MakeCtx(b), resource)
	b.Run(`Creator`, func(b *testing.B) {
		var (
			ctx = c.MakeCtx(b)
			es  []*Ent
		)
		for i := 0; i < b.N; i++ {
			ent := c.MakeEnt(b)
			es = append(es, &ent)
		}
		defer spechelper.TryCleanup(b, c.MakeCtx(b), resource)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = resource.Create(ctx, es[i])
		}
		b.StopTimer()
	})
}

func (c Creator[Ent, ID]) Spec(s *testcase.Spec) {
	var (
		resource = testcase.Let(s, func(t *testcase.T) CreatorSubject[Ent, ID] {
			return c.Subject(t)
		})
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeCtx(t)
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Ent {
			v := c.MakeEnt(t)
			return &v
		})
		getID = func(t *testcase.T) ID {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			return id
		}
	)
	subject := func(t *testcase.T) error {
		ctx := ctxVar.Get(t)
		err := resource.Get(t).Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			t.Defer(resource.Get(t).DeleteByID, ctx, id)
			IsFindable[Ent, ID](t, resource.Get(t), ctx, id)
		}
		return err
	}

	s.When(`entity was not saved before`, func(s *testcase.Spec) {
		s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			t.Must.NotEmpty(getID(t))
		})

		s.Then(`entity could be retrieved by ID`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			t.Must.Equal(ptr.Get(t), IsFindable[Ent, ID](t, resource.Get(t), c.MakeCtx(t), getID(t)))
		})
	})

	s.When(`entity was already saved once`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsFindable[Ent, ID](t, resource.Get(t), c.MakeCtx(t), getID(t))
		})

		s.Then(`it will raise error because ext:ID field already points to a existing record`, func(t *testcase.T) {
			t.Must.NotNil(subject(t))
		})
	})

	s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsFindable[Ent, ID](t, resource.Get(t), c.MakeCtx(t), getID(t))
			t.Must.Nil(resource.Get(t).DeleteByID(c.MakeCtx(t), getID(t)))
			IsAbsent[Ent, ID](t, resource.Get(t), c.MakeCtx(t), getID(t))
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			IsFindable[Ent, ID](t, resource.Get(t), c.MakeCtx(t), getID(t))
		})
	})

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeCtx(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.Equal(context.Canceled, subject(t))
		})
	})

	s.Test(`persist on #Create`, func(t *testcase.T) {
		e := c.MakeEnt(t)

		err := resource.Get(t).Create(c.MakeCtx(t), &e)
		t.Must.Nil(err)

		id, ok := extid.Lookup[ID](&e)
		t.Must.True(ok, "ID is not defined in the entity struct src definition")
		t.Must.NotEmpty(id, "it's expected that repository set the external ID in the entity")

		t.Must.Equal(e, *IsFindable[Ent, ID](t, resource.Get(t), c.MakeCtx(t), id))
		t.Must.Nil(resource.Get(t).DeleteByID(c.MakeCtx(t), id))
	})
}
