package wfschedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

type Scheduler struct {
	// Runtime is used to execute a scheduled Process
	Runtime *workflow.Runtime
	// ProcessRepository is where the scheduled process states are kept
	ProcessRepository ProcessRepository
	// ProcessQueue contains the scheduled metadata about which Process requires execution.
	ProcessQueue ProcessQueue
	// ProcessQueueChangeBroadcast contains the information about whether or not
	// ProcessQueue might have a new higher priority Process to be executed
	ProcessQueueChangeBroadcast ProcessQueueChangeBroadcast
}

type ProcessRepository interface {
	crud.Saver[workflow.Process]
	crud.ByIDFinder[workflow.Process, workflow.ProcessID]
	crud.ByIDDeleter[workflow.ProcessID]
}

// ProcessQueue is an ordered queue, where process execution requests are published.
// It is expected to be a FIFO, Durable, and Ordered by ProcessScheduleEntry#StartTime ASC.
type ProcessQueue interface {
	pubsub.Publisher[ProcessScheduleEntry]
	pubsub.Subscriber[ProcessScheduleEntry]
}

type ProcessScheduleEntry struct {
	ProcessID workflow.ProcessID `json:"pid"`
	StartTime time.Time          `json:"start,omitzero"`
}

// ProcessQueueChangeBroadcast is a Volatile, FanOut exchange based broadcasting pubsub channel,
// where worker nodes can subscribe, to get notified if a new workflow Process was scheduled for execution.
// It allows optimisations, such as sleeping on time until the start of the next event arrives.
type ProcessQueueChangeBroadcast interface {
	pubsub.Publisher[ProcessQueueChange]
	pubsub.Subscriber[ProcessQueueChange]
}

type ProcessQueueChange struct{}

// Schedule will Schedule a Process for eventually processing.
func (s *Scheduler) Schedule(ctx context.Context, p *workflow.Process) error {
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

	var g synckit.Group

	changes := make(chan ProcessQueueChange)

	g.Go(ctx, func(ctx context.Context) error {
		return s.runListenToChanges(ctx, changes)
	})

	g.Go(ctx, func(ctx context.Context) error {
		return s.runListenToSignals(ctx, changes)
	})

	return g.Wait()
}

func (s *Scheduler) runListenToSignals(ctx context.Context, changes <-chan ProcessQueueChange) error {
	var rt, ok = s.lookupRuntime(ctx)
	if !ok {
		return workflow.ErrFatal.F("workflow.Runtime is missing for Scheduler (%T#Runtime or from context)", s)
	}
	for msg, err := range s.ProcessQueue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := s.runSignalHandler(rt, msg, changes); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) runSignalHandler(rt workflow.Runtime, msg pubsub.Message[ProcessScheduleEntry], changes <-chan ProcessQueueChange) (rErr error) {
	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
	var (
		ctx = msg.Context()
		sch = msg.Data()
	)

	if !sch.StartTime.IsZero() && clock.Now().Compare(sch.StartTime) == 0 {

	}

	p, found, err := s.ProcessRepository.FindByID(ctx, sch.ProcessID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("missing process: %v", sch.ProcessID.String())
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

func (s *Scheduler) runListenToChanges(ctx context.Context, changes chan<- ProcessQueueChange) error {
	defer close(changes)
	for msg, err := range s.ProcessQueueChangeBroadcast.Subscribe(ctx) {
		if err != nil {
			return err
		}
		select {
		case changes <- msg.Data():
			msg.ACK()
		case <-ctx.Done():
			return msg.NACK()
		}
	}
	return nil
}

func (s *Scheduler) Validate(ctx context.Context) error {
	return validate.Value(ctx, s)
}

// --- EXP --- //

type ProcessSignalQueue interface {
	pubsub.Publisher[workflow.ProcessID]
	pubsub.Subscriber[workflow.ProcessID]
}

func (s *Scheduler) lookupRuntime(ctx context.Context) (Runtime, bool) {
	if s.Runtime != nil {
		return *s.Runtime, true
	}
	return workflow.RuntimeFromContext(ctx)
}

type AwakeByProcessStatus struct {
	Waiter ProcessID          `json:"waiterID"`
	Target ProcessID          `json:"targetID"`
	Status ProcessStatusEvent `json:"statusEvent"`
}

var _ workflow.Event = AwakeByProcessStatus{}

const typeAwakeByProcessStatus = "workflow::event::awake-by-process-status"

var _ = jsonkit.RegisterTypeID[AwakeByProcessStatus](typeAwakeByProcessStatus)

func (AwakeByProcessStatus) Type() workflow.EventType { return typeAwakeByProcessStatus }

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

var _ workflow.Event = AwakeByExternalEvent{}

const typeAwakeByExternalEvent = "workflow::event::awake-by-external-event"

var _ = jsonkit.RegisterTypeID[AwakeByExternalEvent](typeAwakeByExternalEvent)

func (AwakeByExternalEvent) Type() workflow.EventType { return typeAwakeByExternalEvent }
