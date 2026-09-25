package workflow

import (
	"errors"
	"fmt"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
)

// ErrIsFatal reports whether the error is an non-recoverable workflow related issue, and retry attempt should not be attempted.
func ErrIsFatal(err error) bool {
	return errors.Is(err, ErrInvalidDefinition) ||
		isInvalidParticipantFuncOutsideMismatch(err) ||
		errors.Is(err, ErrInvalidConditionFunc) ||
		errors.As(err, &ErrConditionNotFound{}) ||
		errors.Is(err, ErrNoContextRuntime) ||
		errors.Is(err, ErrNoProcessDefinition) ||
		errors.Is(err, ErrFatal)
}

// Only the mismatch's legacy invalid-function cause is exempt. Inspect each
// branch separately so an independent invalid function in a join remains fatal.
func isInvalidParticipantFuncOutsideMismatch(err error) bool {
	switch err := err.(type) {
	case ErrParticipantSignatureMismatch, *ErrParticipantSignatureMismatch:
		return false
	// These legacy containers expose their causes through recursive Is/As
	// methods rather than Unwrap, so inspect their branches explicitly too.
	case errorkitlite.W:
		return isInvalidParticipantFuncOutsideMismatch(err.E) || isInvalidParticipantFuncOutsideMismatch(err.W)
	case errorkitlite.MultiError:
		for _, cause := range err {
			if isInvalidParticipantFuncOutsideMismatch(cause) {
				return true
			}
		}
		return false
	}
	if err == ErrInvalidParticipantFunc {
		return true
	}
	if matcher, ok := err.(interface{ Is(error) bool }); ok && matcher.Is(ErrInvalidParticipantFunc) {
		return true
	}
	switch err := err.(type) {
	case interface{ Unwrap() error }:
		return isInvalidParticipantFuncOutsideMismatch(err.Unwrap())
	case interface{ Unwrap() []error }:
		for _, cause := range err.Unwrap() {
			if isInvalidParticipantFuncOutsideMismatch(cause) {
				return true
			}
		}
	}
	return false
}

const ErrFatal errorkitlite.Error = "WORKFLOW_FATAL_ERROR"

// ErrInvalidDefinition is an error raised for invalid definition composition.
const ErrInvalidDefinition errorkitlite.Error = "ErrInvalidDefinition"

// ErrParticipantNotFound reports that this node does not have the requested
// participant. It is not fatal and is not retried locally: the scheduler requeues
// the process so another node can resume it, increasing FailureCount and warning
// periodically if availability does not recover.
type ErrParticipantNotFound struct{ ID ParticipantID }

func (err ErrParticipantNotFound) Error() string {
	return fmt.Sprintf("[%T] %s", err, err.ID)
}

// ErrParticipantSignatureMismatch reports that this node's participant cannot
// satisfy the requested invocation. It is a nonfatal availability error; Cause
// preserves the invalid-function or mapping error for errors.Is/errors.As.
type ErrParticipantSignatureMismatch struct {
	ID    ParticipantID
	Cause error
}

func (err ErrParticipantSignatureMismatch) Error() string {
	if err.Cause == nil {
		return fmt.Sprintf("[%T] %s", err, err.ID)
	}
	return fmt.Sprintf("[%T] %s: %v", err, err.ID, err.Cause)
}

func (err ErrParticipantSignatureMismatch) Unwrap() error { return err.Cause }

// Is matches the participant ID independently of the diagnostic cause.
func (err ErrParticipantSignatureMismatch) Is(target error) bool {
	switch target := target.(type) {
	case ErrParticipantSignatureMismatch:
		return err.ID == target.ID
	case *ErrParticipantSignatureMismatch:
		return target != nil && err.ID == target.ID
	default:
		return false
	}
}

func isParticipantSignatureMismatch(err error) bool {
	var value ErrParticipantSignatureMismatch
	var pointer *ErrParticipantSignatureMismatch
	return errors.As(err, &value) || errors.As(err, &pointer)
}

func isParticipantNotFound(err error) bool {
	return errors.As(err, &ErrParticipantNotFound{})
}

type ErrConditionNotFound struct{ ID ConditionID }

func (e ErrConditionNotFound) Error() string {
	return fmt.Sprintf("[ErrConditionNotFound] %s", e.ID)
}

const ErrParticipantFuncMappingMismatch errorkitlite.Error = "ErrParticipantFuncMappingMismatch"

const ErrConditionFuncMappingMismatch errorkitlite.Error = "ErrConditionFuncMappingMismatch"

const ErrInvalidConditionFunc errorkitlite.Error = `Invalid workflow.Condition#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (bool, error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple returns bool as first value and an error value type as last.
The input argument types must be serializable.
`

const ErrAlreadyRunningProcess errorkitlite.Error = "The given workflow process is already in execution and therefore busy somewhere else"

const ErrNoProcessDefinition errorkitlite.Error = "The given workflow process doesn't have a workflow definition to executed, perhaps workflow.Runtime#Bind is forgotten"

const ErrNoContextRuntime errorkitlite.Error = "current context doesn't have workflow.Runtime in it"

type EventError struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Error string
	Path  Path

	ParticipantID ParticipantID
}

var _ Event = EventError{}

func (e EventError) EventType() EventType    { return "workflow::error" }
func (e EventError) GetEventID() EventID     { return e.EventID }
func (e EventError) GetProcessID() ProcessID { return e.ProcessID }
func (e EventError) GetTimestamp() time.Time { return e.Timestamp }
