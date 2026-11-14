package workflow

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
)

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ContextWithParticipants(ctx context.Context, prs ...ParticipantRepository) context.Context {
	if len(prs) == 0 {
		return ctx
	}
	if ctxprs, ok := ctxParticipantsH.Lookup(ctx); ok {
		prs = slicekit.Merge(prs, ctxprs) // new with higher priority
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

var ctxTimeToLiveH contextkit.ValueHandler[ctxKeyTTL, *ctxTTL]

type ctxKeyTTL struct{}

type ctxTTL struct {
	// Active counts fresh nested activities. Defer budget suspension until the
	// outermost fresh activity finishes so its nested progress commits together.
	Active int
	// StartedAt is when the current definition execution started at.
	StartedAt time.Time
	// TTL is the TimeToLive duration
	TTL time.Duration
}

func (ct *ctxTTL) shouldSuspend() bool {
	if ct == nil {
		return false
	}
	if ct.TTL <= 0 {
		return false
	}
	if ct.Active != 0 {
		return false
	}
	return !clock.Now().Before(ct.StartedAt.Add(ct.TTL))
}

// withTTL isolates the budget from any enclosing execution, even when disabled.
func withTTL(rt Runtime, ctx context.Context) context.Context {
	return ctxTimeToLiveH.ContextWith(ctx, &ctxTTL{
		StartedAt: clock.Now(),
		TTL:       rt.TTL,
	})
}

func (ct *ctxTTL) Finish(rErr *error, ctx context.Context) {
	if ct == nil {
		return
	}
	if ct == nil {
		return
	}
	ct.Active--
	if rErr == nil || *rErr != nil {
		return
	}
	if ct.shouldSuspend() {
		*rErr = Suspend{}
	}
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
