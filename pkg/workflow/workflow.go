package workflow

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase/clock"
)

// Definition is the unit of work in a workflow process.
//
// A Definition is intentionally stateless:
// it carries no per-process state of its own.
// All state that affects execution lives in the process' event history,
// which the workflow.Runtime reads on every Execute call.
//
// A Definition must also be safely serialisable by the Runtime#Codec.
// Definitions are persisted N as part of the event history (see UseDefinitionEvent),
// and they are round-tripped through the Runtime#Codec
// so a process can be serialised, saved, loaded, replayed, migrated,
// or inspected across process restarts.
//
// A Definition must be idempotent when executed against the same process (ProcessID).
// The runtime may replay any Execute call as part of crash recovery, retries,
// or scheduler requeueing, and the side effects on the process must converge to the same final state.
// Implementations typically lean on the `workflow.Runtime`'s idempotent event history for this:
// each step records its outcome as an event,
// and a replay skips work whose event is already present.
//
// The workflow.Runtime do its best to ensure that one workflow process' Definition
// is only by a single workflow node if the workflow engine runs in a H.A. setup.
type Definition interface {
	Execute(ctx context.Context, processID ProcessID) error
	// error is embedded in the Definition interface,
	// as a Participant function might decided that after it is completed,
	// it wishes to extend its own life-cycle with further workflow Definition stages.
	//
	// Example:
	//
	//	Sequence{ StepA, StepB, StepC }
	//	// StepB's is a participant execution,
	//  // and it decides that it require follow-up workflow stages.
	//  // So instead to return with nil, it returns with Definition.
	//	// The runtime persists StepB's call results (with the new Definition attached)
	//	// and continues with that Definition in place of StepB,
	//  // effectively, turning the execution into:
	//	Sequence{ StepA, StepB, StepBSubDef, StepC }
	error
}

type Condition interface {
	Evaluate(ctx context.Context, processID ProcessID) (bool, error)
}

// Codec is the (de)serialisation contract used by the workflow package and its
// adapters. It is intentionally minimal — Marshal + Unmarshal — so that any
// implementation of port/codec.Codec qualifies, and so that new wire formats
// (JSON, YAML, msgpack, ...) can be exercised against the same wfcontract.Codec
// contract test.
type Codec interface {
	codec.Codec
}

type ConditionID string

type ConditionConvertible interface {
	ToCondition(ctx context.Context, processID ProcessID) (Condition, bool)
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ProcessID is a UUID v7 which is used to identify workflows.
// UUID v7 should allow underlying EventRepository implementations to use the UUID as a ever increasing ordered unique value.
type ProcessID = uuid.UUID

func MakeProcessID() (ProcessID, error) {
	return uuid.MakeV7()
}

type Event interface {
	EventType() EventType
	GetEventID() EventID
	GetProcessID() ProcessID
	GetTimestamp() time.Time
	// GetPath() Path .
}

type EventType string

type EventID = uuid.UUID

func MakeEventID() (EventID, error) {
	return uuid.MakeV7()
}

func sortEvents(events []Event) {
	slicekit.SortBy(events, func(a, b Event) bool {
		return a.GetEventID().Less(b.GetEventID())
	})
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Participant is a logical unit implemented at workflow engine-level.
//
// If ParticipantRepository is supplied to the workflow runtime context,
// then registered participants can be used from within workflow definitions.
type Participant struct {
	ID   ParticipantID
	Func any // func(context.Context, ...) (..., error)
}

// funcSignature
//
// TODO: replace with OpenAPI definition
func (p Participant) funcSignature() string {
	var fn, err = p.rFunc()
	if err != nil {
		return ""
	}
	var (
		fnType = fn.Type()
		input  []string
		output []string
	)
	for i := range fnType.NumIn() {
		in := fnType.In(i)
		val := in.String()
		if in.IsVariadic() {
			val = "..." + val
		}
		input = append(input, in.String())
	}
	for i := range fnType.NumOut() {
		output = append(output, fnType.Out(i).String())
	}
	return fmt.Sprintf("func(%s) (%s)", strings.Join(input, ", "), strings.Join(output, ", "))
}

type ParticipantID string

func (id ParticipantID) String() string { return string(id) }

var _ validate.Validatable = (*Participant)(nil)

const ErrInvalidParticipantFunc errorkitlite.Error = `Invalid workflow.Participant#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (Result1, Result2..., error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple is also returns user defined types, ending with an error value type.
The input and output argument types must be serializable.
`

var reflectContextType = reflectkit.TypeOf[context.Context]()

var reflectErrorType = reflectkit.TypeOf[error]()

func (p Participant) rFunc() (reflect.Value, error) {
	rFunc := reflect.ValueOf(p.Func)
	if rFunc.Kind() != reflect.Func {
		return rFunc, ErrInvalidParticipantFunc.F("invalid value for participant func")
	}
	var (
		funcType   = rFunc.Type()
		funcNumIn  = funcType.NumIn()
		funcNumOut = funcType.NumOut()
	)
	if funcNumIn < 1 {
		return rFunc, ErrInvalidParticipantFunc
	}
	if funcType.In(0) != reflectContextType {
		return rFunc, ErrInvalidParticipantFunc
	}
	if funcNumOut < 1 {
		return rFunc, ErrInvalidParticipantFunc
	}
	if lastOut := funcType.Out(funcNumOut - 1); lastOut != reflectErrorType || !lastOut.Implements(reflectErrorType) {
		return rFunc, ErrInvalidParticipantFunc
	}
	return rFunc, nil
}

func (p Participant) Validate(ctx context.Context) error {
	_, err := p.rFunc()
	return err
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Participants map[ParticipantID]any

var _ ParticipantRepository = (Participants)(nil)

func (ps Participants) FindByID(ctx context.Context, id ParticipantID) (Participant, bool, error) {
	if len(ps) == 0 {
		var zero Participant
		return zero, false, nil
	}
	fn, ok := ps[id]
	return Participant{ID: id, Func: fn}, ok, nil
}

func (ps Participants) Validate(ctx context.Context) error {
	for id, fn := range ps {
		p := Participant{
			ID:   id,
			Func: fn,
		}
		if err := p.Validate(ctx); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
}

func timeNow() time.Time {
	return clock.Now().UTC()
}
