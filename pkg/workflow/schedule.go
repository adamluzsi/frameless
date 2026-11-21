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

// Queue is an ordered queue, where process execution requests are published.
// It is expected to be a Durable and Ordered queue where ordering is sorted by ProcessScheduleEntry#StartTime ASC.
type Queue interface {
	pubsub.Publisher[ExecutionRequest]
	pubsub.Subscriber[ExecutionRequest]
}

type ExecutionRequest struct {
	// ProcessID specifies which ProcessID should be scheduled for execution
	ProcessID ProcessID
	// StartTime defines when it is expected to schedule the process for the next time
	StartTime time.Time
	// CreatedAt is the time at which the Schedule was created
	CreatedAt time.Time
	// FailureCount is the number of execution attempts which failed previously
	FailureCount int
}

// NotificationBroadcast is a Volatile, FanOut exchange based broadcasting pubsub channel,
// where worker nodes can subscribe, to get notified about workflow related events which might are relevant to them.
type NotificationBroadcast interface {
	pubsub.Publisher[Notification]
	pubsub.Subscriber[Notification]
}

type Notification interface {
	GetProcessID() ProcessID
	NotificationType() NotificationType
}

type NotificationType string

type ProcessSchedule struct{ ProcessID ProcessID }

var _ Notification = ProcessSchedule{}

func (ch ProcessSchedule) NotificationType() NotificationType { return "schedule" }
func (ch ProcessSchedule) GetProcessID() ProcessID            { return ch.ProcessID }

type ProcessCancel struct{ ProcessID ProcessID }

var _ Notification = ProcessCancel{}

func (ch ProcessCancel) NotificationType() NotificationType { return "cancel" }
func (ch ProcessCancel) GetProcessID() ProcessID            { return ch.ProcessID }

// Schedule will Schedule a Process for eventually processing.
//
// The caller owns process identity: Schedule refuses a zero id so that
// retrying a failed Schedule always yields the SAME ProcessID, making the
// scheduling contract safe under any kind of caller-side failure
// (network blip, timeout, panic recovery, etc.).
func (rt Runtime) Schedule(ctx context.Context, pid ProcessID, opts ...func(*ExecutionRequest)) error {
	return rt.withRetry(ctx, func() error {
		return rt.schedule(ctx, pid, opts...)
	})
}

func (rt Runtime) schedule(ctx context.Context, pid ProcessID, opts ...func(*ExecutionRequest)) error {
	if err := rt.validateScheduler(ctx); err != nil {
		return err
	}

	if pid == (ProcessID{}) {
		return ErrZeroProcessID.F("Scheduling requires a non-zero processID")
	}

	var schedule = ExecutionRequest{}
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

	if err := rt.Notifications.Publish(ctx, ProcessSchedule{ProcessID: pid}); err != nil {
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
		Isolation: true,
	}

	var notificationsBC synckit.Broadcast[Notification]
	defer notificationsBC.Close() // shared by every upstream subscription attempt

	g.Go(ctx, func(ctx context.Context) (err error) {
		return rt.withRetry(ctx, func() error {
			return rt.runListenToNotifications(ctx, &notificationsBC)
		})
	})

	for range rt.getNumQueueSubscriber() {
		g.Go(ctx, func(ctx context.Context) error {
			return rt.withRetry(ctx, func() error {
				var notificationsCH, job = rt.listenToLocalChangeBroadcast(ctx, &notificationsBC)
				defer job.Cancel()
				return rt.runListenToScheduling(ctx, notificationsCH)
			})
		})
	}

	return g.Wait()
}

func (rt Runtime) listenToLocalChangeBroadcast(ctx context.Context, broadcast *synckit.Broadcast[Notification]) (<-chan Notification, synckit.Job) {
	// Buffer the per-subscriber notification channel so that the broadcast
	// forwarder is not blocked by the (unbuffered) channel-send waiting for
	// a reader that may not exist — most subscribers sit idle at any given
	// time, and an unbuffered channel would make Broadcast#Publish stall on
	// every idle subscriber for the full outdate window, turning a single
	// cancellation into a multi-second chain. A buffered channel lets the
	// forwarder drop notifications for subscribers whose cancel-watching
	// goroutine is not currently parked on the channel; the very first
	// notification for an active subscriber still goes through the select
	// below and can be dropped by outdate if no one is reading.
	var notifications = make(chan Notification, 256)
	var job = synckit.Go(ctx, func(ctx context.Context) error {
		defer close(notifications)
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
			case notifications <- change.Data():
				change.ACK()
			case <-outdate.C:
				change.ACK()
			}
		}
		return nil
	})
	return notifications, job
}

