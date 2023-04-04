package zeroutil_test

import (
	"github.com/adamluzsi/frameless/pkg/internal/zeroutil"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/let"
	"testing"
)

func TestCoalesce(t *testing.T) {
	s := testcase.NewSpec(t)

	var values = testcase.LetValue[[]int](s, nil)

	act := func(t *testcase.T) int {
		return zeroutil.Coalesce(values.Get(t)...)
	}

	s.When("values are empty", func(s *testcase.Spec) {
		values.LetValue(s, nil)

		s.Then("zero value is returned", func(t *testcase.T) {
			t.Must.Equal(*new(int), act(t))
		})
	})

	s.When("values have a single non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{expected.Get(t)}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})

	s.When("values have multiple values, but the first one is the non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{expected.Get(t), 0, 0}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})

	s.When("values have multiple values, but not the first one is the non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{0, expected.Get(t), 0}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})
}
