package datastructcontract

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/datastruct"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

type SubjectContainable[T any] interface {
	datastruct.Containable[T]
	datastruct.Appendable[T]
}

func Containable[T any, Subject SubjectContainable[T]](mk func(testing.TB) Subject, opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	subject := let.Var(s, func(t *testcase.T) Subject {
		return mk(t)
	})

	SubjectName := reflectkit.TypeOf[Subject]().String()
	TypeName := reflectkit.TypeOf[T]().String()

	s.Describe("#Contains", func(s *testcase.Spec) {
		var (
			element = let.Var(s, func(t *testcase.T) T {
				return c.makeElem(t)
			})
		)
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).Contains(element.Get(t))
		})

		s.When("element is present", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				subject.Get(t).Append(element.Get(t))
			})

			s.Then("it "+SubjectName+" will contains the "+TypeName+" value", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("element is absent", func(s *testcase.Spec) {
			// nothing to do, it should be absent by default

			s.Then("it "+SubjectName+" will NOT contains the checked "+TypeName+" value", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	return s.AsSuite("Containable")
}
