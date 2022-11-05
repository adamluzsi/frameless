package errorutil_test

import (
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
)

func ExampleUserError() {
	err := fmt.Errorf("foo bar baz")
	usrErr := errorutil.UserError(err)
	// usrerr.Error() == "foo bar baz"
	errors.Is(usrErr, err)        // true
	errorutil.IsUserError(usrErr) // true
	errorutil.IsUserError(err)    // false
}

func ExampleIsUserError() {
	err := fmt.Errorf("foo bar baz")
	errorutil.IsUserError(err) // false

	err = errorutil.UserError(err)
	errorutil.IsUserError(err) // true
}

func ExampleMerge() {
	// creates an error value that combines the input errors.
	err := errorutil.Merge(fmt.Errorf("foo"), fmt.Errorf("bar"), nil)
	_ = err
}
