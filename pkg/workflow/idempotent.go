package workflow

import (
	"context"
	"errors"
	"fmt"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/txkit"
)

// idempotentExecutor records each invocation in the process' event history and
// replays the recorded result when the same logical step is encountered again.
//
// Identity is established through the workflow.Path carried in the execution
// context (see workflow.WithName / workflow.CurrentPath): two invocations share
// the same logical identity when they (a) declare the same ID and (b) execute
// under the same path. The path is provided by the surrounding Definition
// hierarchy — for example, a Sequence contributes one segment per iteration —
// so callers do not need to maintain any per-process state themselves.
type idempotentExecutor[E Event, ID ~string] struct {
	ID        ID
	Do        nonIdempotentFunc
	Input     []VarName
	Output    []VarName
	CastEvent func(e E) (executionEvent[ID], bool)
	// MakeEvent takes the call's position in history before Do. The event is
	// appended only after Do returns a successful result or a follow-up.
	MakeEvent func(path Path, input []any) (E, error)
	SetOutput func(e *E, output []any)
	// AcceptDefinition attaches a step's follow-up in place of its outputs.
	AcceptDefinition func(e *E, def Definition)
	// MakeEventError, when set, returns an Event value (typically a workflow.EventError)
	// describing the failure produced by Do. It is recorded once per genuine failure
	// so that the workflow has a per-occurrence audit trail of what went wrong.
	//
	// It is only invoked for errors that are neither a workflow.RuntimeSignal
	// (control flow) nor a workflow.Definition (happy follow-up); those are
	// handled as control flow and successful results respectively.
	//
	// When nil, no error event is recorded (the legacy behaviour).
	MakeEventError func(id ID, path Path, err error) (EventError, error)
}

type nonIdempotentFunc func(ctx context.Context, input []any) (output []any, _ error)

func (ie idempotentExecutor[E, ID]) commitEventsTx(rErr *error, eventsRepo EventRepository, tx context.Context, pid ProcessID) (committed bool) {
	// The commit decision cannot be made on "is rErr nil", because in this
	// package a non-nil error is not necessarily a failure.
	//
	// A RuntimeSignal is not an error. It is the runtime's own control flow
	// (suspend, replace, complete), raised deliberately by a step that ran fine,
	// so it must not be handled as a rainy case.
	//
	// That distinction matters here because these transactions are layered:
	// steps nest, and every nesting level opens its own transaction around the
	// events it records. The layers have no checkpoints between them — a
	// rollback is reported upwards as a failure, and the level above takes it
	// for a rainy case and rolls back in turn, all the way to the top. Rolling
	// back on a signal would therefore not undo "the signal"; it would discard
	// every event recorded by every enclosing step, because one step at the
	// bottom used control flow.
	//
	// Those events describe work that genuinely happened — variables set earlier
	// in the process, and the enclosing steps' own recorded executions.
	// TestRuntime_Execute_participantFollowUpDefinition pins one such case.
	// Missing or incompatible participants likewise yield to another node;
	// preserve prior work unless an independent fatal error is also present.
	// Classify a break as success only for the commit decision. The loop
	// still needs the original error to exit; without a loop it is a failure.
	err := *rErr
	if loopPID, ok := ctxEnclosingLoop.Lookup(tx); ok && loopPID == pid {
		err = loopDone(err)
	}
	if err != nil && !isRuntimeSignal(err) && (ErrIsFatal(err) || (!isParticipantNotFound(err) && !isParticipantSignatureMismatch(err))) {
		*rErr = errorkitlite.Merge(*rErr, eventsRepo.RollbackTx(tx))
		return false
	}
	if err := eventsRepo.CommitTx(tx); err != nil {
		*rErr = errorkitlite.Merge(*rErr, err)
		return false
	}
	return true
}

// finishDoTx settles the txkit transaction the non-idempotent function ran in.
//
// It is a best-effort compensation, not a distributed transaction:
// when the event log did not record the call (the function failed, or the
// events could not be persisted), the rollback steps registered through
// txkit.OnRollback are run, so the function gets a chance to undo its side
// effects before it is repeated. Once the call is recorded, it is committed.
func finishDoTx(rErr *error, doTx context.Context, committed bool) {
	// doTx derives from the event log's transaction, which is already finished
	// (and possibly canceled) by now; that must not turn a commit into a rollback.
	doTx = context.WithoutCancel(doTx)
	if committed {
		_ = txkit.Commit(doTx)
		return
	}
	if err := txkit.Rollback(doTx); err != nil && !errors.Is(err, txkit.ErrTxDone) {
		*rErr = errorkitlite.Merge(*rErr, err)
	}
}

