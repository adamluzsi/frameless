package workflow

import (
	"context"
	"iter"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/guard"
)

// Runtime is the default runtime to execute process definitions.
// It can be extended or reimplemented if it doesn't fit your workflow related use-cases.
type Runtime struct {
	// Events is the source of truth for the workflow process states.
	Events EventRepository
	// Participants is the system provided Participant repository that can be used by workflow builders.
	Participants ParticipantRepository
	// Conditions is the system provided Condition repository that can be used by workflow builders.
	Conditions ConditionRepository
	// Locks is a external distributed lock that enables the blocking by ProcessID.
	Locks ProcessLocks
	// Queue contains the scheduled metadata about which Process requires execution.
	Queue ProcessExecutionQueue
	// Changes contains the information about whether or not
	// ProcessExecutionQueue might have a new higher priority Process to be executed
	Changes ProcessChangeBroadcast

	// --- [OPTIONAL FIELDS] --- //

	// Codec [optional] is the codec.Codec which is used to serialise workflow related values.
	Codec Codec
	// NumQueueSubscriber [optional] is the number of queue subscribers.
	//
	// Each subscriber takes one entry off the ProcessExecutionQueue at a time and
	// executes it to completion before reaching for the next one, which makes this
	// the number of workflow processes the node runs at once.
	//
	// Default: number of CPU
	NumQueueSubscriber int
	// WaitTime [optional] is the time waited in case a workflow process requires rescheduling due to suspension.
	//
	// Default: 30 seconds
	WaitTime time.Duration
	// RetryStrategy [optional] is the retry strategy applied when a non-fatal error occurs during a task execution.
	RetryStrategy resilience.RetryStrategy
	// BindGracePeriod [optional] is how long a scheduled Process may wait for
	// its Definition to arrive through Runtime#Bind.
	//
	// Scheduling and binding are separate operations, so a Process can reach the
	// execution queue slightly ahead of its Definition. While the grace period
	// lasts, such a Process is simply requeued. Once it runs out, the schedule
	// entry is discarded, because nothing suggests a Definition is still coming,
	// and an entry that can never execute must not stay in a shared queue.
	//
	// The grace period is measured from the moment the Process was scheduled
	// (ProcessExecution#CreatedAt), and it is expressed in wall-clock time rather
	// than in a number of attempts on purpose: attempts accumulate faster the more
	// worker nodes are running, so an attempt based budget would expire much
	// sooner on a large cluster than on a small one.
	//
	// Default: 1 minute
	BindGracePeriod time.Duration
	// ContextSetup [optional] allows you to configure the request context of a workflow process execution.
	// Ideal for adding tracing and logging fields to the workflow execution context
	ContextSetup ContextSetup
}

type ProcessLocks interface {
	guard.LockerFactory[ProcessID, ProcessLock]
}

