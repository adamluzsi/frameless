package workflow

import (
	"context"
	"iter"
	"slices"
	"time"

	"go.llib.dev/frameless/pkg/comp"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds"
)

type VarName string

// EventDeclareVar records that a variable was declared in a variable scope.
//
// A declaration brings a variable into existence within its variable scope,
// without assigning it a value yet — it is the `var name` part, separate from
// the `= value` assignment that an EventSetVar records.
type EventDeclareVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path Path
	Name VarName
}

var _ Event = EventDeclareVar{}

func (e EventDeclareVar) GetEventID() EventID     { return e.EventID }
func (e EventDeclareVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDeclareVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDeclareVar) EventType() EventType    { return "workflow:var:declare" }

// EventSetVar records that a variable got a value assigned.
type EventSetVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path  Path
	Name  VarName
	Value any
}

var _ Event = EventSetVar{}

func (e EventSetVar) GetEventID() EventID     { return e.EventID }
func (e EventSetVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSetVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventSetVar) EventType() EventType    { return "workflow:var:set" }

// EventDeleteVar records that a variable binding got removed.
type EventDeleteVar struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path Path
	Name VarName
}

var _ Event = EventDeleteVar{}

func (e EventDeleteVar) GetEventID() EventID     { return e.EventID }
func (e EventDeleteVar) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDeleteVar) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDeleteVar) EventType() EventType    { return "workflow:var:delete" }

// GetVars can be used to retrieve the ProcessVars of the current workflow execution.
func GetVars(ctx context.Context) (Vars, error) {
	pid, ok := ProcessIDFromContext(ctx)
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
	return lookupVar(ctx, events, name)
}

// lookupVar resolves a single variable out of a Process event history, as it is
// seen from the variable scope of ctx.
func lookupVar(ctx context.Context, events []Event, name VarName) (any, bool, error) {
	b, ok, err := visibleVarsOf(ctx, events).lookup(name)
	return b.Value, ok, err
}

