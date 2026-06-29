package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
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
}

type ProcessID = uuid.UUID

func MakeProcessID() (ProcessID, error) {
	return uuid.MakeV7()
}

// ProcessEvents is the interactor that holds the event history of a Process.
//
// Instead of appending into a slice, the engine always creates a new Event in
// the history. The ordering used during replay (idempotency checks, variable
// resolution, completion detection) follows the order in which the backing
// EventsRepository returns the events, which is expected to match their creation
// order.
type ProcessEvents interface {
	comproto.OnePhaseCommitProtocol
	crud.Creator[Event]
	crud.AllFinder[Event]
}

type Event interface {
	Type() EventType
	GetProcessID() ProcessID
	GetTimestamp() time.Time

	// SetProcessID(ProcessID)
}

func Events(ctx context.Context, processID ProcessID) (ProcessEvents, error) {
	repo, ok := historyH.Lookup(ctx)
	if !ok || repo == nil {
		return nil, ErrFatal.F("missing EventRepository in workflow context")
	}
	return history{EventsRepository: repo, ProcessID: processID}, nil
}

type history struct {
	EventsRepository EventsRepository
	ProcessID        ProcessID
}

var _ ProcessEvents = history{}

func (h history) BeginTx(ctx context.Context) (context.Context, error) {
	return h.EventsRepository.BeginTx(ctx)
}

func (h history) CommitTx(ctx context.Context) error {
	return h.EventsRepository.CommitTx(ctx)
}

func (h history) RollbackTx(ctx context.Context) error {
	return h.EventsRepository.RollbackTx(ctx)
}

func (h history) Create(ctx context.Context, ptr *Event) error {
	(*ptr).SetProcessID(h.ProcessID)
	return h.EventsRepository.Create(ctx, ptr)
}

func (h history) FindAll(ctx context.Context) iter.Seq2[Event, error] {
	return h.EventsRepository.FindByProcessID(ctx, h.ProcessID)
}

func (h history) History(ctx context.Context) ([]Event, error) {
	vs, err := iterkit.CollectE(h.FindAll(ctx))
	if err != nil {
		return nil, err
	}
	slicekit.SortBy(vs, func(a, b Event) bool {
		return a.Timestamp().Before(b.Timestamp())
	})
	return vs, nil
}

type ctxKeyEventRepository struct{}

var historyH contextkit.ValueHandler[ctxKeyEventRepository, EventsRepository]

type EventType string

// EventCompleted is emitted when a workflow definition successfully completes execution.
type EventCompleted struct {
	ProcessID ProcessID `json:"process_id"`
	Timestamp time.Time `json:"timestamp"`
}

var _ Event = (*EventCompleted)(nil)

func (EventCompleted) Type() EventType { return typeIDCompletedEvent }

func (e EventCompleted) GetProcessID() ProcessID {
	return e.ProcessID
}

func (e EventCompleted) GetTimestamp() time.Time {
	return e.Timestamp
}

