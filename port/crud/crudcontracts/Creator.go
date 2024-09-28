package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/port/contract"
	. "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
)

func Creator[Entity, ID any](subject crd[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID], Option[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Entity {
			v := c.MakeEntity(t)
			return &v
		})
		getID = func(t *testcase.T) ID {
			id, _ := lookupID[ID](c, *ptr.Get(t))
			return id
		}
	)
	act := func(t *testcase.T) error {
		ctx := ctxVar.Get(t)
		err := subject.Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := lookupID[ID](c, *ptr.Get(t))
			t.Defer(subject.DeleteByID, ctx, id)
			IsPresent[Entity, ID](t, subject, ctx, id)
		}
		return err
	}

	s.When(`entity was not saved before`, func(s *testcase.Spec) {
		s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			t.Must.NotEmpty(getID(t))
		})

		s.Then(`after creation, the freshly made entity can be retrieved by its id`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			t.Must.Equal(ptr.Get(t), IsPresent[Entity, ID](t, subject, c.MakeContext(t), getID(t)))
		})
	})

	s.When(`entity ID is reused or provided ahead of time`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !c.SupportIDReuse {
				t.Skip()
			}
		})

		s.Before(func(t *testcase.T) {
			t.Must.Nil(act(t))
			IsPresent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
			t.Must.Nil(subject.DeleteByID(c.MakeContext(t), getID(t)))
			IsAbsent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			IsPresent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
		})
	})

	s.When(`entity is already created and then remove before`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !c.SupportRecreate {
				t.Skip()
			}
		})

		s.Before(func(t *testcase.T) {
			ogEnt := *ptr.Get(t) // a deep copy might be better
			t.Must.Nil(act(t))
			IsPresent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
			t.Must.Nil(subject.DeleteByID(c.MakeContext(t), getID(t)))
			IsAbsent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
			ptr.Set(t, &ogEnt)
		})

		s.Then(`it will accept it`, func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.Then(`persisted object can be found`, func(t *testcase.T) {
			t.Must.Nil(act(t))

			IsPresent[Entity, ID](t, subject, c.MakeContext(t), getID(t))
		})
	})

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

		err := subject.Create(c.MakeContext(t), &e)
		t.Must.Nil(err)

		id, ok := extid.Lookup[ID](&e)
		t.Must.True(ok, "ID is not defined in the entity struct src definition")
		t.Must.NotEmpty(id, "it's expected that repository set the external ID in the entity")

		t.Must.Equal(e, *IsPresent[Entity, ID](t, subject, c.MakeContext(t), id))
		t.Must.Nil(subject.DeleteByID(c.MakeContext(t), id))
	})

	return s.AsSuite("Creator")
}

// type CreatorSubject[Entity, ID any] struct {
// 	Resource    spechelper.CRD[Entity, ID]
// 	MakeContext func() context.Context
// 	MakeEntity  func() Entity

// 	// SupportIDReuse is an optional configuration value that tells the contract
// 	// that recreating an entity with an ID which belongs to a previously deleted entity is accepted.
// 	SupportIDReuse bool
// 	// SupportRecreate is an optional configuration value that tells the contract
// 	// that deleting an Entity then recreating it with the same values is supported by the Creator.
// 	SupportRecreate bool

// 	forSaverSuite bool
// }

// func (c Creator[Entity, ID]) Name() string {
// 	return "Creator"
// }

// func (c Creator[Entity, ID]) subject() testcase.Var[CreatorSubject[Entity, ID]] {
// 	return testcase.Var[CreatorSubject[Entity, ID]]{
// 		ID: "CreatorSubject[Entity, ID]",
// 		Init: func(t *testcase.T) CreatorSubject[Entity, ID] {
// 			return c(t)
// 		},
// 	}
// }

// func (c Creator[Entity, ID]) Test(t *testing.T) {
// 	c.Spec(testcase.NewSpec(t))
// }

// func (c Creator[Entity, ID]) Benchmark(b *testing.B) {
// 	subject := c(testcase.ToT(b))

// 	spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)
// 	b.Run(`Creator`, func(b *testing.B) {
// 		var (
// 			ctx = subject.MakeContext()
// 			es  []*Entity
// 		)
// 		for i := 0; i < b.N; i++ {
// 			ent := subject.MakeEntity()
// 			es = append(es, &ent)
// 		}
// 		defer spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)

// 		b.ResetTimer()
// 		for i := 0; i < b.N; i++ {
// 			_ = subject.Resource.Create(ctx, es[i])
// 		}
// 		b.StopTimer()
// 	})
// }
