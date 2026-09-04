package workflow

import "context"

// Halt stops the current execution of a Process without marking it complete
// and without rescheduling it for further execution.
//
// Halt is a RuntimeSignal and only that. It is not a Definition and cannot
// appear inside a Sequence, an If, a loop, or anywhere else in the workflow
// tree — the only place Halt can come from is a participant function returning
// it in place of nil. A participant that returns Halt is telling the runtime
// to stop asking, on purpose, for reasons the participant alone knows.
//
// Halt is the no-reschedule cousin of Suspend. Suspend says "ask me again
// later, on your schedule"; Halt says "do not ask again, full stop". The
// runtime acknowledges the queue entry and walks away — the Process is left
// inert in the event log, not completed, not retried, not requeued. Resuming
// is the caller's job, by calling Runtime#Schedule again with the same
// ProcessID.
//
// Like every RuntimeSignal, raising Halt is not a step outcome: the
// participant call that produced it is deliberately not recorded in the event
// history, and the events recorded by enclosing steps are committed, not
// rolled back (see idempotent.commitEventsTx).
type Halt struct{}

var _ RuntimeSignal = Halt{}

// Error identifies the signal in error messages and stack traces.
func (Halt) Error() string { return "workflow::halt" }

// RuntimeSignalExecute returns the Halt so the scheduler can recognise it and
// acknowledge the queue entry without requeueing it. The signal is the
// contract; the scheduler decides what to do with it.
func (Halt) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	return Halt{}
}
