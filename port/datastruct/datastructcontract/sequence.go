package datastructcontract

import (
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/datastruct"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type SequenceConfig[T any] struct {
	MakeElem func(tb testing.TB) T
}

func (sc SequenceConfig[T]) Configure(t *SequenceConfig[T]) {
	t.MakeElem = zerokit.Coalesce(sc.MakeElem, t.MakeElem)
}

func (sc SequenceConfig[T]) ToListConfig() ListConfig[T] {
	return ListConfig[T](sc)
}

type SequenceOption[T any] option.Option[SequenceConfig[T]]

func Sequence[T any](make contract.Make[datastruct.Sequence[T]], opts ...SequenceOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	seq := let.Var(s, func(t *testcase.T) datastruct.Sequence[T] {
		return make(t)
	})

	OrderedList(func(tb testing.TB) datastruct.List[T] {
		return make(tb)
	}, c.ToListConfig()).Spec(s)

	s.Describe("#Lookup", func(s *testcase.Spec) {
		var (
			index = let.Var[int](s, nil)
		)
		act := let.Act2(func(t *testcase.T) (T, bool) {
			return seq.Get(t).Lookup(index.Get(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Equal(t, 0, seq.Get(t).Len(), `The "Make" sequence should be empty but isn't—please check the setup.`)
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(0, 42)
				})

				s.Then("the requested value is repoted to be missing", func(t *testcase.T) {
					_, ok := act(t)
					assert.False(t, ok, "expected that value is not found")
				})
			})
		})

		s.When("sequence contains values", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(3, 7), func() T {
					return mk[T](t, c.MakeElem)
				})
			})

			seq.Let(s, func(t *testcase.T) datastruct.Sequence[T] {
				seq := seq.Super(t)
				seq.Append(values.Get(t)...)
				return seq
			})

			s.And("index points to an existing value", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntN(len(values.Get(t)))
				})

				s.Then("the expected value is returned", func(t *testcase.T) {
					got, ok := act(t)
					assert.True(t, ok, "expected that value is found")
					exp := values.Get(t)[index.Get(t)]
					assert.Equal(t, exp, got)
				})
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return len(values.Get(t)) + t.Random.IntBetween(0, 42)
				})

				s.Then("the requested value is repoted to be missing", func(t *testcase.T) {
					_, ok := act(t)
					assert.False(t, ok, "expected that value is not found")
				})
			})
		})
	})

	s.Describe("#Set", func(s *testcase.Spec) {
		var (
			index = let.Var[int](s, nil)
			value = let.Var(s, func(t *testcase.T) T {
				return mk[T](t, c.MakeElem)
			})
		)
		act := let.Act(func(t *testcase.T) bool {
			return seq.Get(t).Set(index.Get(t), value.Get(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Equal(t, 0, seq.Get(t).Len(), `The "Make" sequence should be empty but isn't—please check the setup.`)
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(0, 42)
				})

				s.Then("it reports that it was not possible", func(t *testcase.T) {
					ok := act(t)
					assert.False(t, ok)
				})
			})
		})

		s.When("sequence contains values", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(3, 7), func() T {
					return mk[T](t, c.MakeElem)
				})
			})

			seq.Let(s, func(t *testcase.T) datastruct.Sequence[T] {
				seq := seq.Super(t)
				seq.Append(values.Get(t)...)
				return seq
			})

			s.And("index points to an existing value", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntN(len(values.Get(t)))
				})

				s.Then("the new value is set for the given index", func(t *testcase.T) {
					assert.True(t, act(t), "expected a success report for Set")

					got, ok := seq.Get(t).Lookup(index.Get(t))
					assert.True(t, ok)
					assert.Equal(t, value.Get(t), got)
				})

				s.Then("the total length remains the same", func(t *testcase.T) {
					befLen := seq.Get(t).Len()
					act(t)
					aftLen := seq.Get(t).Len()

					assert.Equal(t, befLen, aftLen)
				})

				s.Then("apart from the changed value, everything else remains the original one", func(t *testcase.T) {
					act(t)

					for i := 0; i < seq.Get(t).Len(); i++ {
						v, ok := seq.Get(t).Lookup(i)
						assert.True(t, ok)
						if i == index.Get(t) {
							assert.Equal(t, v, value.Get(t))
						} else {
							assert.Equal(t, v, values.Get(t)[i])
						}
					}
				})
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return len(values.Get(t)) + t.Random.IntBetween(0, 42)
				})

				s.Then("failure to set the value is reported", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})
	})

	s.Describe("#Insert", func(s *testcase.Spec) {
		var (
			index     = let.Var[int](s, nil)
			newValues = let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(3, 7), func() T {
					return mk[T](t, c.MakeElem)
				})
			})
		)
		act := let.Act(func(t *testcase.T) bool {
			return seq.Get(t).Insert(index.Get(t), newValues.Get(t)...)
		})
		onSuccess := let.Act0(func(t *testcase.T) {
			t.Helper()
			assert.True(t, act(t), "test depends on successful test execution")
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Equal(t, 0, seq.Get(t).Len(), `The "Make" sequence should be empty but isn't—please check the setup.`)
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(1, 42)
				})

				s.Then("it reports failure", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			s.And("index is zero", func(s *testcase.Spec) {
				index.LetValue(s, 0)

				s.Then("it inserts the values", func(t *testcase.T) {
					assert.True(t, act(t))
					assert.Equal(t, len(newValues.Get(t)), seq.Get(t).Len(),
						"expected that sequence length reflects the newly inserted values")

					for i := 0; i < seq.Get(t).Len(); i++ {
						v, ok := seq.Get(t).Lookup(i)
						assert.True(t, ok)
						assert.Equal(t, newValues.Get(t)[i], v)
					}
				})
			})
		})

		s.When("sequence contains values", func(s *testcase.Spec) {
			// A B C <- insert X Y Z at 1
			// 0 1 2
			//
			// -> A X Y Z B C
			// -> 0 1 2 3 4 5
			//
			// 1:B -> 4:B -> 1 + /* len new values */ 3 == 4 4:B

			values := let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(3, 7), func() T {
					return mk[T](t, c.MakeElem)
				})
			})

			seq.Let(s, func(t *testcase.T) datastruct.Sequence[T] {
				seq := seq.Super(t)
				seq.Append(values.Get(t)...)
				return seq
			})

			s.And("index points to an existing value", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntN(len(values.Get(t)))
				})

				s.Then("the total length increases to the sum of values", func(t *testcase.T) {
					onSuccess(t)

					expLen := len(values.Get(t)) + len(newValues.Get(t))
					gotLen := seq.Get(t).Len()
					assert.Equal(t, expLen, gotLen)
				})

				s.Then("values not affected prior to the index", func(t *testcase.T) {
					onSuccess(t)

					var newValueFromIndex = index.Get(t)
					for i := 0; i < newValueFromIndex; i++ {
						v, ok := seq.Get(t).Lookup(i)
						assert.True(t, ok)
						assert.Equal(t, v, values.Get(t)[i])
					}
				})

				s.Then("new values inserted from the given index", func(t *testcase.T) {
					onSuccess(t)

					var offset = index.Get(t)
					for i, exp := range newValues.Get(t) {
						tindex := i + offset
						got, ok := seq.Get(t).Lookup(tindex)
						assert.True(t, ok)
						assert.Equal(t, exp, got)
					}
				})

				s.Then("old values from the original index are present after the last elem of the newly inserted values", func(t *testcase.T) {
					onSuccess(t)

					var remainder = values.Get(t)[index.Get(t):]
					var offset = len(values.Get(t)[:index.Get(t)]) + len(newValues.Get(t))
					for i, exp := range remainder {
						outIndex := i + offset
						got, ok := seq.Get(t).Lookup(outIndex)
						assert.True(t, ok)
						assert.Equal(t, exp, got)
					}
				})
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return len(values.Get(t)) + t.Random.IntBetween(1, 42)
				})

				s.Then("failure is reported", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})
	})

	s.Describe("#Delete", func(s *testcase.Spec) {
		var (
			index = let.Var[int](s, nil)
		)
		act := let.Act(func(t *testcase.T) bool {
			return seq.Get(t).Delete(index.Get(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Equal(t, 0, seq.Get(t).Len(), `The "Make" sequence should be empty but isn't—please check the setup.`)
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(0, 42)
				})

				s.Then("it reports that it was not possible", func(t *testcase.T) {
					ok := act(t)
					assert.False(t, ok)
				})
			})
		})

		s.When("sequence contains values", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(3, 7), func() T {
					return mk[T](t, c.MakeElem)
				})
			})

			seq.Let(s, func(t *testcase.T) datastruct.Sequence[T] {
				seq := seq.Super(t)
				seq.Append(values.Get(t)...)
				return seq
			})

			s.And("index points to an existing value", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntN(len(values.Get(t)))
				})

				s.Then("the total length shrinks by one", func(t *testcase.T) {
					befLen := seq.Get(t).Len()
					act(t)
					aftLen := seq.Get(t).Len()
					assert.Equal(t, befLen, aftLen+1)
				})

				s.Then("apart from the changed value, everything else remains the original one", func(t *testcase.T) {
					exp := slicekit.Clone(values.Get(t))
					assert.True(t, slicekit.Delete(&exp, index.Get(t)))
					act(t)

					assert.Equal(t, exp, iterkit.Collect(seq.Get(t).Iter()))

					if cts, ok := seq.Get(t).(datastruct.Slicer[T]); ok {
						assert.Equal(t, exp, cts.Slice())
					}
				})
			})

			s.And("index is out of bound", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return len(values.Get(t)) + t.Random.IntBetween(0, 42)
				})

				s.Then("failure to set the value is reported", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})
	})

	return s.AsSuite("Sequence[T]")
}