func (rt Runtime) getNumQueueSubscriber() int {
	if rt.NumQueueSubscriber <= 0 {
		var defaultNumQueueSubscriber = runtime.NumCPU()
		return defaultNumQueueSubscriber
	}
	return rt.NumQueueSubscriber
}

func (rt Runtime) withRetry(ctx context.Context, do func() error) error {
	var err error
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
		case isParticipantNotFound(err), isParticipantSignatureMismatch(err):
			// Retrying on this node cannot supply a compatible participant.
			// Let the scheduler offer the process to another node instead.
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
	if err == nil { // the strategy stopped before the first attempt
		return ctx.Err()
	}
	return err
}

func (rt Runtime) runListenToScheduling(ctx context.Context, notifications <-chan Notification) error {
	if rt.Queue == nil {
		return ErrFatal.F("Error, missing %T#Queue", rt)
	}
	for msg, err := range rt.Queue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := rt.runSignalHandler(msg, notifications); err != nil {
			return err
		}
	}
	return nil
}

func (rt Runtime) guardAgainstEarlyExecution(ctx context.Context, sch ExecutionRequest, notifications <-chan Notification) (ok bool) {
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

	case n, ok := <-notifications:
		if !ok {
			return false
		}
		switch n.NotificationType() {
		case (ProcessSchedule{}).NotificationType():
			// The ordered queue may now have an earlier entry. Release this
			// delivery through NACK and select again, without publishing a
			// replacement or a notification that would wake us in a loop.
			return false
		case (ProcessCancel{}).NotificationType():
			if n.GetProcessID() == sch.ProcessID {
				return false // interruption is not proof of durable termination
			}
			goto waiting
		default:
			return false
		}

	case <-ticker.C:
		// A wake-up is only a hint: the clock may have changed since the
		// timer was created. Never execute before the entry's start time.
		if timeNow().Before(sch.StartTime) {
			goto waiting
		}
		return true
	}
}

func (rt Runtime) runSignalHandler(msg pubsub.Message[ExecutionRequest], notifications <-chan Notification) (rErr error) {
	// Only ACK after handling returns successfully. In particular, a panic
	// leaves ack false and propagates after the unfinished delivery is NACKed.
	var ack bool
	defer func() {
		if ack {
			rErr = msg.ACK()
		} else {
			rErr = errorkitlite.Merge(rErr, msg.NACK())
		}
	}()

	ctx, sch := msg.Context(), msg.Data()
	if !rt.guardAgainstEarlyExecution(ctx, sch, notifications) {
		return nil
	}

	const ErrProcessCancel errorkitlite.Error = "ErrProcessCancel"
	executeCTX, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var job = synckit.Go(ctx, func(ctx context.Context) error {
	waiting:
		select {
		case <-ctx.Done():
			return nil
		case ch, ok := <-notifications:
			if !ok {
				return nil
			}
			switch ch.NotificationType() {
			case (ProcessSchedule{}).NotificationType():
				goto waiting
			case (ProcessCancel{}).NotificationType():
				if sch.ProcessID == ch.GetProcessID() {
					cancel(ErrProcessCancel)
					return nil
				}
				goto waiting
			}
		}
		return nil
	})
	defer func() { job.Cancel(); job.Wait() }()

	err := rt.Execute(executeCTX, sch.ProcessID)
	if err != nil && errors.Is(context.Cause(executeCTX), ErrProcessCancel) {
		// Terminate still needs the lock. Immediate redelivery could reclaim
		// it on every poll. Leave a durable retry even if the caller times out.
		rErr = rt.requeue(ctx, sch, 2*cancelPropagationWait)
	} else {
		rErr = rt.handleExecutionResult(ctx, sch, err)
	}
	ack = rErr == nil
	return rErr
}

