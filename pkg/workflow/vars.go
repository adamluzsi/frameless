package workflow

import (
	"context"
	"iter"
	"time"

	"go.llib.dev/frameless/pkg/comp"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds"
)

type VarName string

// EventDeclareVar records that a variable was declared in a variable scope.
//
// A declaration brings a variable into existence within its Scope, without
// assigning it a value yet — it is the `var name` part, separate from the
// `= value` assignment that an EventSetVar records.
type EventDeclareVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	Path      Path

	Name  VarName
	Scope VarScope
}

var _ Event = EventDeclareVar{}

func (e EventDeclareVar) GetEventID() EventID     { return e.EventID }
func (e EventDeclareVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDeclareVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDeclareVar) EventType() EventType    { return "workflow::event::var::declare" }

// EventSetVar records that a variable got a value assigned.
type EventSetVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	Path      Path

	Name  VarName
	Value any
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
	Path      Path

	Name VarName
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

func (vs Vars) history(ctx context.Context) ([]Event, error) {
	return iterkit.CollectE(vs.EventsRepository.FindByProcessID(ctx, vs.ProcessID))
}

func (vs Vars) Lookup(ctx context.Context, name VarName) (any, bool, error) {
	var events, err = vs.history(ctx)
	if err != nil {
		return nil, false, err
	}
	if _, value, ok := lookupVariable(ctx, events, name); ok {
		return value, true, nil
	}
	return lookupIterationVar(ctx, name, events)
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
	Scope VarScope
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
	_, prevVal, ok := lookupVariable(ctx, events, name)
	if ok && comp.Equal(prevVal, val) { // idempotent execution
		return nil
	}
	// A set only applies to a variable whose declaration is already folded in,
	// so the declaration has to come first. The fold orders by EventID, which is
	// a UUIDv7 and therefore already in creation order, so the ordering does not
	// rest on these timestamps. They are still derived from a single reading and
	// spaced apart, so that the recorded times agree with that order instead of
	// contradicting it under a coarse or frozen clock.
	var now = timeNow()
	if !ok {
		// The variable is not declared in a scope visible from here yet, so the
		// first assignment declares it. The declare and the set are recorded as
		// two separate events without a transaction: each is idempotent on its
		// own, and the intermediate state — declared but not yet assigned — is a
		// valid one that a retry simply resumes from.
		declareEventID, err := MakeEventID()
		if err != nil {
			return err
		}
		var declareEvent Event = EventDeclareVar{
			EventID:   declareEventID,
			ProcessID: vs.ProcessID,
			Timestamp: now,
			Path:      CurrentPath(ctx),
			Name:      name,
			Scope:     CurrentVarScope(ctx),
		}
		if err := vs.EventsRepository.Create(ctx, &declareEvent); err != nil {
			return err
		}
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventSetVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: now.Add(time.Nanosecond),
		Path:      CurrentPath(ctx),
		Name:      name,
		Value:     val,
	}
	return vs.EventsRepository.Create(ctx, &event)
}

func (vs Vars) declareOnce(ctx context.Context, name VarName, global bool) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	if vs.varMutationDone(events, EventDeclareVar{}.EventType(), CurrentPath(ctx), name) {
		return nil
	}
	return vs.declare(ctx, events, name, global)
}

func (vs Vars) declare(ctx context.Context, events []Event, name VarName, global bool) error {
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var scope VarScope
	if !global {
		scope = CurrentVarScope(ctx)
	}
	var declareEvent Event = EventDeclareVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: timeNow(),
		Path:      CurrentPath(ctx),
		Name:      name,
		Scope:     scope,
	}
	return vs.EventsRepository.Create(ctx, &declareEvent)
}

