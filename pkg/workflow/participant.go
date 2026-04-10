package workflow

import (
	"context"
	"reflect"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type ExecuteParticipant struct {
	ID     ParticipantID `json:"id"`
	Input  []VariableKey `json:"input,omitempty"`
	Output []VariableKey `json:"output,omitempty"`
}

var _ Definition = (*ExecuteParticipant)(nil)

func (d *ExecuteParticipant) Execute(ctx context.Context, p *Process) error {
	return d.cachedExecute(ctx, p)
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
		// TODO: cover with extra tests
		// TODO: extend functionality with use-cases where similar kinds can be used interchangeably, as long they can be converted to another.
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

func (d *ExecuteParticipant) cachedExecute(ctx context.Context, p *Process) (rerr error) {
	prc, ok := executionCacheH.Lookup(ctx)
	if !ok {
		return ErrMissingExecutionIndex
	}

	pindex := prc.ParticipantCallIndex(d.ID)
	events := getExecuteParticipantEvents(p.Events)

	prevCall, found := iterkit.Find(iterkit.ToV(slicekit.IterReverse(events)), func(e ExecuteParticipantEvent) bool {
		// find last matching call by call index and PID .
		return e.ParticipantID == d.ID && e.CallIndex == pindex
	})

	if found {
		if len(d.Input) == len(prevCall.Input) {
			for i, key := range d.Input {
				// invalidate on input value mismatch
				// it is idempotent olny if input arguments the same too.
				if p.Variables.Get(key) != prevCall.Input[i] {
					found = false
					break
				}
			}
		} else {
			found = false // invalidate previous call
		}
	}

	if found && len(d.Output) != len(prevCall.Output) {
		found = false
	}

	if found {
		for i, key := range d.Output {
			p.Variables.Set(key, prevCall.Output[i])
		}
		return nil
	}

	var newEvent ExecuteParticipantEvent
	newEvent.CallIndex = pindex
	newEvent.ParticipantID = d.ID
	newEvent.Input = make([]any, len(d.Input))
	for i, key := range d.Input {
		newEvent.Input[i] = p.Variables.Get(key)
	}

	err := d.execute(ctx, p)
	if err != nil {
		return err
	}

	newEvent.Output = make([]any, len(d.Output))
	for i, key := range d.Output {
		newEvent.Output[i] = p.Variables.Get(key)
	}

	{
		// memorise the call event, and make it idempotent for the next occurence
		// transaction might be needed here,
		// but to pull it off sciencifically correctly requires some thinking.
		prc.IncrementParticipantCallIndex(d.ID)
		p.Events = append(p.Events, newEvent)
	}

	return nil
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
	CallIndex     int           `json:"index"`
	ParticipantID ParticipantID `json:"pid,omitempty"`
	Input         []any         `json:"input"`
	Output        []any         `json:"output"`
}

const eidExecuteParticipantEvent = "execute-participant"

func (ExecuteParticipantEvent) Type() EventType {
	return eidExecuteParticipantEvent
}
