package workflow

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

// ProcessExecutionQueue is an ordered queue, where process execution requests are published.
// It is expected to be a Durable and Ordered queue where ordering is sorted by ProcessScheduleEntry#StartTime ASC.
type ProcessExecutionQueue interface {
	pubsub.Publisher[ProcessExecution]
	pubsub.Subscriber[ProcessExecution]
}

type ProcessExecution struct {
	// ProcessID specifies which ProcessID should be scheduled for execution
	ProcessID ProcessID
	// StartTime defines when it is expected to schedule the process for the next time
	StartTime time.Time
	// CreatedAt is the time at which the Schedule was created
	CreatedAt time.Time
	// FailureCount is the number of execution attempts which failed previously
	FailureCount int
}

// ProcessChangeBroadcast is a Volatile, FanOut exchange based broadcasting pubsub channel,
// where worker nodes can subscribe, to get notified if a new workflow Process was scheduled for execution.
// It allows optimisations, such as sleeping on time until the start of the next event arrives.
type ProcessChangeBroadcast interface {
	pubsub.Publisher[ProcessChangeEvent]
	pubsub.Subscriber[ProcessChangeEvent]
}

type ProcessChangeEvent interface {
	GetProcessID() ProcessID
	ChangeType() ProcessChangeType
}

type ProcessChangeType string

type ProcessStart struct{ ProcessID ProcessID }

func (ch ProcessStart) ChangeType() ProcessChangeType { return "start" }
func (ch ProcessStart) GetProcessID() ProcessID       { return ch.ProcessID }

type ProcessStop struct{ ProcessID ProcessID }

func (ch ProcessStop) ChangeType() ProcessChangeType { return "stop" }
func (ch ProcessStop) GetProcessID() ProcessID       { return ch.ProcessID }

type ProcessSleep struct{ ProcessID ProcessID }

func (ch ProcessSleep) ChangeType() ProcessChangeType { return "sleep" }
func (ch ProcessSleep) GetProcessID() ProcessID       { return ch.ProcessID }

// Schedule will Schedule a Process for eventually processing.
//
// The caller owns process identity: Schedule refuses a zero id so that
// retrying a failed Schedule always yields the SAME ProcessID, making the
// scheduling contract safe under any kind of caller-side failure
// (network blip, timeout, panic recovery, etc.).
func (rt Runtime) Schedule(ctx context.Context, pid ProcessID, opts ...func(*ProcessExecution)) error {
	return rt.withRetry(ctx, func() error {
		return rt.schedule(ctx, pid, opts...)
	})
}

func (rt Runtime) schedule(ctx context.Context, pid ProcessID, opts ...func(*ProcessExecution)) error {
	if err := rt.Validate(ctx); err != nil {
		return err
	}

	if pid.IsZero() {
		return ErrZeroProcessID.F("Scheduling requires a non-zero processID")
	}

	var schedule = ProcessExecution{}
	for _, opt := range opts {
		opt(&schedule)
	}
	schedule.ProcessID = pid
	schedule.CreatedAt = clock.Now()
	if schedule.StartTime.IsZero() {
		schedule.StartTime = clock.Now()
	}

	if err := rt.ProcessExecutionQueue.Publish(ctx, schedule); err != nil {
		return err
	}

	if err := rt.ProcessChangeBroadcast.Publish(ctx, ProcessStart{ProcessID: pid}); err != nil {
		return err
	}

	return nil
}

var _ taskerlite.Runnable = Runtime{}

// Run is the background task to be execute executed on the worker nodes.
func (rt Runtime) Run(ctx context.Context) error {
	if err := rt.Validate(ctx); err != nil {
		return err
	}

	var g = synckit.Group{
		ErrorOnGoexit: true,
		Isolation:     true,
	}

	var changeBroadcast synckit.Broadcast[ProcessChangeEvent]

	g.Go(ctx, func(ctx context.Context) (err error) {
		return rt.withRetry(ctx, func() error {
			return rt.runListenToRemoteChanges(ctx, &changeBroadcast)
		})
	})

	for range rt.getNumQueueSubscriber() {
		g.Go(ctx, func(ctx context.Context) error {
			return rt.withRetry(ctx, func() error {
				changes, job := rt.listenToLocalChangeBroadcast(ctx, &changeBroadcast)
				defer job.Cancel()
				return rt.runListenToScheduling(ctx, changes)
			})
		})
	}

	return g.Wait()
}

