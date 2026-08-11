package workflow

import (
	"errors"
	"fmt"

	"go.llib.dev/frameless/internal/errorkitlite"
)

// ErrIsFatal reports whether the error is an non-recoverable workflow related issue, and retry attempt should not be attempted.
func ErrIsFatal(err error) bool {
	return errors.Is(err, ErrInvalidDefinition) ||
		errors.Is(err, ErrInvalidParticipantFunc) ||
		errors.Is(err, ErrInvalidConditionFunc) ||
		errors.As(err, &ErrParticipantNotFound{}) ||
		errors.As(err, &ErrConditionNotFound{}) ||
		errors.Is(err, ErrNoContextRuntime) ||
		errors.Is(err, ErrNoProcessDefinition) ||
		errors.Is(err, ErrFatal)
}

const ErrFatal errorkitlite.Error = "WORKFLOW_FATAL_ERROR"

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

const ErrInvalidConditionFunc errorkitlite.Error = `Invalid workflow.Condition#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (bool, error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple returns bool as first value and an error value type as last.
The input argument types must be serializable.
`

const ErrAlreadyRunningProcess errorkitlite.Error = "The given workflow process is already in execution and therefore busy somewhere else"

const ErrNoProcessDefinition errorkitlite.Error = "The given workflow process doesn't have a workflow definition to executed, perhaps workflow.Runtime#Bind is forgotten"

const ErrNoContextRuntime errorkitlite.Error = "current context doesn't have workflow.Runtime in it"
