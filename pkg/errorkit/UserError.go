package errorkit

import (
	"fmt"

	"go.llib.dev/frameless/internal/constant"
)

type UserError struct {
	// ID is a constant string value that expresses the user's error scenario.
	// The caller who receives the error will use this code to present the UserError to their users and,
	// most likely, provide a localised error message about the error scenario to the end user.
	// Traditionally this should be a string without any white space.
	//
	// Example: "foo-is-forbidden-with-active-baz"
	ID constant.String
	// Message is the error message meant to be read by a developer working on the implementation of the caller.
	// It is not expected to be seen by end users.
	// It might be written in English for portability reasons.
	//
	// Example: "Authentication failed due to incorrect username or password."
	Message constant.String
}

func (err UserError) Error() string {
	return "[" + string(err.ID) + "] " + string(err.Message)
}

func (err UserError) Wrap(oth error) error {
	if oth == nil {
		return err
	}
	return WithTrace(wrapF("[%s] %s", err, oth))
}

// F will format the error value
func (err UserError) F(format string, a ...any) error {
	return err.Wrap(fmt.Errorf(format, a...))
}

func LookupUserError(err error) (UserError, bool) {
	return As[UserError](err)
}
