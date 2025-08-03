package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/pp"
)

type Definition interface {
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

var _ Definition = (*ParticipantID)(nil)

func (pid ParticipantID) Execute(ctx context.Context, s *State) error {
	if err := pid.Validate(ctx); err != nil {
		return err
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok := mapkit.Lookup(c.Participants, pid)
	if !ok || p == nil {
		return ErrParticipantNotFound{PID: pid}
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
		return ErrParticipantNotFound{PID: pid}
	}
	return nil
}

type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, s *State) error {

	c, _ := ctxConfigH.Lookup(ctx)
	pp.PP(c)

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

type If struct {
	Cond Condition  `json:"cond"`
	Then Definition `json:"then"`
	Else Definition `json:"else,omitempty"`
}

var _ Definition = (*If)(nil)

func (d If) Execute(ctx context.Context, p *State) error {
	if err := d.Validate(ctx); err != nil {
		return err
	}
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
