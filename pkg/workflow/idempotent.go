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

type participantCacheValue struct {
	Output Variables
}

type pcRes struct {
}

type idempotentExecutor[E Event, ID ~string] struct {
	ID        ID
	Func      func(ctx context.Context, input []any) (output []any, _ error)
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
	// Result is the cached reutn value of the executeWR
	Result []any
}

func (ie idempotentExecutor[E, ID]) Execute(ctx context.Context, p *Process) (rerr error) {
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
		mindex     int = -1
		found      bool
	)
	for _, event := range p.Events {
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
			mindex++
		}

		if ee.ID == ie.ID && mindex == index {
			found = true
			matchingEE = ee
			mEvents = slicekit.Clone(events)
			break
		}
	}

	if found {
		var mProcess = Process{
			Variables: p.Variables, // this won't be needed after variables became part of the events
			Events:    mEvents,
		}

		if len(ie.Input) == len(matchingEE.Input) {
			for i, key := range ie.Input {
				// invalidate on input value mismatch
				// it is idempotent olny if input arguments the same too.
				if !reflectkit.Equal(mProcess.Variables.Get(key), matchingEE.Input[i]) {
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
		for i, key := range ie.Output {
			// TODO: remove me when variables are event log based
			// this won't be needed after Variables became part of the Events
			p.Variables.Set(key, matchingEE.Output[i])
		}
		return slicekit.Clone(matchingEE.Result), nil
	}

	var input []any = make([]any, len(ie.Input))
	for i, key := range ie.Input {
		value, ok := p.Variables.Lookup(key)
		if !ok { // validate this at process definition level too as static validation
			return nil, ErrFatal.F("missing input argument: input argument of #%d -> %s", i, key)
		}
		input[i] = value
	}

	output, err := ie.Func(ctx, slicekit.Clone(input))
	if err != nil {
		return nil, err
	}

	for i, key := range ie.Output {
		p.Variables.Set(key, output[i])
	}

	newEvent := ie.NewEvent(ie.ID, input, output)
	{
		// memorise the call event, and make it idempotent for the next occurence
		// transaction might be needed here,
		// but to pull it off sciencifically correctly requires some thinking.
		incCallIndex(ei, ie.ID)
		p.Events = append(p.Events, newEvent)
	}

	return slicekit.Clone(output), nil
}
