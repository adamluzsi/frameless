package workflow

import (
	"context"
	"fmt"
	"slices"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/comproto"
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
	Do        func(ctx context.Context, input []any) (output []any, _ error)
	Input     []VarKey
	Output    []VarKey
	CastEvent func(e E) (executionEvent[ID], bool)
	// MakeEvent returns an E value (intentionally not a *E) which can be used with Event interface variables.
	MakeEvent func(id ID, path Path, input []any, output []any) (E, error)
	// AcceptDefinition configures idempotentExecutor to let it know,
	// the E event type supports signal persistence.
	AcceptDefinition func(e *E, def Definition)
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

	var (
		mEvents    []Event
		matchingEE executionEvent[ID]
		found      bool
	)

	eventsRepo, err := LookupEventsRepository(ctx)
	if err != nil {
		return nil, err
	}

	var events []Event
	for event, err := range eventsRepo.FindByProcessID(ctx, pid) {
		if err != nil {
			return nil, err
		}

		events = append(events, event)

		e, ok := event.(E)
		if !ok {
			continue
		}

		ee, ok := ie.CastEvent(e)
		if !ok {
			continue
		}

		if ee.ID == ie.ID && slices.Equal(ee.Path, path) {
			found = true
			matchingEE = ee
			mEvents = slicekit.Clone(events)
			break
		}
	}

	if found {
		if len(ie.Input) == len(matchingEE.Input) {
			for i, key := range ie.Input {
				// invalidate on input value mismatch
				// it is idempotent only if input arguments the same too.
				//
				// mEvents holds the history up to and including the matching
				// execution event, so the variable value is evaluated as of that
				// historical position rather than the current one.
				historicalValue, _ := lookupVariable(mEvents, key)
				if !reflectkit.Equal(historicalValue, matchingEE.Input[i]) {
					found = false
					break
				}
			}
		} else { // invalidate previous call since input argument count changed
			found = false
		}
	}

	if found && len(ie.Output) != len(matchingEE.Output) {
		found = false
	}

	if found {
		// since as part of normal execution,
		// the event history is updated with variable mutation already
		// we are good to just return with the result here
		return slicekit.Clone(matchingEE.Result), nil
	}

	vars := ProcessVars{
		ProcessID:        pid,
		EventsRepository: eventsRepo,
	}

	var input []any = make([]any, len(ie.Input))
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

	output, err := ie.Do(ctx, slicekit.Clone(input))
	if err != nil {
		if def, ok := err.(Definition); ok {
			return ie.handleResultDefinition(ctx, eventsRepo, pid, path, input, def)
		}
		return nil, err
	}

	{
		// memorize the call event, and make it idempotent for the next occurrence
		// transaction might be needed here,
		// but to pull it off scientifically correctly requires some thinking.
		var newEvent Event
		newEvent, err = ie.MakeEvent(ie.ID, path, input, output)
		if err != nil {
			return nil, err
		}
		if !newEvent.GetProcessID().Equal(pid) {
			return nil, fmt.Errorf("incorrect MakeEvent[%T] implementation, process id mismatch", newEvent)
		}
		if err := eventsRepo.Create(ctx, &newEvent); err != nil {
			return nil, err
		}
	}

	rt, ok := RuntimeFromContext(ctx)
	if !ok {
		return nil, ErrFatal.F("missing workflow runtime in context")
	}
	if rt.Events == nil {
		return nil, ErrFatal.F("missing events repository in workflow runtime")
	}

	ctx, err = rt.Events.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, rt.Events, ctx)

	// add new variables as well to the event history
	for i, key := range ie.Output {
		if err := vars.Set(ctx, key, output[i]); err != nil {
			return nil, err
		}
	}

	return slicekit.Clone(output), nil
}

// handleResultDefinition will handle a Definition result from a participant's
// function execution.
//
// A Definition return value is a "happy" error: it represents control flow
// that the runtime will execute on the caller's behalf (e.g. Spawn launches a
// sub-workflow, SwitchDefinition mutates the active definition). It is
// handled here so that:
//
//   - The event is persisted with the Definition attached, making a replay
//     of the same logical step a no-op.
//   - Dispatching the Definition happens once per logical step (the second
//     pass is a cache hit and short-circuits before this branch).
//
// The cached event's Definition is replayed as nil output, so the surrounding
// Definition can continue normally without surfacing the returned Definition.
func (ie idempotentExecutor[E, ID]) handleResultDefinition(
	ctx context.Context, eventsRepo EventRepository,
	pid ProcessID, path Path, input []any, def Definition,
) (_ []any, rErr error) {
	if def == nil {
		return nil, fmt.Errorf("nil workflow.Definition is an implementation error")
	}
	event, err := ie.MakeEvent(ie.ID, path, input, nil)
	if err != nil {
		return nil, err
	}
	if ie.AcceptDefinition != nil {
		ie.AcceptDefinition(&event, def)
	}
	var newEvent Event = event
	if !newEvent.GetProcessID().Equal(pid) {
		return nil, fmt.Errorf("incorrect MakeEvent[%T] implementation, process id mismatch", event)
	}
	if err := eventsRepo.Create(ctx, &newEvent); err != nil {
		return nil, err
	}
	if err := def.Execute(ctx, pid); err != nil {
		return nil, err
	}
	return nil, nil
}
