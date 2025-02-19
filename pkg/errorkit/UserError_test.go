package errorkit_test

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func ExampleUserError() {
	fooEntityID := rand.Int()
	bazEntityID := rand.Int()

	usrerr := errorkit.UserError{
		Code:    "foo-is-forbidden-with-active-baz",
		Message: "It is forbidden to execute Foo when you have an active Baz",
	}

	var err error = usrerr.F("Foo(ID:%d) /Baz(ID:%d)", fooEntityID, bazEntityID)

	// add some details using error wrapping
	err = fmt.Errorf("some wrapper layer: %w", err)

	// retrieve with errorkit.As
	if ue, ok := errorkit.As[errorkit.UserError](err); ok {
		fmt.Printf("%#v\n", ue)
	}
	// retrieve with errors pkg
	if ue := (errorkit.UserError{}); errors.As(err, &ue) {
		fmt.Printf("%#v\n", ue)
	}
	if errors.Is(err, errorkit.UserError{}) {
		fmt.Println("it's a Layer 8 error")
	}

	// retrieve with errorkit pkg
	if userError, ok := errorkit.LookupUserError(err); ok {
		fmt.Printf("%#v\n", userError)
	}
}

func ExampleLookupUserError() {
	err := errorkit.UserError{
		Code:    "constant-err-scenario-code",
		Message: "some message for the dev",
	}
	if userError, ok := errorkit.LookupUserError(err); ok {
		fmt.Printf("%#v\n", userError)
	}
}

func TestUserError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("when user error has no documentation", func(t *testcase.T) {
		usrErr := errorkit.UserError{
			Code:    "foo-bar-baz",
			Message: "foo is not ",
		}
		err := fmt.Errorf("wrapped: %w", usrErr)
		gotUserErr, ok := errorkit.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errors.As(err, &errorkit.UserError{}))
		t.Must.ErrorIs(usrErr, err)
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.Code)

		gotUsrErr := errorkit.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, usrErr))
	})

	s.Test("when it is not a user error", func(t *testcase.T) {
		err := fmt.Errorf("wrapped: %w", t.Random.Error())
		gotUserErr, ok := errorkit.LookupUserError(err)
		t.Must.False(ok)
		t.Must.Empty(gotUserErr)
		t.Must.False(errors.As(err, &errorkit.UserError{}))
		t.Must.False(errors.Is(err, errorkit.UserError{}))
	})
}

func TestUserError_F_smoke(t *testing.T) {
	var ErrExample = errorkit.UserError{
		Code:    "the-error-unique-id-or-code",
		Message: "the message",
	}
	t.Run("sprintf", func(t *testing.T) {
		got := ErrExample.F("foo - bar - %s", "baz")
		assert.ErrorIs(t, got, ErrExample)
		assert.Contain(t, got.Error(), "foo - bar - baz")
	})
	t.Run("errorf", func(t *testing.T) {
		exp := rnd.Error()
		got := ErrExample.F("%w", exp)
		assert.ErrorIs(t, got, ErrExample)
		assert.ErrorIs(t, got, exp)
		assert.Contain(t, got.Error(), ErrExample.Error())
	})
}

func TestUserError_traced(t *testing.T) {
	const ErrBase errorkit.Error = "base error"

	var assertTraced = func(t *testing.T, err error) {
		var traced errorkit.Traced
		assert.True(t, errors.As(err, &traced))
		assert.NotNil(t, traced.Err)
		assert.NotEmpty(t, traced.Trace)
	}

	assertTraced(t, ErrBase.F("traced"))
	assertTraced(t, ErrBase.Wrap(rnd.Error()))
}
