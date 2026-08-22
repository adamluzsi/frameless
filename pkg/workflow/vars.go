package workflow

import (
	"context"
	"iter"
	"time"

	"go.llib.dev/frameless/pkg/comp"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/testcase/clock"
)

type VarName string

// EventSetVar records that a variable got a value assigned.
type EventSetVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	// Path is the position within the definition tree at which the assignment
	// was executed. Together with Name and Scope it identifies the logical step
	// that produced this event, which is what makes replaying that step a no-op.
	//
	// See varMutationDone for the details of the identity.
	Path Path

	Name  VarName
	Value any
	// Scope is the variable scope the assignment belongs to. It is the scope in
	// which the variable was originally declared, not necessarily the scope the
	// assignment was issued from.
	Scope Path
}

var _ Event = EventSetVar{}

func (e EventSetVar) GetEventID() EventID     { return e.EventID }
func (e EventSetVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSetVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventSetVar) EventType() EventType    { return "workflow::event::var::set" }

// EventDeleteVar records that a variable binding got removed.
type EventDeleteVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	// Path is the position within the definition tree at which the deletion was
	// executed. Together with Name and Scope it identifies the logical step that
	// produced this event, which is what makes replaying that step a no-op.
	//
	// See varMutationDone for the details of the identity.
	Path Path

	Name VarName
	// Scope is the variable scope the deletion applies to. It is the scope the
	// deleted variable was declared in, so a deletion issued from a sub scope
	// does not reach beyond the binding it was able to see.
	Scope Path
}

var _ Event = EventDeleteVar{}

func (e EventDeleteVar) GetEventID() EventID     { return e.EventID }
func (e EventDeleteVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDeleteVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDeleteVar) EventType() EventType    { return "workflow::event::var::delete" }

// GetVars can be used to retrieve the ProcessVars of the current workflow execution.
func GetVars(ctx context.Context) (Vars, error) {
	pid, ok := ctxHProcessID.Lookup(ctx)
	if !ok {
		return Vars{}, ErrFatal.F("missing Pid in workflow context")
	}
	return getVarsFor(ctx, pid)
}

func getVarsFor(ctx context.Context, processID ProcessID) (Vars, error) {
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return Vars{}, err
	}
	return Vars{
		ProcessID:        processID,
		EventsRepository: repo,
	}, nil
}

type Vars struct {
	ProcessID        ProcessID
	EventsRepository EventRepository
}

var _ ds.MapE[VarName, any] = Vars{}
var _ ds.MapConvertibleE[VarName, any] = Vars{}
var _ ds.ReadOnlyMapE[VarName, any] = Vars{}
var _ ds.MapE[VarName, any] = (*Vars)(nil)
var _ ds.MapConvertibleE[VarName, any] = (*Vars)(nil)

// history returns the Process event history via the EventsRepository resolved
// from the proxy context. On error (e.g. no Runtime in context) it returns nil,
// so read operations degrade to "no variables".
func (vs Vars) history(ctx context.Context) ([]Event, error) {
	return iterkit.CollectE(vs.EventsRepository.FindByProcessID(ctx, vs.ProcessID))
}

func (vs Vars) Lookup(ctx context.Context, name VarName) (any, bool, error) {
	var events, err = vs.history(ctx)
	if err != nil {
		return nil, false, err
	}
	var _, value, ok = lookupVariable(ctx, events, name)
	return value, ok, nil
}

func (vs Vars) ToMap(ctx context.Context) (map[VarName]any, error) {
	events, err := vs.history(ctx)
	if err != nil {
		return nil, err
	}
	return variablesToMap(ctx, events), nil
}

func (vs Vars) Keys(ctx context.Context) iter.Seq2[VarName, error] {
	return func(yield func(VarName, error) bool) {
		for kv, err := range vs.All(ctx) {
			if err != nil {
				if !yield(kv.Name, err) {
					return
				}
				continue
			}
			if !yield(kv.Name, err) {
				return
			}
		}
	}
}

type VarBinding struct {
	Name  VarName
	Value any
	Scope Path
}