// setOnce is Set, guarded against being applied twice for the same logical
// definition step. It is what workflow Definitions use, so that re-executing a
// Process replays the assignment instead of appending it again.
func (vs Vars) setOnce(ctx context.Context, name VarName, val any) error {
	events, err := vs.history(ctx)
	if err != nil {
		return err
	}
	if vs.varMutationDone(events, EventSetVar{}.EventType(), CurrentPath(ctx), name) {
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
	_, _, ok := lookupVariable(ctx, events, name)
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
		Timestamp: timeNow(),
		Path:      CurrentPath(ctx),
		Name:      name,
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
	_, _, ok := lookupVariable(ctx, events, name)
	if !ok {
		return nil
	}
	if vs.varMutationDone(events, EventDeleteVar{}.EventType(), CurrentPath(ctx), name) {
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
func (vs Vars) varMutationDone(events []Event, typ EventType, path Path, name VarName) bool {
	for _, e := range events {
		var (
			eName VarName
			ePath Path
		)
		switch e := e.(type) {
		case EventDeclareVar:
			eName, ePath = e.Name, e.Path
		case EventSetVar:
			eName, ePath = e.Name, e.Path
		case EventDeleteVar:
			eName, ePath = e.Name, e.Path
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
		return true
	}
	return false
}

// varBindings folds the variable bindings visible from the variable scope of
// ctx out of a Process event history.
//
// Events are folded in creation order, which the EventID encodes, so the fold
// does not depend on the wall clock the Timestamps happened to be taken from.
//
// A binding is keyed by name alone. A declaration made in a scope that is not
// visible from here still takes the name over, so the assignments that follow
// it belong to that hidden binding and are skipped, until the name gets
// declared again in a scope this one can see. That is what lets an explicit
// re-declaration shadow an enclosing binding, while a plain assignment keeps
// writing through to it.
func varBindings(ctx context.Context, events []Event) map[VarName]VarBinding {
	var (
		currentScope = CurrentVarScope(ctx)
		bindings     = map[VarName]VarBinding{}
		// declaredIn holds the scope of the most recent declaration of a name,
		// visible from here or not, since that is the binding which a following
		// assignment or deletion of that name refers to.
		declaredIn = map[VarName]VarScope{}
	)
	var visible = func(name VarName) bool {
		scope, ok := declaredIn[name]
		return ok && slicekit.MatchPrefix(currentScope, scope)
	}
	events = slicekit.Clone(events)
	sortEvents(events)
	for _, e := range events {
		switch e := e.(type) {
		case EventDeclareVar:
			declaredIn[e.Name] = e.Scope
			if !visible(e.Name) {
				continue
			}
			// A declaration is the `var name` half of a variable: it brings the
			// binding into existence, carrying no value of its own.
			bindings[e.Name] = VarBinding{
				Name:  e.Name,
				Scope: e.Scope,
			}
		case EventSetVar:
			if !visible(e.Name) {
				continue
			}
			bindings[e.Name] = VarBinding{
				Name:  e.Name,
				Value: e.Value,
				Scope: declaredIn[e.Name],
			}
		case EventDeleteVar:
			if !visible(e.Name) {
				continue
			}
			delete(bindings, e.Name)
		}
	}
	return bindings
}

// lookupVariable folds a single variable out of a Process event history, as it
// is seen from the variable scope of ctx.
//
// found reports that a declaration of the variable is visible from here, which
// is not the same as the variable having a value: a variable which was declared
// but not yet assigned is found, holding a nil value.
func lookupVariable(ctx context.Context, events []Event, name VarName) (scope VarScope, value any, found bool) {
	b, ok := varBindings(ctx, events)[name]
	return b.Scope, b.Value, ok
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
	return func(yield func(VarBinding) bool) {
		var bindings []VarBinding
		for _, b := range varBindings(ctx, events) {
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

// DeclareVar brings a workflow variable into existence as part of a process
// definition, without assigning a value to it — the `var name` half of a
// variable, separate from the `= value` that SetVar records.
//
// Declaring is what makes shadowing explicit. A plain SetVar writes through to
// whichever binding is visible from here, including one that an enclosing
// variable scope owns. Declaring first creates a fresh binding in the current
// variable scope, so the assignments that follow it stay local to that scope
// and leave the enclosing binding untouched.
type DeclareVar struct {
	Name VarName
	// Global declares the variable in the root variable scope instead of the
	// one the step happens to run under, which makes the name visible from
	// every variable scope of the Process.
	Global bool
}

var _ Definition = DeclareVar{}

func (DeclareVar) Error() string { return "workflow::declare-var" }

func (d DeclareVar) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "declare-var")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var vars = Vars{ProcessID: pid, EventsRepository: repo}
	return vars.declareOnce(ctx, d.Name, d.Global)
}

// SetVar sets the value of a workflow variable as part of a process definition.
type SetVar struct {
	Name  VarName
	Value any
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
