package workflow

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
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

type Scheduler struct {
	// Runtime is used to execute a scheduled Process
	Runtime *Runtime
	// ProcessRepository is where the scheduled process states are kept
	ProcessRepository ProcessRepository
	// ProcessQueue contains the scheduled metadata about which Process requires execution.
	ProcessQueue ProcessQueue
	// ProcessQueueChangeBroadcast contains the information about whether or not
	// ProcessQueue might have a new higher priority Process to be executed
	ProcessQueueChangeBroadcast ProcessQueueChangeBroadcast
}

type ProcessRepository interface {
	crud.Creator[Process]
	crud.ByIDFinder[Process, ProcessID]
	crud.Updater[Process]
	crud.ByIDDeleter[ProcessID]
}

// ProcessQueue is an ordered queue, where process execution requests are published.
// It is expected to be a Durable and Ordered queue where ordering is sorted by ProcessScheduleEntry#StartTime ASC.
type ProcessQueue interface {
	pubsub.Publisher[ProcessScheduleEntry]
	pubsub.Subscriber[ProcessScheduleEntry]
}

type ProcessScheduleEntry struct {
	ProcessID ProcessID `json:"pid"`
	// StartTime defines when it is expected to schedule the process for the next time.
	StartTime time.Time `json:"start,omitzero"`
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
func (s *Scheduler) Schedule(ctx context.Context, p *Process, startTime time.Time) error {
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

	// Persisting the process is idempotent so Schedule can be safely repeated
	// until the queue publish succeeds. Only the initial state is persisted when
	// the process is not yet stored; if it already exists we must not overwrite
	// it, otherwise any progress the runtime has already made (e.g. recorded
	// participant executions used for idempotency) would be wiped out.
	if _, found, err := s.ProcessRepository.FindByID(ctx, p.ID); err != nil {
		return err
	} else if !found {
		if err := s.ProcessRepository.Create(ctx, p); err != nil {
			return err
		}
	}

	if err := s.ProcessQueue.Publish(ctx, ProcessScheduleEntry{
		ProcessID: p.ID,
		StartTime: startTime,
	}); err != nil {
		return err
	}

	if err := s.ProcessQueueChangeBroadcast.Publish(ctx, ProcessQueueChange{}); err != nil {
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
	g.ErrorOnGoexit = true

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
		return ErrFatal.F("Runtime is missing for Scheduler (%T#Runtime or from context)", s)
	}
	if s.ProcessQueue == nil {
		return fmt.Errorf("Error, missing %T#ProcessQueue", s)
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

func (s *Scheduler) guardAgainstEarlyExecution(ctx context.Context, sch ProcessScheduleEntry, changes <-chan ProcessQueueChange) (ok bool) {
	if sch.StartTime.IsZero() {
		return true
	}

	var now = clock.Now()
	if sch.StartTime.Before(now) {
		return true
	}

	var ticker = clock.NewTicker(sch.StartTime.Sub(clock.Now()))
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		return false

	case <-changes:
		return false

	case <-ticker.C:
		return true
	}
}

func (s *Scheduler) runSignalHandler(rt Runtime, msg pubsub.Message[ProcessScheduleEntry], changes <-chan ProcessQueueChange) (rErr error) {
	var (
		ctx = msg.Context()
		sch = msg.Data()
	)

	if !s.guardAgainstEarlyExecution(ctx, sch, changes) {
		// Re-queue the entry for later; it is not yet time to execute it.
		// NOTE: FinishTx must be deferred only after this point, otherwise a
		// nil rErr from a successful NACK would make FinishTx ACK (delete) the
		// entry, dropping the scheduled process before its start time arrives.
		return msg.NACK()
	}

	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)

	p, found, err := s.ProcessRepository.FindByID(ctx, sch.ProcessID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("missing process: %v", sch.ProcessID.String())
	}

	err = rt.Execute(ctx, &p)

	var suspend Suspend
	switch {
	case err == nil:
		if err := s.ProcessRepository.Update(ctx, &p); err != nil {
			return err
		}
		return nil
	case errors.As(err, &suspend):
		if err := s.ProcessRepository.Update(ctx, &p); err != nil {
			return err
		}
		return s.ProcessQueue.Publish(ctx, ProcessScheduleEntry{
			ProcessID: p.ID,
			StartTime: clock.Now().Add(time.Second),
		})
	}
	return err
}

func (s *Scheduler) runListenToChanges(ctx context.Context, changes chan<- ProcessQueueChange) error {
	defer close(changes)
	if s.ProcessQueueChangeBroadcast == nil {
		return fmt.Errorf("Error, missing %T#ProcessQueueChangeBroadcast", s)
	}
	for msg, err := range s.ProcessQueueChangeBroadcast.Subscribe(ctx) {
		if err != nil {
			return err
		}
		select {
		case changes <- msg.Data():
			msg.ACK()
		case <-ctx.Done():
			return msg.NACK()
		case <-time.After(time.Second):
			// if no-one is sleeping on a scheduled task,
			// then workers are already busy picking up changes
			// so no need to wait with this relatively old change.
		}
	}
	return nil
}

func (s *Scheduler) Validate(ctx context.Context) error {
	if s.ProcessQueue == nil {
		return fmt.Errorf("missing %T#ProcessQueue", s)
	}
	if s.ProcessQueueChangeBroadcast == nil {
		return fmt.Errorf("missing %T#ProcessQueueChangeBroadcast", s)
	}
	return validate.Value(ctx, s)
}

// --- EXP --- //

func (s *Scheduler) lookupRuntime(ctx context.Context) (Runtime, bool) {
	if s.Runtime != nil {
		return *s.Runtime, true
	}
	return RuntimeFromContext(ctx)
}

type AwakeByProcessStatus struct {
	ProcessID ProcessID          `json:"process_id"`
	Timestamp time.Time          `json:"timestamp"`
	Waiter    ProcessID          `json:"waiter_id"`
	Target    ProcessID          `json:"targetID"`
	Status    ProcessStatusEvent `json:"statusEvent"`
}

var _ Event = (*AwakeByProcessStatus)(nil)

const typeAwakeByProcessStatus = "workflow::event::awake-by-process-status"

var _ = jsonkit.RegisterTypeID[AwakeByProcessStatus](typeAwakeByProcessStatus)

func (AwakeByProcessStatus) Type() EventType { return typeAwakeByProcessStatus }

func (e AwakeByProcessStatus) GetProcessID() ProcessID { return e.ProcessID }
func (e AwakeByProcessStatus) GetTimestamp() time.Time { return e.Timestamp }

type ProcessStatusEvent string

const (
	ProcessCompletion   ProcessStatusEvent = "process-completion"
	ProcessCancellation ProcessStatusEvent = "process-cancellation"
	ProcessProgression  ProcessStatusEvent = "process-progression"
)

var _ = enum.Register[ProcessStatusEvent](ProcessCompletion, ProcessCancellation, ProcessProgression)

type AwakeByExternalEvent struct {
	ProcessID ProcessID `json:"process_id"`
	Timestamp time.Time `json:"timestamp"`
	Waiter    ProcessID `json:"waiterID"`
	EventCode string    `json:"eventCode"`
}

var _ Event = (*AwakeByExternalEvent)(nil)

const typeAwakeByExternalEvent = "workflow::event::awake-by-external-event"

var _ = jsonkit.RegisterTypeID[AwakeByExternalEvent](typeAwakeByExternalEvent)

func (AwakeByExternalEvent) Type() EventType           { return typeAwakeByExternalEvent }
func (e AwakeByExternalEvent) GetProcessID() ProcessID { return e.ProcessID }
func (e AwakeByExternalEvent) GetTimestamp() time.Time { return e.Timestamp }