func (vs Vars) ToMap(ctx context.Context) (map[VarName]any, error) {
	events, err := vs.history(ctx)
	if err != nil {
		return nil, err
	}
	bindings, err := visibleVarsOf(ctx, events).bindings()
	if err != nil {
		return nil, err
	}
	var m = make(map[VarName]any, len(bindings))
	for _, b := range bindings {
		m[b.Name] = b.Value
	}
	return m, nil
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
	Path  Path
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
	var vv = visibleVarsOf(ctx, events)
	// A value that can't be resolved is not the same value, so it doesn't make
	// the assignment a no-op. The assignment itself doesn't need to resolve the
	// previous value, which is why the error is not reported from here.
	if prev, ok, err := vv.lookup(name); err == nil && ok && comp.Equal(prev.Value, val) { // idempotent execution
		return nil
	}
	// A set only applies to a variable whose declaration is already folded in,
	// so the declaration has to come first. The fold orders by EventID, which is
	// a UUIDv7 and therefore already in creation order, so the ordering does not
	// rest on these timestamps. They are still derived from a single reading and
	// spaced apart, so that the recorded times agree with that order instead of
	// contradicting it under a coarse or frozen clock.
	var now = timeNow()
	var ref = vv.resolve(name)
	var scope Path
	if ref.declared() {
		scope = ref.decl.Scope
	} else {
		// There is no variable of this name to assign to yet, so the assignment
		// declares one first: an iteration variable in the loop body iteration it
		// belongs to, and anything else in the current variable scope. The
		// declare and the set are recorded as two separate events without a
		// transaction: each is idempotent on its own, and the intermediate state
		// — declared but not yet assigned — is a valid one that a retry simply
		// resumes from.
		scope = CurrentPath(ctx).VarScope()
		if ref.iteration != nil {
			scope = ref.iteration.Path
		}
		if err := vs.recordDeclaration(ctx, name, scope, now); err != nil {
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
		Path:      declarationPath(CurrentPath(ctx), scope),
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
	var scope Path
	if !global {
		scope = CurrentPath(ctx).VarScope()
	}
	return vs.recordDeclaration(ctx, name, scope, timeNow())
}

// recordDeclaration records the declaration of a variable in a variable scope,
// which is the variable scope of the current Path or one enclosing it.
func (vs Vars) recordDeclaration(ctx context.Context, name VarName, scope Path, at time.Time) error {
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var declareEvent Event = EventDeclareVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: at,
		Path:      declarationPath(CurrentPath(ctx), scope),
		Name:      name,
	}
	return vs.EventsRepository.Create(ctx, &declareEvent)
}

// declarationPath is the Path of a variable event made at path, about a variable of scope.
//
// The scope is the VarScope of path, or a variable scope enclosing it.
// The variable scopes of path which are nested in scope get unmarked,
// so the VarScope of the event's Path is the scope of its variable,
// while the Path keeps the names of the location the event was made at.
func declarationPath(path, scope Path) Path {
	path = slices.Clone(path)
	copy(path, scope)
	for i := len(scope); i < len(path); i++ {
		path[i].Var = false
	}
	return path
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
	// The deletion is recorded against the variable the name refers to from the
	// current variable scope, and only when there is such a variable. This keeps
	// a deletion from reaching variables the caller can't even see.
	var (
		now   = timeNow()
		ref   = visibleVarsOf(ctx, events).resolve(name)
		scope Path
	)
	switch {
	case ref.declared():
		scope = ref.decl.Scope
	case ref.iteration != nil:
		// An iteration variable is not recorded as a variable, so to delete it,
		// it gets declared in its loop body iteration first, the same way an
		// assignment to it would declare it.
		scope = ref.iteration.Path
		if err := vs.recordDeclaration(ctx, name, scope, now); err != nil {
			return err
		}
		now = now.Add(time.Nanosecond)
	default:
		return nil
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventDeleteVar{
		EventID:   eventID,
		ProcessID: vs.ProcessID,
		Timestamp: now,
		Path:      declarationPath(CurrentPath(ctx), scope),
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
	if vs.varMutationDone(events, EventDeleteVar{}.EventType(), CurrentPath(ctx), name) {
		return nil
	}
	return vs.delete(ctx, events, name)
}

// varMutationDone reports whether the Process event history already contains a
// variable mutation event recorded for a given logical step.
//
// A step is identified by the pair of
//
//   - Path — where in the definition tree the mutation was executed, and
//   - Name — which variable it mutates.
//
// The Path pins the mutation to a single execution of a single location of the
// workflow definition, as repeated executions of the very same location, such
// as loop iterations, are at Paths of their own.
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

// varDecl is a single variable: the binding an EventDeclareVar brought into
// existence, as the assignments and the deletions made to it since left it.
type varDecl struct {
	Name  VarName
	Scope Path
	Value any
	// deleted marks a variable which got deleted. A deleted variable is not
	// found anymore, but it keeps shadowing the variables of the same name that
	// enclosing scopes hold, the same way it shadowed them before.
	deleted bool
}

// varFold is the variables of a Process, folded out of its event history.
//
// Events are folded in creation order, which the EventID encodes, so the fold
// does not depend on the wall clock the Timestamps happened to be taken from.
//
// Every declaration is a variable of its own, which lives in the variable
// scope it was declared in. A variable scope holds at most one variable of a
// name, so the assignments and the deletions refer to a variable by its Name
// and its Scope. As the events are folded in order, that is always the very
// variable the assignment or the deletion reached when it was made. So an
// explicit re-declaration in a nested scope is a new variable that shadows the
// enclosing one of the same name, while an assignment keeps changing whichever
// variable it was made to, no matter what got declared elsewhere since.
//
// The iterations of ForEach loop bodies are folded next to the variables, but
// apart from them: an iteration binds its iteration variables without
// declaring them, so it never replaces a variable, nor gets replaced by one.
type varFold struct {
	// scoped holds the variables of each name, one per variable scope, as a
	// re-declaration in the same scope replaces the variable.
	scoped map[VarName][]*varDecl
	// iterations holds the ForEach loop body iterations, in creation order.
	iterations []EventForEachIteration
}

func foldVars(events []Event) varFold {
	var f = varFold{scoped: map[VarName][]*varDecl{}}
	events = slicekit.Clone(events)
	sortEvents(events)
	for _, e := range events {
		switch e := e.(type) {
		case EventDeclareVar:
			// A declaration is the `var name` half of a variable: it brings the
			// variable into existence, carrying no value of its own.
			var d = &varDecl{Name: e.Name, Scope: e.Path.VarScope()}
			f.scoped[d.Name] = append(slices.DeleteFunc(f.scoped[d.Name], func(o *varDecl) bool {
				return o.Scope.Equal(d.Scope)
			}), d)
		case EventSetVar:
			if d, ok := f.variableOf(e.Name, e.Path.VarScope()); ok {
				d.Value, d.deleted = e.Value, false
			}
		case EventDeleteVar:
			if d, ok := f.variableOf(e.Name, e.Path.VarScope()); ok {
				d.Value, d.deleted = nil, true
			}
		case EventForEachIteration:
			f.iterations = append(f.iterations, e)
		}
	}
	return f
}

// historyAt is the history of a Process as it was when the event at index i
// got recorded: the events up to and including it, together with every ForEach
// loop body iteration on record.
//
// An iteration tells where the steps within it are, rather than changing the
// state of the Process, so it holds for every step within it, whether it got
// recorded before or after them. A Process from before the iterations got
// recorded has its iterations recorded after its steps, once it is replayed.
func historyAt(events []Event, i int) []Event {
	var history = slices.Clone(events[:i+1])
	for _, e := range events[i+1:] {
		if it, ok := e.(EventForEachIteration); ok {
			history = append(history, it)
		}
	}
	return history
}

// variableOf finds the variable of a name which lives in a variable scope.
func (f varFold) variableOf(name VarName, scope Path) (*varDecl, bool) {
	for _, d := range f.scoped[name] {
		if d.Scope.Equal(scope) {
			return d, true
		}
	}
	return nil, false
}

// visible returns the variables of a name which are visible from a position of
// the workflow, the innermost first.
//
// A variable is visible from every position within the variable scope it was
// declared in, nested scopes included. The visible scopes are all prefixes of
// the position's Path, so they form a single chain, each holding at most one
// variable of a name, so the innermost is the one the name refers to and the
// others are shadowed by it.
func (f varFold) visible(name VarName, at Path) []*varDecl {
	var decls []*varDecl
	for _, d := range f.scoped[name] {
		if at.MatchPrefix(d.Scope) {
			decls = append(decls, d)
		}
	}
	slices.SortFunc(decls, func(a, b *varDecl) int {
		return len(b.Scope) - len(a.Scope)
	})
	return decls
}

// iteration finds the innermost ForEach loop body iteration which binds a name,
// visible from a position of the workflow.
//
// An iteration is visible from every position within it, the iterations of the
// loops nested in its body included. The Paths of the visible iterations are
// all prefixes of the position's Path, so the longest is the innermost.
// An iteration is recorded once at its Path, and should it be on record more
// than once, the first recorded one is kept.
func (f varFold) iteration(name VarName, at Path) (EventForEachIteration, bool) {
	var (
		found EventForEachIteration
		ok    bool
	)
	for _, it := range f.iterations {
		if !it.binds(name) || !at.MatchPrefix(it.Path) {
			continue
		}
		if !ok || len(found.Path) < len(it.Path) {
			found, ok = it, true
		}
	}
	return found, ok
}

// visibleVars is how the variables of a Process are seen from a position of
// the workflow: the variables declared in the Process event history, together
// with the iteration variables of the ForEach loop body iterations recorded in
// it, as they are visible from the Path of the position.
//
// Every access to the variables resolves a name through it — the reads, from
// the single variable Vars#Lookup to the whole-scope Vars#ToMap and Vars#All,
// just like the writes — so none of them can disagree on which variable a name
// refers to.
type visibleVars struct {
	fold varFold
	// at is the Path of the position.
	at Path
}

func visibleVarsOf(ctx context.Context, events []Event) visibleVars {
	return visibleVars{fold: foldVars(events), at: CurrentPath(ctx)}
}

// varRef is what a variable name refers to: either a declared variable, or
// an iteration variable of a loop body, or nothing at all.
type varRef struct {
	decl      *varDecl
	iteration *EventForEachIteration
}

// declared tells whether the name refers to a declared variable which still
// exists.
func (ref varRef) declared() bool {
	return ref.decl != nil && !ref.decl.deleted
}

// resolve tells which variable a name refers to, the same way Go resolves an
// identifier: the innermost one wins.
//
// An iteration variable belongs to the variable scope of its loop body
// iteration, so it shadows the variables declared in the enclosing scopes.
// In turn, a variable declared within the loop body iteration shadows the
// iteration variable, and so does the declaration which the first assignment
// to an iteration variable makes.
//
// No other execution of a loop body shares the variable scope of an iteration,
// so a visible declaration in that scope, or in one nested in it, was made
// within the very same iteration. Both the iteration's scope and the visible
// declarations' scopes are prefixes of the position's Path, so the longer one
// is the inner one, whichever was recorded first.
func (vv visibleVars) resolve(name VarName) varRef {
	var ref varRef
	if decls := vv.fold.visible(name, vv.at); 0 < len(decls) {
		ref.decl = decls[0]
	}
	if it, ok := vv.fold.iteration(name, vv.at); ok {
		if ref.decl == nil || len(ref.decl.Scope) < len(it.Path) {
			return varRef{iteration: &it}
		}
	}
	return ref
}

// lookup resolves a variable by its name.
//
// found reports that the name refers to a variable, which is not the same as
// the variable having a value: a variable which was declared but not yet
// assigned is found, holding a nil value.
func (vv visibleVars) lookup(name VarName) (VarBinding, bool, error) {
	var ref = vv.resolve(name)
	switch {
	case ref.iteration != nil:
		value, ok, err := vv.iterationValue(*ref.iteration, name)
		if err != nil || !ok {
			return VarBinding{}, false, err
		}
		return VarBinding{Name: name, Value: value, Path: ref.iteration.Path}, true, nil
	case ref.declared():
		return VarBinding{Name: name, Value: ref.decl.Value, Path: ref.decl.Scope}, true, nil
	default:
		return VarBinding{}, false, nil
	}
}

// iterationValue resolves an iteration variable of a loop body.
//
// The element of the iteration is located in the collection by the position
// its Path names, and while it can't be located, neither of the iteration
// variables is found. The collection is resolved the way the loop sees it,
// from where the loop is, and not from within the loop body, so the loop body
// can't hide its own collection by declaring a variable of the same name.
func (vv visibleVars) iterationValue(it EventForEachIteration, name VarName) (any, bool, error) {
	var loop = visibleVars{fold: vv.fold, at: it.loopPath()}
	over, ok, err := loop.lookup(it.Over)
	if err != nil || !ok {
		return nil, false, err
	}
	key, value, ok := locateIteration(it.Path, over.Value)
	if !ok {
		return nil, false, nil
	}
	if name == it.K {
		return key, true, nil
	}
	return value, true, nil
}

// bindings resolves every visible variable through #lookup, ordered by name.
func (vv visibleVars) bindings() ([]VarBinding, error) {
	var names []VarName
	for name := range vv.fold.scoped {
		if 0 < len(vv.fold.visible(name, vv.at)) {
			names = append(names, name)
		}
	}
	for _, it := range vv.fold.iterations {
		if !vv.at.MatchPrefix(it.Path) {
			continue
		}
		for _, name := range it.names() {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	var bindings = make([]VarBinding, 0, len(names))
	for _, name := range names {
		b, ok, err := vv.lookup(name)
		if err != nil {
			return nil, err
		}
		if ok {
			bindings = append(bindings, b)
		}
	}
	return bindings, nil
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
		bindings, err := visibleVarsOf(ctx, eventsVS).bindings()
		if err != nil {
			yield(VarBinding{}, err)
			return
		}
		for _, b := range bindings {
			if !yield(b, nil) {
				return
			}
		}
	}
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

func (DeclareVar) Error() string { return "workflow:declare-var" }

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

func (SetVar) Error() string { return "workflow:set-var" }

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

func (DeleteVar) Error() string { return "workflow:delete-var" }

func (d DeleteVar) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "delete-var")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var vars = Vars{ProcessID: pid, EventsRepository: repo}
	return vars.deleteOnce(ctx, d.Name)
}
