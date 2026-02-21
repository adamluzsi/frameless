package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/validate"
)

type Definition interface {
	JSONSerialisable
	validate.Validatable

	// definition communicates clearly that Definition is not something that can be implemented outside of the workflow engine.
	// components are provided to build a wide range of possible definitions, through composition,
	// but creating one outside of the framework would require the runtime to know how to traverse that definition,
	// and how to create checkpoints to it.
	definition()

	// Execute(ctx Context, s *State) error
}

type Context interface {
	context.Context
	workflowContext()
}

type wfContext struct {
	context.Context
	path  []string
	cache synckit.Map[string, any]
}

func (*wfContext) workflowContext() {}

var _ minDefinition = (Definition)(nil)

type minDefinition interface {
	JSONSerialisable
	validate.Validatable
}

type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (Sequence) definition() {}

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

type Concurrence []Definition

var _ Definition = (*Concurrence)(nil)

func (d *Concurrence) Execute(ctx context.Context, s *State) error {
	if err := d.Validate(ctx); err != nil {
		return err
	}
	if len(*d) == 0 {
		return nil
	}
	var tasks []tasker.Task
	for _, pdef := range *d {
		tasks = append(tasks, d.toTask(pdef, s))
	}

	return nil
}

func (d *Concurrence) toTask(pdef Definition, s *State) tasker.Task {
	return func(ctx context.Context) error {
		return pdef.Execute(ctx, s)
	}
}

func (d *Concurrence) Validate(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("nil %T", d)
	}
	for i, pdef := range *d {
		if pdef == nil {
			return fmt.Errorf("nil PDEF at %T[%d]", *d, i)
		}
		if err := pdef.Validate(ctx); err != nil {
			return fmt.Errorf("error with %T[%d]: %w", *d, i, err)
		}
	}
	return nil
}

type ExecuteParticipant struct {
	ID        ParticipantID `json:"id"`
	Arguments []VariableKey `json:"args,omitempty"`
	Out       []VariableKey `json:"res,omitempty"`
}

func (d *ExecuteParticipant) Execute(ctx context.Context, s *State) error {

	if err := pid.Validate(ctx); err != nil {
		return err
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, found, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !found {
		return ErrParticipantNotFound{PID: pid}
	}
	return p.Execute(ctx, s)

}

func (d *ExecuteParticipant) Validate(ctx context.Context) error {
	pid := d.ID
	if len(pid) == 0 {
		return validate.Error{Cause: fmt.Errorf("empty participant ID")}
	}
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !ok || p == nil {
		return ErrParticipantNotFound{PID: pid}
	}
	return nil
}
