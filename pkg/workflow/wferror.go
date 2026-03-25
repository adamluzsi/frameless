package workflow

import (
	"errors"
	"fmt"

	"go.llib.dev/frameless/internal/errorkitlite"
)

// ErrIsFatal reports whether the error is an non-recoverable workflow related issue, and retry attempt should not be attempted.
func ErrIsFatal(err error) bool {
	return errors.Is(err, ErrInvalidDefinition) ||
		errors.Is(err, ErrInvalidParicipantFunc) ||
		errors.As(err, &ErrParticipantNotFound{}) ||
		errors.As(err, &ErrConditionNotFound{}) ||
		errors.Is(err, ErrFatal)
}

const ErrFatal errorkitlite.Error = "WFFATALERROR"

// ErrInvalidDefinition is an error raised for invalid definition composition.
const ErrInvalidDefinition errorkitlite.Error = "ErrInvalidDefinition"

type ErrParticipantNotFound struct{ ID ParticipantID }

func (err ErrParticipantNotFound) Error() string {
	return fmt.Sprintf("[%T] %s", err, err.ID)
}

type ErrConditionNotFound struct{ ID ConditionID }

func (e ErrConditionNotFound) Error() string {
	return fmt.Sprintf("[ErrConditionNotFound] %s", e.ID)
}

const ErrParticipantFuncMappingMismatch errorkitlite.Error = "ErrParticipantFuncMappingMismatch"
