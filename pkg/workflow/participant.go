package workflow

import (
	"context"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
)

// Participant is a logical unit implemented at workflow engine-level.
//
// If ParticipantRepository is supplied to the workflow runtime context,
// then registered particpants can be used from within workflow definitions.
type Participant struct {
	ID   ParticipantID
	Func any // func(context.Context, ...) (..., error)
}

var _ validate.Validatable = (*Participant)(nil)

const ErrInvalidParicipantFunc errorkitlite.Error = `Invalid workflow.Participant#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (Result1, Result2..., error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple is also returns user defined types, ending with an error value type.
The input and output argument types must be serializable.
`

var reflectContextType = reflectkit.TypeOf[context.Context]()

var reflectErrorType = reflectkit.TypeOf[error]()

func (p Participant) Validate(ctx context.Context) error {
	rfunc := reflect.ValueOf(p.Func)
	if rfunc.Kind() != reflect.Func {
		return fmt.Errorf("invalid value for participant func")
	}
	var (
		funcType   = rfunc.Type()
		funcNumIn  = funcType.NumIn()
		funcNumOut = funcType.NumOut()
	)
	if funcNumIn < 1 {
		return ErrInvalidParicipantFunc
	}
	if funcType.In(0) != reflectContextType {
		return ErrInvalidParicipantFunc
	}
	if funcNumOut < 1 {
		return ErrInvalidParicipantFunc
	}
	if funcType.Out(funcNumOut-1) != reflectErrorType {
		return ErrInvalidParicipantFunc
	}
	return nil
}

func ContextWithParticipants(ctx context.Context, pr ParticipantRepository) context.Context {
	if pr == nil {
		return ctx
	}
	c, _ := ctxConfigH.Lookup(ctx)
	c.Participants = pr
	return ctxConfigH.ContextWith(ctx, c)
}

func PID[STR ~string](s STR) *ParticipantID {
	var pid = ParticipantID(s)
	return &pid
}

// ParticipantID is the process definition ID,
type ParticipantID string

var _ Definition = (*ParticipantID)(nil)

func (pid ParticipantID) Execute(ctx context.Context, s *State) error {
	if err := pid.Validate(ctx); err != nil {
		return err
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, found, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !found {
		return ErrParticipantNotFound{PID: pid}
	}
	return p.Execute(ctx, s)
}

func (pid ParticipantID) Validate(ctx context.Context) error {
	if len(pid) == 0 {
		return validate.Error{Cause: fmt.Errorf("empty participant ID")}
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !ok || p == nil {
		return ErrParticipantNotFound{PID: pid}
	}
	return nil
}
