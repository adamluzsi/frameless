package workflow

import (
	"context"
	"encoding/json"

	"go.llib.dev/frameless/pkg/jsonkit"
)

type Sequence []Definition

var _ = jsonkit.RegisterTypeID[Sequence]("workflow::sequence")

var _ Definition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, p *Process) error {
	for _, participant := range seq {
		if err := participant.Execute(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

const sequenceJSONType jsonkit.TypeID = "workflow::sequence"

var _ json.Marshaler = (Sequence)(nil)

func (s Sequence) MarshalJSON() ([]byte, error) {
	var result = make([]json.RawMessage, len(s))
	for i, def := range s {
		data, err := json.Marshal(def)
		if err != nil {
			return nil, err
		}
		result[i] = data
	}

	type dto struct {
		Type  string            `json:"@type"`
		Value []json.RawMessage `json:"@value"`
	}
	return json.Marshal(dto{
		Type:  sequenceJSONType.String(),
		Value: result,
	})
}

var _ json.Unmarshaler = (*Sequence)(nil)

func (s *Sequence) UnmarshalJSON(data []byte) error {
	type dto struct {
		Type  string            `json:"@type"`
		Value []json.RawMessage `json:"@value"`
	}
	var d dto
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	var defs = make(Sequence, len(d.Value))
	for i, raw := range d.Value {
		var iface jsonkit.Interface[Definition]
		if err := json.Unmarshal(raw, &iface); err != nil {
			return err
		}
		defs[i] = iface.V
	}
	*s = defs
	return nil
}

type If struct {
	Cond Condition  `json:"cond"`
	Then Definition `json:"then"`
	Else Definition `json:"else,omitempty"`
}

var _ Definition = (*If)(nil)

const ifJSONType jsonkit.TypeID = "workflow::if"

var _ = jsonkit.RegisterTypeID[If]("workflow::if")

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

type dtoJSONIf struct {
	Type string          `json:"@type"`
	Cond json.RawMessage `json:"cond,omitempty"`
	Then json.RawMessage `json:"then,omitempty"`
	Else json.RawMessage `json:"else,omitempty"`
}

func (d If) MarshalJSON() ([]byte, error) {
	var condMsg, thenMsg, elseMsg json.RawMessage
	var err error

	if d.Cond != nil {
		condMsg, err = json.Marshal(d.Cond)
		if err != nil {
			return nil, err
		}
	}
	if d.Then != nil {
		thenMsg, err = json.Marshal(d.Then)
		if err != nil {
			return nil, err
		}
	}
	if d.Else != nil {
		elseMsg, err = json.Marshal(d.Else)
		if err != nil {
			return nil, err
		}
	}

	return json.Marshal(dtoJSONIf{
		Type: ifJSONType.String(),
		Cond: condMsg,
		Then: thenMsg,
		Else: elseMsg,
	})
}

func (d *If) UnmarshalJSON(data []byte) error {
	var dto dtoJSONIf
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}

	var condIface jsonkit.Interface[Condition]
	if len(dto.Cond) > 0 {
		if err := json.Unmarshal(dto.Cond, &condIface); err != nil {
			return err
		}
	}

	var thenIface jsonkit.Interface[Definition]
	if len(dto.Then) > 0 {
		if err := json.Unmarshal(dto.Then, &thenIface); err != nil {
			return err
		}
	}

	var elseIface jsonkit.Interface[Definition]
	if len(dto.Else) > 0 {
		if err := json.Unmarshal(dto.Else, &elseIface); err != nil {
			return err
		}
	}

	*d = If{
		Cond: condIface.V,
		Then: thenIface.V,
		Else: elseIface.V,
	}
	return nil
}
