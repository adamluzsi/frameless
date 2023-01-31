package iteratorcontracts

import (
	"testing"

	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase"
)

type Iterator[V any] struct {
	// MakeSubject returns a non-empty iterator that returns V value on each iteration.
	MakeSubject func(tb testing.TB) iterators.Iterator[V]
}

func (c Iterator[V]) Spec(s *testcase.Spec) {
	s.Describe("it behaves like an iterator", func(s *testcase.Spec) {
		subject := testcase.Let(s, func(t *testcase.T) iterators.Iterator[V] {
			return c.MakeSubject(t)
		})

		s.Then("values can be collected from the iterator", func(t *testcase.T) {
			vs, err := iterators.Collect[V](subject.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(vs)
		})

		s.Then("closing the iterator is possible, even multiple times, without an issue", func(t *testcase.T) {
			sub := subject.Get(t)
			for i, n := 0, t.Random.IntB(3, 7); i < n; i++ {
				t.Must.NoError(sub.Close())
				t.Must.NoError(sub.Err())
			}
		})

		s.When("iterator is closed", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.NoError(subject.Get(t).Close())
			})

			s.Then("no more value is iterated", func(t *testcase.T) {
				vs, err := iterators.Collect(subject.Get(t))
				t.Must.NoError(err)
				t.Must.Empty(vs)
			})
		})
	})
}

func (c Iterator[V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Iterator[V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
