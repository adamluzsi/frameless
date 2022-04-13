package contracts

import (
	"testing"

	"github.com/adamluzsi/frameless"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/testcase"
)

type Purger[Ent, ID any] struct {
	Subject func(testing.TB) PurgerSubject[Ent, ID]
	MakeEnt func(testing.TB) Ent
}

type PurgerSubject[Ent, ID any] interface {
	CRD[Ent, ID]
	frameless.Purger
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
	ctxVar.Bind(s)
	subject := func(t *testcase.T) error {
		return c.resourceGet(t).Purge(ctxVar.Get(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		t.Must.Nil(subject(t))
		CountIs(t, c.resourceGet(t).FindAll(ctxVar.Get(t)), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := toPtr(c.MakeEnt(t))
				Create[Ent, ID](t, c.resourceGet(t), ctxVar.Get(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			CountIs(t, c.resourceGet(t).FindAll(ctxVar.Get(t)), 0)
		})
	})
}

func (c Purger[Ent, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Purger[Ent, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
