package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"strings"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dsmap"
)

type Definition interface {
	Execute(ctx context.Context, p *Process) error
}

type Condition interface {
	Evaluate(ctx context.Context, p *Process) (bool, error)
}

type ConditionID string

type ConditionConvertible interface {
	ToCondition(ctx context.Context, p *Process) (Condition, bool)
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Process struct {
	ID         ProcessID  `json:"id" ext:"id"`
	Definition Definition `json:"def"`
	Events     Events     `json:"events"`
}

type ProcessID = uuid.UUID

func MakeProcessID() (ProcessID, error) {
	return uuid.MakeV7()
}

type Events []Event

func (es Events) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonkit.Array[Event](es))
}

func (es *Events) UnmarshalJSON(data []byte) error {
	var arr jsonkit.Array[Event]
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*es = Events(arr)
	return nil
}

type Event interface {
	Type() EventType
}

type EventType string

// EventCompleted is emitted when a workflow definition successfully completes execution.
type EventCompleted struct{}

func (EventCompleted) Type() EventType { return "workflow::completed" }

// IsCompleted checks if the given events contain an EventCompleted event.
func IsCompleted[T Events | Process | *Process](v T) bool {
	var events Events
	switch v := any(v).(type) {
	case Events:
		events = v
	case Process:
		events = v.Events
	case *Process:
		events = v.Events
	}
	for _, e := range slicekit.IterReverse(events) {
		if _, ok := e.(EventCompleted); ok {
			return true
		}
	}
	return false
}

func FilterEvent[T Event](es []Event) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range es {
			if v, ok := e.(T); ok {
				if !yield(v) {
					return
				}
			}
		}
	}
}

type Vars interface {
	ds.Map[VariableKey, any]
	ds.MapConvertible[VariableKey, any]
}

type VariableKey string

func (p *Process) Var() Vars {
	return varProcessProxy{Process: p}
}

type varProcessProxy struct {
	Process *Process
}

var _ ds.ReadOnlyMap[VariableKey, any] = varProcessProxy{}
var _ ds.Map[VariableKey, any] = (*varProcessProxy)(nil)
var _ ds.MapConvertible[VariableKey, any] = (*varProcessProxy)(nil)

const (
	typeIDVariableEvent  = "workflow::variable-event"
	typeIDCompletedEvent = "workflow::completed"
)

var _ = jsonkit.RegisterTypeID[VariableEvent](typeIDVariableEvent)
var _ = jsonkit.RegisterTypeID[EventCompleted](typeIDCompletedEvent)

type VariableEvent struct {
	Operation VariableEventOperation `json:"op"`
	Key       VariableKey            `json:"var-key"`
	Value     any                    `json:"value,omitempty"`
}

func (ve VariableEvent) Validate(ctx context.Context) error {
	if err := enum.Validate(ve.Operation); err != nil {
		return fmt.Errorf("invalid variable event operation: %w", err)
	}
	return nil
}

type VariableEventOperation string

const (
	SetVariableEventOperation VariableEventOperation = "set"
	DelVariableEventOperation VariableEventOperation = "del"
)

var _ = enum.Register[VariableEventOperation](SetVariableEventOperation, DelVariableEventOperation)

func (ve VariableEvent) Type() EventType {
	return typeIDVariableEvent
}

func (vs varProcessProxy) event(e Event) (VariableEvent, bool) {
	if e == nil {
		return VariableEvent{}, false
	}
	if e.Type() != typeIDVariableEvent {
		return VariableEvent{}, false
	}
	var event, ok = e.(VariableEvent)
	return event, ok
}

func (vs varProcessProxy) Lookup(key VariableKey) (any, bool) {
	var (
		value any
		found bool
	)
	for _, e := range vs.Process.Events {
		event, ok := vs.event(e)
		if !ok {
			continue
		}
		if event.Key != key {
			continue
		}
		switch event.Operation {
		case SetVariableEventOperation:
			found = true
			value = event.Value
		case DelVariableEventOperation:
			found = false
			value = nil
		}
	}
	// TODO: deep copy for value
	return value, found
}

func (vs varProcessProxy) ToMap() map[VariableKey]any {
	var m = map[VariableKey]any{}
	for _, e := range vs.Process.Events {
		event, ok := vs.event(e)
		if !ok {
			continue
		}
		switch event.Operation {
		case SetVariableEventOperation:
			m[event.Key] = event.Value
		case DelVariableEventOperation:
			delete(m, event.Key)
		}
	}
	// TODO: deep copy each value
	return m
}

func (vs varProcessProxy) All() iter.Seq2[VariableKey, any] {
	return maps.All(vs.ToMap())
}

func (vs varProcessProxy) Get(key VariableKey) any {
	var value, _ = vs.Lookup(key)
	return value
}

func (vs varProcessProxy) Set(key VariableKey, val any) {
	vs.Process.Events = append(vs.Process.Events, VariableEvent{
		Operation: SetVariableEventOperation,
		Key:       key,
		Value:     val,
	})
}

func (vs varProcessProxy) Delete(key VariableKey) {
	vs.Process.Events = append(vs.Process.Events, VariableEvent{
		Operation: DelVariableEventOperation,
		Key:       key,
	})
}

func (vs varProcessProxy) Validate(ctx context.Context) error {
	return vs.validateVariables(ctx)
}

func (vs varProcessProxy) Merge(oth Vars) {
	if oth == nil {
		return
	}
	for k, v := range oth.All() {
		if og, ok := vs.Lookup(k); ok { // to avoid polluting the
			if reflectkit.Equal(og, v) {
				continue
			}
		}
		vs.Set(k, v)
	}
}

func (s varProcessProxy) validateVariables(context.Context) error {
	m := s.ToMap()
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("workflow variables are not valid because the values must be json encodable")
	}
	var got dsmap.Map[VariableKey, any]
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("workflow variables are not valid because the json encoded values should be okay to unmarshal")
	}
	if !reflectkit.Equal(m, got) {
		return fmt.Errorf("workflow variables are not valid because json encoding should not affect its values")
	}
	return nil
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

var _ validate.Validatable = (*Participant)(nil)

const ErrInvalidParicipantFunc errorkitlite.Error = `Invalid workflow.Participant#Func signature:
expected func(context.Context, arg1 T1, ...OtherArgs) (Result1, Result2..., error)
where the function signature starts with a context.Context, then user defined argument types,
and the results tuple is also returns user defined types, ending with an error value type.
The input and output argument types must be serializable.
`

var reflectContextType = reflectkit.TypeOf[context.Context]()

var reflectErrorType = reflectkit.TypeOf[error]()

func (p Participant) rFunc() (reflect.Value, error) {
	rfunc := reflect.ValueOf(p.Func)
	if rfunc.Kind() != reflect.Func {
		return rfunc, ErrInvalidParicipantFunc.F("invalid value for participant func")
	}
	var (
		funcType   = rfunc.Type()
		funcNumIn  = funcType.NumIn()
		funcNumOut = funcType.NumOut()
	)
	if funcNumIn < 1 {
		return rfunc, ErrInvalidParicipantFunc
	}
	if funcType.In(0) != reflectContextType {
		return rfunc, ErrInvalidParicipantFunc
	}
	if funcNumOut < 1 {
		return rfunc, ErrInvalidParicipantFunc
	}
	if lastOut := funcType.Out(funcNumOut - 1); lastOut != reflectErrorType || !lastOut.Implements(reflectErrorType) {
		return rfunc, ErrInvalidParicipantFunc
	}
	return rfunc, nil
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

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func DefaultJSONCodec() jsonkit.Codec {
	var c jsonkit.Codec
	return c
}
