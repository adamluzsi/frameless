package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/port/iterators"
	iteratorcontracts "go.llib.dev/frameless/port/iterators/iteratorcontracts"
	"go.llib.dev/frameless/port/iterators/ranges"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestLimit_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterators.Limit(ranges.Int(2, 6), 3)
	vs, err := iterators.Collect(subject)
	it.Must.NoError(err)
	it.Must.Equal([]int{2, 3, 4}, vs)
}

func TestLimit(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		iter = testcase.Let[iterators.Iterator[int]](s, func(t *testcase.T) iterators.Iterator[int] {
			return ranges.Int(1, iterLen)
		})
		n = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iterators.Iterator[int] {
		return iterators.Limit(iter.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs, err := iterators.Collect(subject.Get(t))
		t.Must.NoError(err)
		t.Must.Equal(n.Get(t), len(vs))
	})

	s.Then("it will limited amount of value", func(t *testcase.T) {
		vs, err := iterators.Collect(subject.Get(t))
		t.Must.NoError(err)

		t.Log("n", n.Get(t))
		var exp []int
		for i := 0; i < n.Get(t); i++ {
			exp = append(exp, i+1)
		}

		t.Must.Equal(exp, vs)
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

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs, err := iterators.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Equal(iterLen, len(vs))
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got, err := iterators.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)

			total, err := iterators.Collect(ranges.Int(1, iterLen))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)

			t.Logf("%v < %v", got, total)
			t.Must.True(len(got) < len(total), "got count is less than total")
		})
	})
}

func TestLimit_implementsIterator(t *testing.T) {
	iteratorcontracts.Iterator[int](func(tb testing.TB) iterators.Iterator[int] {
		t := testcase.ToT(&tb)
		return iterators.Limit(
			ranges.Int(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}
