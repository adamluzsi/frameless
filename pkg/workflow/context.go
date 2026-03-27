package workflow

import (
	"context"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ContextWithParticipants(ctx context.Context, prs ...ParticipantRepository) context.Context {
	if len(prs) == 0 {
		return ctx
	}
	if ctxprs, ok := ctxParticipantsH.Lookup(ctx); ok {
		prs = slicekit.Merge(prs, ctxprs) // new with higher priority
		return ctx
	}
	return ctxParticipantsH.ContextWith(ctx, ctxPRS(prs))
}

type ctxPRS []ParticipantRepository

var _ ParticipantRepository = (*ctxPRS)(nil)

func (prs ctxPRS) FindByID(ctx context.Context, id ParticipantID) (v Participant, found bool, err error) {
	for _, pr := range prs {
		v, found, err = pr.FindByID(ctx, id)
		if err != nil || found {
			return v, found, err
		}
	}
	return Participant{}, false, nil
}

var ctxParticipantsH contextkit.ValueHandler[ctxKeyPRS, ctxPRS]

type ctxKeyPRS struct{}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ContextWithConditions(ctx context.Context, crs ...ConditionRepository) context.Context {
	if len(crs) == 0 {
		return ctx
	}
	if ctxcrs, ok := ctxConditionsH.Lookup(ctx); ok {
		crs = slicekit.Merge(crs, ctxcrs) // new with higher priority
		return ctx
	}
	return ctxConditionsH.ContextWith(ctx, ctxCRS(crs))
}

type ctxCRS []ConditionRepository

var _ ConditionRepository = (*ctxCRS)(nil)

func (prs ctxCRS) FindByID(ctx context.Context, id ConditionID) (v Condition, found bool, err error) {
	for _, pr := range prs {
		v, found, err = pr.FindByID(ctx, id)
		if err != nil || found {
			return v, found, err
		}
	}
	return nil, false, nil
}

var ctxConditionsH contextkit.ValueHandler[ctxKeyCRS, ctxCRS]

type ctxKeyCRS struct{}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
