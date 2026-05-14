package wfschedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/pubsub"
)

type Scheduler struct {
	// Runtime is used to execute a scheduled Process
	Runtime *workflow.Runtime
	// ProcessRepository is where the scheduled process states are kept
	ProcessRepository ProcessRepository
	// ProcessSignalQueue is used to schedule process execution tasks.
	ProcessSignalQueue ProcessSignalQueue
	// ProcessScheduleRepository contains meta data about the process schedule
	ProcessScheduleRepository ProcessScheduleRepository
	// ScheduleChangeBroadcast
	ScheduleChangeBroadcast ScheduleChangeBroadcast
}

type ProcessRepository interface {
	crud.Saver[workflow.Process]
	crud.ByIDFinder[workflow.Process, workflow.ProcessID]
	crud.ByIDDeleter[workflow.ProcessID]
}

// --- EXP --- //

type ScheduleChange struct{}

type ScheduleChangeBroadcast interface {
	pubsub.Publisher[ScheduleChange]
	pubsub.Subscriber[ScheduleChange]
}

type ProcessSignalQueue interface {
	pubsub.Publisher[workflow.ProcessID]
	pubsub.Subscriber[workflow.ProcessID]
}

type ProcessScheduleEntry struct {
	ProcessID workflow.ProcessID `json:"pid"`
	StartTime time.Time          `json:"start,omitzero"`
}

type ProcessScheduleRepository interface {
	crud.Saver[ProcessScheduleEntry]
	crud.ByIDFinder[ProcessScheduleEntry, workflow.ProcessID]
	crud.ByIDDeleter[workflow.ProcessID]

	FindNextFrom(ctx context.Context, from time.Time) (ProcessScheduleEntry, bool, error)
}

// Schedule will Schedule a Process for eventually processing.
func (s Scheduler) Schedule(ctx context.Context, p *workflow.Process) error {
	if err := s.Validate(ctx); err != nil {
		return err
	}

	if p.ID.IsZero() {
		id, err := workflow.MakeProcessID()
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

	if err := s.ProcessSignalQueue.Publish(ctx, ProcessSignal{ProcessID: p.ID}); err != nil {
		return err
	}

	return nil
}

var _ taskerlite.Runnable = (*Scheduler)(nil)

func (s Scheduler) Run(ctx context.Context) error {
	if err := s.Validate(ctx); err != nil {
		return err
	}
	var rt, ok = s.lookupRuntime(ctx)
	if !ok {
		return workflow.ErrFatal.F("workflow.Runtime is missing for Scheduler (workflow.Scheduler#Runtime or from context)")
	}
	var handle = func(msg pubsub.Message[ProcessSignal]) (rErr error) {
		defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
		var (
			ctx = msg.Context()
			sig = msg.Data()
		)

		p, found, err := s.ProcessRepository.FindByID(ctx, sig.ProcessID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("missing process: %v", sig.ProcessID.String())
		}

		err = rt.Execute(ctx, &p)

		var suspend workflow.Suspend
		switch {
		case err == nil:
			if err := s.ProcessRepository.Save(ctx, &p); err != nil {
				return err
			}
			return nil
		case errors.As(err, &suspend):
			if err := s.ProcessRepository.Save(ctx, &p); err != nil {
				return err
			}

			if err := s.ProcessScheduleRepository.Save(ctx, &ProcessScheduleEntry{
				ProcessID: p.ID,
			}); err != nil {
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

func (s Scheduler) Validate(ctx context.Context) error {
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

func (s Scheduler) lookupRuntime(ctx context.Context) (Runtime, bool) {
	if s.Runtime != nil {
		return *s.Runtime, true
	}
	return RuntimeFromContext(ctx)
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
