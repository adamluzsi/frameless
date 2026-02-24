package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/validate"
)

type Condition interface {
	minCondition
	minDefinition
}

func ContextWithConditions(ctx context.Context, cr ConditionRepository) context.Context {
	if cr == nil {
		return ctx
	}
	c, _ := ctxConfigH.Lookup(ctx)
	c.Conditions = cr
	return ctxConfigH.ContextWith(ctx, c)
}

func CID[STR ~string](s STR) *ConditionID {
	var cid = ConditionID(s)
	return &cid
}

// ParticipantID is the process definition ID,
type ConditionID string

var _ Condition = (*ConditionID)(nil)
var _ minDefinition = (*ConditionID)(nil)

type ErrConditionNotFound struct{ CID ConditionID }

func (e ErrConditionNotFound) Error() string {
	return fmt.Sprintf("[ErrConditionNotFound] %s", e.CID)
}

func (cid ConditionID) Evaluate(ctx context.Context, s *State) (bool, error) {
	if len(cid) == 0 {
		return false, ErrConditionNotFound{}
	}
	pid := ParticipantID(cid)
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrConditionNotFound{CID: cid}
	}
	cond, ok := p.(minCondition)
	if !ok {
		return false, ErrConditionNotFound{CID: cid}
	}
	return cond.Evaluate(ctx, s)
}

type ErrParticipantIsNotCondition struct{ PID ParticipantID }

func (e ErrParticipantIsNotCondition) Error() string {
	return fmt.Sprintf("[ErrParticipantIsNotCondition] %s", e.PID)
}

func (cid ConditionID) Validate(ctx context.Context) error {
	if len(cid) == 0 {
		return validate.Error{Cause: fmt.Errorf("empty participant ID")}
	}
	pid := ParticipantID(cid)
	c, _ := ctxConfigH.Lookup(ctx)
	p, ok, err := lookupParticipant(c.Participants, ctx, pid)
	if err != nil {
		return err
	}
	if !ok || p == nil {
		return ErrConditionNotFound{CID: cid}
	}
	if cond, ok := p.(minCondition); !ok || cond == nil {
		return ErrConditionNotFound{CID: cid}
	}
	return nil
}
