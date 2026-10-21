package workflow

import (
	"context"
	"time"

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
// Defer is a step of its own, and it records its registration on its own.
// Like Go's defer, which follows the call that acquired what it releases,
// place it right after the step whose effect it cleans up after,
// so the cleanup is registered only once that effect took place and the step is on record:
//
//	workflow.Sequence{
//		workflow.Execute{Participant: "reserve-seat"},
//		workflow.Defer{Definition: workflow.Execute{Participant: "release-seat"}},
//	}
//
// If the step fails, or its effect takes place but its events fail to persist,
// the Process doesn't get past it, so there is no cleanup registered for it yet.
// The step is retried instead, so it has to stay repeatable,
// and it can undo its effect right away through txkit.OnRollback.
//
// A participant may also return a Defer as its follow-up Definition, once its effect took place.
// That registration is recorded in the same commit as the participant's call,
// while a Defer step placed after it commits separately,
// so a Process terminated between the two commits ends without the cleanup.
//
// A deferred Definition runs at every ending of the Process, the successful completion included,
// so it should only undo what the Process no longer needs.
// It runs like any other part of the workflow, with no completion event of its own.
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
// The registration is made at the step's own path, under a "defer" segment,
// and the hooked Definition runs there, in a variable scope of the hook's own, named after its EventID.
// So a Defer at foo/bar/baz runs its Definition at foo/bar/baz/defer/<hook event id>.
// So hooks never collide on their idempotent steps or on their local variables,
// while they still see every variable visible to the Defer step.
//
// A registration is identified by its Path,
// so replaying the same step doesn't register the hook twice,
// while the same step re-entered at another Path (e.g. in a loop iteration) registers its own.
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
	var path = CurrentPath(ctx)
	for event, err := range repo.FindByProcessID(ctx, pid) {
		if err != nil {
			return err
		}
		ed, ok := event.(EventDefer)
		if !ok {
			continue
		}
		if ed.Path.Equal(path) {
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
	// The hook's Definition runs in a variable scope nested under it (see EventDefer#Context).
	Path       Path
	Definition Definition
}

func (e EventDefer) GetEventID() EventID     { return e.EventID }
func (e EventDefer) GetProcessID() ProcessID { return e.ProcessID }
func (e EventDefer) GetTimestamp() time.Time { return e.Timestamp }
func (e EventDefer) EventType() EventType    { return "workflow::defer" }

// Context returns the context in which the hooked Definition is meant to run.
//
// Whatever Path ctx carries is replaced with the registration's,
// extended with a variable scope named after the hook's EventID: <hook Path>/<EventID>.
// The hook runs outside of the loop bodies that ctx may be in,
// and every other value of ctx, such as the runtime, is kept.
func (e EventDefer) Context(ctx context.Context) context.Context {
	ctx = atProcessRoot(ctx)
	ctx = withPath(ctx, e.Path)
	return WithVarScope(ctx, e.EventID.String())
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
