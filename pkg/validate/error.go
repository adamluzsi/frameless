package validate

import "go.llib.dev/frameless/pkg/errorkit"

// Error is a validation error, that represents an incorrect content.
type Error struct{ Cause error }

func (err Error) Error() string {
	var msg = "[ValidationError]"
	if err.Cause != nil {
		msg += ": " + err.Cause.Error()
	}
	return msg
}

func (err Error) Unwrap() error {
	return err.Cause
}

const ImplementationError errorkit.Error = "ImplementationError"