type ProcessLock interface {
	guard.NonBlockingLocker
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

type EventRepository interface {
	comproto.OnePhaseCommitProtocol
	crud.Creator[Event]
	crud.AllFinder[Event]
	crud.ByIDFinder[Event, EventID]
	FindByProcessID(ctx context.Context, pid ProcessID) iter.Seq2[Event, error]
}

func LookupEventsRepository(ctx context.Context) (EventRepository, error) {
	rt, ok := RuntimeFromContext(ctx)
	if !ok {
		return nil, ErrNoContextRuntime
	}
	if rt.Events == nil {
		return nil, ErrFatal.F("missing %T#EventsRepository in context", rt)
	}
	return rt.Events, nil
}

// RuntimeSignal is a type of error
// which is considered as happy error value.
//
// It is used in end-user code (Participant functions)
// to instruct the workflow.Runtime to a certain action.
type RuntimeSignal interface {
	error
	// RuntimeSignalExecute is executed by the Runtime
	// when a signal bubbles up from a participant to the Runtime.
	//
	// If error occurs, then Runtime abandon the execution.
	// If SignalExecution succeeds, then Runtime will re-execute the workflow for the ProcessID.
	RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error
}

// isRuntimeSignal reports whether err is one of the runtime's happy errors.
//
// The check is deliberately a plain type assertion rather than errors.As:
// Runtime#execute dispatches a signal with the very same unwrapped assertion,
// so a signal that the runtime would no longer recognise must not count as a
// happy outcome anywhere else either.
func isRuntimeSignal(err error) bool {
	_, ok := err.(RuntimeSignal)
	return ok
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
	ctx = rt.ContextSetup.SetUp(ctx)
	return ctx
}

// Spawn starts a new workflow Process bound to the given id and Definition.
// It binds the Definition via the event history (see Bind) and then schedules it for execution.
//
// Spawn is idempotent, repeated execution should not cause different final outcome for execution.
func (rt Runtime) Spawn(ctx context.Context, id ProcessID, def Definition) error {
	if err := rt.Bind(ctx, id, def); err != nil {
		return err
	}
	return rt.Schedule(ctx, id)
}

// Execute runs the process by replaying the UseDefinitionEvent entries in
// its event history. A Process is intentionally stateless: it carries only
// its ID. The current definition is read from the event history — the most
// recent UseDefinitionEvent in fast-forward mode, or every UseDefinitionEvent
// in NoFastForward mode.
//
// If the process has no UseDefinitionEvent yet (e.g. a brand-new Process that
// has never been Spawned), Execute is a no-op that records EventCompleted:
// there is nothing to execute, and the process is considered trivially
// complete. Use Spawn to bind a Process to its initial definition via the
// event history.
//
// Execution strategy:
//
//   - Default (fast-forward): only the most recent UseDefinitionEvent is
//     executed. The idempotent executor within each Definition will skip
//     already-recorded steps, so re-running Execute is safe and progress
//     resumes from the last persisted state.
//   - NoFastForward: every UseDefinitionEvent is executed in order, from the
//     first onward. Useful for replaying the entire process lifetime.
func (rt Runtime) Execute(ctx context.Context, pid ProcessID) error {
	return rt.withRetry(ctx, func() error {
		return rt.execute(ctx, pid)
	})
}

func (rt Runtime) execute(ctx context.Context, pid ProcessID) (rErr error) {
	if rt.Events == nil {
		return ErrFatal.F("the workflow.Runtime has no EventsRepository configured")
	}

	if pid.IsZero() {
		return ErrZeroProcessID.F("workflow.Runtime#Execute")
	}

	ctx = ctxHProcessID.ContextWith(ctx, pid)

	ctx, acquired, unlock, err := rt.tryLock(ctx, pid)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrAlreadyRunningProcess.F("%v", pid)
	}
	defer errorkit.Finish(&rErr, unlock)

	ctx = rt.Context(ctx)

processing:

	var useDefEvents []EventUseDefinition

	for event, err := range rt.Events.FindByProcessID(ctx, pid) {
		if err != nil {
			return err
		}
		if useDef, ok := event.(EventUseDefinition); ok {
			useDefEvents = append(useDefEvents, useDef)
		}
		if _, ok := event.(EventCompleted); ok {
			return nil
		}
		if _, ok := event.(EventTerminated); ok {
			return nil
		}
	}

	slicekit.SortBy(useDefEvents, func(a, b EventUseDefinition) bool {
		return a.Timestamp.Before(b.Timestamp)
	})

	def, ok := slicekit.Last(useDefEvents)
	if !ok {
		return ErrNoProcessDefinition.F("process-id: %s", pid.String())
	}

	var definitionContext = WithName(ctx, def.EventID.String())
	if err := def.Definition.Execute(definitionContext, pid); err != nil {
		if sig, ok := err.(RuntimeSignal); ok {
			if err := sig.RuntimeSignalExecute(definitionContext, rt, pid); err != nil {
				return err
			}
			goto processing
		}
		return err
	}

	return Complete{}.RuntimeSignalExecute(ctx, rt, pid)
}

