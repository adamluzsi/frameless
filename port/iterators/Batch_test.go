package iterators_test

import (
	"testing"
	"time"

	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/port/iterators"

	"go.llib.dev/testcase"
)

const defaultBatchSize = 64

func TestBatch(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = testcase.Let[[]int](s, func(t *testcase.T) []int {
			return random.Slice[int](t.Random.IntB(50, 200), func() int {
				return t.Random.Int()
			})
		})
		src = testcase.Let[iterators.Iterator[int]](s, func(t *testcase.T) iterators.Iterator[int] {
			return iterators.Slice(values.Get(t))
		})
		size = testcase.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) * 2
		})
	)
	act := testcase.Let[iterators.Iterator[[]int]](s, func(t *testcase.T) iterators.Iterator[[]int] {
		return iterators.Batch(src.Get(t), size.Get(t))
	})

	s.When("size is a valid positive value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(1, len(values.Get(t)))
		})

		s.Then("batching size is used", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Log(len(iter.Value()) <= size.Get(t), len(iter.Value()), size.Get(t))
				t.Must.True(len(iter.Value()) <= size.Get(t))
				t.Must.NotEmpty(iter.Value())
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})

	s.When("size is an invalid value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			// negative value is not acceptable
			return t.Random.IntB(1, 7) * -1
		})

		s.Then("iterate with default value(s)", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.NotEmpty(iter.Value())
				t.Must.True(len(iter.Value()) <= defaultBatchSize, "iteration ")
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})
}

func TestBatchWithTimeout(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = testcase.Let[[]int](s, func(t *testcase.T) []int {
			var vs []int
			for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
				vs = append(vs, t.Random.Int())
			}
			return vs
		})
		src = testcase.Let[iterators.Iterator[int]](s, func(t *testcase.T) iterators.Iterator[int] {
			return iterators.Slice(values.Get(t))
		})
		size = testcase.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) * 2
		})
		timeout = testcase.LetValue[time.Duration](s, 0)
	)
	act := testcase.Let[iterators.Iterator[[]int]](s, func(t *testcase.T) iterators.Iterator[[]int] {
		return iterators.BatchWithTimeout(src.Get(t), size.Get(t), timeout.Get(t))
	})

	ThenIterateWithDefaultValue := func(s *testcase.Spec) {
		s.Then("iterate with default value(s)", func(t *testcase.T) {
			iter := act.Get(t)

			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.NotEmpty(iter.Value())
				t.Must.True(len(iter.Value()) < defaultBatchSize, "iterate with default batch size")
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	}

	ThenIterateWithDefaultValue(s)

	s.When("size is a valid positive value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(1, len(values.Get(t)))
		})

		s.Then("batch size corresponds to the configuration", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.True(len(iter.Value()) <= size.Get(t))
				t.Must.NotEmpty(iter.Value())
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})

	s.And("size is an invalid value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			// negative value is not acceptable
			return t.Random.IntB(1, 7) * -1
		})

		ThenIterateWithDefaultValue(s)
	})

	s.When("timeout is valid positive value", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return 100 * time.Millisecond
		})

		type Pipe struct {
			In  *iterators.PipeIn[int]
			Out *iterators.PipeOut[int]
		}
		pipe := testcase.Let[Pipe](s, func(t *testcase.T) Pipe {
			in, out := iterators.Pipe[int]()
			t.Defer(in.Close)
			t.Defer(out.Close)
			go func() {
				for _, v := range values.Get(t) {
					if !in.Value(v) {
						break
					}
				}
				// wait forever to trigger batching
			}()
			return Pipe{
				In:  in,
				Out: out,
			}
		})
		src.Let(s, func(t *testcase.T) iterators.Iterator[int] {
			return pipe.Get(t).Out
		})

		s.Then("batch timeout corresponds to the configuration", func(t *testcase.T) {
			iter := act.Get(t)
			t.Must.True(iter.Next()) // trigger batching
			t.Must.ContainExactly(values.Get(t), iter.Value())
		})
	})

	s.When("timeout is an invalid value", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntB(500, 1000)) * time.Microsecond * -1
		})

		ThenIterateWithDefaultValue(s)
	})
}
