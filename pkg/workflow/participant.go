package workflow

import (
	"context"
	"reflect"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds/dsmap"
)

type ExecuteParticipant struct {
	ID     ParticipantID `json:"id"`
	Input  []VariableKey `json:"input,omitempty"`
	Output []VariableKey `json:"output,omitempty"`
}

var _ Definition = (*ExecuteParticipant)(nil)

func (d *ExecuteParticipant) Execute(ctx context.Context, p *Process) error {
	return d.execute(ctx, p)
}

func (d *ExecuteParticipant) execute(ctx context.Context, p *Process) error {
	pr, ok := ctxParticipantsH.Lookup(ctx)
	if !ok {
		return ErrFatal.F("missing participant mapping from workflow runtime")
	}
	participant, found, err := pr.FindByID(ctx, d.ID)
	if err != nil {
		return err
	}
	if !found {
		return ErrParticipantNotFound{ID: d.ID}
	}

	fn, err := participant.rfn(ctx)
	if err != nil {
		return err
	}

	var args []reflect.Value
	args = append(args, reflect.ValueOf(ctx))
	for i, key := range d.Input {
		value, ok := p.Variables.Lookup(key)
		if !ok { // validate this at process definition level too as static validation
			return ErrFatal.F("missing participant input argument: input argument of #%d -> %s", i, key)
		}
		rval := reflect.ValueOf(value)
		// TODO: cover
		// switch reflect.ValueOf(value) {
		// default:
		args = append(args, rval)
		// }
	}

	if len(args) != fn.Type().NumIn() {
		const format = "participant execution arguments don't match the input arguments mapping.\nsignature in the format of func(inputs) (outputs)\n%s"
		return ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature(ctx))
	}

	var lastIsError bool
	var expectedOuputMappingLen = fn.Type().NumOut()
	if 0 < expectedOuputMappingLen {
		lastOut := fn.Type().Out(expectedOuputMappingLen - 1)
		if lastOut == reflectErrorType || lastOut.Implements(reflectErrorType) {
			expectedOuputMappingLen-- // we don't count error output with output mapping
			lastIsError = true
		}
	}

	if len(d.Output) != expectedOuputMappingLen {
		const format = "participant execution result values count don't match the output mapping\nsignature in the format of func(inputs) (outputs)\n%s"
		return ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature(ctx))
	}

	var out = fn.Call(args)
	if lastIsError {
		if errRV, ok := slicekit.Last(out); ok {
			if err, ok := errRV.Interface().(error); ok && err != nil {
				return err
			}
		}
	}

	for i, vn := range d.Output {
		p.Variables.Set(vn, out[i].Interface())
	}

	return nil
}

func (d *ExecuteParticipant) cachedExecute(ctx context.Context, p *Process, do func() error) (rerr error) {
	prc, ok := executionCacheH.Lookup(ctx)
	if !ok {
		return ErrMissingParticipantExecutionCache
	}

	offset := prc.indexes.Get(d.ID)

	// prc.offset.Set(d.ID, offset+1)

	p.Cache[participantCacheKey{}]
	p.Cache.results[participantCacheKey{
		CallIndex: offset,
	}]

	return nil
}

// var _ ConditionConveratble = (*ExecuteParticipant)(nil)

// func (d ExecuteParticipant) ToCondition(ctx context.Context, p *Process) (Condition, bool) {
// 	return nil, false
// }

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type ParticipantCache struct {
}

type ExecuteEvent struct {
	CallIndex     int           `json:"index"`
	ParticipantID ParticipantID `json:"pid,omitempty"`
	ConditionID   ConditionID   `json:"cid,omitempty"`
	Input         []any         `json:"input"`
	Output        []any         `json:"output"`
}

const ErrMissingParticipantExecutionCache errorkitlite.Error = `ErrMissingParticipantExecutionCache
missing from execution context, initiate it with
workflow.Runtime#Context or workflow.WithParticipantExecuteCache`

var executionCacheH contextkit.ValueHandler[ctxKeyCache, *ParticipantExecuteCache]

type ctxKeyCache struct{}

func WithParticipantExecuteCache(ctx context.Context) context.Context {
	return executionCacheH.ContextWith(ctx, &ParticipantExecuteCache{})
}

type ParticipantExecuteCache struct {
	indexes dsmap.Map[ParticipantID, int]
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
