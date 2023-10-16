package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/ports/iterators"
	iteratorcontracts "go.llib.dev/frameless/ports/iterators/iteratorcontracts"
	"go.llib.dev/frameless/ports/iterators/ranges"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestOffset_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterators.Offset(ranges.Int(2, 6), 2)
	vs, err := iterators.Collect(subject)
	it.Must.NoError(err)
	it.Must.Equal([]int{4, 5, 6}, vs)
}

func TestOffset(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		makeIter = func() iterators.Iterator[int] {
			return ranges.Int(1, iterLen)
		}
		iter = testcase.Let(s, func(t *testcase.T) iterators.Iterator[int] {
			return makeIter()
		})
		offset = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iterators.Iterator[int] {
		return iterators.Offset(iter.Get(t), offset.Get(t))
	})

	s.Then("it will limit the results by skipping by the offset number", func(t *testcase.T) {
		got, err := iterators.Collect(subject.Get(t))
		t.Must.NoError(err)

		all, err := iterators.Collect(makeIter())
		t.Must.NoError(err)

		var exp = make([]int, 0)
		for i := offset.Get(t); i < len(all); i++ {
			exp = append(exp, all[i])
		}

		t.Must.Equal(exp, got)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		iter.Let(s, func(t *testcase.T) iterators.Iterator[int] {
			return iterators.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			iter := subject.Get(t)
			t.Must.False(iter.Next())
			t.Must.NoError(iter.Err())
			t.Must.NoError(iter.Close())
		})
	})

	s.When("the source iterator has less values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterators.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Empty(got)
		})
	})

	s.When("the source iterator has as many values as the offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterators.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Empty(got)
		})
	})

	s.When("the source iterator has more values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen-1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterators.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)
			t.Must.Equal([]int{iterLen}, got)
		})
	})
}

func TestOffset_implementsIterator(t *testing.T) {
	iteratorcontracts.Iterator[int](func(tb testing.TB) iterators.Iterator[int] {
		t := testcase.ToT(&tb)
		return iterators.Offset(
			ranges.Int(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}
