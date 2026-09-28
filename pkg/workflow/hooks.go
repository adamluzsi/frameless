package workflow

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

// Hooks is the Process bound registry of deferred definitions.
// Akin to Vars, its state lives in the Process's event log.
type Hooks struct {
	ProcessID        ProcessID
	EventsRepository EventRepository
}

// Defer will register a hook in the current process execution, as a cleanup.
// Defer will execute the hook at the end of the normal execution or when the application is marked as terminated/completed early on.
// When multiple Definition is set with consequent Defer calls, the execution order will be LIFO.
//
// The hooked Definition shares the location of its declaration.
// Both the Path and the VarScope of the context Defer was called with
// are extended with a segment of the hook's own, named after its EventID.
// Hooks registered at foo/bar/baz therefore run at foo/bar/baz/<hook event id>,
// and likewise in the <declaration scope>/<hook event id> variable scope.
// So hooks declared at the same place never collide on their idempotent steps
// or on their local variables, while they still see every variable visible to their declaration.
//
// A registration is identified by the Path and VarScope of the context it is made in,
// so replaying the same step doesn't register the hook twice,
// while the same step re-entered under a new variable scope (e.g. a loop iteration) registers its own.
func (h Hooks) Defer(ctx context.Context, def Definition) error {
	if def == nil {
		return ErrFatal.F("workflow.Hooks#Defer: nil Definition")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	events, err := iterkit.CollectE(h.EventsRepository.FindByProcessID(ctx, h.ProcessID))
	if err != nil {
		return err
	}
	var (
		path  = CurrentPath(ctx)
		scope = CurrentVarScope(ctx)
	)
	for _, e := range events {
		if hook, ok := e.(EventHook); ok &&
			slicekit.Equal(hook.Path, path) &&
			slicekit.Equal(hook.VarScope, scope) {
			return nil
		}
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventHook{
		EventID:    eventID,
		ProcessID:  h.ProcessID,
		Timestamp:  timeNow(),
		Path:       path,
		VarScope:   scope,
		Definition: def,
	}
	return h.EventsRepository.Create(ctx, &event)
}

// EventHook marks a hook registered to a specific Process.
type EventHook struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	// Path is the registration's location.
	Path Path
	// VarScope is the variable scope the registration was made in.
	// The hook's Definition runs in a scope nested under it (see EventHook#Context).
	VarScope   VarScope
	Definition Definition
}

func (e EventHook) GetEventID() EventID     { return e.EventID }
func (e EventHook) GetProcessID() ProcessID { return e.ProcessID }
func (e EventHook) GetTimestamp() time.Time { return e.Timestamp }
func (e EventHook) EventType() EventType    { return "workflow::hook" }

// Context returns the context in which the hooked Definition is meant to run.
//
// Whatever path and variable scope ctx carries is replaced with the declaration's,
// each extended with the hook's EventID:
// the Path becomes <hook Path>/<EventID>,
// and the VarScope becomes <hook VarScope>/<EventID>.
// Every other value of ctx, such as the runtime, is kept.
func (e EventHook) Context(ctx context.Context) context.Context {
	var segment = e.EventID.String()
	ctx = withPath(ctx, ctxCurrentPath, append(slicekit.Clone(e.Path), segment))
	return withPath(ctx, ctxVarScope, append(slicekit.Clone(e.VarScope), segment))
}

// EventHookCompleted is the event completion
type EventHookCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	// HookID is the event id of the hook registration,
	// which makes it uniquely identifiable.
	HookID EventID
}

func (e EventHookCompleted) GetEventID() EventID     { return e.EventID }
func (e EventHookCompleted) GetProcessID() ProcessID { return e.ProcessID }
func (e EventHookCompleted) GetTimestamp() time.Time { return e.Timestamp }
func (e EventHookCompleted) EventType() EventType    { return "workflow::hook::completed" }
