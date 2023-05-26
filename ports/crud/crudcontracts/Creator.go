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

type Creator[Entity, ID any] func(tb testing.TB) CreatorSubject[Entity, ID]

type CreatorSubject[Entity, ID any] struct {
	Resource    spechelper.CRD[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity

	// SupportIDReuse is an optional configuration value that tells the contract
	// that recreating an entity with an ID which belongs to a previously deleted entity is accepted.
	SupportIDReuse bool
	// SupportRecreate is an optional configuration value that tells the contract
	// that deleting an Entity then recreating it with the same values is supported by the Creator.
	SupportRecreate bool

	forSaverSuite bool
}

func (c Creator[Entity, ID]) Name() string {
	return "Creator"
}

func (c Creator[Entity, ID]) subject() testcase.Var[CreatorSubject[Entity, ID]] {
	return testcase.Var[CreatorSubject[Entity, ID]]{
		ID: "CreatorSubject[Entity, ID]",
		Init: func(t *testcase.T) CreatorSubject[Entity, ID] {
			return c(t)
		},
	}
}

func (c Creator[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Creator[Entity, ID]) Benchmark(b *testing.B) {
	subject := c(testcase.ToT(b))

	spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)
	b.Run(`Creator`, func(b *testing.B) {
		var (
			ctx = subject.MakeContext()
			es  []*Entity
		)
		for i := 0; i < b.N; i++ {
			ent := subject.MakeEntity()
			es = append(es, &ent)
		}
		defer spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = subject.Resource.Create(ctx, es[i])
		}
		b.StopTimer()
	})
}

func (c Creator[Entity, ID]) Spec(s *testcase.Spec) {
	var (
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.subject().Get(t).MakeContext()
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Entity {
			v := c.subject().Get(t).MakeEntity()
			return &v
		})
		getID = func(t *testcase.T) ID {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			return id
		}
	)
	act := func(t *testcase.T) error {
		ctx := ctxVar.Get(t)
		err := c.subject().Get(t).Resource.Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := extid.Lookup[ID](ptr.Get(t))
			t.Defer(c.subject().Get(t).Resource.DeleteByID, ctx, id)
			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, ctx, id)
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
			t.Must.Equal(ptr.Get(t), IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t)))
		})
	})

	s.When(`entity was already saved once`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if c.subject().Get(t).forSaverSuite {
				t.Skip()
			}
			t.Must.Nil(act(t))
			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
		})

		s.Then(`it will return an error informing that the entity record already exists`, func(t *testcase.T) {
			t.Must.ErrorIs(crud.ErrAlreadyExists, act(t))
		})
	})

	s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !c.subject().Get(t).SupportIDReuse {
				t.Skip()
			}
		})

		s.Before(func(t *testcase.T) {
			t.Must.Nil(act(t))
			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
			t.Must.Nil(c.subject().Get(t).Resource.DeleteByID(c.subject().Get(t).MakeContext(), getID(t)))
			IsAbsent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
		})
	})

	s.When(`entity is already created and then remove before`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !c.subject().Get(t).SupportRecreate {
				t.Skip()
			}
		})

		s.Before(func(t *testcase.T) {
			ogEnt := *ptr.Get(t) // a deep copy might be better
			t.Must.Nil(act(t))
			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
			t.Must.Nil(c.subject().Get(t).Resource.DeleteByID(c.subject().Get(t).MakeContext(), getID(t)))
			IsAbsent[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
			ptr.Set(t, &ogEnt)
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			t.Must.Nil(act(t))

			IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), getID(t))
		})
	})

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.subject().Get(t).MakeContext())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	s.Test(`persist on #Create`, func(t *testcase.T) {
		e := c.subject().Get(t).MakeEntity()

		err := c.subject().Get(t).Resource.Create(c.subject().Get(t).MakeContext(), &e)
		t.Must.Nil(err)

		id, ok := extid.Lookup[ID](&e)
		t.Must.True(ok, "ID is not defined in the entity struct src definition")
		t.Must.NotEmpty(id, "it's expected that repository set the external ID in the entity")

		t.Must.Equal(e, *IsFindable[Entity, ID](t, c.subject().Get(t).Resource, c.subject().Get(t).MakeContext(), id))
		t.Must.Nil(c.subject().Get(t).Resource.DeleteByID(c.subject().Get(t).MakeContext(), id))
	})
}