func (vs Vars) All(ctx context.Context) iter.Seq2[VarBinding, error] {
	return func(yield func(VarBinding, error) bool) {
		for kv, err := range eventsToVarBindingsE(ctx, vs.EventsRepository.FindByProcessID(ctx, vs.ProcessID), vs.ProcessID) {
			if !yield(kv, err) {
				return
			}
		}
	}
}

func (vs Vars) Get(ctx context.Context, name VarName) (any, error) {
	value, _, err := vs.Lookup(ctx, name)
	return value, err
}

func (vs Vars) Set(ctx context.Context, name VarName, val any) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	return vs.set(ctx, events, name, val)
}

func (vs Vars) set(ctx context.Context, events []Event, name VarName, val any) error {
	scope, prevVal, ok := lookupVariable(ctx, events, name)
	if ok && comp.Equal(prevVal, val) { // idempotent execution
		return nil
	}
	if !ok {
		scope = VarScope(ctx)
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventSetVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: clock.Now().UTC(),
		Path:      CurrentPath(ctx),
		Name:      name,
		Value:     val,
		Scope:     scope,
	}
	return vs.EventsRepository.Create(ctx, &event)
}

// setOnce is Set, guarded against being applied twice for the same logical
// definition step. It is what workflow Definitions use, so that re-executing a
// Process replays the assignment instead of appending it again.
func (vs Vars) setOnce(ctx context.Context, name VarName, val any) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	scope, _, ok := lookupVariable(ctx, events, name)
	if !ok {
		scope = VarScope(ctx)
	}
	if vs.varMutationDone(events, EventSetVar{}.EventType(), CurrentPath(ctx), name, scope) {
		return nil
	}
	return vs.set(ctx, events, name, val)
}

func (vs Vars) Delete(ctx context.Context, name VarName) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	return vs.delete(ctx, events, name)
}

func (vs Vars) delete(ctx context.Context, events []Event, name VarName) error {
	// The deletion is recorded against the scope the variable was declared in,
	// and only when that declaration is visible from the current variable scope.
	// This keeps a deletion from reaching bindings the caller can't even see.
	scope, _, ok := lookupVariable(ctx, events, name)
	if !ok {
		return nil
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventDeleteVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: clock.Now().UTC(),
		Path:      CurrentPath(ctx),
		Name:      name,
		Scope:     scope,
	}
	return vs.EventsRepository.Create(ctx, &event)
}

// deleteOnce is Delete, guarded against being applied twice for the same
// logical definition step.
//
// Without the guard a replay would delete again whatever the variable happens
// to hold at that point, which is wrong as soon as a later step reassigned it:
// the deletion would then remove more than it originally did.
func (vs Vars) deleteOnce(ctx context.Context, name VarName) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	scope, _, ok := lookupVariable(ctx, events, name)
	if !ok {
		return nil
	}
	if vs.varMutationDone(events, EventDeleteVar{}.EventType(), CurrentPath(ctx), name, scope) {
		return nil
	}
	return vs.delete(ctx, events, name)
}

// varMutationDone reports whether the Process event history already contains a
// variable mutation event recorded for a given logical step.
//
// A step is identified by the triplet of
//
//   - Path — where in the definition tree the mutation was executed,
//   - Name — which variable it mutates, and
//   - Scope — which variable scope the mutation applies to.
//
// The Path pins the mutation to a single location of the workflow definition,
// while the Scope tells apart repeated executions of that very same location
// which run under different variable scopes, such as loop iterations.
//
// Replaying a step that is already present in the history must not append a
// second event, otherwise a retry would apply the mutation twice.
func (vs Vars) varMutationDone(events []Event, typ EventType, path Path, name VarName, scope Path) bool {
	for _, e := range events {
		var (
			eName  VarName
			eScope Path
			ePath  Path
		)
		switch e := e.(type) {
		case EventSetVar:
			eName, eScope, ePath = e.Name, e.Scope, e.Path
		case EventDeleteVar:
			eName, eScope, ePath = e.Name, e.Scope, e.Path
		default:
			continue
		}
		if e.EventType() != typ {
			continue
		}
		if eName != name {
			continue
		}
		if !ePath.Equal(path) {
			continue
		}
		if !eScope.Equal(scope) {
			continue
		}
		return true
	}
	return false
}

