package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/validate"
)

type Definition interface {
	Participant
	minDefinition
	Visitable
}

type minDefinition interface {
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
	p, _ := mapkit.Lookup(c.Participants, pid)
	return p.Execute(ctx, s)
}

func (pid ParticipantID) AcceptVisitor(p DefinitionPath, v Visitor) {
	v.Visit(p.In(string(pid)), &pid)
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

func CID[STR ~string](s STR) *ConditionID {
	var cid = ConditionID(s)
	return &cid
}

// ParticipantID is the process definition ID,
type ConditionID string

var _ Condition = (*ConditionID)(nil)
var _ minDefinition = (*ConditionID)(nil)

func (cid ConditionID) Evaluate(ctx context.Context, s *State) (bool, error) {
	if err := cid.Validate(ctx); err != nil {
		return false, err
	}
	pid := ParticipantID(cid)
	c, _ := ctxConfigH.Lookup(ctx)
	p, _ := mapkit.Lookup(c.Participants, pid)
	cond := p.(minCondition)
	return cond.Evaluate(ctx, s)
}

type ErrParticipantIsNotCondition struct {
	PID ParticipantID
}

func (e ErrParticipantIsNotCondition) Error() string {
	return fmt.Sprintf("[ErrParticipantIsNotCondition] %s", e.PID)
}

func (cid ConditionID) Validate(ctx context.Context) error {
	if len(cid) == 0 {
		return validate.Error{Cause: fmt.Errorf("empty participant ID")}
	}
	pid := ParticipantID(cid)
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok := mapkit.Lookup(c.Participants, pid)
	if !ok || p == nil {
		return ErrParticipantNotFound{PID: pid}
	}
	if cond, ok := p.(minCondition); !ok || cond == nil {
		return ErrParticipantIsNotCondition{PID: pid}
	}
	return nil
}

type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (seq Sequence) Execute(ctx context.Context, s *State) error {
	for _, participant := range seq {
		if err := participant.Execute(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (seq Sequence) AcceptVisitor(root DefinitionPath, v Visitor) {
	var path = root.In("sequence")
	v.Visit(path, &seq)
	for i, participant := range seq {
		if participant == nil {
			continue
		}
		participant.AcceptVisitor(path.In(fmt.Sprintf("[%d]", i)), v)
	}
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

func (d If) AcceptVisitor(root DefinitionPath, v Visitor) {
	var path = root.In("if")
	v.Visit(path, &d)
	if d.Then != nil {
		d.Then.AcceptVisitor(path.In("then"), v)
	}
	if d.Else != nil {
		d.Else.AcceptVisitor(path.In("else"), v)
	}
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

type Concurrence []Definition

var _ Definition = (*Concurrence)(nil)

func (spl *Concurrence) Execute(ctx context.Context, s *State) error {
	if err := spl.Validate(ctx); err != nil {
		return err
	}
	if len(*spl) == 0 {
		return nil
	}
	var tasks []tasker.Task
	for _, pdef := range *spl {
		tasks = append(tasks, spl.toTask(pdef, s))
	}

	return nil
}

func (spl *Concurrence) toTask(pdef Definition, s *State) tasker.Task {
	return func(ctx context.Context) error {
		return pdef.Execute(ctx, s)
	}
}

func (spl *Concurrence) Validate(ctx context.Context) error {
	if spl == nil {
		return fmt.Errorf("nil %T", spl)
	}
	for i, pdef := range *spl {
		if pdef == nil {
			return fmt.Errorf("nil PDEF at %T[%d]", *spl, i)
		}
		if err := pdef.Validate(ctx); err != nil {
			return fmt.Errorf("error with %T[%d]: %w", *spl, i, err)
		}
	}
	return nil
}

func (spl *Concurrence) AcceptVisitor(p DefinitionPath, v Visitor) {
	if spl == nil {
		return
	}
	p = p.In("split")
	v.Visit(p, spl)
	for i, def := range *spl {
		if def == nil {
			continue
		}
		def.AcceptVisitor(p.In(fmt.Sprintf("[%d]", i)), v)
	}
}
