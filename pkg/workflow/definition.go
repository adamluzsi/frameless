package workflow

import (
	"context"
	"encoding/json"

	"go.llib.dev/frameless/pkg/jsonkit"
)

type Sequence []Definition

var _ = jsonkit.Register[Sequence]("workflow.Sequence")

var _ Definition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, p *Process) error {
	for _, participant := range seq {
		if err := participant.Execute(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

var _ json.Marshaler = (Sequence)(nil)

func (s Sequence) MarshalJSON() ([]byte, error) {
	var list = jsonkit.Array[Definition](s)
	return json.Marshal(list)
}

var _ json.Unmarshaler = (*Sequence)(nil)

func (s *Sequence) UnmarshalJSON(data []byte) error {
	var list jsonkit.Array[Definition]
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*s = Sequence(list)
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
