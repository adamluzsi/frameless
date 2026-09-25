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

func (sig Complete) Error() string { return "workflow::complete" }

func (sig Complete) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	for event, err := range rt.Events.FindByProcessID(ctx, id) {
		if err != nil {
			return err
		}
		if completed, ok := event.(EventCompleted); ok && completed.ProcessID.Equal(id) {
			return nil
		}
		if _, ok := event.(EventTerminated); ok {
			return nil
		}
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

// IsCompleted reports whether pid has completed — it ran to its natural end
// and has not subsequently been terminated.
//
// A Process' outcome is recorded in the event log as either EventCompleted
// (it ran to its end) or EventTerminated (it was called off). When the log
// holds both for one Process — which can only arise from outside the runtime
// because Complete and Terminate refuse to write over an outcome already on
// record — IsCompleted answers false: the Process has been called off, and
// the trailing completion does not undo that. Symmetrically, IsTerminated
// answers false in the same history.
func IsCompleted(ctx context.Context, events EventRepository, pid ProcessID) (bool, error) {
	var (
		completed  bool
		terminated bool
	)
	for event, err := range events.FindByProcessID(ctx, pid) {
		if err != nil {
			return false, err
		}
		switch event.(type) {
		case EventCompleted:
			completed = true
		case EventTerminated:
			terminated = true
		}
	}
	return completed && !terminated, nil
}

// EventCompleted is emitted when a workflow definition successfully completes execution.
type EventCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
}

var _ Event = (*EventCompleted)(nil)

func (EventCompleted) EventType() EventType      { return "workflow::completed" }
func (e EventCompleted) GetEventID() EventID     { return e.EventID }
func (e EventCompleted) GetProcessID() ProcessID { return e.ProcessID }
func (e EventCompleted) GetTimestamp() time.Time { return e.Timestamp }

//---

type Terminate struct{}

var _ RuntimeSignal = Terminate{}

func (sig Terminate) Error() string { return "workflow::terminate" }

func (sig Terminate) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	return sig.sigTERM(ctx, rt, id)
}

func (sig Terminate) sigTERM(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	if id.IsZero() {
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
	const cancelPropagationWait = 10 * time.Millisecond
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

func (sig Terminate) terminate(ctx context.Context, rt Runtime, id ProcessID) (rerr error) {
	ctx, err := rt.Events.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rerr, rt.Events, ctx)
	var children []ProcessID
	for event, err := range rt.Events.FindByProcessID(ctx, id) {
		if err != nil {
			return err
		}
		if spawn, ok := event.(EventSpawn); ok {
			children = append(children, spawn.ChildID)
		}
		if _, ok := event.(EventCompleted); ok {
			return nil
		}
		if terminated, ok := event.(EventTerminated); ok && terminated.ProcessID.Equal(id) {
			return nil
		}
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
	if err := rt.Events.Create(ctx, &event); err != nil {
		return err
	}
	for _, childID := range children {
		if err := sig.RuntimeSignalExecute(ctx, rt, childID); err != nil {
			return err
		}
	}
	return nil
}

// IsTerminated reports whether pid was called off — terminated without
// completing — and has not subsequently completed.
//
// A Process' outcome is recorded in the event log as either EventCompleted
// (it ran to its end) or EventTerminated (it was called off). When the log
// holds both for one Process — which can only arise from outside the runtime
// because Complete and Terminate refuse to write over an outcome already on
// record — IsTerminated answers false: the Process ran to its end, and a
// trailing termination does not undo that. Symmetrically, IsCompleted answers
// false in the same history.
func IsTerminated(ctx context.Context, events EventRepository, pid ProcessID) (bool, error) {
	var (
		completed  bool
		terminated bool
	)
	for event, err := range events.FindByProcessID(ctx, pid) {
		if err != nil {
			return false, err
		}
		switch event.(type) {
		case EventCompleted:
			completed = true
		case EventTerminated:
			terminated = true
		}
	}
	return terminated && !completed, nil
}

type EventTerminated struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
}

var _ Event = (*EventTerminated)(nil)

func (EventTerminated) EventType() EventType      { return "workflow::terminated" }
func (e EventTerminated) GetEventID() EventID     { return e.EventID }
func (e EventTerminated) GetProcessID() ProcessID { return e.ProcessID }
func (e EventTerminated) GetTimestamp() time.Time { return e.Timestamp }
