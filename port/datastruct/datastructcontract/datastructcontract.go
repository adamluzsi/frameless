package datastructcontract

import (
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/datastruct"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Appendable[T any, Subject datastruct.Appendable[T]](mk func(tb testing.TB) Subject, opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	var subject = let.Var(s, func(t *testcase.T) Subject {
		return mk(t)
	})

	s.Describe("#Append", func(s *testcase.Spec) {
		var (
			vs = let.Var(s, func(t *testcase.T) []T {
				return random.Slice(t.Random.IntBetween(0, 7), func() T {
					return c.makeElem(t)
				}, random.UniqueValues)
			})
		)
		act := let.Act0(func(t *testcase.T) {
			subject.Get(t).Append(vs.Get(t)...)
		})

		if sub, ok := testcase.Implements[datastruct.Iterable[T]](subject); ok {
			s.Then("appended values should be returned during iteration", func(t *testcase.T) {
				act(t)

				assert.Contains(t, vs.Get(t), iterkit.Collect(sub.Get(t).Iter()))
			})
		}

		if sub, ok := testcase.Implements[datastruct.Slicer[T]](subject); ok {
			s.Then("appended values retrievable through #Slice()", func(t *testcase.T) {
				act(t)

				assert.ContainsExactly(t, vs.Get(t), sub.Get(t).Slice())
			})
		}

		if sub, ok := testcase.Implements[datastruct.Sizer](subject); ok {
			s.Then("appending values will affect the length of the container", func(t *testcase.T) {
				assert.Empty(t, sub.Get(t).Len())

				act(t)

				assert.Equal(t, len(vs.Get(t)), sub.Get(t).Len())
			})
		}

		if sub, ok := testcase.Implements[datastruct.Sequence[T]](subject); ok {
			s.Then("Lookup will return the appended entries by their sequential index", func(t *testcase.T) {
				act(t)

				for i, exp := range vs.Get(t) {
					got, ok := sub.Get(t).Lookup(i)
					assert.True(t, ok)
					assert.Equal(t, exp, got)
				}
			})
		}
	})

	return s.AsSuite("Appendable")
}
