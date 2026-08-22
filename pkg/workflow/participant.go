package workflow

import (
	"context"
	"reflect"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
)

type ExecuteParticipant struct {
	ID     ParticipantID
	Input  []VarName
	Output []VarName
}

var _ Definition = (*ExecuteParticipant)(nil)

const executeParticipantJSONType jsonkit.TypeID = "workflow::participant"

func (ExecuteParticipant) Error() string { return executeParticipantJSONType.String() }

func (d ExecuteParticipant) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "participant")
	return d.cachedExecute(ctx, pid)
}

func (d ExecuteParticipant) execute(ctx context.Context, input []any) (_output []any, _ error) {
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

	fn, err := participant.rFunc()
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
		return nil, ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature())
	}

	var lastIsError bool
	var expectedOutputMappingLen = fn.Type().NumOut()
	if 0 < expectedOutputMappingLen {
		lastOut := fn.Type().Out(expectedOutputMappingLen - 1)
		if lastOut == reflectErrorType || lastOut.Implements(reflectErrorType) {
			expectedOutputMappingLen-- // we don't count error output with output mapping
			lastIsError = true
		}
	}

	if len(d.Output) != expectedOutputMappingLen {
		const format = "participant execution result values count don't match the output mapping\nsignature in the format of func(inputs) (outputs)\n%s"
		return nil, ErrParticipantFuncMappingMismatch.F(format, participant.funcSignature())
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

func (d *ExecuteParticipant) cachedExecute(ctx context.Context, pid ProcessID) (rErr error) {
	exec := idempotentExecutor[EventParticipant, ParticipantID]{
		ID:     d.ID,
		Do:     d.execute,
		Input:  d.Input,
		Output: d.Output,
		CastEvent: func(e EventParticipant) (executionEvent[ParticipantID], bool) {
			return executionEvent[ParticipantID]{
				ID:         e.ParticipantID,
				Path:       e.Path,
				Input:      e.Input,
				Output:     e.Output,
				Definition: e.Definition,
			}, true
		},
		MakeEvent: func(id ParticipantID, path Path, input, output []any) (EventParticipant, error) {
			eventID, err := MakeEventID()
			if err != nil {
				return EventParticipant{}, err
			}
			return EventParticipant{
				EventID:       eventID,
				ProcessID:     pid,
				Timestamp:     clock.Now().UTC(),
				ParticipantID: id,
				Path:          path,
				Input:         input,
				Output:        output,
			}, nil
		},
		AcceptDefinition: func(e *EventParticipant, def Definition) {
			e.Definition = def
		},
	}
	return exec.Execute(ctx, pid)
}

// var _ ConditionConveratble = (*ExecuteParticipant)(nil)

// func (d ExecuteParticipant) ToCondition(ctx context.Context, p *Process) (Condition, bool) {
// 	return nil, false
// }

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func getExecuteParticipantEvents(es []Event) []*EventParticipant {
	var epes []*EventParticipant
	for _, e := range es {
		if e == nil {
			continue
		}
		if e.EventType() != eidExecuteParticipantEvent {
			continue
		}
		if epe, ok := e.(*EventParticipant); ok {
			epes = append(epes, epe)
		}
	}
	return epes
}

type EventParticipant struct {
	EventID       EventID `ext:"id"`
	ProcessID     ProcessID
	Timestamp     time.Time
	ParticipantID ParticipantID
	Path          Path
	Input         []any
	Output        []any
	Definition    Definition
}

var _ Event = (*EventParticipant)(nil)

const eidExecuteParticipantEvent = "execute-participant"

func (EventParticipant) EventType() EventType      { return eidExecuteParticipantEvent }
func (e EventParticipant) GetEventID() EventID     { return e.EventID }
func (e EventParticipant) GetProcessID() ProcessID { return e.ProcessID }
func (e EventParticipant) GetTimestamp() time.Time { return e.Timestamp }