func (rt Runtime) tryLock(ctx context.Context, pid ProcessID) (context.Context, bool, func() error, error) {
	if rt.Locks == nil {
		return nil, false, nil, ErrFatal.F("missing workflow.Runtime#ProcessLocks dependency")
	}
	var locker = rt.Locks.LockerFor(pid)
	lockContext, acquired, err := locker.TryLock(ctx)
	if err != nil {
		return nil, false, nil, err
	}
	if !acquired {
		return nil, false, nil, nil
	}
	return lockContext, true, func() error { return locker.Unlock(lockContext) }, nil
}

// ErrZeroProcessID is returned when a workflow.Runtime operation is called
// with a zero ProcessID. The caller must own process identity so that retries
// remain idempotent — the runtime never mints process IDs on the caller's
// behalf.
const ErrZeroProcessID errorkitlite.Error = "workflow.Runtime requires a non-zero ProcessID, " +
	"because the caller must own process identity for safe retry semantics"

// Bind associates a Definition with the given ProcessID by recording a
// UseDefinitionEvent in the runtime's EventsRepository. It is idempotent:
// calling Bind multiple times with the same (ctx, id, def) only writes one
// UseDefinitionEvent.
//
// Bind is the canonical way to make a Process "exist" — Execute reads the
// current definition from the event history, so until Bind has been called
// (or Spawn has been used) the process has no definition and Execute is a
// no-op.
func (rt Runtime) Bind(ctx context.Context, processID ProcessID, def Definition) (err error) {
	return rt.withRetry(ctx, func() error {
		return rt.bind(ctx, processID, def)
	})
}

func (rt Runtime) bind(ctx context.Context, processID ProcessID, def Definition) error {
	if processID.IsZero() {
		return ErrZeroProcessID
	}

	if rt.Events == nil {
		return ErrFatal.F("missing workflow.Runtime#EventsRepository")
	}

	// Idempotency at the UseDefinitionEvent level: if the process is already
	// bound to a definition, do not overwrite it.
	//
	// TODO: this is not safe against race conditions, but safe for repeated
	// execution. Concurrent Binds with the same id can produce duplicate
	// UseDefinitionEvent entries — callers that need strict race-safety
	// must enforce it at their integration layer.
	for event, err := range rt.Events.FindByProcessID(ctx, processID) {
		if err != nil {
			return err
		}
		if _, ok := event.(EventUseDefinition); ok {
			return nil
		}
	}

	var eventID, err = MakeEventID()
	if err != nil {
		return err
	}
	var useDef Event = EventUseDefinition{
		EventID:    eventID,
		ProcessID:  processID,
		Timestamp:  timeNow(),
		Definition: def,
	}

	return rt.Events.Create(ctx, &useDef)
}

func (rt Runtime) Terminate(ctx context.Context, pid ProcessID) (err error) {
	return rt.withRetry(ctx, func() error {
		return rt.terminate(ctx, pid)
	})
}

func (rt Runtime) terminate(ctx context.Context, pid ProcessID) (err error) {
	if pid.IsZero() {
		return ErrZeroProcessID.F("workflow.Runtime#Terminate")
	}
	var (
		lockContext context.Context
		unlock      func() error
	)
	for {
		var (
			acquired bool
			lockErr  error
		)
		lockContext, acquired, unlock, lockErr = rt.tryLock(ctx, pid)
		if lockErr != nil {
			return lockErr
		}
		if acquired { // no processing is running, and it is safe to terminate
			break
		}
		if err := rt.Changes.Publish(ctx, ProcessCancel{ProcessID: pid}); err != nil {
			return err
		}
	}
	defer errorkit.Finish(&err, unlock)
	return Terminate{}.RuntimeSignalExecute(lockContext, rt, pid)
}

type ctxKeyRuntime struct{}

var ctxHRuntime contextkit.ValueHandler[ctxKeyRuntime, Runtime]

func RuntimeFromContext(ctx context.Context) (Runtime, bool) {
	return ctxHRuntime.Lookup(ctx)
}

type ctxKeyProcessID struct{}

var ctxHProcessID contextkit.ValueHandler[ctxKeyProcessID, ProcessID]

func ProcessIDFromContext(ctx context.Context) (ProcessID, bool) {
	return ctxHProcessID.Lookup(ctx)
}
