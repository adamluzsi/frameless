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

	s.Test("when user error is a wrapper to specify an underlying error", func(t *testcase.T) {
		expectedErr := t.Random.Error()
		usrErr := errorutil.UserError{
			Err:     expectedErr,
			Code:    t.Random.UUID(),
			Message: t.Random.Error().Error(),
		}
		err := fmt.Errorf("wrapped: %w", usrErr)
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errorutil.IsUserError(err))
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), expectedErr.Error())
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.Code)
		t.Must.ErrorIs(expectedErr, err)

		gotUsrErr := errorutil.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, errorutil.UserError{}))
	})

	s.Test("when user error is used as a standalone error", func(t *testcase.T) {
		usrErr := errorutil.UserError{
			Code:    t.Random.UUID(),
			Message: t.Random.Error().Error(),
		}
		err := fmt.Errorf("wrapped: %w", usrErr)
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.True(ok)
		t.Must.Equal(usrErr, gotUserErr)
		t.Must.True(errorutil.IsUserError(err))
		t.Must.ErrorIs(usrErr, err)
		t.Must.Contain(err.Error(), "wrapped: ")
		t.Must.Contain(err.Error(), usrErr.Message)
		t.Must.Contain(err.Error(), usrErr.Code)

		gotUsrErr := errorutil.UserError{}
		t.Must.True(errors.As(err, &gotUsrErr))
		t.Must.Equal(usrErr, gotUsrErr)
		t.Must.True(errors.Is(err, errorutil.UserError{}))
	})

	s.Test("when it is not a user error", func(t *testcase.T) {
		err := fmt.Errorf("wrapped: %w", t.Random.Error())
		gotUserErr, ok := errorutil.LookupUserError(err)
		t.Must.False(ok)
		t.Must.Empty(gotUserErr)
		t.Must.False(errorutil.IsUserError(err))
		t.Must.False(errors.As(err, &errorutil.UserError{}))
		t.Must.False(errors.Is(err, errorutil.UserError{}))
	})
}