type executionEvent[ID ~string] struct {
	// ID of the participant/condition/etc which did the trick
	ID ID
	// Path identifies the position within the definition tree at which the
	// execution took place. Two events with the same ID but different paths
	// represent distinct logical steps and are cached independently.
	Path Path

	// Input is the cached input variable setting
	Input []any
	// Output is the cached output variable setting
	Output []any
	// Result is the cached return value of the executeWR
	Result []any
	// Definition in case the execution result is a Definition,
	// then we handle that as a happy case and cache it too
	Definition Definition
}

func (ie idempotentExecutor[E, ID]) Execute(ctx context.Context, pid ProcessID) error {
	_, err := ie.executeWR(ctx, pid)
	return err
}

func (ie idempotentExecutor[E, ID]) executeWR(ctx context.Context, pid ProcessID) (_ []any, rErr error) {
	ctx = WithName(ctx, string(ie.ID))

	path := CurrentPath(ctx)

	eventsRepo, err := LookupEventsRepository(ctx)
	if err != nil {
		return nil, err
	}

	// Nested calls share an audit buffer. Persist it only after the outermost
	// executor has finished its transaction: an independent write can otherwise
	// wait on that transaction's own lock, even if it has only read history.
	audits, ok := ctx.Value(ctxKeyErrorEventRecorder{}).(*errorEventRecorder)
	if !ok {
		audits = new(errorEventRecorder)
		defer audits.flush(ctx)
		ctx = context.WithValue(ctx, ctxKeyErrorEventRecorder{}, audits)
	}

	tx, err := eventsRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	// tx is finished instead of ctx on purpose: ctx is reassigned further down
	// for the output variable transaction, and a closure would capture that
	// later value rather than this transaction.
	var (
		ttl  *ctxTTL
		doTx context.Context
	)
	// Every normal return supplies an explicit error (including nil). A panic
	// bypasses that assignment, so default to rollback while it unwinds.
	rErr = errorkitlite.Error("workflow execution did not return")
	defer func() {
		committed := ie.commitEventsTx(&rErr, eventsRepo, tx, pid)
		if doTx != nil {
			finishDoTx(&rErr, doTx, committed)
		}
		ttl.Finish(&rErr, tx)
	}()

	ctx = tx

	events, err := iterkit.CollectE(eventsRepo.FindByProcessID(ctx, pid))
	if err != nil {
		return nil, err
	}
	sortEvents(events)
	if matchingEE, found := ie.recordedCall(ctx, path, events); found {
		// The producer is complete, but its recorded follow-up may still be
		// suspended. Resume it under the original path and transaction so its
		// completed steps remain cached and unfinished work is not skipped.
		if matchingEE.Definition != nil {
			if err := matchingEE.Definition.Execute(ctx, pid); err != nil {
				return nil, err
			}
		}
		// Variable mutations are already recorded in the event history.
		return slicekit.Clone(matchingEE.Result), nil
	}

	ttl, _ = ctxTimeToLiveH.Lookup(ctx)
	if ttl != nil {
		ttl.Active++
	}

	vars := Vars{
		ProcessID:        pid,
		EventsRepository: eventsRepo,
	}

	// A call without Input records no Input values, so the event is the same after a round-trip through a codec.
	var input []any
	if len(ie.Input) > 0 {
		input = make([]any, len(ie.Input))
	}
	for i, key := range ie.Input {
		value, ok, err := vars.Lookup(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok { // validate this at process definition level too as static validation
			return nil, ErrFatal.F("missing input argument: input argument of #%d -> %s", i, key)
		}
		input[i] = value
	}

	// Take the EventID (and timestamp) at entry, not after Do's writes.
	// This makes historyAt the input snapshot, even if a previous signaling
	// attempt already wrote variables at exactly this path.
	call, err := ie.MakeEvent(path, input)
	if err != nil {
		return nil, err
	}
	if call.GetProcessID() != pid {
		return nil, fmt.Errorf("incorrect MakeEvent[%T] implementation, process id mismatch", call)
	}

	doTx, err = txkit.Begin(ctx)
	if err != nil {
		doTx = nil
		return nil, err
	}

	// The success event keeps the arguments as read, even if the participant
	// mutates a map, slice, or a value nested inside one during the call.
	output, err := ie.Do(doTx, reflectkit.CloneT(input))
	def, followUp := err.(Definition)
	switch {
	case followUp:
		ie.AcceptDefinition(&call, def)
	case err != nil:
		// A RuntimeSignal is the runtime's own control flow (suspend, terminate,
		// complete, halt) and is deliberately raised by a step that ran fine.
		// It is not a failure to be audited, so it must not produce an EventError.
		//
		// A missing or incompatible participant alone is a node availability
		// issue, not a failed call. Independently fatal errors still count as
		// genuine failures. Record failures as
		// an EventError so the process has a per-occurrence audit trail.
		if !isRuntimeSignal(err) && (ErrIsFatal(err) || (!isParticipantNotFound(err) && !isParticipantSignatureMismatch(err))) {
			ie.recordErrorEvent(audits, eventsRepo, pid, path, err)
		}
		return nil, err
	default:
		ie.SetOutput(&call, output)
	}

	var event Event = call
	if err := eventsRepo.Create(ctx, &event); err != nil {
		return nil, err
	}
	if followUp {
		return nil, def.Execute(ctx, pid)
	}

	// add new variables as well to the event history
	for i, key := range ie.Output {
		if err := vars.Set(ctx, key, output[i]); err != nil {
			return nil, err
		}
	}

	return slicekit.Clone(output), nil
}

type ctxKeyErrorEventRecorder struct{}

// errorEventRecorder keeps failure audits outside all enclosing executor
// transactions. It is local to one execution tree, not retained across retries.
// The originating repository is kept with each event in case a nested call
// uses another runtime.
type errorEventRecorder struct {
	pending synckit.Slice[errorEventRecord]
}

type errorEventRecord struct {
	repository EventRepository
	event      EventError
}

func (r *errorEventRecorder) flush(ctx context.Context) {
	for _, pending := range r.pending.ToSlice() {
		var event Event = pending.event
		_ = pending.repository.Create(ctx, &event)
	}
}

// recordErrorEvent constructs an EventError at the point of failure, retaining
// its original error, path and timestamp. Persistence waits until all enclosing
// executor transactions have finished, so it neither waits on their locks nor
// loses the audit to their rollback.
//
// It is best-effort: event construction and persistence failures are swallowed.
// The audit record is a diagnostic, never a reason to mask the original error.
func (ie idempotentExecutor[E, ID]) recordErrorEvent(audits *errorEventRecorder, eventsRepo EventRepository, pid ProcessID, path Path, gotErr error) {
	if ie.MakeEventError == nil {
		return
	}
	eventError, err := ie.MakeEventError(ie.ID, path, gotErr)
	if err != nil {
		return
	}
	if eventError.GetProcessID() != pid {
		return
	}
	audits.pending.Append(errorEventRecord{repository: eventsRepo, event: eventError})
}

// recordedCall returns the first compatible success at this coordinate. A
// genuine mapping miss appends a new call; the older incompatible one must
// not hide it on the next replay. events is in EventID order.
func (ie idempotentExecutor[E, ID]) recordedCall(ctx context.Context, path Path, events []Event) (executionEvent[ID], bool) {
nextCall:
	for i, event := range events {
		e, ok := event.(E)
		if !ok {
			continue
		}
		call, ok := ie.CastEvent(e)
		if !ok || call.ID != ie.ID || !call.Path.Equal(path) || len(call.Input) != len(ie.Input) {
			continue
		}
		// Definition results intentionally discard outputs, regardless of mapping.
		if call.Definition == nil && len(ie.Output) != len(call.Output) {
			continue
		}
		if len(ie.Input) != 0 {
			vars := visibleVarsOf(ctx, historyAt(events, i))
			for k, name := range ie.Input {
				v, found, err := vars.lookup(name)
				if err != nil || !found || !reflectkit.Equal(v.Value, call.Input[k]) {
					continue nextCall
				}
			}
		}
		return call, true
	}
	return executionEvent[ID]{}, false
}
