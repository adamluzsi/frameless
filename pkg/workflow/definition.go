package workflow

import (
	"context"
	"reflect"

	"go.llib.dev/frameless/pkg/jsonkit"
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
var _ = jsonkit.Register[Sequence]("workflow.Sequence")

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
var _ = jsonkit.Register[If]("workflow.If")

func (d If) Execute(ctx context.Context, p *Process) error {
	var ok, err = d.Cond.Evaluate(ctx, p)
	if err != nil {
		return err
	}
	if ok {
		if d.Then != nil {
			return d.Then.Execute(ctx, p)
		}
	} else {
		if d.Else != nil {
			return d.Else.Execute(ctx, p)
		}
	}
	return nil
}