func (rt Runtime) listenToLocalChangeBroadcast(ctx context.Context, broadcast *synckit.Broadcast[ProcessChangeEvent]) (<-chan ProcessChangeEvent, synckit.Job) {
	var changes = make(chan ProcessChangeEvent)
	var job = synckit.Go(ctx, func(ctx context.Context) error {
		defer close(changes)
		const timeout = time.Second
		var outdate = clock.NewTicker(timeout)
		defer outdate.Stop()
		for change, err := range broadcast.Subscribe(ctx) {
			if err != nil {
				return err
			}
			// we don't care about this value pretty much after a second passed already,
			// so no meaning in any in the future NACK
			_ = change.ACK()
			outdate.Reset(timeout)
			select {
			case <-ctx.Done():
				return nil
			case changes <- change.Data():
				change.ACK()
			case <-outdate.C:
				change.ACK()
			}
		}
		return nil
	})
	return changes, job
}

func (rt Runtime) getNumQueueSubscriber() int {
	var NumQueueSubscriber = rt.NumQueueSubscriber
	if 0 <= NumQueueSubscriber {
		var defaultNumQueueSubscriber = runtime.NumCPU() * 4
		NumQueueSubscriber = defaultNumQueueSubscriber
	}
	return NumQueueSubscriber
}

func (rt Runtime) withRetry(ctx context.Context, do func() error) (err error) {
	for range resilience.Retries(ctx, rt.RetryStrategy) {
		err = do()
		if err == nil {
			return nil
		}
		switch {
		case ErrIsFatal(err):
			return err
		case errors.Is(err, ErrFatal):
			return err
		case errors.Is(err, ErrAlreadyRunningProcess):
			return err
		case errors.Is(err, ErrNoProcessDefinition):
			return err
		case errors.Is(err, ErrZeroProcessID):
			return err
		}
	}
	return
}

func (rt Runtime) runListenToScheduling(ctx context.Context, changes <-chan ProcessChangeEvent) error {
	if rt.ProcessExecutionQueue == nil {
		return fmt.Errorf("Error, missing %T#ProcessQueue", rt)
	}
	for msg, err := range rt.ProcessExecutionQueue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := rt.runSignalHandler(rt, msg, changes); err != nil {
			return err
		}
	}
	return nil
}

func (rt Runtime) guardAgainstEarlyExecution(ctx context.Context, sch ProcessExecution, changes <-chan ProcessChangeEvent) (ok bool) {
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

func (s Runtime) runSignalHandler(rt Runtime, msg pubsub.Message[ProcessExecution], changes <-chan ProcessChangeEvent) (rErr error) {
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

	var err = rt.Execute(ctx, sch.ProcessID)

	var suspend Suspend
	switch {
	case err == nil:
		return nil

	case errors.Is(err, ErrAlreadyRunningProcess):
		// a simple requeue should be okay,
		// as whoever picks it up when the process no longer busy,
		// can either verify process completion
		// or continue with processing.
		return nil

	case errors.Is(err, ErrNoProcessDefinition):
		if time.Minute < clock.Now().Sub(sch.StartTime) {
			// should we return error here ?
			return err
		}
		return s.ProcessExecutionQueue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			CreatedAt:    sch.CreatedAt,
			FailureCount: sch.FailureCount,
		})

	// Suspend requires some revision
	case errors.As(err, &suspend):
		return s.ProcessExecutionQueue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			FailureCount: sch.FailureCount,
			CreatedAt:    sch.CreatedAt,
		})

	default:
		return s.ProcessExecutionQueue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			FailureCount: sch.FailureCount + 1,
			CreatedAt:    sch.CreatedAt,
		})
	}
}

func (rt Runtime) backoffStartTime() time.Time {
	var waitTime = rt.WaitTime
	if waitTime <= 0 {
		const defaultWaitTime = 30 * time.Second
		waitTime = defaultWaitTime
	}
	return clock.Now().Add(waitTime)
}

type errOutdatedChange struct{}

func (errOutdatedChange) Error() string {
	return "[error] outdated change"
}

func (rt Runtime) runListenToRemoteChanges(ctx context.Context, changes *synckit.Broadcast[ProcessChangeEvent]) error {
	defer changes.Close()
	if rt.ProcessChangeBroadcast == nil {
		return ErrFatal.F("Error, missing %T#ProcessChangeBroadcast", rt)
	}
	var handle = func(msg pubsub.Message[ProcessChangeEvent]) (rerr error) {
		defer comproto.FinishTx(&rerr, msg.ACK, msg.NACK)
		return changes.Publish(ctx, msg.Data())
	}
	for msg, err := range rt.ProcessChangeBroadcast.Subscribe(ctx) {
		if err != nil {
			return err
		}
		err := handle(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rt Runtime) Validate(ctx context.Context) error {
	if rt.Events == nil {
		return fmt.Errorf("missing %T#EventsRepository", rt)
	}
	if rt.ProcessExecutionQueue == nil {
		return fmt.Errorf("missing %T#ProcessQueue", rt)
	}
	if rt.ProcessChangeBroadcast == nil {
		return fmt.Errorf("missing %T#ProcessQueueChangeBroadcast", rt)
	}
	return validate.Value(ctx, rt)
}
