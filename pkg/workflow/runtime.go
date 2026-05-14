package workflow

import (
	"context"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/port/crud"
)

// Runtime is the default runtime to execute process definitions.
// It can be extended or reimplemented if it doesn't fit your workflow related use-cases.
type Runtime struct {
	Participants ParticipantRepository
	Conditions   ConditionRepository
	ContextSetup ContextSetup
}

type ContextSetup []func(context.Context) context.Context

func (cs ContextSetup) SetUp(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, init := range cs {
		if init == nil {
			continue
		}
		var next = init(ctx)
		if next != nil {
			ctx = next
		}
	}
	return ctx
}

type ParticipantRepository interface {
	crud.ByIDFinder[Participant, ParticipantID]
}

type ConditionRepository interface {
	crud.ByIDFinder[Condition, ConditionID]
}

// Context returns a fresh execution runtime context intended to be used for calling Definition#Execute.
func (rt Runtime) Context(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = ctxHRuntime.ContextWith(ctx, rt)
	if rt.Participants != nil {
		ctx = ContextWithParticipants(ctx, rt.Participants)
	}
	if rt.Conditions != nil {
		ctx = ContextWithConditions(ctx, rt.Conditions)
	}
	ctx = WithExecutionIndex(ctx)
	ctx = rt.ContextSetup.SetUp(ctx)
	return ctx
}

func (rt Runtime) Execute(ctx context.Context, p *Process) error {
	var err error
	if p.Definition != nil {
		err = p.Definition.Execute(rt.Context(ctx), p)
	}
	if err == nil {
		if !IsCompleted(p) {
			p.Events = append(p.Events, EventCompleted{})
		}
	}
	return err
}

type ctxKeyRuntime struct{}

var ctxHRuntime contextkit.ValueHandler[ctxKeyRuntime, Runtime]

func RuntimeFromContext(ctx context.Context) (Runtime, bool) {
	return ctxHRuntime.Lookup(ctx)
}
