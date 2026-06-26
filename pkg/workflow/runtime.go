package workflow

import (
	"context"
	"iter"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
)

// Runtime is the default runtime to execute process definitions.
// It can be extended or reimplemented if it doesn't fit your workflow related use-cases.
type Runtime struct {
	Participants ParticipantRepository
	Conditions   ConditionRepository
	ContextSetup ContextSetup
	// EventsRepository [optional] is the external resource where Process events
	// are stored. When supplied, a Process that has no event history yet will use
	// it as the backing store, scoping the events to the Process by its ID.
	//
	// When left nil, the runtime keeps the event history in memory.
	EventsRepository EventsRepository
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

type EventsRepository interface {
	comproto.OnePhaseCommitProtocol
	crud.Creator[Event]
	FindByProcessID(ctx context.Context, pid ProcessID) iter.Seq2[Event, error]
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
	if err := rt.attachEvents(p); err != nil {
		return err
	}
	var err error
	if p.Definition != nil {
		err = p.Definition.Execute(rt.Context(ctx), p)
	}
	if err == nil {
		if !IsCompleted(p) {
			var event Event = &EventCompleted{}
			_ = p.events().Create(ctx, &event)
		}
	}
	return err
}

// attachEvents wires the Process event history to the runtime's EventsRepository
// when the Process doesn't have an event history yet. The Process is assigned a
// fresh ID when needed, so its events can be associated with it in the
// repository.
func (rt Runtime) attachEvents(p *Process) error {
	if p.Events != nil || rt.EventsRepository == nil {
		return nil
	}
	if p.ID.IsZero() {
		id, err := MakeProcessID()
		if err != nil {
			return err
		}
		p.ID = id
	}
	p.Events = processEvents{repo: rt.EventsRepository, pid: p.ID}
	return nil
}

type ctxKeyRuntime struct{}

var ctxHRuntime contextkit.ValueHandler[ctxKeyRuntime, Runtime]

func RuntimeFromContext(ctx context.Context) (Runtime, bool) {
	return ctxHRuntime.Lookup(ctx)
}
