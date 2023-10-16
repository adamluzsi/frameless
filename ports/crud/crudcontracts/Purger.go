package crudcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	. "go.llib.dev/frameless/ports/crud/crudtest"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

type Purger[Entity, ID any] func(testing.TB) PurgerSubject[Entity, ID]

type PurgerSubject[Entity, ID any] struct {
	Resource    purgerSubjectResource[Entity, ID]
	MakeEntity  func() Entity
	MakeContext func() context.Context
}

type purgerSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.Purger
}

func (c Purger[Entity, ID]) subjectGet(t *testcase.T) PurgerSubject[Entity, ID] {
	return testcase.Var[PurgerSubject[Entity, ID]]{
		ID:   "PurgerSubject[Entity, ID]",
		Init: func(t *testcase.T) PurgerSubject[Entity, ID] { return c(t) },
	}.Get(t)
}

func (c Purger[Entity, ID]) Spec(s *testcase.Spec) {
	s.Describe(`.Purge`, c.specPurge)
}

func (c Purger[Entity, ID]) specPurge(s *testcase.Spec) {
	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.subjectGet(t).MakeContext()
	})

	subject := func(t *testcase.T) error {
		return c.subjectGet(t).Resource.Purge(spechelper.ContextVar.Get(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		sub := c.subjectGet(t)
		allFinder, ok := sub.Resource.(crud.AllFinder[Entity])
		if !ok {
			t.Skip("crud.AllFinder is not supported")
		}
		t.Must.Nil(subject(t))
		CountIs(t, allFinder.FindAll(sub.MakeContext()), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := pointer.Of(c.subjectGet(t).MakeEntity())
				Create[Entity, ID](t, c.subjectGet(t).Resource, spechelper.ContextVar.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			sub := c.subjectGet(t)
			allFinder, ok := sub.Resource.(crud.AllFinder[Entity])
			if !ok {
				t.Skip("crud.AllFinder is not supported")
			}
			t.Must.Nil(subject(t))
			CountIs(t, allFinder.FindAll(spechelper.ContextVar.Get(t)), 0)
		})
	})
}

func (c Purger[Entity, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Purger[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
