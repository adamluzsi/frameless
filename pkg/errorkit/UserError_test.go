package errorkit_test

import (
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"github.com/adamluzsi/testcase"
)

func TestUserError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("when user error has no documentation", func(t *testcase.T) {
		usrErr := errorkit.UserError{
			ID:      "foo-bar-baz",
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
		t.Must.Contain(err.Error(), usrErr.ID)

		gotUsrErr := errorkit.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, usrErr))
	})

	s.Test("when user error .With used to add documentation", func(t *testcase.T) {
		usrErr := errorkit.UserError{
			ID:      "foo-bar-baz",
			Message: "foo is not ",
		}

		var err error = usrErr.With().Detail("some detail")
		err = fmt.Errorf("wrapped: %w", err)
		gotUserErr, ok := errorkit.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errors.As(err, &errorkit.UserError{}))
		t.Must.ErrorIs(usrErr, err)
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.ID)
		t.Must.Contain(err.Error(), "some detail")

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
