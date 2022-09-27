package crudcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/testcase"
)

type Purger[Ent, ID any] struct {
	Subject func(testing.TB) PurgerSubject[Ent, ID]
	MakeEnt func(testing.TB) Ent
	MakeCtx func(testing.TB) context.Context
}

type PurgerSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	crud.Purger
}

func (c Purger[Ent, ID]) resourceGet(t *testcase.T) PurgerSubject[Ent, ID] {
	return testcase.Var[PurgerSubject[Ent, ID]]{
		ID:   "PurgerSubject",
		Init: func(t *testcase.T) PurgerSubject[Ent, ID] { return c.Subject(t) },
	}.Get(t)
}

func (c Purger[Ent, ID]) Spec(s *testcase.Spec) {
	s.Describe(`.Purge`, c.specPurge)
}

func (c Purger[Ent, ID]) specPurge(s *testcase.Spec) {
	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeCtx(t)
	})

	subject := func(t *testcase.T) error {
		return c.resourceGet(t).Purge(spechelper.ContextVar.Get(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		r := c.resourceGet(t)
		allFinder, ok := r.(crud.AllFinder[Ent, ID])
		if !ok {
			t.Skip("crud.AllFinder is not supported")
		}
		t.Must.Nil(subject(t))
		CountIs(t, allFinder.FindAll(c.MakeCtx(t)), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := spechelper.ToPtr(c.MakeEnt(t))
				Create[Ent, ID](t, c.resourceGet(t), spechelper.ContextVar.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			r := c.resourceGet(t)
			allFinder, ok := r.(crud.AllFinder[Ent, ID])
			if !ok {
				t.Skip("crud.AllFinder is not supported")
			}
			t.Must.Nil(subject(t))
			CountIs(t, allFinder.FindAll(spechelper.ContextVar.Get(t)), 0)
		})
	})
}

func (c Purger[Ent, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Purger[Ent, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
