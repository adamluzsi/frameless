package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/validate"
)

type ProcessDefinition interface {
	Participant
	JSONSerialisable
	validate.Validatable
}

func PID[STR ~string](s STR) *ParticipantID {
	var pid = ParticipantID(s)
	return &pid
}

// ParticipantID is the process definition ID,
type ParticipantID string

var _ ProcessDefinition = (*ParticipantID)(nil)

func (pid ParticipantID) Execute(ctx context.Context, s *State) error {
	if err := pid.Validate(ctx); err != nil {
		return err
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok := mapkit.Lookup(c.Participants, pid)
	if !ok || p == nil {
		return ErrParticipantNotFound.F("pid=%s", pid)
	}
	return p.Execute(ctx, s)
}

func (pid ParticipantID) Validate(ctx context.Context) error {
	if len(pid) == 0 {
		return validate.Error{Cause: fmt.Errorf("empty participant ID")}
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok := mapkit.Lookup(c.Participants, pid)
	if !ok || p == nil {
		return ErrParticipantNotFound.F("pid=%s", pid)
	}
	return nil
}

func (pid ParticipantID) MarshalJSON() (_ []byte, _ error) {
	return json.Marshal(string(pid))
}

func (pid *ParticipantID) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*pid = ParticipantID(v)
	return nil
}

type Sequence []ProcessDefinition

var _ ProcessDefinition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, s *State) error {
	for _, participant := range seq {
		if err := participant.Execute(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (seq Sequence) Validate(ctx context.Context) error {
	for i, def := range seq {
		var name = fmt.Sprintf("seq[%d]", i)
		if err := vd(ctx, name, def, true); err != nil {
			return err
		}
	}
	return nil
}

func (s Sequence) MarshalJSON() ([]byte, error) {
	type T Sequence
	return json.Marshal(T(s))
}

func (s *Sequence) UnmarshalJSON(data []byte) error {
	type T Sequence
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = Sequence(v)
	return nil
}

func vd(ctx context.Context, name string, v validate.Validatable, req bool) error {
	if req && v == nil {
		return validate.Error{Cause: fmt.Errorf("%s is nil", name)}
	}
	if v == nil {
		return nil
	}
	return v.Validate(ctx)
}

type If struct {
	Cond Condition
	Then ProcessDefinition
	Else ProcessDefinition
}

var _ ProcessDefinition = (*If)(nil)

func (d If) Execute(ctx context.Context, p *State) error {
	ok, err := d.Cond.Evaluate(ctx, p)
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

func (d If) Validate(ctx context.Context) error {
	if err := vd(ctx, "if.cond", d.Cond, true); err != nil {
		return err
	}
	if err := vd(ctx, "if.then", d.Then, true); err != nil {
		return err
	}
	if err := vd(ctx, "if.else", d.Else, false); err != nil {
		return err
	}
	return nil
}

func (d If) MarshalJSON() ([]byte, error) {
	type T If
	return json.Marshal(T(d))
}

func (d *If) UnmarshalJSON(data []byte) error {
	type T If
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*d = If(v)
	return nil
}
