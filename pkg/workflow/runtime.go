package workflow

import (
	"context"
	"errors"
	"fmt"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/pubsub"
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
		nctx := init(ctx)
		if nctx != nil {
			ctx = nctx
		}
	}
	return ctx
}

type ctxKeyRuntime struct{}

var ctxHRuntime contextkit.ValueHandler[ctxKeyRuntime, Runtime]

func RuntimeFromContext(ctx context.Context) (Runtime, bool) {
	return ctxHRuntime.Lookup(ctx)
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

type Scheduler struct {
	ProcessSignalQueue ProcessSignalQueue
	ProcessRepository  ProcessRepository

	Runtime *Runtime
}

// Schedule will Schedule a Process for eventually processing.
func (s *Scheduler) Schedule(ctx context.Context, p *Process) error {
	if err := s.Validate(ctx); err != nil {
		return err
	}

	if p.ID.IsZero() {
		id, err := MakeProcessID()
		if err != nil {
			return err
		}
		p.ID = id
	}

	// TODO: 	check for current process executed by runtime and if it is the same,
	// 			then instead of save, we should just return a restart or something similar

	// save is idempotent to execute, thus Schedule can be repeated until process signal publish succeeds.
	if err := s.ProcessRepository.Save(ctx, p); err != nil {
		return err
	}

	if err := s.ProcessSignalQueue.Publish(ctx, p.ID); err != nil {
		return err
	}

	return nil
}

var _ taskerlite.Runnable = (*Scheduler)(nil)

func (s *Scheduler) Run(ctx context.Context) error {
	if err := s.Validate(ctx); err != nil {
		return err
	}
	var rt, ok = s.lookupRuntime(ctx)
	if !ok {
		return ErrFatal.F("workflow.Runtime is missing for Scheduler (workflow.Scheduler#Runtime or from context)")
	}
	var handle = func(msg pubsub.Message[ProcessID]) (rErr error) {
		defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
		var (
			ctx = msg.Context()
			pid = msg.Data()
		)

		p, found, err := s.ProcessRepository.FindByID(ctx, pid)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("missing process: %v", pid.String())
		}

		err = rt.Execute(ctx, &p)
		switch {
		case err == nil:
			if err := s.ProcessRepository.Save(ctx, &p); err != nil {
				return err
			}
			return nil
		case errors.As(err, &Suspend{}):
			if err := s.ProcessRepository.Save(ctx, &p); err != nil {
				return err
			}
			return s.ProcessSignalQueue.Publish(ctx, p.ID)
		}
		return err
	}
	for msg, err := range s.ProcessSignalQueue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := handle(msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) lookupRuntime(ctx context.Context) (Runtime, bool) {
	if s.Runtime != nil {
		return *s.Runtime, true
	}
	return RuntimeFromContext(ctx)
}

func (s *Scheduler) Validate(ctx context.Context) error {
	if s == nil {
		return ErrFatal.F("uninitialized workflow.Scheduler")
	}
	if s.ProcessSignalQueue == nil {
		return ErrFatal.F("uninitialized workflow.Scheduler#ProcessSignalQueue")
	}
	if s.ProcessRepository == nil {
		return ErrFatal.F("uninitialized workflow.Scheduler#ProcessRepository")
	}
	return nil
}

type ProcessSignalQueue interface {
	pubsub.Publisher[ProcessID]
	pubsub.Subscriber[ProcessID]
}

type ProcessRepository interface {
	crud.Saver[Process]
	crud.ByIDFinder[Process, ProcessID]
	crud.ByIDDeleter[ProcessID]
}

type AwakeByProcessStatus struct {
	Waiter ProcessID          `json:"waiterID"`
	Target ProcessID          `json:"targetID"`
	Status ProcessStatusEvent `json:"statusEvent"`
}

var _ Event = AwakeByProcessStatus{}

const typeAwakeByProcessStatus = "workflow::event::awake-by-process-status"

var _ = jsonkit.RegisterTypeID[AwakeByProcessStatus](typeAwakeByProcessStatus)

func (AwakeByProcessStatus) Type() EventType { return typeAwakeByProcessStatus }

type ProcessStatusEvent string

const (
	ProcessCompletion   ProcessStatusEvent = "process-completion"
	ProcessCancellation ProcessStatusEvent = "process-cancellation"
	ProcessProgression  ProcessStatusEvent = "process-progression"
)

var _ = enum.Register[ProcessStatusEvent](ProcessCompletion, ProcessCancellation, ProcessProgression)

type AwakeByExternalEvent struct {
	Waiter    ProcessID `json:"waiterID"`
	EventCode string    `json:"eventCode"`
}

var _ Event = AwakeByExternalEvent{}

const typeAwakeByExternalEvent = "workflow::event::awake-by-external-event"

var _ = jsonkit.RegisterTypeID[AwakeByExternalEvent](typeAwakeByExternalEvent)

func (AwakeByExternalEvent) Type() EventType { return typeAwakeByExternalEvent }
