package workflow

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow/internal"
)

// Defer is a step that registers its Definition to run when the Process ends, much like Go's defer does for a function.
//
// The deferred Definitions run when the Process completes, whether it reaches the end of its Definition or a step raises Complete,
// and when it is terminated, whether a step raises Terminate or Runtime#Terminate is called.
// They run in the reverse order of their registration (LIFO),
// before the outcome (EventCompleted or EventTerminated) is recorded.
// A Definition deferred while the deferred Definitions are running
// runs right after the one which registered it, like a nested defer in Go.
// A Process that suspends or halts hasn't ended, so its deferred Definitions wait.
//
// Defer is a step of its own, and it records its registration on its own, apart from the steps that follow it.
// Place it ahead of the step whose effect it cleans up after,
// so the cleanup is on record even if that step's events fail to persist after its effect already took place:
//
//	workflow.Sequence{
//		workflow.Defer{Definition: workflow.Execute{Participant: "release-seat"}},
//		workflow.Execute{Participant: "reserve-seat"},
//	}
//
// The deferred Definition runs at every ending of the Process,
// so it has to tolerate that the effect it cleans up after never happened, or was already undone.
// A participant may also return a Defer as its follow-up Definition;
// that registration is recorded together with the participant's call, and it is lost along with it.
//
// A deferred Definition runs like any other part of the workflow, with no completion event of its own.
// Its steps are recorded and replayed idempotently, so the Definition should be made of idempotent steps.
// If one fails, the unwinding stops, the error is returned and the outcome is not recorded.
// The next attempt runs the deferred Definitions again,
// skipping the steps already on record and retrying the one that failed.
// A deferred Definition raising Complete or Terminate only ends itself,
// while other signals, like Suspend, stop the unwinding just like an error does.
//
// Defer is not a rollback hook: a deferred Definition needs a working EventRepository to run.
// To undo a participant's effect right when its call fails to be recorded, use txkit.OnRollback.
//
// The hooked Definition shares the location of the Defer step.
// The registration is made at the step's own path, under a "workflow::defer" segment,
// and in the variable scope the step runs in.
// Both are extended with a segment of the hook's own, named after its EventID,
// so a Defer at foo/bar/baz runs its Definition at foo/bar/baz/workflow::defer/<hook event id>,
// and in the <step scope>/<hook event id> variable scope.
// So hooks never collide on their idempotent steps or on their local variables,
// while they still see every variable visible to the Defer step.
//
// A registration is identified by its Path and VarScope,
// so replaying the same step doesn't register the hook twice,
// while the same step re-entered under a new variable scope (e.g. a loop iteration) registers its own.
// A Defer without a Definition has nothing to defer, so it registers nothing.
type Defer struct {
	Definition Definition
}

var _ Definition = Defer{}

func (d Defer) Error() string { return "workflow::defer" }

func (d Defer) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "defer")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	if d.Definition == nil {
		return nil // no definition, nothing to defer
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var (
		path  = CurrentPath(ctx)
		scope = CurrentVarScope(ctx)
	)
	for event, err := range repo.FindByProcessID(ctx, pid) {
		if err != nil {
			return err
		}
		ed, ok := event.(EventDefer)
		if !ok {
			continue
		}
		if slicekit.Equal(ed.Path, path) && slicekit.Equal(ed.VarScope, scope) {
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
		Definition: d.Definition,
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
	// The hook's Definition runs in a scope nested under it (see EventDefer#Context).
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

// runDeferred unwinds the Definitions deferred by the Process, newest first.
//
// The event log is read again after each deferred Definition,
// so the ones registered while unwinding are run as well.
func runDeferred(ctx context.Context, rt Runtime, pid ProcessID) error {
	if _, ok := RuntimeFromContext(ctx); !ok {
		// The TTL budget is for scheduled executions,
		// and an unwinding from outside of one, like Runtime#Terminate, has nobody to resume it.
		var unbudgeted = rt
		unbudgeted.TTL = 0
		ctx = unbudgeted.Context(ctx)
	}
	// ctx may belong to another Process, like a parent cascading its termination to its children.
	ctx = internal.ContextProcessID.ContextWith(ctx, pid)
	var done = make(map[EventID]struct{})
	for {
		hook, ok, err := nextDeferred(ctx, rt.Events, pid, done)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		done[hook.EventID] = struct{}{}
		if err := hook.Definition.Execute(hook.Context(ctx), pid); err != nil && !endsDeferred(err) {
			return err
		}
	}
}

// nextDeferred finds the most recently registered deferred Definition that is not done yet.
func nextDeferred(ctx context.Context, repo EventRepository, pid ProcessID, done map[EventID]struct{}) (EventDefer, bool, error) {
	var (
		next  EventDefer
		found bool
	)
	for event, err := range repo.FindByProcessID(ctx, pid) {
		if err != nil {
			return EventDefer{}, false, err
		}
		hook, ok := event.(EventDefer)
		if !ok {
			continue
		}
		if _, ok := done[hook.EventID]; ok {
			continue
		}
		next, found = hook, true
	}
	return next, found, nil
}

// endsDeferred reports whether err only ends the deferred Definition that raised it.
//
// Like the Runtime's own signal dispatch, it is a plain type check, so a wrapped signal counts as a failure.
func endsDeferred(err error) bool {
	switch err.(type) {
	case Complete, Terminate:
		return true
	default:
		return false
	}
}
