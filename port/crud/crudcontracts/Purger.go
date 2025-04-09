package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudkit"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/port/crud"
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
		t.Must.NoError(act(t))

		vs, err := crudkit.CollectQueryMany(allFinder.FindAll(c.MakeContext(t)))
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := pointer.Of(c.MakeEntity(t))
				c.Helper().Create(t, subject, ctx.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			allFinder, ok := any(subject).(crud.AllFinder[Entity])
			if !ok {
				t.Skip("crud.AllFinder is not supported")
			}
			t.Must.NoError(act(t))

			vs, err := crudkit.CollectQueryMany(allFinder.FindAll(ctx.Get(t)))
			assert.NoError(t, err)
			assert.Empty(t, vs)
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
