package errorutil_test

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestUserError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("when error is a user error", func(t *testcase.T) {
		expectedErr := t.Random.Error()
		err := errorutil.UserError(expectedErr)
		err = fmt.Errorf("wrapped: %w", err)
		t.Must.True(errorutil.IsUserError(err))
		t.Must.Equal(fmt.Sprintf("wrapped: %s", expectedErr.Error()), err.Error())
		t.Must.ErrorIs(expectedErr, err)
	})

	s.Test("when it is not a user error", func(t *testcase.T) {
		t.Must.False(errorutil.IsUserError(t.Random.Error()))
	})
}
