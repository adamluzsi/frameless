package errorutil

import (
	"errors"
)

func IsUserError(err error) bool {
	_, ok := LookupUserError(err)
	return ok
}

func LookupUserError(err error) (UserError, bool) {
	var ue UserError
	return ue, errors.As(err, &ue)
}

type UserError struct {
	// Err is the wrapped error which is the Cause of this UserError.
	Err error
	// Code is a constant string value that expresses the user's error scenario.
	// The caller who receives the error will use this code to present the UserError to their users and,
	// most likely, provide a localised error message about the error scenario to the end user.
	// Traditionally this should be a string without any white space.
	// Example: "foo-is-forbidden-with-active-baz"
	Code string
	// Message is the error message meant to be read by a developer working on the implementation of the caller.
	// It is not expected to be seen by end users.
	// It should be written in English for portability reasons.
	Message string
}

func (err UserError) Error() string {
	return err.Err.Error()
}

func (err UserError) Unwrap() error {
	return err.Err
}

func (err UserError) Is(target error) bool {
	return target == UserError{}
}
