package crudcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

type Creator[Entity, ID any] struct {
	MakeSubject func(testing.TB) CreatorSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	// SupportIDReuse is an optional configuration value that tells the contract
	// that recreating an entity with an ID which belongs to a previously deleted entity is accepted.
	SupportIDReuse bool
	// SupportRecreate is an optional configuration value that tells the contract
	// that deleting an Entity then recreating it with the same values is supported by the Creator.
	SupportRecreate bool
}

type CreatorSubject[Entity, ID any] spechelper.CRD[Entity, ID]

func (c Creator[T, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Creator[Entity, ID]) Benchmark(b *testing.B) {
	resource := c.MakeSubject(b)
	spechelper.TryCleanup(b, c.MakeContext(b), resource)
	b.Run(`Creator`, func(b *testing.B) {
		var (
			ctx = c.MakeContext(b)
			es  []*Entity
		)
		for i := 0; i < b.N; i++ {
			ent := c.MakeEntity(b)
			es = append(es, &ent)
		}
		defer spechelper.TryCleanup(b, c.MakeContext(b), resource)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = resource.Create(ctx, es[i])
		}
		b.StopTimer()
	})
}

func (c Creator[Entity, ID]) Spec(s *testcase.Spec) {
	var (
		resource = testcase.Let(s, func(t *testcase.T) CreatorSubject[Entity, ID] {
			return c.MakeSubject(t)
		})
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Entity {
			v := c.MakeEntity(t)
			return &v
		})
		getID = func(t *testcase.T) ID {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			return id
		}
	)
	act := func(t *testcase.T) error {
		ctx := ctxVar.Get(t)
		err := resource.Get(t).Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			t.Defer(resource.Get(t).DeleteByID, ctx, id)
			IsFindable[Entity, ID](t, resource.Get(t), ctx, id)
		}
		return err
	}

	s.When(`entity was not saved before`, func(s *testcase.Spec) {
		s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			t.Must.NotEmpty(getID(t))
		})

		s.Then(`entity could be retrieved by ID`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			t.Must.Equal(ptr.Get(t), IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t)))
		})
	})

	s.When(`entity was already saved once`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.Nil(act(t))
			IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
		})

		s.Then(`it will return an error informing that the entity record already exists`, func(t *testcase.T) {
			t.Must.ErrorIs(crud.ErrAlreadyExists, act(t))
		})
	})

	if c.SupportIDReuse {
		s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(act(t))
				IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
				t.Must.Nil(resource.Get(t).DeleteByID(c.MakeContext(t), getID(t)))
				IsAbsent[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			s.Then(`persisted object can be found`, func(t *testcase.T) {
				t.Must.Nil(act(t))
				IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
			})
		})
	}

	if c.SupportRecreate {
		s.When(`entity is already created and then remove before`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ogEnt := *ptr.Get(t) // a deep copy might be better
				t.Must.Nil(act(t))
				IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
				t.Must.Nil(resource.Get(t).DeleteByID(c.MakeContext(t), getID(t)))
				IsAbsent[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
				ptr.Set(t, &ogEnt)
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			s.Then(`persisted object can be found`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), getID(t))
			})
		})
	}

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	s.Test(`persist on #Create`, func(t *testcase.T) {
		e := c.MakeEntity(t)

		err := resource.Get(t).Create(c.MakeContext(t), &e)
		t.Must.Nil(err)

		id, ok := extid.Lookup[ID](&e)
		t.Must.True(ok, "ID is not defined in the entity struct src definition")
		t.Must.NotEmpty(id, "it's expected that repository set the external ID in the entity")

		t.Must.Equal(e, *IsFindable[Entity, ID](t, resource.Get(t), c.MakeContext(t), id))
		t.Must.Nil(resource.Get(t).DeleteByID(c.MakeContext(t), id))
	})
}
