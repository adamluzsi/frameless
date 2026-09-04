package workflow

import (
	"context"
	"fmt"
)

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Sequence executes its child definitions in order.
type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (Sequence) Error() string { return "workflow::sequence" }

func (seq Sequence) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "sequence")
	for i, participant := range seq {
		// Each iteration derives its own context from the
		// sequence-scoped ctx so iterations don't leak path
		// segments into one another.
		if err := participant.Execute(WithName(ctx, fmt.Sprintf("[%d]", i)), pid); err != nil {
			return err
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If executes Then when Cond is true, Else otherwise.
type If struct {
	Cond Condition
	Then Definition
	Else Definition
}

var _ Definition = (*If)(nil)

func (If) Error() string { return "workflow::if" }

func (d If) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "if")
	if d.Cond == nil {
		return ErrFatal.F("missing %s condition", d.Error())
	}
	var ok, err = d.Cond.Evaluate(ctx, pid)
	if err != nil {
		return err
	}
	if ok {
		if d.Then != nil {
			return d.Then.Execute(WithName(ctx, "then"), pid)
		}
	} else {
		if d.Else != nil {
			return d.Else.Execute(WithName(ctx, "else"), pid)
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Sleep pauses execution until its While condition is false or its Until
// condition is true. It signals suspension by returning the Sleep error.
type Sleep struct {
	While Condition
	Until Condition
}

var _ Definition = Sleep{}

func (d Sleep) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "sleep")
	var Continue bool
	switch {
	case d.While != nil:
		ok, err := d.While.Evaluate(ctx, pid)
		if err != nil {
			return err
		}
		Continue = !ok
	case d.Until != nil:
		ok, err := d.Until.Evaluate(ctx, pid)
		if err != nil {
			return err
		}
		Continue = ok
	}
	if Continue {
		return nil // OK
	}
	return Suspend{}
}

func (d Sleep) Error() string { return "workflow::sleep" }

type Suspend struct{}

var _ RuntimeSignal = Suspend{}

func (sig Suspend) Error() string { return "workflow::suspend" }

func (sig Suspend) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	return sig
}
