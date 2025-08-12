package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestLinkedList(t *testing.T) {
	s := testcase.NewSpec(t)

	ll := let.Var(s, func(t *testcase.T) *datastruct.LinkedList[int] {
		return &datastruct.LinkedList[int]{}
	})

	s.Test("smoke", func(t *testcase.T) {
		var ll datastruct.LinkedList[int]

		ll.Append(1, 2, 3)
		ll.Append(4)
		ll.Prepend(-1, 0)
		assert.Equal(t, []int{-1, 0, 1, 2, 3, 4}, ll.ToSlice())

		last, ok := ll.Pop()
		assert.True(t, ok)
		assert.Equal(t, 4, last)

		var popped []int
		for {
			last, ok := ll.Pop()
			if !ok {
				break
			}
			popped = append(popped, last)
		}

		assert.Equal(t, []int{3, 2, 1, 0, -1}, popped)

		ll.Append(1, 2, 3)
		ll.Prepend(0)
		assert.Equal(t, []int{0, 1, 2, 3}, ll.ToSlice())

		var shifted []int
		for {
			first, ok := ll.Shift()
			if !ok {
				break
			}
			shifted = append(shifted, first)
		}
		assert.Equal(t, []int{0, 1, 2, 3}, shifted)

		ll.Prepend(0, 1)
		ll.Append(2, 3)
		assert.Equal(t, 4, ll.Length())
		assert.Equal(t, []int{0, 1, 2, 3}, ll.ToSlice())
	})

	s.Describe("#Append", func(s *testcase.Spec) {
		var (
			newVS = let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(1, 3), t.Random.Int)
			})
		)
		act := let.Act0(func(t *testcase.T) {
			ll.Get(t).Append(newVS.Get(t)...)
		})

		s.Then("value is appended to the list", func(t *testcase.T) {
			act(t)

			gotVS := ll.Get(t).ToSlice()
			expVS := newVS.Get(t)
			assert.Equal(t, gotVS, expVS)
		})

		s.When("no new value is provided", func(s *testcase.Spec) {
			newVS.LetValue(s, nil)

			s.Then("nothing changes", func(t *testcase.T) {
				bl := ll.Get(t).Length()
				act(t)
				al := ll.Get(t).Length()
				assert.Equal(t, bl, al)
			})
		})

		s.When("elements were already present in the slice", func(s *testcase.Spec) {
			existing := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(1, 5), t.Random.Int)
			})

			s.Before(func(t *testcase.T) {
				ll.Get(t).Append(existing.Get(t)...)
			})

			s.Then("the new value will be appended at the end", func(t *testcase.T) {
				act(t)

				expVS := slicekit.Merge(existing.Get(t), newVS.Get(t))
				gotVS := ll.Get(t).ToSlice()

				assert.Equal(t, expVS, gotVS)
			})

			s.Then("length is updated", func(t *testcase.T) {
				act(t)

				expLen := len(newVS.Get(t)) + len(existing.Get(t))
				assert.Equal(t, expLen, ll.Get(t).Length())
			})
		})
	})

	s.Describe("#Prepend", func(s *testcase.Spec) {
		var (
			newVS = let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(1, 3), t.Random.Int)
			})
		)
		act := let.Act0(func(t *testcase.T) {
			ll.Get(t).Prepend(newVS.Get(t)...)
		})

		s.Then("value is added to the list", func(t *testcase.T) {
			act(t)

			expVS := newVS.Get(t)
			gotVS := ll.Get(t).ToSlice()
			assert.Equal(t, expVS, gotVS)
		})

		s.Then("length is updated", func(t *testcase.T) {
			act(t)

			assert.Equal(t, len(newVS.Get(t)), ll.Get(t).Length())
		})

		s.When("no new value is provided", func(s *testcase.Spec) {
			newVS.LetValue(s, nil)

			s.Then("nothing changes", func(t *testcase.T) {
				bl := ll.Get(t).Length()
				act(t)
				al := ll.Get(t).Length()
				assert.Equal(t, bl, al)
			})
		})

		s.When("elements were already present in the slice", func(s *testcase.Spec) {
			existing := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(1, 5), t.Random.Int, random.UniqueValues)
			})

			s.Before(func(t *testcase.T) {
				ll.Get(t).Append(existing.Get(t)...)
			})

			s.Then("the new value will be appended at the beginning", func(t *testcase.T) {
				act(t)

				expVS := slicekit.Merge(newVS.Get(t), existing.Get(t))
				gotVS := ll.Get(t).ToSlice()
				assert.Equal(t, expVS, gotVS)
			})

			s.Then("length is updated", func(t *testcase.T) {
				act(t)

				expLen := len(newVS.Get(t)) + len(existing.Get(t))
				assert.Equal(t, expLen, ll.Get(t).Length())
			})
		})
	})

	s.Describe("#Length", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) int {
			return ll.Get(t).Length()
		})

		s.When("list is empty", func(s *testcase.Spec) {
			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				return &datastruct.LinkedList[int]{}
			})

			s.Then("zero length is reported", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})

		s.When("list is not empty", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(values.Get(t)...)
				return &list
			})

			s.Then("expected length is reported", func(t *testcase.T) {
				assert.Equal(t, len(values.Get(t)), act(t))
			})
		})
	})

	s.Describe("#Pop", func(s *testcase.Spec) {
		act := let.Act2(func(t *testcase.T) (int, bool) {
			return ll.Get(t).Pop()
		})

		// When the list is empty
		s.When("list is empty", func(s *testcase.Spec) {
			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				return &datastruct.LinkedList[int]{}
			})

			s.Then("result signals that the list has no more elements to be popped", func(t *testcase.T) {
				gotVal, gotFlag := act(t)
				assert.Equal(t, 0, gotVal) // default zero for int
				assert.False(t, gotFlag)   // no element to pop
			})

			s.Then("length stays zero", func(t *testcase.T) {
				assert.Equal(t, 0, ll.Get(t).Length())
			})
		})

		s.When("list has an element", func(s *testcase.Spec) {
			value := let.Var(s, func(t *testcase.T) int {
				return t.Random.Int()
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(value.Get(t))
				return &list
			})

			s.Then("the last element is returned", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, value.Get(t), got)
			})

			s.Then("list length is decreased to zero", func(t *testcase.T) {
				act(t)

				assert.Equal(t, 0, ll.Get(t).Length())
			})

			s.Then("list became empty", func(t *testcase.T) {
				act(t)

				assert.Empty(t, ll.Get(t).ToSlice())
			})
		})

		s.When("list has element(s)", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(
					t.Random.IntBetween(2, 5),
					t.Random.Int,
					random.UniqueValues,
				)
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(values.Get(t)...)
				return &list
			})

			s.Then("the last element is returned", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)

				exp, ok := slicekit.Last(values.Get(t))
				assert.True(t, ok)

				assert.Equal(t, got, exp)
			})

			s.Then("list length is decreased by one", func(t *testcase.T) {
				act(t)

				expLen := len(values.Get(t)) - 1
				assert.Equal(t, expLen, ll.Get(t).Length())
			})

			s.Then("remaining slice matches expected", func(t *testcase.T) {
				act(t)

				expVS := values.Get(t)[:len(values.Get(t))-1]
				gotVS := ll.Get(t).ToSlice()
				assert.Equal(t, expVS, gotVS)
			})
		})
	})

	s.Describe("#Shift", func(s *testcase.Spec) {
		act := let.Act2(func(t *testcase.T) (int, bool) {
			return ll.Get(t).Shift()
		})

		s.When("list is empty", func(s *testcase.Spec) {
			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				return &datastruct.LinkedList[int]{}
			})

			s.Then("result signals that the list has no more elements to be popped", func(t *testcase.T) {
				gotVal, gotFlag := act(t)
				assert.Equal(t, 0, gotVal) // default zero for int
				assert.False(t, gotFlag)   // no element to pop
			})

			s.Then("length stays zero", func(t *testcase.T) {
				assert.Equal(t, 0, ll.Get(t).Length())
			})
		})

		s.When("list has an element", func(s *testcase.Spec) {
			value := let.Var(s, func(t *testcase.T) int {
				return t.Random.Int()
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(value.Get(t))
				return &list
			})

			s.Then("the first and only element is returned", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, value.Get(t), got)
			})

			s.Then("list length is decreased to zero", func(t *testcase.T) {
				act(t)

				assert.Equal(t, 0, ll.Get(t).Length())
			})

			s.Then("list became empty", func(t *testcase.T) {
				act(t)

				assert.Empty(t, ll.Get(t).ToSlice())
			})
		})

		s.When("list has element(s)", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(
					t.Random.IntBetween(2, 5),
					t.Random.Int,
					random.UniqueValues,
				)
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(values.Get(t)...)
				return &list
			})

			s.Then("the first element is returned", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)

				exp, ok := slicekit.First(values.Get(t))
				assert.True(t, ok)

				assert.Equal(t, got, exp)
			})

			s.Then("list length is decreased by one", func(t *testcase.T) {
				act(t)

				expLen := len(values.Get(t)) - 1
				assert.Equal(t, expLen, ll.Get(t).Length())
			})

			s.Then("remaining slice matches expected", func(t *testcase.T) {
				act(t)

				gotVS := ll.Get(t).ToSlice()
				expVS := values.Get(t)[1:]
				assert.Equal(t, expVS, gotVS)
			})
		})
	})

	s.Describe("#Lookup", func(s *testcase.Spec) {
		var (
			index = let.VarOf(s, 0)
		)
		act := let.Act2(func(t *testcase.T) (int, bool) {
			return ll.Get(t).Lookup(index.Get(t))
		})

		var whenIndexIsNegative = func(s *testcase.Spec) {
			s.When("index is negative", func(s *testcase.Spec) {
				index.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(-100, -1)
				})

				s.Then("not found is reported", func(t *testcase.T) {
					got, ok := act(t)
					assert.False(t, ok)
					assert.Empty(t, got)
				})
			})
		}

		s.When("list is empty", func(s *testcase.Spec) {
			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				return &datastruct.LinkedList[int]{}
			})

			s.Then("not found is reportad for any index", func(t *testcase.T) {
				v, ok := act(t)
				assert.Empty(t, v)
				assert.False(t, ok)
			})

			s.Then("length stays zero", func(t *testcase.T) {
				assert.Equal(t, 0, ll.Get(t).Length())
			})

			whenIndexIsNegative(s)
		})

		s.When("list has elements", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(
					t.Random.IntBetween(2, 5),
					t.Random.Int,
					random.UniqueValues,
				)
			})

			index.Let(s, func(t *testcase.T) int {
				return t.Random.IntN(len(values.Get(t)))
			})

			ll.Let(s, func(t *testcase.T) *datastruct.LinkedList[int] {
				var list datastruct.LinkedList[int]
				list.Append(values.Get(t)...)
				return &list
			})

			s.Then("the expected element is returned", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)

				exp, ok := slicekit.Lookup(values.Get(t), index.Get(t))
				assert.True(t, ok)

				assert.Equal(t, got, exp)
			})

			whenIndexIsNegative(s)
		})
	})
}