// IsCompleted checks if the given process event history contains an EventCompleted event.
func IsCompleted[T Process | *Process](v T) bool {
	var p *Process
	switch v := any(v).(type) {
	case Process:
		p = &v
	case *Process:
		p = v
	}
	if p == nil {
		return false
	}
	events, err := p.History()
	if err != nil {
		return false
	}
	for _, e := range slicekit.IterReverse(events) {
		if _, ok := e.(*EventCompleted); ok {
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

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// NewEvents returns an Events history seeded with the given events.
//
// The provided events are stored as-is and replayed in the order given. It is
// primarily useful to reconstruct a previously recorded history or to set up an
// explicit history in tests.
func NewEvents(events ...Event) ProcessEvents {
	return newEventLog(events...)
}

func newEventLog(events ...Event) *eventLog {
	return &eventLog{events: slicekit.Clone(events)}
}

// eventLog is the default in-memory Events interactor.
//
// It records events in an append-only fashion: instead of mutating existing
// entries, callers always create a new Event, and the events are replayed in
// creation order.
type eventLog struct {
	mu     sync.RWMutex
	events []Event
}

var _ ProcessEvents = (*eventLog)(nil)

func (el *eventLog) Create(ctx context.Context, ptr *Event) error {
	if ptr == nil || *ptr == nil {
		return fmt.Errorf("workflow: nil Event passed to Events.Create")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	el.mu.Lock()
	el.events = append(el.events, *ptr)
	el.mu.Unlock()
	return nil
}

func (el *eventLog) FindAll(ctx context.Context) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		if err := ctx.Err(); err != nil {
			var zero Event
			yield(zero, err)
			return
		}
		for _, e := range el.snapshot() {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (el *eventLog) snapshot() []Event {
	el.mu.RLock()
	defer el.mu.RUnlock()
	return slicekit.Clone(el.events)
}

func (el *eventLog) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonkit.Array[Event](el.snapshot()))
}

func (el *eventLog) UnmarshalJSON(data []byte) error {
	var arr jsonkit.Array[Event]
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	el.mu.Lock()
	el.events = []Event(arr)
	el.mu.Unlock()
	return nil
}

type ctxKeyEventLogTx struct{ log *eventLog }

type eventLogTx struct {
	snapshot []Event
	done     bool
}

func (el *eventLog) BeginTx(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	el.mu.RLock()
	snapshot := slicekit.Clone(el.events)
	el.mu.RUnlock()
	return context.WithValue(ctx, ctxKeyEventLogTx{el}, &eventLogTx{snapshot: snapshot}), nil
}

func (el *eventLog) CommitTx(ctx context.Context) error {
	tx, ok := ctx.Value(ctxKeyEventLogTx{el}).(*eventLogTx)
	if !ok {
		return fmt.Errorf("workflow: no event log transaction in context")
	}
	if tx.done {
		return fmt.Errorf("workflow: event log transaction already finished")
	}
	tx.done = true
	return nil
}

func (el *eventLog) RollbackTx(ctx context.Context) error {
	tx, ok := ctx.Value(ctxKeyEventLogTx{el}).(*eventLogTx)
	if !ok {
		return fmt.Errorf("workflow: no event log transaction in context")
	}
	if tx.done {
		return fmt.Errorf("workflow: event log transaction already finished")
	}
	tx.done = true
	el.mu.Lock()
	el.events = tx.snapshot
	el.mu.Unlock()
	return nil
}

// processEvents adapts an EventsRepository to the Events interface for a single
// Process. It stamps the ProcessID on every created Event and scopes reads to
// the events that belong to the Process.
type processEvents struct {
	repo EventsRepository
	pid  ProcessID
}

var _ ProcessEvents = processEvents{}

func (pe processEvents) Create(ctx context.Context, ptr *Event) error {
	if ptr == nil || *ptr == nil {
		return fmt.Errorf("workflow: nil Event passed to Events.Create")
	}
	(*ptr).SetProcessID(pe.pid)
	return pe.repo.Create(ctx, ptr)
}

func (pe processEvents) FindAll(ctx context.Context) iter.Seq2[Event, error] {
	return pe.repo.FindByProcessID(ctx, pe.pid)
}

func (pe processEvents) BeginTx(ctx context.Context) (context.Context, error) {
	return pe.repo.BeginTx(ctx)
}

func (pe processEvents) CommitTx(ctx context.Context) error {
	return pe.repo.CommitTx(ctx)
}

func (pe processEvents) RollbackTx(ctx context.Context) error {
	return pe.repo.RollbackTx(ctx)
}

// events returns the Process event history, lazily initialising the default
// in-memory implementation when none was supplied.
func (p *Process) events() ProcessEvents {
	if p.Events == nil {
		p.Events = newEventLog()
	}
	return p.Events
}

// History returns the Process events in their recorded order.
func (p *Process) History() ([]Event, error) {
	if p.Events == nil {
		return nil, nil
	}
	return iterkit.CollectE(p.Events.FindAll(context.Background()))
}

type processJSONDTO struct {
	ID     ProcessID       `json:"id"`
	Def    json.RawMessage `json:"def,omitempty"`
	Events json.RawMessage `json:"events,omitempty"`
}

func (p Process) MarshalJSON() ([]byte, error) {
	var dto processJSONDTO
	dto.ID = p.ID
	if p.Definition != nil {
		data, err := json.Marshal(jsonkit.Interface[Definition]{V: p.Definition})
		if err != nil {
			return nil, err
		}
		dto.Def = data
	}
	events, err := p.History()
	if err != nil {
		return nil, err
	}
	eventsData, err := json.Marshal(jsonkit.Array[Event](events))
	if err != nil {
		return nil, err
	}
	dto.Events = eventsData
	return json.Marshal(dto)
}

func (p *Process) UnmarshalJSON(data []byte) error {
	var dto processJSONDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ID = dto.ID
	p.Definition = nil
	if len(dto.Def) > 0 && string(dto.Def) != "null" {
		var iface jsonkit.Interface[Definition]
		if err := json.Unmarshal(dto.Def, &iface); err != nil {
			return err
		}
		p.Definition = iface.V
	}
	var arr jsonkit.Array[Event]
	if len(dto.Events) > 0 && string(dto.Events) != "null" {
		if err := json.Unmarshal(dto.Events, &arr); err != nil {
			return err
		}
	}
	p.Events = newEventLog([]Event(arr)...)
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

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
	ProcessID ProcessID              `json:"process_id"`
	Operation VariableEventOperation `json:"op"`
	Key       VariableKey            `json:"var_key"`
	Value     any                    `json:"value,omitempty"`
}

var _ Event = (*VariableEvent)(nil)

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

func (vs varProcessProxy) event(e Event) (*VariableEvent, bool) {
	if e == nil {
		return nil, false
	}
	if e.Type() != typeIDVariableEvent {
		return nil, false
	}
	var event, ok = e.(*VariableEvent)
	return event, ok
}

func (vs varProcessProxy) Lookup(key VariableKey) (any, bool) {
	var (
		value any
		found bool
	)
	events, err := vs.Process.History()
	if err != nil {
		return nil, false
	}
	for _, e := range events {
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
	events, err := vs.Process.History()
	if err != nil {
		return m
	}
	for _, e := range events {
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
	var event Event = &VariableEvent{
		Operation: SetVariableEventOperation,
		Key:       key,
		Value:     val,
	}
	_ = vs.Process.events().Create(context.Background(), &event)
}

func (vs varProcessProxy) Delete(key VariableKey) {
	var event Event = &VariableEvent{
		Operation: DelVariableEventOperation,
		Key:       key,
	}
	_ = vs.Process.events().Create(context.Background(), &event)
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