// lookupVariable folds a variable's value from a slice of events in creation order.
func lookupVariable(ctx context.Context, events []Event, name VarName) (Path, any, bool) {
	var (
		value any
		found bool
		scope Path
	)
	var currentScope = VarScope(ctx)
	for _, e := range events {
		switch e := e.(type) {
		case EventSetVar:
			if e.Name != name {
				continue
			}
			if !currentScope.MatchPrefix(e.Scope) {
				continue
			}
			found = true
			value = e.Value
			scope = zerokit.Coalesce(e.Scope, scope)

		case EventDeleteVar:
			if e.Name != name {
				continue
			}
			if !currentScope.MatchPrefix(e.Scope) {
				continue
			}
			found = false
			value = nil
		default:
			continue
		}
	}
	return scope, value, found
}

// eventsToVarBindingsE folds all variable bindings from a Process event stream,
// keeping only the events which belong to the given Process.
func eventsToVarBindingsE(ctx context.Context, events iter.Seq2[Event, error], pid ProcessID) iter.Seq2[VarBinding, error] {
	return func(yield func(VarBinding, error) bool) {
		events = iterkit.Filter(events, func(e Event) bool {
			return e.GetProcessID().Equal(pid)
		})
		eventsVS, err := iterkit.CollectE(events)
		if err != nil {
			yield(VarBinding{}, err)
			return
		}
		for b := range eventsToVarBindings(ctx, eventsVS) {
			if !yield(b, nil) {
				return
			}
		}
	}
}

func eventsToVarBindings(ctx context.Context, events []Event) iter.Seq[VarBinding] {
	events = slicekit.Clone(events)
	slicekit.SortBy(events, func(a, b Event) bool {
		return a.GetTimestamp().Compare(b.GetTimestamp()) == -1
	})
	return func(yield func(VarBinding) bool) {
		var m = map[VarName]VarBinding{}
		var currentVarScope = VarScope(ctx)
		for _, e := range events {
			switch e := e.(type) {
			case EventSetVar:
				if !currentVarScope.MatchPrefix(e.Scope) {
					continue
				}
				m[e.Name] = VarBinding{
					Name:  e.Name,
					Value: e.Value,
					Scope: e.Scope,
				}
			case EventDeleteVar:
				if !currentVarScope.MatchPrefix(e.Scope) {
					continue
				}
				delete(m, e.Name)
			}
		}
		var bindings []VarBinding
		for _, b := range m {
			bindings = append(bindings, b)
		}
		slicekit.SortBy(bindings, func(a, b VarBinding) bool {
			return a.Name < b.Name
		})
		for _, b := range bindings {
			if !yield(b) {
				return
			}
		}
	}
}

func variablesToMap(ctx context.Context, events []Event) map[VarName]any {
	var m = map[VarName]any{}
	for b := range eventsToVarBindings(ctx, events) {
		m[b.Name] = b.Value
	}
	return m
}

// --------------------------------------------------------------------------------------------- //

// SetVar sets the value of a workflow variable as part of a process definition.
type SetVar struct {
	Name  VarName
	Value any
	// Global will set the variable scope to be globally available,
	// therefore allowing to escape the current workflow definition assigned variable scope.
	Global bool
}

var _ Definition = SetVar{}

func (SetVar) Error() string { return "workflow::set-var" }

func (d SetVar) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "set-var")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var vars = Vars{ProcessID: pid, EventsRepository: repo}
	// if d.Global {
	// 	ctx = context.WithValue(ctx, ctxKeyVarScope{}, nil)
	// }
	return vars.setOnce(ctx, d.Name, d.Value)
}

// DeleteVar will delete a variable by name
type DeleteVar struct {
	Name VarName
}

var _ Definition = DeleteVar{}

func (DeleteVar) Error() string { return "workflow::delete-var" }

func (d DeleteVar) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "delete-var")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var vars = Vars{ProcessID: pid, EventsRepository: repo}
	return vars.deleteOnce(ctx, d.Name)
}
