package workflow

import (
	"context"
	"reflect"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type ExecuteParticipant struct {
	ID     ParticipantID `json:"id"`
	Input  []VariableKey `json:"input,omitempty"`
	Output []VariableKey `json:"output,omitempty"`
}

var _ Definition = (*ExecuteParticipant)(nil)
var _ = jsonkit.Register[ExecuteParticipant]("workflow.ExecuteParticipant")

func (d *ExecuteParticipant) Execute(ctx context.Context, p *Process) error {
	return d.cachedExecute(ctx, p)
}

func (d *ExecuteParticipant) execute(ctx context.Context, input []any) (_output []any, _ error) {
	pr, ok := ctxParticipantsH.Lookup(ctx)
	if !ok {
		return nil, ErrFatal.F("missing participant mapping from workflow runtime")
	}
	participant, found, err := pr.FindByID(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrParticipantNotFound{ID: d.ID}
	}

	fn, err := participant.rfn(ctx)
	if err != nil {
		return nil, err
	}

	var args []reflect.Value
	args = append(args, reflect.ValueOf(ctx))
	for _, value := range input {
		// TODO: validate input argument type by func argument type
		// TODO: cover with extra tests
		// TODO: extend functionality with use-cases where similar kinds can be used interchangeably, as long they can be converted to another.
		rval := reflect.ValueOf(value)

		// switch reflect.ValueOf(value) {
		// default:
		args = append(args, rval)
		// }
	}

	if len(args) != fn.Type().NumIn() {
		const format = "participant execution arguments don't match the input arguments mapping.\nsignature in the format of func(inputs) (outputs)\n%s"
		return nil, ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature(ctx))
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
		return nil, ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature(ctx))
	}

	var out = fn.Call(args)

	var output []any
	if lastIsError {
		if errRV, ok := slicekit.Last(out); ok {
			if err, ok := errRV.Interface().(error); ok && err != nil {
				return nil, err
			}
			// dispose last error from output values
			out = out[:len(out)-1]
		}
	}

	for _, val := range out {
		output = append(output, val.Interface())
	}

	return output, nil
}

func (d *ExecuteParticipant) cachedExecute(ctx context.Context, p *Process) (rerr error) {
	exec := idempotentExecutor[ExecuteParticipantEvent, ParticipantID]{
		ID:     d.ID,
		Func:   d.execute,
		Input:  d.Input,
		Output: d.Output,
		CastEvent: func(e ExecuteParticipantEvent) (executionEvent[ParticipantID], bool) {
			return executionEvent[ParticipantID]{
				ID:     e.ParticipantID,
				Input:  e.Input,
				Output: e.Output,
			}, true
		},
		NewEvent: func(id ParticipantID, input, output []any) ExecuteParticipantEvent {
			return ExecuteParticipantEvent{
				ParticipantID: id,
				Input:         input,
				Output:        output,
			}
		},
	}
	return exec.Execute(ctx, p)
}

// var _ ConditionConveratble = (*ExecuteParticipant)(nil)

// func (d ExecuteParticipant) ToCondition(ctx context.Context, p *Process) (Condition, bool) {
// 	return nil, false
// }

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func getExecuteParticipantEvents(es []Event) []ExecuteParticipantEvent {
	var epes []ExecuteParticipantEvent
	for _, e := range es {
		if e == nil {
			continue
		}
		if e.Type() != eidExecuteParticipantEvent {
			continue
		}
		if epe, ok := e.(ExecuteParticipantEvent); ok {
			epes = append(epes, epe)
		}
	}
	return epes
}

type ExecuteParticipantEvent struct {
	ParticipantID ParticipantID `json:"pid,omitempty"`
	Input         []any         `json:"input"`
	Output        []any         `json:"output"`
}

var _ = jsonkit.Register[ExecuteParticipantEvent]("workflow:event:ExecuteParticipant")

const eidExecuteParticipantEvent = "execute-participant"

func (ExecuteParticipantEvent) Type() EventType {
	return eidExecuteParticipantEvent
}
