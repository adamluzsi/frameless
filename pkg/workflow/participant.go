package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/validate"
)

// Participant is a logical unit implemented at workflow engine-level.
//
// If ParticipantRepository is supplied to the workflow runtime context,
// then registered particpants can be used from within workflow definitions.
type Participant interface {
	Execute(ctx context.Context, s *State) error
}

func ContextWithParticipants(ctx context.Context, pr ParticipantRepository) context.Context {
	if pr == nil {
		return ctx
	}
	c, _ := ctxConfigH.Lookup(ctx)
	c.Participants = pr
	return ctxConfigH.ContextWith(ctx, c)
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
	p, found, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !found {
		return ErrParticipantNotFound{PID: pid}
	}
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
	p, ok, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !ok || p == nil {
		return ErrParticipantNotFound{PID: pid}
	}
	return nil
}
