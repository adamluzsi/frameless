package errorkit

import (
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

var (
	_ ErrorUnwrap = withContextError{}

	_ ErrorAs = wrapper{}
	_ ErrorIs = wrapper{}

	_ ErrorIs = errorkitlite.MultiError{}
	_ ErrorAs = errorkitlite.MultiError{}
)

type ErrorUnwrap interface {
	Unwrap() error
}

type ErrorAs interface {
	As(target any) bool
}

type ErrorIs interface {
	Is(target error) bool
}

type ErrT struct{ V any }

func (err ErrT) Error() string { return fmt.Sprintf("%T:%v", err, err.V) }

func TestWrap(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		err = testcase.Let(s, func(t *testcase.T) ErrT {
			return ErrT{V: t.Random.Contact().Email}
		})
		usrErr = testcase.Let(s, func(t *testcase.T) UserError {
			return UserError{
				Code:    "42",
				Message: "The answer to the ultimate question of life, the universe, and everything.",
			}
		})
	)
	act := func(t *testcase.T) error {
		return wrapF("%s:%s", usrErr.Get(t), err.Get(t))
	}

	s.Then("wrapped error can be checked with errors.Is", func(t *testcase.T) {
		gotErr := act(t)
		assert.True(t, errors.Is(gotErr, usrErr.Get(t)))
		assert.True(t, errors.Is(gotErr, err.Get(t)))
	})

	s.Then("wrapped error can be checked with errors.As", func(t *testcase.T) {
		gotErr := act(t)
		var gotUsrErr UserError
		assert.True(t, errors.As(gotErr, &gotUsrErr))
		assert.Must(t).Equal(usrErr.Get(t), gotUsrErr)
		var gotErrT ErrT
		assert.True(t, errors.As(gotErr, &gotErrT))
		assert.Must(t).Equal(err.Get(t), gotErrT)
	})
}
