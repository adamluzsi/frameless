package iterators_test

import (
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
)

func TestBatch(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = testcase.Let[[]int](s, func(t *testcase.T) []int {
			var vs []int
			for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
				vs = append(vs, t.Random.Int())
			}
			return vs
		})
		src = testcase.Let[frameless.Iterator[int]](s, func(t *testcase.T) frameless.Iterator[int] {
			return iterators.Slice(values.Get(t))
		})
		config = testcase.Let(s, func(t *testcase.T) iterators.BatchConfig {
			return iterators.BatchConfig{}
		})
		subject = testcase.Let[*iterators.BatchIter[int]](s, func(t *testcase.T) *iterators.BatchIter[int] {
			return iterators.Batch(src.Get(t), config.Get(t))
		})
	)

	ThenIterateWithDefaultValue := func(s *testcase.Spec) {
		s.Then("iterate with default value(s)", func(t *testcase.T) {
			t.Must.True(len(values.Get(t)) < 100)
			iter := subject.Get(t)

			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.NotEmpty(iter.Value())
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	}

	ThenIterateWithDefaultValue(s)

	s.When("batch size is defined", func(s *testcase.Spec) {
		size := testcase.Let[int](s, nil)

		config.Let(s, func(t *testcase.T) iterators.BatchConfig {
			return iterators.BatchConfig{Size: size.Get(t)}
		})

		s.And("it is valid positive value", func(s *testcase.Spec) {
			size.Let(s, func(t *testcase.T) int {
				return t.Random.IntB(1, len(values.Get(t)))
			})

			s.Then("batch size corresponds to the configuration", func(t *testcase.T) {
				iter := subject.Get(t)
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

		s.And("it is an invalid value", func(s *testcase.Spec) {
			size.Let(s, func(t *testcase.T) int {
				// negative value is not acceptable
				return t.Random.IntB(1, 7) * -1
			})

			ThenIterateWithDefaultValue(s)
		})
	})

	s.When("batch timeout is defined", func(s *testcase.Spec) {
		timeout := testcase.Let[time.Duration](s, nil)

		config.Let(s, func(t *testcase.T) iterators.BatchConfig {
			return iterators.BatchConfig{
				Timeout: timeout.Get(t),
				Size:    len(values.Get(t)) * 2,
			}
		})

		s.And("it is valid positive value", func(s *testcase.Spec) {
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
			src.Let(s, func(t *testcase.T) frameless.Iterator[int] {
				return pipe.Get(t).Out
			})

			s.Then("batch timeout corresponds to the configuration", func(t *testcase.T) {
				iter := subject.Get(t)
				t.Must.True(iter.Next()) // trigger batching
				t.Must.ContainExactly(values.Get(t), iter.Value())
			})
		})

		s.And("it is an invalid value", func(s *testcase.Spec) {
			timeout.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(t.Random.IntB(500, 1000)) * time.Microsecond * -1
			})

			//s.Before(func(t *testcase.T) {
			//	wg := &sync.WaitGroup{}
			//	wg.Add(1)
			//	go func() {
			//		defer wg.Done()
			//		in := pipe.Get(t).In
			//		for _, v := range values.Get(t) {
			//			in.Value(v)
			//		}
			//		t.Must.Nil(in.Close())
			//	}()
			//	t.Defer(wg.Wait)
			//})

			ThenIterateWithDefaultValue(s)
		})
	})
}
