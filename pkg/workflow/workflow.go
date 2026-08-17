package workflow

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/comp"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/ds"
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
}

type EventType string

type EventID = uuid.UUID

func MakeEventID() (EventID, error) {
	return uuid.MakeV7()
}

// Complete is a completion RuntimeSignal.
type Complete struct {
	ProcessID ProcessID
}

var _ RuntimeSignal = Complete{}

func (sig Complete) Error() string { return "workflow::complete" }

func (sig Complete) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	for event, err := range rt.Events.FindByProcessID(ctx, sig.ProcessID) {
		if err != nil {
			return err
		}
		if completed, ok := event.(EventCompleted); ok && completed.ProcessID.Equal(sig.ProcessID) {
			return nil
		}
	}
	var eventID, err = MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventCompleted{
		EventID:   eventID,
		ProcessID: sig.ProcessID,
		Timestamp: clock.Now(),
	}
	return rt.Events.Create(ctx, &event)
}

func (sig Complete) IsCompleted(ctx context.Context, events EventRepository) (bool, error) {
	for event, err := range events.FindByProcessID(ctx, sig.ProcessID) {
		if err != nil {
			return false, err
		}
		if completed, ok := event.(EventCompleted); ok && completed.ProcessID.Equal(sig.ProcessID) {
			return true, nil
		}
	}
	return false, nil
}

// EventCompleted is emitted when a workflow definition successfully completes execution.
type EventCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
}

var _ Event = (*EventCompleted)(nil)

func (EventCompleted) EventType() EventType      { return "workflow::completed" }
func (e EventCompleted) GetEventID() EventID     { return e.EventID }
func (e EventCompleted) GetProcessID() ProcessID { return e.ProcessID }
func (e EventCompleted) GetTimestamp() time.Time { return e.Timestamp }

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type VarKey string

type EventSetVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Key   VarKey
	Value any
	Scope Path
}

var _ Event = EventSetVar{}

func (e EventSetVar) GetEventID() EventID     { return e.EventID }
func (e EventSetVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSetVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventSetVar) EventType() EventType    { return "workflow::event::var::set" }

type EventDeleteVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Key VarKey
}

var _ Event = EventDeleteVar{}

func (e EventDeleteVar) GetEventID() EventID     { return e.EventID }
func (e EventDeleteVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDeleteVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDeleteVar) EventType() EventType    { return "workflow::event::var::delete" }

// GetVars can be used to retrieve the ProcessVars of the current workflow execution.
func GetVars(ctx context.Context) (Vars, error) {
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return Vars{}, err
	}
	pid, ok := ctxHProcessID.Lookup(ctx)
	if !ok {
		return Vars{}, ErrFatal.F("missing Pid in workflow context")
	}
	return Vars{
		ProcessID:        pid,
		EventsRepository: repo,
	}, nil
}

type Vars struct {
	ProcessID        ProcessID
	EventsRepository EventRepository
}

var _ ds.MapE[VarKey, any] = Vars{}
var _ ds.MapConvertibleE[VarKey, any] = Vars{}
var _ ds.ReadOnlyMapE[VarKey, any] = Vars{}
var _ ds.MapE[VarKey, any] = (*Vars)(nil)
var _ ds.MapConvertibleE[VarKey, any] = (*Vars)(nil)

// history returns the Process event history via the EventsRepository resolved
// from the proxy context. On error (e.g. no Runtime in context) it returns nil,
// so read operations degrade to "no variables".
func (vs Vars) history(ctx context.Context) ([]Event, error) {
	return iterkit.CollectE(vs.EventsRepository.FindByProcessID(ctx, vs.ProcessID))
}

func (vs Vars) Lookup(ctx context.Context, key VarKey) (any, bool, error) {
	var events, err = vs.history(ctx)
	if err != nil {
		return nil, false, err
	}
	var _, value, ok = lookupVariable(ctx, events, key)
	return value, ok, nil
}

func (vs Vars) ToMap(ctx context.Context) (map[VarKey]any, error) {
	events, err := vs.history(ctx)
	if err != nil {
		return nil, err
	}
	return variablesToMap(ctx, events), nil
}

type VarKeyValue struct {
	Key   VarKey
	Value any
}

func (vs Vars) Keys(ctx context.Context) iter.Seq2[VarKey, error] {
	return func(yield func(VarKey, error) bool) {
		for kv, err := range vs.All(ctx) {
			if err != nil {
				if !yield(kv.Key, err) {
					return
				}
				continue
			}
			if !yield(kv.Key, err) {
				return
			}
		}
	}
}

func (vs Vars) All(ctx context.Context) iter.Seq2[VarKeyValue, error] {
	return func(yield func(VarKeyValue, error) bool) {
		for kv, err := range eventsToKeyValue(ctx, vs.EventsRepository.FindByProcessID(ctx, vs.ProcessID), vs.ProcessID) {
			if !yield(kv, err) {
				return
			}
		}
	}
}

func (vs Vars) Get(ctx context.Context, key VarKey) (any, error) {
	value, _, err := vs.Lookup(ctx, key)
	return value, err
}

func (vs Vars) Set(ctx context.Context, key VarKey, val any) (rErr error) {
	v, ok, err := vs.Lookup(ctx, key)
	if err != nil {
		return err
	}
	if ok && comp.Equal(v, val) {
		return nil
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventSetVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Key:       key,
		Value:     val,
		Scope:     VarScope(ctx),
		Timestamp: clock.Now().UTC(),
	}
	return vs.EventsRepository.Create(ctx, &event)
}

func (vs Vars) Delete(ctx context.Context, key VarKey) error {
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventDeleteVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: clock.Now().UTC(),
		Key:       key,
	}
	return vs.EventsRepository.Create(ctx, &event)
}

// lookupVariable folds a variable's value from a slice of events in creation order.
func lookupVariable(ctx context.Context, events []Event, key VarKey) (Path, any, bool) {
	var (
		value any
		found bool
		scope Path
	)
	var currentScope = VarScope(ctx)
	for _, e := range events {
		switch e := e.(type) {
		case EventSetVar:
			if e.Key != key {
				continue
			}
			if !currentScope.MatchPrefix(e.Scope) {
				continue
			}
			found = true
			value = e.Value
			scope = zerokit.Coalesce(e.Scope, scope)

		case EventDeleteVar:
			if e.Key != key {
				continue
			}
			found = false
			value = nil
		default:
			continue
		}
	}
	// TODO: deep copy for value
	return currentScope, value, found
}

// variablesToMap folds all variables from a slice of events in creation order.
func eventsToKeyValue(ctx context.Context, events iter.Seq2[Event, error], pid ProcessID) iter.Seq2[VarKeyValue, error] {
	return func(yield func(VarKeyValue, error) bool) {
		events = iterkit.Filter(events, func(e Event) bool {
			return e.GetProcessID().Equal(pid)
		})
		eventsVS, err := iterkit.CollectE(events)
		if err != nil {
			yield(VarKeyValue{}, err)
			return
		}
		for k, v := range variablesToMap(ctx, eventsVS) {
			if !yield(VarKeyValue{Key: k, Value: v}, nil) {
				return
			}
		}
	}
}

func variablesToMap(ctx context.Context, events []Event) map[VarKey]any {
	var currentPath = CurrentPath(ctx)
	var m = map[VarKey]any{}
	for _, e := range events {
		switch e := e.(type) {
		case EventSetVar:
			if !currentPath.MatchPrefix(e.Scope) {
				continue
			}
			m[e.Key] = e.Value
		case EventDeleteVar:
			delete(m, e.Key)
		}
	}
	return m
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
