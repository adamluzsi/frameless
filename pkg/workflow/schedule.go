package workflow

import (
	"context"
	"errors"
	"runtime"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
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

type ProcessSchedule struct{ ProcessID ProcessID }

func (ch ProcessSchedule) ChangeType() ProcessChangeType { return "schedule" }
func (ch ProcessSchedule) GetProcessID() ProcessID       { return ch.ProcessID }

type ProcessCancel struct{ ProcessID ProcessID }

func (ch ProcessCancel) ChangeType() ProcessChangeType { return "cancel" }
func (ch ProcessCancel) GetProcessID() ProcessID       { return ch.ProcessID }

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
	schedule.CreatedAt = timeNow()
	if schedule.StartTime.IsZero() {
		schedule.StartTime = timeNow()
	}

	if err := rt.Queue.Publish(ctx, schedule); err != nil {
		return err
	}

	if err := rt.Changes.Publish(ctx, ProcessSchedule{ProcessID: pid}); err != nil {
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
	if rt.NumQueueSubscriber <= 0 {
		var defaultNumQueueSubscriber = runtime.NumCPU()
		return defaultNumQueueSubscriber
	}
	return rt.NumQueueSubscriber
}

func (rt Runtime) withRetry(ctx context.Context, do func() error) (err error) {
	for range resilience.Retries(ctx, rt.RetryStrategy) {
		err = do()
		if err == nil {
			return nil
		}
		switch {
		case isRuntimeSignal(err):
			// A RuntimeSignal is not a fault, so there is nothing here to
			// retry: it is the runtime's own control flow, raised on purpose
			// by a step that ran fine.
			//
			// Suspend shows why retrying it is actively wrong. It means "come
			// back later", and coming back later is the scheduler's job —
			// Runtime#runSignalHandler re-queues a suspended process without
			// incrementing its FailureCount, exactly because a suspension is
			// not a failure. Retrying it here contradicts that, and it is not
			// free either: raising a signal is deliberately never recorded in
			// the event history, so each attempt walks back to the waiting
			// step and asks it again straight away.
			return err
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
	if rt.Queue == nil {
		return ErrFatal.F("Error, missing %T#ProcessQueue", rt)
	}
	for msg, err := range rt.Queue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := rt.runSignalHandler(rt, msg, changes); err != nil {
			return err
		}
	}
	return nil
}

func (rt Runtime) guardAgainstEarlyExecution(ctx context.Context, msg pubsub.Message[ProcessExecution], changes <-chan ProcessChangeEvent) (ok bool) {
	var sch = msg.Data()
	if sch.StartTime.IsZero() {
		return true
	}

	var now = timeNow()
	if !sch.StartTime.After(now) {
		// StartTime has already passed (or is exactly now), so the guard is
		// unnecessary. Clock#NewTicker panics on a non-positive duration,
		// matching time#NewTicker, so the call below would crash the worker
		// goroutine at the boundary moment instead of letting it run.
		return true
	}

	var remainder = sch.StartTime.Sub(now)
	if remainder <= 0 {
		return true
	}

	var ticker = clock.NewTicker(remainder)
	defer ticker.Stop()

waiting:
	select {
	case <-ctx.Done():
		return false

	case ch, ok := <-changes:
		if !ok {
			return false
		}
		switch ch.ChangeType() {
		case (ProcessSchedule{}).ChangeType():
			goto waiting
		case (ProcessCancel{}).ChangeType():
			_ = msg.ACK() // process execution no longer needed
			return false
		default:
			return false
		}

	case <-ticker.C:
		return true
	}
}

func (s Runtime) runSignalHandler(rt Runtime, msg pubsub.Message[ProcessExecution], changes <-chan ProcessChangeEvent) (rErr error) {
	var (
		ctx = msg.Context()
		sch = msg.Data()
	)

	if !s.guardAgainstEarlyExecution(ctx, msg, changes) {
		// Re-queue the entry for later; it is not yet time to execute it.
		// NOTE: FinishTx must be deferred only after this point, otherwise a
		// nil rErr from a successful NACK would make FinishTx ACK (delete) the
		// entry, dropping the scheduled process before its start time arrives.
		return msg.NACK()
	}

	const ErrProcessCancel errorkitlite.Error = "ErrProcessCancel"
	executeCTX, cancel := context.WithCancelCause(ctx)

	var job = synckit.Go(ctx, func(ctx context.Context) error {
	waiting:
		select {
		case <-ctx.Done():
			return nil
		case ch, ok := <-changes:
			if !ok {
				return nil
			}
			switch ch.ChangeType() {
			case (ProcessSchedule{}).ChangeType():
				goto waiting
			case (ProcessCancel{}).ChangeType():
				if sch.ProcessID.Equal(ch.GetProcessID()) {
					_ = msg.ACK()
					cancel(ErrProcessCancel)
					return nil
				}
				goto waiting
			}
		}
		return nil
	})
	defer job.Cancel()

	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)

	var err = rt.Execute(executeCTX, sch.ProcessID)
	if errors.Is(err, ErrProcessCancel) {
		// ProcessCancel is not a real problematic error,
		// but a control flow, for process termination
		return nil
	}

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
		if rt.isBindGracePeriodExpired(sch) {
			// The Definition is not coming anymore, so the entry can never execute.
			//
			// It must still be disposed of gently. Returning the error would be
			// fatal (see ErrIsFatal) and would tear down this queue subscriber,
			// while NACK-ing would hand the very same unexecutable entry to the
			// next subscriber, until every worker of the pool is gone. The entry
			// is worthless, but the workers serving the rest of the queue are not,
			// so the entry is acknowledged and dropped.
			logger.Warn(ctx, "a scheduled workflow process is dropped because no definition was bound to it",
				logging.Field("process_id", sch.ProcessID.String()),
				logging.Field("scheduled_at", sch.CreatedAt),
				logging.Field("bind_grace_period", rt.getBindGracePeriod().String()))
			return nil
		}
		return s.Queue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			CreatedAt:    sch.CreatedAt,
			FailureCount: sch.FailureCount,
		})

	// Suspend requires some revision
	case errors.Is(err, Suspend{}):
		return s.Queue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			FailureCount: sch.FailureCount,
			CreatedAt:    sch.CreatedAt,
		})

	// Halt is the no-reschedule signal: the participant asked the runtime to
	// stop asking. The queue entry is acknowledged and dropped, no fresh
	// ProcessExecution is published, FailureCount does not advance. Resuming
	// the Process is the caller's responsibility, by calling Schedule again
	// with the same ProcessID.
	//
	// The check is errors.Is rather than errors.As to mirror how the rest of
	// the runtime recognises Halt: it is a bare value, with no payload to
	// unwrap into. errors.As would still work here but would also match any
	// future type that happened to embed Halt, which is the opposite of
	// what we want — control flow wrapped into another error is no longer
	// control flow.
	case errors.Is(err, Halt{}):
		return nil

	default:
		return s.Queue.Publish(ctx, ProcessExecution{
			ProcessID:    sch.ProcessID,
			StartTime:    rt.backoffStartTime(),
			FailureCount: sch.FailureCount + 1,
			CreatedAt:    sch.CreatedAt,
		})
	}
}

