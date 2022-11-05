package errorutil

import (
	"errors"
	"testing"

	"github.com/adamluzsi/testcase"
)

func TestErrors(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) multiError {
		return multiError{}
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

	s.Describe(".As or errors.As(err, target)", func(s *testcase.Spec) {
		var (
			target = testcase.Let[*TargetErrorType](s, func(t *testcase.T) *TargetErrorType {
				return &TargetErrorType{}
			})
		)
		act := func(t *testcase.T) bool {
			return errors.As(subject.Get(t), target.Get(t))
		}

		s.When("error list is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Empty(subject.Get(t))
			})

			s.Then("target value not found", func(t *testcase.T) {
				t.Must.False(act(t))
			})
		})

		s.When("error values are present", func(s *testcase.Spec) {
			var (
				expectedErr1 = testcase.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})
				expectedErr2 = testcase.Let[error](s, nil)
			)

			s.Before(func(t *testcase.T) {
				subject.Set(t, append(subject.Get(t), expectedErr1.Get(t), expectedErr2.Get(t)))
			})

			s.And("target is part of them", func(s *testcase.Spec) {
				expectedErr2.Let(s, func(t *testcase.T) error {
					return TargetErrorType{ID: t.Random.Int()}
				})

				s.Then("then target is found", func(t *testcase.T) {
					t.Must.True(act(t))

					t.Must.Equal(*target.Get(t), expectedErr2.Get(t))
				})
			})

			s.And("target is not part of them", func(s *testcase.Spec) {
				expectedErr2.Let(s, func(t *testcase.T) error {
					return t.Random.Error()
				})

				s.Then("then target is not found", func(t *testcase.T) {
					t.Must.False(act(t))
				})
			})
		})
	})
}

func TestToErr_slice(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errs = testcase.Let[[]error](s, nil)
	)
	act := func(t *testcase.T) error {
		return Merge(errs.Get(t)...)
	}

	s.When("error list is empty", func(s *testcase.Spec) {
		errs.Let(s, func(t *testcase.T) []error {
			return []error{}
		})

		s.Then("it returns nil", func(t *testcase.T) {
			t.Must.Nil(act(t))
		})
	})

	s.When("a valid error value is part of the error list", func(s *testcase.Spec) {
		expectedErr := testcase.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})
		errs.Let(s, func(t *testcase.T) []error {
			return []error{expectedErr.Get(t)}
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
		errs.Let(s, func(t *testcase.T) []error {
			return []error{expectedErr1.Get(t), expectedErr2.Get(t)}
		})

		s.Then("error value is returned", func(t *testcase.T) {
			out := act(t).(multiError)
			t.Must.Contain(out, expectedErr1.Get(t))
			t.Must.Contain(out, expectedErr2.Get(t))
		})
	})
}

type TargetErrorType struct {
	ID int
}

func (err TargetErrorType) Error() string {
	return "42"
}
