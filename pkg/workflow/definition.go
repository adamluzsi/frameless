package workflow

import (
	"context"
	"encoding/json"
	"reflect"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type ValidateDefinitionContext struct {
	context.Context

	vs map[VariableKey]ValidateDefinitionContextVariable
}

// func (vdc ValidateDefinitionContext) Variables() ds.Map[VariableKey, ValidateDefinitionContextVariable] {
// 	return dsmap.Map[VariableKey, ValidateDefinitionContextVariable](vdc.vs)
// }

type ValidateDefinitionContextVariable struct {
	Key  VariableKey
	Type reflect.Type
}

type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, p *Process) error {
	for _, participant := range seq {
		if err := participant.Execute(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

type If struct {
	Cond Condition  `json:"cond"`
	Then Definition `json:"then"`
	Else Definition `json:"else,omitempty"`
}

var _ Definition = (*If)(nil)

func (d If) Execute(ctx context.Context, p *Process) error {
	var ok, err = d.Cond.Evaluate(ctx, p)
	if err != nil {
		return err
	}
	if ok {
		return d.Then.Execute(ctx, p)
	} else if d.Else != nil {
		return d.Else.Execute(ctx, p)
	}
	return nil
}

type ExecuteParticipant struct {
	ID     ParticipantID `json:"id"`
	Input  []VariableKey `json:"input,omitempty"`
	Output []VariableKey `json:"output,omitempty"`
}

var _ Definition = (*ExecuteParticipant)(nil)

func (d *ExecuteParticipant) Execute(ctx context.Context, p *Process) error {
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
	for _, key := range d.Input {
		value, ok := p.Variables.Lookup(key)
		if !ok { // validate this at process definition level too as static validation
			return ErrFatal.F("missing participant input argument: %s", key)
		}
		args = append(args, reflect.ValueOf(value))
	}

	if len(d.Output) != fn.Type().NumOut()-1 { // exept the last err argument
		return ErrParticipantFuncMappingMismatch.F("invalid mapping for return value mapping")
	}

	out := fn.Call(args)
	if errRV, ok := slicekit.Last(out); ok {
		if err, ok := errRV.Interface().(error); ok && err != nil {
			return err
		}
	}

	return nil
}

var _ json.Marshaler = (*ExecuteParticipant)(nil)

func (d ExecuteParticipant) MarshalJSON() ([]byte, error) {
	type T ExecuteParticipant
	return json.Marshal(T(d))
}

var _ json.Unmarshaler = (*ExecuteParticipant)(nil)

func (d *ExecuteParticipant) UnmarshalJSON(data []byte) error {
	return jsonkit.DefaultUnmarshalJSON[ExecuteParticipant](data, d)
}

var _ ConditionConveratble = (*ExecuteParticipant)(nil)

func (d ExecuteParticipant) ToCondition(ctx context.Context, p *Process) (Condition, bool) {

	return nil, false
}