// isBindGracePeriodExpired tells whether a schedule entry has been waiting for
// its Definition longer than the Runtime is willing to wait for it.
//
// The waiting is measured from ProcessExecution#CreatedAt, the moment the
// Process was scheduled, and not from StartTime, which is pushed forward on
// every requeue and would consequently never grow old enough to expire.
func (rt Runtime) isBindGracePeriodExpired(sch ProcessExecution) bool {
	if sch.CreatedAt.IsZero() { // age unknown, assume the entry is still fresh
		return false
	}
	return rt.getBindGracePeriod() < timeNow().Sub(sch.CreatedAt)
}

func (rt Runtime) getBindGracePeriod() time.Duration {
	if rt.BindGracePeriod <= 0 {
		const defaultBindGracePeriod = time.Minute
		return defaultBindGracePeriod
	}
	return rt.BindGracePeriod
}

func (rt Runtime) backoffStartTime() time.Time {
	var waitTime = rt.WaitTime
	if waitTime <= 0 {
		const defaultWaitTime = 30 * time.Second
		waitTime = defaultWaitTime
	}
	return timeNow().Add(waitTime)
}

type errOutdatedChange struct{}

func (errOutdatedChange) Error() string {
	return "[error] outdated change"
}

func (rt Runtime) runListenToRemoteChanges(ctx context.Context, changes *synckit.Broadcast[ProcessChangeEvent]) error {
	defer changes.Close()
	if rt.Changes == nil {
		return ErrFatal.F("Error, missing %T#ProcessChangeBroadcast", rt)
	}
	var handle = func(msg pubsub.Message[ProcessChangeEvent]) (rerr error) {
		defer comproto.FinishTx(&rerr, msg.ACK, msg.NACK)
		return changes.Publish(ctx, msg.Data())
	}
	for msg, err := range rt.Changes.Subscribe(ctx) {
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
	// Missing-dependency errors must be classified as fatal so that
	// Runtime#withRetry does not consume the retry budget on configuration
	// mistakes. A misconfigured Runtime otherwise blocks for up to ~20s
	// (default resilience.Jitter: 5 attempts × up to 5s jitter) before the
	// caller observes the error, which looks like a hang.
	if rt.Events == nil {
		return ErrFatal.F("missing %T#EventsRepository", rt)
	}
	if rt.Queue == nil {
		return ErrFatal.F("missing %T#ProcessQueue", rt)
	}
	if rt.Changes == nil {
		return ErrFatal.F("missing %T#ProcessQueueChangeBroadcast", rt)
	}
	return validate.Value(ctx, rt)
}
