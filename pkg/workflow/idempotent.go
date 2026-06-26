package workflow

import (
	"context"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds/dsmap"
)

const ErrMissingExecutionIndex errorkitlite.Error = `ErrMissingParticipantExecutionCache
missing from execution context, initiate it with
workflow.Runtime#Context or workflow.WithParticipantExecuteCache`

var executionCacheH contextkit.ValueHandler[ctxKeyCache, *ExecutionIndex]

type ctxKeyCache struct{}

// WithExecutionIndex initializes or returns a context containing the execution cache.
// It is used to keep track of the participant execution indexes/offsets,
// recording which participant was executed how many times.
func WithExecutionIndex(ctx context.Context) context.Context {
	if _, ok := executionCacheH.Lookup(ctx); ok {
		return ctx
	}
	return executionCacheH.ContextWith(ctx, &ExecutionIndex{})
}

type ExecutionIndex struct {
	pind dsmap.Map[eiKeyInt, int]
}

type eiKeyInt interface{ key() }

type eiKey[T any] struct{ ID string }

var _ eiKeyInt = (*eiKey[any])(nil)

func (eiKey[T]) key() {}

func getCallIndex[ID ~string](ec *ExecutionIndex, id ID) int {
	return ec.pind.Get(eiKey[ID]{ID: string(id)})
}

func incCallIndex[ID ~string](ec *ExecutionIndex, id ID) {
	key := eiKey[ID]{ID: string(id)}
	ec.pind.Set(key, ec.pind.Get(key)+1)
}

func (ec *ExecutionIndex) IncrementParticipantCallIndex(pid ParticipantID) {
	incCallIndex(ec, pid)
}

func (ec *ExecutionIndex) ParticipantCallIndex(pid ParticipantID) int {
	return getCallIndex(ec, pid)
}

type participantCacheKey struct {
	CallIndex int
	Input     string // []VariableValue
}

type pcRes struct {
}

type idempotentExecutor[E Event, ID ~string] struct {
	ID        ID
	Do        func(ctx context.Context, input []any) (output []any, _ error)
	Input     []VariableKey
	Output    []VariableKey
	CastEvent func(e E) (executionEvent[ID], bool)
	NewEvent  func(id ID, input []any, output []any) E
}

type executionEvent[ID ~string] struct {
	// ID of the participant/condition/etc which did the trick
	ID ID
	// Input is the cached input variable setting
	Input []any
	// Output is the cached output variable setting
	Output []any
	// Result is the cached return value of the executeWR
	Result []any
}

func (ie idempotentExecutor[E, ID]) Execute(ctx context.Context, p *Process) error {
	_, err := ie.executeWR(ctx, p)
	return err
}

func (ie idempotentExecutor[E, ID]) executeWR(ctx context.Context, p *Process) ([]any, error) {
	ei, ok := executionCacheH.Lookup(ctx)
	if !ok {
		return nil, ErrMissingExecutionIndex
	}

	index := getCallIndex(ei, ie.ID)

	var events []Event

	var (
		mEvents    []Event
		matchingEE executionEvent[ID]
		mIndex     int = -1
		found      bool
	)
	history, err := p.History()
	if err != nil {
		return nil, err
	}
	for _, event := range history {
		events = append(events, event)

		e, ok := event.(E)
		if !ok {
			continue
		}

		ee, ok := ie.CastEvent(e)
		if !ok {
			continue
		}

		if ee.ID == ie.ID {
			mIndex++
		}

		if ee.ID == ie.ID && mIndex == index {
			found = true
			matchingEE = ee
			mEvents = slicekit.Clone(events)
			break
		}
	}

	if found {
		var mProcess = Process{
			Events: NewEvents(mEvents...),
		}

		if len(ie.Input) == len(matchingEE.Input) {
			for i, key := range ie.Input {
				// invalidate on input value mismatch
				// it is idempotent only if input arguments the same too.
				if !reflectkit.Equal(mProcess.Var().Get(key), matchingEE.Input[i]) {
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

	var input []any = make([]any, len(ie.Input))
	for i, key := range ie.Input {
		value, ok := p.Var().Lookup(key)
		if !ok { // validate this at process definition level too as static validation
			return nil, ErrFatal.F("missing input argument: input argument of #%d -> %s", i, key)
		}
		input[i] = value
	}

	output, err := ie.Do(ctx, slicekit.Clone(input))
	if err != nil {
		return nil, err
	}

	{
		// memorize the call event, and make it idempotent for the next occurrence
		// transaction might be needed here,
		// but to pull it off scientifically correctly requires some thinking.
		incCallIndex(ei, ie.ID)
		var newEvent Event = ie.NewEvent(ie.ID, input, output)
		if err := p.events().Create(ctx, &newEvent); err != nil {
			return nil, err
		}
	}

	// add new variables as well to the event history
	for i, key := range ie.Output {
		p.Var().Set(key, output[i])
	}

	return slicekit.Clone(output), nil
}
