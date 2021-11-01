package contracts

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts/assert"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type Purger struct {
	T              T
	Subject        func(testing.TB) PurgerSubject
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type PurgerSubject interface {
	CRD
	frameless.Purger
}

func (c Purger) resourceGet(t *testcase.T) PurgerSubject {
	return testcase.Var{
		Name: "PurgerSubject",
		Init: func(t *testcase.T) interface{} { return c.Subject(t) },
	}.Get(t).(PurgerSubject)
}

func (c Purger) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)
	s.Describe(`.Purge`, c.specPurge)
}

func (c Purger) specPurge(s *testcase.Spec) {
	ctx.Bind(s)
	subject := func(t *testcase.T) error {
		return c.resourceGet(t).Purge(ctxGet(t))
	}

	s.Then(`after the purge, resource is empty`, func(t *testcase.T) {
		require.NoError(t, subject(t))
		assert.CountIs(t, c.resourceGet(t).FindAll(ctxGet(t)), 0)
	})

	s.When(`entities is created prior to Purge`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			n := t.Random.IntN(42)
			for i := 0; i < n; i++ {
				ptr := assert.TakePtr(factoryGet(t).Fixture(c.T, ctxGet(t)))
				assert.CreateEntity(t, c.resourceGet(t), ctxGet(t), ptr)
			}
		})

		s.Then(`it will purge the entities`, func(t *testcase.T) {
			require.NoError(t, subject(t))
			assert.CountIs(t, c.resourceGet(t).FindAll(ctxGet(t)), 0)
		})
	})
}

func (c Purger) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Purger) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
