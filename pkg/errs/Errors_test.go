package errs_test

import (
	"github.com/adamluzsi/frameless/pkg/errs"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestErrors(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) errs.Errors {
		return errs.Errors{}
	})

	s.Describe(".Error", func(s *testcase.Spec) {
		act := func(t *testcase.T) string {
			return subject.Get(t).Error()
		}

		s.Then("on empty", func(t *testcase.T) {
			t.Must.Empty(act(t))
		})

		s.When("a valid error value is part of the error list", func(s *testcase.Spec) {
			expectedErr := testcase.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Before(func(t *testcase.T) {
				subject.Set(t, append(subject.Get(t), expectedErr.Get(t)))
			})

			s.Then("error value is returned", func(t *testcase.T) {
				t.Must.Equal(expectedErr.Get(t).Error(), act(t))
			})
		})

		s.When("multiple value is present in the error list", func(s *testcase.Spec) {
			var (
				expectedErr1 = testcase.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})
				expectedErr2 = testcase.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})
			)

			s.Before(func(t *testcase.T) {
				subject.Set(t, append(subject.Get(t), expectedErr1.Get(t), expectedErr2.Get(t)))
			})

			s.Then("error value is returned", func(t *testcase.T) {
				out := act(t)
				t.Must.Contain(out, expectedErr1.Get(t).Error())
				t.Must.Contain(out, expectedErr2.Get(t).Error())
			})
		})
	})

	s.Describe(".Err", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Err()
		}

		s.Then("on empty", func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.When("a valid error value is part of the error list", func(s *testcase.Spec) {
			expectedErr := testcase.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Before(func(t *testcase.T) {
				subject.Set(t, append(subject.Get(t), expectedErr.Get(t)))
			})

			s.Then("error value is returned", func(t *testcase.T) {
				t.Must.ErrorIs(expectedErr.Get(t), act(t))
			})
		})

		s.When("multiple value is present in the error list", func(s *testcase.Spec) {
			var (
				expectedErr1 = testcase.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})
				expectedErr2 = testcase.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})
			)

			s.Before(func(t *testcase.T) {
				subject.Set(t, append(subject.Get(t), expectedErr1.Get(t), expectedErr2.Get(t)))
			})

			s.Then("error value is returned", func(t *testcase.T) {
				out := act(t).(errs.Errors)
				t.Must.Contain(out, expectedErr1.Get(t))
				t.Must.Contain(out, expectedErr2.Get(t))
			})
		})
	})
}
