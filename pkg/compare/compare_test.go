package compare_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/compare"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestNumbers(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		A = let.Int(s)
		B = let.Int(s)
	)
	act := let.Act(func(t *testcase.T) int {
		cmp := compare.Numbers(A.Get(t), B.Get(t))
		t.OnFail(func() {
			t.Log("cmp:", cmp)
		})
		return cmp
	})

	s.Before(func(t *testcase.T) {
		t.OnFail(func() {
			t.Log("A:", A.Get(t))
			t.Log("B:", B.Get(t))
		})
	})

	s.Then("comparison result returned", func(t *testcase.T) {
		got := act(t)

		assert.AnyOf(t, func(a *assert.A) {
			a.Case(func(t testing.TB) { assert.Equal(t, got, -1) })
			a.Case(func(t testing.TB) { assert.Equal(t, got, 0) })
			a.Case(func(t testing.TB) { assert.Equal(t, got, 1) })
		}, "expected that result is one of -1, 0 or 1")
	})

	s.When("A is equal to B", func(s *testcase.Spec) {
		A.LetValue(s, 42)
		B.LetValue(s, 42)

		s.Then("cmp is 0", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})

		s.Then("equality will be true", func(t *testcase.T) {
			assert.True(t, compare.IsEqual(act(t)))
		})

		s.Then("less will be false", func(t *testcase.T) {
			assert.False(t, compare.IsLess(act(t)))
		})

		s.Then("greater will be false", func(t *testcase.T) {
			assert.False(t, compare.IsMore(act(t)))
			assert.False(t, compare.IsGreater(act(t)))
		})

		s.Then("less or equal will be true", func(t *testcase.T) {
			assert.True(t, compare.IsLessOrEqual(act(t)))
		})

		s.Then("greater or equal will be true", func(t *testcase.T) {
			assert.True(t, compare.IsMoreOrEqual(act(t)))
			assert.True(t, compare.IsGreaterOrEqual(act(t)))
		})
	})

	s.When("A is less than B", func(s *testcase.Spec) {
		A.LetValue(s, 24)
		B.LetValue(s, 42)

		s.Then("cmp is -1", func(t *testcase.T) {
			assert.Equal(t, -1, act(t))
		})

		s.Then("equality will be false", func(t *testcase.T) {
			assert.False(t, compare.IsEqual(act(t)))
		})

		s.Then("less will be true", func(t *testcase.T) {
			assert.True(t, compare.IsLess(act(t)))
		})

		s.Then("greater will be false", func(t *testcase.T) {
			assert.False(t, compare.IsMore(act(t)))
			assert.False(t, compare.IsGreater(act(t)))
		})

		s.Then("less or equal will be true", func(t *testcase.T) {
			assert.True(t, compare.IsLessOrEqual(act(t)))
		})

		s.Then("greater or equal will be false", func(t *testcase.T) {
			assert.False(t, compare.IsMoreOrEqual(act(t)))
			assert.False(t, compare.IsGreaterOrEqual(act(t)))
		})
	})

	s.When("A is greater than B", func(s *testcase.Spec) {
		A.LetValue(s, 42)
		B.LetValue(s, 24)

		s.Then("cmp is 1", func(t *testcase.T) {
			assert.Equal(t, 1, act(t))
		})

		s.Then("equality will be false", func(t *testcase.T) {
			assert.False(t, compare.IsEqual(act(t)))
		})

		s.Then("less will be false", func(t *testcase.T) {
			assert.False(t, compare.IsLess(act(t)))
		})

		s.Then("greater will be true", func(t *testcase.T) {
			assert.True(t, compare.IsMore(act(t)))
			assert.True(t, compare.IsGreater(act(t)))
		})

		s.Then("less or equal will be false", func(t *testcase.T) {
			assert.False(t, compare.IsLessOrEqual(act(t)))
		})

		s.Then("greater or equal will be true", func(t *testcase.T) {
			assert.True(t, compare.IsMoreOrEqual(act(t)))
			assert.True(t, compare.IsGreaterOrEqual(act(t)))
		})
	})
}
