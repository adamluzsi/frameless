package validate

import "go.llib.dev/frameless/pkg/errorkit"

// ValidationError is a validation error, that represents an incorrect content.
type ValidationError struct{ Cause error }

func (err ValidationError) Error() string {
	var msg = "[ValidationError]"
	if err.Cause != nil {
		msg += ": " + err.Cause.Error()
	}
	return msg
}

func (err ValidationError) Unwrap() error {
	return err.Cause
}

const ImplementationError errorkit.Error = "ImplementationError"
