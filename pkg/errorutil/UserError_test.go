package errorutil_test

import (
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestUserError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("when user error has no documentation", func(t *testcase.T) {
		usrErr := errorutil.UserError{
			ID:      "foo-bar-baz",
			Message: "foo is not ",
		}
		err := fmt.Errorf("wrapped: %w", usrErr)
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errors.As(err, &errorutil.UserError{}))
		t.Must.ErrorIs(usrErr, err)
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.ID)

		gotUsrErr := errorutil.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, usrErr))
	})

	s.Test("when user error .With used to add documentation", func(t *testcase.T) {
		usrErr := errorutil.UserError{
			ID:      "foo-bar-baz",
			Message: "foo is not ",
		}

		var err error = usrErr.With().Detail("some detail")
		err = fmt.Errorf("wrapped: %w", err)
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errors.As(err, &errorutil.UserError{}))
		t.Must.ErrorIs(usrErr, err)
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.ID)
		t.Must.Contain(err.Error(), "some detail")

		gotUsrErr := errorutil.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, usrErr))
	})

	s.Test("when it is not a user error", func(t *testcase.T) {
		err := fmt.Errorf("wrapped: %w", t.Random.Error())
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.False(ok)
		t.Must.Empty(gotUserErr)
		t.Must.False(errors.As(err, &errorutil.UserError{}))
		t.Must.False(errors.Is(err, errorutil.UserError{}))
	})
}
