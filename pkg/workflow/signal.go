package workflow

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/port/comproto"
)

// Complete is a completion RuntimeSignal.
type Complete struct{}

var _ RuntimeSignal = Complete{}

func (sig Complete) Error() string { return "workflow:complete" }

func (sig Complete) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	for event, err := range rt.Events.FindByProcessID(ctx, id) {
		if err != nil {
			return err
		}
		if completed, ok := event.(EventCompleted); ok && completed.ProcessID == id {
			return nil
		}
		if _, ok := event.(EventTerminated); ok {
			return nil
		}
	}
	if err := runDeferred(ctx, rt, id); err != nil {
		return err
	}
	var eventID, err = MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventCompleted{
		EventID:   eventID,
		ProcessID: id,
		Timestamp: timeNow(),
	}
	return rt.Events.Create(ctx, &event)
}

// IsCompleted reports whether pid ran to its natural end without being terminated.
func IsCompleted(ctx context.Context, events EventRepository, pid ProcessID) (bool, error) {
	o, err := readOutcome(ctx, events, pid)
	return o.completed && !o.terminated, err
}

// IsFinished reports whether pid completed or was terminated, in one history read.
// An unknown, suspended or halted process has no outcome and is not finished.
// If both outcomes are present, the history is inconsistent: like IsCompleted and
// IsTerminated, IsFinished reports false. A failed read returns false and the error.
func IsFinished(ctx context.Context, events EventRepository, pid ProcessID) (bool, error) {
	o, err := readOutcome(ctx, events, pid)
	return o.completed != o.terminated, err
}

type processOutcome struct{ completed, terminated bool }

func readOutcome(ctx context.Context, events EventRepository, pid ProcessID) (processOutcome, error) {
	var o processOutcome
	for event, err := range events.FindByProcessID(ctx, pid) {
		if err != nil {
			return processOutcome{}, err
		}
		switch event.(type) {
		case EventCompleted:
			o.completed = true
		case EventTerminated:
			o.terminated = true
		}
	}
	return o, nil
}

// EventCompleted is emitted when a workflow definition successfully completes execution.
type EventCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
}

var _ Event = (*EventCompleted)(nil)

func (EventCompleted) EventType() EventType      { return "workflow:completed" }
func (e EventCompleted) GetEventID() EventID     { return e.EventID }
func (e EventCompleted) GetProcessID() ProcessID { return e.ProcessID }
func (e EventCompleted) GetTimestamp() time.Time { return e.Timestamp }

//---

type Terminate struct{}

var _ RuntimeSignal = Terminate{}

func (sig Terminate) Error() string { return "workflow:terminate" }

func (sig Terminate) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	return sig.sigTERM(ctx, rt, id)
}

// How often Terminate rechecks the lock after asking its holder to unwind.
const cancelPropagationWait = 10 * time.Millisecond

func (sig Terminate) sigTERM(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	if id == (ProcessID{}) {
		return ErrZeroProcessID.F("terminate")
	}
	var (
		lockContext context.Context
		unlock      func() error
	)
	// Give the in-flight worker a moment to react to the cancel before
	// we re-check the lock. Without this wait, sigTERM would spin as a
	// tight CPU loop: each iteration publishes another cancel and tries
	// to re-acquire the lock before the worker has had a chance to
	// observe it and release the lock. The wait here is what lets the
	// worker drain its current step, write its outcome, and return from
	// Execute so the lock becomes available.
	for {
		var (
			acquired bool
			lockErr  error
		)
		lockContext, acquired, unlock, lockErr = rt.tryLock(ctx, id)
		if lockErr != nil {
			return lockErr
		}
		if acquired { // no processing is running, and it is safe to terminate
			break
		}
		if err := rt.Notifications.Publish(ctx, ProcessCancel{ProcessID: id}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cancelPropagationWait):
		}
	}
	defer errorkit.Finish(&rerr, unlock)
	return sig.terminate(lockContext, rt, id)
}

func (sig Terminate) terminate(ctx context.Context, rt Runtime, id ProcessID) error {
	ended, children, err := sig.inspect(ctx, rt, id)
	if err != nil {
		return err
	}
	if ended {
		return nil
	}
	// The children are called off first, so their deferred Definitions unwind before their parent's.
	for _, childID := range children {
		if err := sig.RuntimeSignalExecute(ctx, rt, childID); err != nil {
			return err
		}
	}
	// The deferred Definitions run outside the transaction that records the termination.
	// Their steps commit as they finish, so a failure midway doesn't roll back the ones that already ran,
	// and the next attempt resumes the unwinding instead of redoing it.
	if err := runDeferred(ctx, rt, id); err != nil {
		return err
	}
	return sig.record(ctx, rt, id)
}

func (sig Terminate) record(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	ctx, err := rt.Events.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rerr, rt.Events, ctx)
	ended, _, err := sig.inspect(ctx, rt, id)
	if err != nil {
		return err
	}
	if ended {
		return nil
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventTerminated{
		EventID:   eventID,
		ProcessID: id,
		Timestamp: timeNow(),
	}
	return rt.Events.Create(ctx, &event)
}

// inspect reports whether the Process already has its outcome on record, and which child Processes it spawned.
func (sig Terminate) inspect(ctx context.Context, rt Runtime, id ProcessID) (ended bool, children []ProcessID, _ error) {
	for event, err := range rt.Events.FindByProcessID(ctx, id) {
		if err != nil {
			return false, nil, err
		}
		if spawn, ok := event.(EventSpawn); ok {
			children = append(children, spawn.ChildID)
		}
		if _, ok := event.(EventCompleted); ok {
			return true, nil, nil
		}
		if terminated, ok := event.(EventTerminated); ok && terminated.ProcessID == id {
			return true, nil, nil
		}
	}
	return false, children, nil
}

// IsTerminated reports whether pid was called off without completing.
func IsTerminated(ctx context.Context, events EventRepository, pid ProcessID) (bool, error) {
	o, err := readOutcome(ctx, events, pid)
	return o.terminated && !o.completed, err
}

type EventTerminated struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
}

var _ Event = (*EventTerminated)(nil)

func (EventTerminated) EventType() EventType      { return "workflow:terminated" }
func (e EventTerminated) GetEventID() EventID     { return e.EventID }
func (e EventTerminated) GetProcessID() ProcessID { return e.ProcessID }
func (e EventTerminated) GetTimestamp() time.Time { return e.Timestamp }
