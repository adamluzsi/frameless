package crudcontracts

import (
	"context"
	"testing"

	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

type Purger[Entity, ID any] struct {
	MakeSubject func(testing.TB) PurgerSubject[Entity, ID]
	MakeEntity  func(testing.TB) Entity
	MakeContext func(testing.TB) context.Context
}

type PurgerSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.Purger
}

func (c Purger[Entity, ID]) resourceGet(t *testcase.T) PurgerSubject[Entity, ID] {
	return testcase.Var[PurgerSubject[Entity, ID]]{
		ID:   "PurgerSubject",
		Init: func(t *testcase.T) PurgerSubject[Entity, ID] { return c.MakeSubject(t) },
	}.Get(t)
}

func (c Purger[Entity, ID]) Spec(s *testcase.Spec) {
	s.Describe(`.Purge`, c.specPurge)
}

func (c Purger[Entity, ID]) specPurge(s *testcase.Spec) {
	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeContext(t)
	})

	subject := func(t *testcase.T) error {
		return c.resourceGet(t).Purge(spechelper.ContextVar.Get(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		r := c.resourceGet(t)
		allFinder, ok := r.(crud.AllFinder[Entity])
		if !ok {
			t.Skip("crud.AllFinder is not supported")
		}
		t.Must.Nil(subject(t))
		CountIs(t, allFinder.FindAll(c.MakeContext(t)), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := spechelper.ToPtr(c.MakeEntity(t))
				Create[Entity, ID](t, c.resourceGet(t), spechelper.ContextVar.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			r := c.resourceGet(t)
			allFinder, ok := r.(crud.AllFinder[Entity])
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
