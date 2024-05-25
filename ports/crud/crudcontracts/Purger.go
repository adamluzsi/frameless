package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/ports/contract"
	. "go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/testcase"
)

func Purger[Entity, ID any](subject purgerSubjectResource[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = let.With[context.Context](s, c.MakeContext)
	)
	act := func(t *testcase.T) error {
		return subject.Purge(ctx.Get(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		allFinder, ok := any(subject).(crud.AllFinder[Entity])
		if !ok {
			t.Skip("crud.AllFinder is not supported")
		}
		t.Must.Nil(act(t))
		CountIs(t, allFinder.FindAll(c.MakeContext()), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := pointer.Of(c.MakeEntity(t))
				Create[Entity, ID](t, subject, ctx.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			allFinder, ok := any(subject).(crud.AllFinder[Entity])
			if !ok {
				t.Skip("crud.AllFinder is not supported")
			}
			t.Must.Nil(act(t))
			CountIs(t, allFinder.FindAll(ctx.Get(t)), 0)
		})
	})

	return s.AsSuite("Purger")
}

type purgerSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.Purger
}
