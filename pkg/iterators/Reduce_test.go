package iterators_test

import (
	"github.com/adamluzsi/frameless/pkg/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestReduce(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		src = testcase.Let(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iter = testcase.Let(s, func(t *testcase.T) iterators.Iterator[string] {
			return iterators.Slice(src.Get(t))
		})
		initial = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = testcase.Let(s, func(t *testcase.T) func(int, string) int {
			return func(r int, v string) int {
				return r + len(v)
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterators.Reduce(iter.Get(t), initial.Get(t), reducer.Get(t))
	}

	expectedErr := testcase.Let(s, func(t *testcase.T) error {
		return t.Random.Error()
	})

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r, err := act(t)
		t.Must.Nil(err)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})

	s.When("Iterator.Close encounters an error", func(s *testcase.Spec) {
		iter.Let(s, func(t *testcase.T) iterators.Iterator[string] {
			stub := iterators.Stub(iter.Init(t))
			stub.StubClose = func() error {
				return expectedErr.Get(t)
			}
			return stub
		})

		s.Then("it will return the close error", func(t *testcase.T) {
			_, err := act(t)
			t.Must.ErrorIs(expectedErr.Get(t), err)
		})
	})

	s.When("Iterator.Err yields an error an error", func(s *testcase.Spec) {
		iter.Let(s, func(t *testcase.T) iterators.Iterator[string] {
			stub := iterators.Stub(iter.Init(t))
			stub.StubErr = func() error {
				return expectedErr.Get(t)
			}
			return stub
		})

		s.Then("it will return the close error", func(t *testcase.T) {
			_, err := act(t)
			t.Must.ErrorIs(expectedErr.Get(t), err)
		})
	})
}

func TestReduce_reducerWithError(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		src = testcase.Let(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
			}
		})
		iter = testcase.Let(s, func(t *testcase.T) iterators.Iterator[string] {
			return iterators.Slice(src.Get(t))
		})
		initial = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = testcase.Let(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(r int, v string) (int, error) {
				return r + len(v), nil
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterators.Reduce(iter.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will reduce", func(t *testcase.T) {
		r, err := act(t)
		t.Must.Nil(err)
		t.Must.Equal(1+2+3+initial.Get(t), r)
	})

	s.When("reducer returns with an error", func(s *testcase.Spec) {
		expectedErr := testcase.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		reducer.Let(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(r int, v string) (int, error) {
				return r + len(v), expectedErr.Get(t)
			}
		})

		s.Then("it will return the close error", func(t *testcase.T) {
			_, err := act(t)
			t.Must.ErrorIs(expectedErr.Get(t), err)
		})
	})
}
