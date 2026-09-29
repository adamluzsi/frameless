package workflow

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

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
func Defer(ctx context.Context, def Definition) error {
	pid, ok := ProcessIDFromContext(ctx)
	if !ok {
		return ErrFatal.F("missing Pid in workflow context")
	}
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	if def == nil {
		return ErrFatal.F("workflow.Defer: nil Definition")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	events, err := iterkit.CollectE(repo.FindByProcessID(ctx, pid))
	if err != nil {
		return err
	}
	var (
		path  = CurrentPath(ctx)
		scope = CurrentVarScope(ctx)
	)
	for _, e := range events {
		if hook, ok := e.(EventDefer); ok &&
			slicekit.Equal(hook.Path, path) &&
			slicekit.Equal(hook.VarScope, scope) {
			return nil
		}
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventDefer{
		EventID:    eventID,
		ProcessID:  pid,
		Timestamp:  timeNow(),
		Path:       path,
		VarScope:   scope,
		Definition: def,
	}
	return repo.Create(ctx, &event)
}

// A hook needs no completion event of its own. It runs as an extension of the
// normal workflow, on a path unique to the hook (see EventDefer#Context), so its
// steps are recorded and replayed by the same idempotent execution as any other
// step: once its last step is on record, a re-run skips through the whole hook.
//
// EventDefer marks a hook registered to a specific Process.
type EventDefer struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	// Path is the registration's location.
	Path Path
	// VarScope is the variable scope the registration was made in.
	// The hook's Definition runs in a scope nested under it (see EventDeferHook#Context).
	VarScope   VarScope
	Definition Definition
}

func (e EventDefer) GetEventID() EventID     { return e.EventID }
func (e EventDefer) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDefer) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDefer) EventType() EventType    { return "workflow::defer" }

// Context returns the context in which the hooked Definition is meant to run.
//
// Whatever path and variable scope ctx carries is replaced with the declaration's,
// each extended with the hook's EventID:
// the Path becomes <hook Path>/<EventID>,
// and the VarScope becomes <hook VarScope>/<EventID>.
// Every other value of ctx, such as the runtime, is kept.
func (e EventDefer) Context(ctx context.Context) context.Context {
	var segment = e.EventID.String()
	ctx = withPath(ctx, ctxCurrentPath, append(slicekit.Clone(e.Path), segment))
	return withPath(ctx, ctxVarScope, append(slicekit.Clone(e.VarScope), segment))
}