func (rt Runtime) handleExecutionResult(ctx context.Context, sch ExecutionRequest, err error) error {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, ErrAlreadyRunningProcess):
		// The lock holder may be a synchronous Execute with no queue entry of
		// its own. Retry later without counting contention as a failure.

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

	case errors.Is(err, Suspend{}):
		// Retry after WaitTime without counting the suspension as a failure.

	case ErrIsFatal(err):
		// A permanent execution error leaves the process inert, not complete.
		// Drop this delivery rather than requeueing it or stopping the worker,
		// even when it is joined with a participant availability error.
		logger.Warn(ctx, "a scheduled workflow process is dropped because execution failed fatally",
			logging.Field("process_id", sch.ProcessID.String()),
			logging.Field("error", err.Error()))
		return nil

	case isParticipantNotFound(err), isParticipantSignatureMismatch(err):
		// Another node may have a compatible participant. Count each deferred
		// attempt for periodic warnings, but never impose a hard drop limit.
		sch.FailureCount++
		if sch.FailureCount%rt.getParticipantWarningInterval() == 0 {
			logger.Warn(ctx, "a scheduled workflow process is waiting for a compatible participant",
				logging.Field("process_id", sch.ProcessID.String()),
				logging.Field("failure_count", sch.FailureCount),
				logging.Field("error", err.Error()))
		}

	// Halt is the no-reschedule signal: the participant asked the runtime to
	// stop asking. The queue entry is acknowledged and dropped, no fresh
	// ExecutionRequest is published, FailureCount does not advance. Resuming
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
		sch.FailureCount++
	}
	return rt.requeue(ctx, sch, 0)
}

// isBindGracePeriodExpired tells whether a schedule entry has been waiting for
// its Definition longer than the Runtime is willing to wait for it.
//
// The waiting is measured from ExecutionRequest#CreatedAt, the moment the
// Process was scheduled, and not from StartTime, which is pushed forward on
// every requeue and would consequently never grow old enough to expire.
func (rt Runtime) isBindGracePeriodExpired(sch ExecutionRequest) bool {
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

func (rt Runtime) getParticipantWarningInterval() int {
	if rt.ParticipantWarningInterval <= 0 {
		return 5
	}
	return rt.ParticipantWarningInterval
}

func (rt Runtime) requeue(ctx context.Context, sch ExecutionRequest, minDelay time.Duration) error {
	var waitTime = rt.WaitTime
	if waitTime <= 0 {
		const defaultWaitTime = 30 * time.Second
		waitTime = defaultWaitTime
	}
	sch.StartTime = timeNow().Add(max(waitTime, minDelay))
	return rt.Queue.Publish(ctx, sch)
}

type errOutdatedChange struct{}

func (errOutdatedChange) Error() string {
	return "[error] outdated change"
}

func (rt Runtime) runListenToNotifications(ctx context.Context, notifications *synckit.Broadcast[Notification]) error {
	var handle = func(msg pubsub.Message[Notification]) (rerr error) {
		defer comproto.FinishTx(&rerr, msg.ACK, msg.NACK)
		return notifications.Publish(ctx, msg.Data())
	}
	for msg, err := range rt.Notifications.Subscribe(ctx) {
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

// Validate checks the configuration needed to run a worker. Schedule alone does not need Locks.
func (rt Runtime) Validate(ctx context.Context) error {
	if err := rt.validateScheduler(ctx); err != nil {
		return err
	}
	if rt.Locks == nil {
		return ErrFatal.F("missing workflow.Runtime.Locks (ProcessLocks)")
	}
	return nil
}

func (rt Runtime) validateScheduler(ctx context.Context) error {
	// A configuration error must not spend the runtime's retry budget.
	if rt.Events == nil {
		return ErrFatal.F("missing workflow.Runtime.Events (EventRepository)")
	}
	if rt.Queue == nil {
		return ErrFatal.F("missing workflow.Runtime.Queue")
	}
	if rt.Notifications == nil {
		return ErrFatal.F("missing workflow.Runtime.Notifications (NotificationBroadcast)")
	}
	// Validate the fields without invoking the worker's Validate method.
	type fields Runtime
	return validate.Value(ctx, fields(rt))
}
