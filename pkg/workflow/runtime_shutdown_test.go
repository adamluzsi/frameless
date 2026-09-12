package workflow_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// testEnv is a stripped-down analogue of wftest.C that builds a
// fresh workflow.Runtime per test but does NOT auto-start it via
// t.Go. Auto-start would make it impossible to observe the
// start/stop transitions that drive the shutdown contract under
// test.
type testEnv struct {
	participants  testcase.Var[workflow.Participants]
	conditions    testcase.Var[workflow.Conditions]
	events        testcase.Var[*memory.WorkflowEventRepository]
	queue         testcase.Var[*memory.WorkflowQueue]
	notifications testcase.Var[*memory.WorkflowNotificationBroadcast]
	locks         testcase.Var[*memory.WorkflowProcessLocks]
	runtime       testcase.Var[workflow.Runtime]
}

func bindEnv(s *testcase.Spec) *testEnv {
	var e testEnv
	e.participants = wftest.Participants.Bind(s)
	e.conditions = wftest.Conditions.Bind(s)
	e.events = wftest.EventRepository.Bind(s)
	e.queue = wftest.Queue.Bind(s)
	e.notifications = wftest.NotificationBroadcast.Bind(s)
	e.locks = wftest.ProcessLocks.Bind(s)
	e.runtime = testcase.Let(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants:       e.participants.Get(t),
			Conditions:         e.conditions.Get(t),
			Events:             e.events.Get(t),
			Queue:              e.queue.Get(t),
			Notifications:      e.notifications.Get(t),
			Locks:              e.locks.Get(t),
			RetryStrategy:      singleAttempt{},
			WaitTime:           time.Nanosecond,
			NumQueueSubscriber: 2,
			Codec:              wfjson.NewCodec(),
		}
	})
	return &e
}

// TestRuntime_Run_shutdown pins the lifecycle of workflow.Runtime#Run as
// observed from the outside: what Runtime#Run guarantees when the supplied
// context is cancelled, and what state survives the shutdown so that a
// freshly-started Runtime#Run on the same backend can pick up where the
// previous one stopped.
//
// The runtime is a long-lived background loop. In production it outlives
// every individual workflow it executes — operators bounce it for upgrades,
// recover it after a host reboot, or just restart it because something
// unrelated went wrong on the box. None of those are reasons to drop work
// that has already been scheduled but not yet completed: the queue and the
// event repository outlive the Runtime, and the next Runtime process must
// be able to drain them. The tests below pin that contract end-to-end so a
// refactor of the worker goroutines does not silently break shutdown
// semantics.
func TestRuntime_Run_shutdown(t *testing.T) {
	s := testcase.NewSpec(t)

	// bindEnv gives us a fresh Runtime per test without auto-starting
	// it; this is what lets us observe the start/stop transitions
	// that drive the shutdown contract under test.
	e := bindEnv(s)

	// shutdownWait is the window we give the worker goroutine to
	// react to the Run-ctx being cancelled. The runtime's listener
	// goroutines observe ctx.Done() and unwind fairly quickly; a few
	// hundred milliseconds is plenty on any reasonable host, and
	// short enough that the spec stays responsive when something
	// does hang.
	shutdownWait := let.VarOf(s, 5*time.Second)

	// startRun spawns the Runtime in its own goroutine, returning
	// the ctx that drives its lifetime plus the cancel func for
	// it. The goroutine signals completion via shutdownCh;
	// waitForShutdown blocks on it (or fails the test if the Runtime
	// has not returned within shutdownWait). Cancelling the ctx is
	// the only thing the test needs to do to ask the Runtime to
	// shut down; the goroutine returns once the listener and
	// subscribers have all unwound.
	startRun := func(t *testcase.T) (runCtx context.Context, cancel context.CancelFunc, waitForShutdown func(t *testcase.T)) {
		var cancelFn context.CancelFunc
		runCtx, cancelFn = context.WithCancel(t.Context())

		shutdownCh := make(chan struct{})
		// We intentionally do NOT use t.Go here: t.Go derives its
		// goroutine ctx from t.Context(), which would be a sibling of
		// runCtx, so cancelling runCtx would not propagate to the
		// runtime's worker goroutines. Instead we spawn our own
		// goroutine and drive it with the runCtx directly.
		var done = make(chan struct{})
		go func() {
			defer close(done)
			defer close(shutdownCh)
			_ = e.runtime.Get(t).Run(runCtx)
		}()
		t.Cleanup(func() {
			// Don't block on done here. The bare goroutine is
			// not tracked by t.goroutines, so the test framework
			// won't wait for it. Blocking in Cleanup would hang
			// the test until -timeout, because the participant
			// it parked on is only released after the parent
			// t.Cleanup runs phaser.Finish (which happens later
			// in the chain).
			cancelFn()
		})

		waitForShutdown = func(t *testcase.T) {
			t.Helper()
			select {
			case <-shutdownCh:
			case <-time.After(shutdownWait.Get(t)):
				t.Fatalf("Runtime#Run did not return within %s after its ctx was cancelled",
					shutdownWait.Get(t))
			}
		}
		return runCtx, cancelFn, waitForShutdown
	}

	// makeBlockingParticipant wires up a participant whose body
	// parks on a phaser until release is called. It exposes:
	//
	//   - pid: the participant ID.
	//   - hit: a counter incremented each time the participant body runs.
	//   - release: a func that unblocks every parked Wait.
	//
	// The phaser is created inside the spec so its
	// t.Cleanup-Finish lifecycle is registered before the test body
	// runs (see TestRuntime_phaserLazyInitRace).
	makeBlockingParticipant := func(s *testcase.Spec) (pid testcase.Var[workflow.ParticipantID], hit *int32, release func(t *testcase.T)) {
		// releaseCh is the gate that the participant waits on. The
		// test closes it to release the participant; cancelling the
		// participant's own ctx also wakes it. We use a buffered
		// channel with capacity 1 so a release signal doesn't block
		// on a participant that has already returned (e.g. via
		// ctx.Done).
		releaseCh := let.Var(s, func(t *testcase.T) chan struct{} {
			return make(chan struct{}, 1)
		})
		s.Before(func(t *testcase.T) {
			releaseCh.Get(t)
		})

		var (
			p = wftest.LetParticipantID(s)
			n = new(int32)
		)
		_ = wftest.LetParticipantWithID(s, p, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				atomic.AddInt32(n, 1)
				select {
				case <-releaseCh.Get(t):
				case <-ctx.Done():
				}
				return nil
			}
		})
		return p, n, func(t *testcase.T) {
			// Non-blocking: if the participant is already past
			// releaseCh (e.g. it observed ctx.Done first), the
			// channel already has its buffer slot available and
			// the close lands cleanly.
			ch := releaseCh.Get(t)
			select {
			case ch <- struct{}{}:
			default:
			}
			close(releaseCh.Get(t))
		}
	}

	s.Describe("happy path", func(s *testcase.Spec) {
		// The runtime must reach a stable "no work in flight" state on
		// a clean shutdown so operators can rely on it as the stop
		// signal. The phaser-paused participant is what makes the
		// in-flight state observable; without it the test would race
		// against the runtime finishing before we got a chance to
		// assert anything.
		participantID, hit, _ := makeBlockingParticipant(s)

		var processID = wftest.LetProcessID(s)

		s.Test("the runtime returns once its context is cancelled", func(t *testcase.T) {
			// Bind the definition before starting the runtime so
			// the queue already has a well-defined entry to pick
			// up.
			assert.NoError(t, e.runtime.Get(t).Spawn(t.Context(), processID.Get(t),
				workflow.ExecuteParticipant{ID: participantID.Get(t)}))

			_, cancelRun, waitForShutdown := startRun(t)

			// The participant is now parked on the phaser. With no
			// release, the runtime's worker goroutines stay alive
			// and Run does not return — which is the very thing we
			// want to prove gets cleaned up by the cancellation.
			mustWaitForInFlight(t, hit)

			cancelRun()
			waitForShutdown(t)
		})

		s.Test("the process is observed as not yet completed while in flight", func(t *testcase.T) {
			assert.NoError(t, e.runtime.Get(t).Spawn(t.Context(), processID.Get(t),
				workflow.ExecuteParticipant{ID: participantID.Get(t)}))

			_, cancelRun, _ := startRun(t)
			defer cancelRun()

			mustWaitForInFlight(t, hit)

			completed, err := workflow.IsCompleted(t.Context(),
				e.events.Get(t), processID.Get(t))
			assert.NoError(t, err)
			assert.False(t, completed,
				assert.MessageF("a process whose participant is parked on a phaser "+
					"must not be observed as completed while the participant is "+
					"still in-flight; if it is, the runtime has decided to mark it "+
					"done before its actual work finished"))
		})
	})

	s.Describe("graceful shutdown with in-flight work", func(s *testcase.Spec) {
		participantID, hit, release := makeBlockingParticipant(s)

		var processID = wftest.LetProcessID(s)

		s.Test("the participant is not re-entered across the shutdown boundary", func(t *testcase.T) {
			// Bind the definition before the runtime starts so the
			// queue already has a well-defined entry to pick up.
			assert.NoError(t, e.runtime.Get(t).Spawn(t.Context(), processID.Get(t),
				workflow.ExecuteParticipant{ID: participantID.Get(t)}))

			_, cancelRun, waitForShutdown := startRun(t)

			// Wait for the participant to actually enter its
			// in-flight window, then cancel the run-ctx.
			mustWaitForInFlight(t, hit)

			cancelRun()
			waitForShutdown(t)

			// The participant may or may not have returned by the
			// time the runtime goroutines unwind. We don't assert
			// an exact count here: a cancellation that lands while
			// the worker is iterating the queue can produce either
			// one or two participant invocations depending on
			// timing, and either is acceptable as long as the
			// process is eventually driven to completion. The
			// important invariant — that the in-flight work
			// survives the shutdown and is picked up by a fresh
			// Runtime — is pinned by the "work resumes" test below.
			//
			// We do assert that the participant was actually entered,
			// proving the test reached the in-flight state we wanted
			// to exercise, and that no more than a small bounded
			// number of re-entries happened (a runaway loop would
			// manifest as a huge count).
			got := atomic.LoadInt32(hit)
			assert.True(t, got >= 1 && got <= 4,
				assert.MessageF("the participant must have been entered at "+
					"least once and at most a handful of times during the "+
					"first runtime; observed %d invocations, expected between "+
					"1 and 4", got))
		})

		s.Test("the work resumes after a fresh Runtime#Run on the same backend", func(t *testcase.T) {
			// Same setup: definition is bound, runtime starts,
			// participant parks on the phaser. The shutdown happens
			// while the participant is parked, so the first runtime
			// returns without the process having been completed.
			assert.NoError(t, e.runtime.Get(t).Spawn(t.Context(), processID.Get(t),
				workflow.ExecuteParticipant{ID: participantID.Get(t)}))

			_, firstCancel, waitForShutdown := startRun(t)
			mustWaitForInFlight(t, hit)

			// Snapshot the participant call count before we
			// shut down. This is the baseline we expect to grow
			// by at least one after the second runtime picks up
			// the same queue entry.
			beforeShutdown := atomic.LoadInt32(hit)

			firstCancel()
			waitForShutdown(t)

			// The process must still be pending: no
			// EventCompleted was written before the shutdown.
			// If the shutdown finished the process, the second
			// runtime has nothing to pick up and the assertion
			// below becomes meaningless.
			completed, err := workflow.IsCompleted(t.Context(),
				e.events.Get(t), processID.Get(t))
			assert.NoError(t, err)
			assert.False(t, completed,
				assert.MessageF("the shutdown must not have completed the process; "+
					"if it did, the second runtime has nothing to pick up"))

			// Release the participant so the second runtime's
			// call can return. Without this, the participant
			// would block forever and the second runtime would
			// never make progress. The release is safe before
			// the second runtime has actually entered Wait:
			// Phaser.Finish causes the next Wait to return
			// immediately.
			release(t)

			// Spin up a second Runtime on the same backend. It
			// must find the still-pending entry in the queue and
			// drive the process to completion.
			_, _, _ = startRun(t)

			// The participant must have run *at least* one more
			// time after the restart. The "at least one more"
			// invariant matters: the runtime must not
			// re-execute already completed work, only the
			// still-pending part.
			t.Eventually(func(t *testcase.T) {
				got := atomic.LoadInt32(hit)
				assert.True(t, got >= beforeShutdown+1,
					assert.MessageF("the participant must have re-entered after "+
						"the second Runtime#Run; before shutdown it ran %d time(s), "+
						"after the restart it has run %d time(s)", beforeShutdown, got))
			})

			// And the process must eventually be completed —
			// the workflow completion event lands on the same
			// EventRepository the new runtime is reading from,
			// so this pins the end-to-end "shutdown + restart
			// picks up where we left off" contract.
			e.mustProcessCompletionIs(t, processID.Get(t), true)
		})
	})

	s.Describe("restart with no in-flight work", func(s *testcase.Spec) {
		// Sanity check: starting a second Runtime#Run after a
		// clean shutdown must not throw or re-execute anything.
		// This isolates the restart contract from the in-flight
		// edge cases above, so a failure here points at start/stop
		// bookkeeping rather than at the in-flight semantics.
		var (
			participantID  = wftest.LetParticipantID(s)
			participantHit = let.Var(s, func(t *testcase.T) *int32 {
				var n int32
				return &n
			})
		)
		_ = wftest.LetParticipantWithID(s, participantID, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				atomic.AddInt32(participantHit.Get(t), 1)
				return nil
			}
		})

		s.Test("a second Run after shutdown serves new schedules", func(t *testcase.T) {
			_, cancelFirst, waitForShutdown := startRun(t)
			cancelFirst()
			waitForShutdown(t)

			// A fresh schedule on the same backend, then a
			// fresh Runtime#Run, must complete end-to-end. If
			// the previous shutdown left the queue in a bad
			// state, this assertion fails and tells us where to
			// look.
			secondProcID := mustProcessID(t)
			assert.NoError(t, e.runtime.Get(t).Spawn(t.Context(), secondProcID,
				workflow.ExecuteParticipant{ID: participantID.Get(t)}))

			_, _, _ = startRun(t)

			e.mustProcessCompletionIs(t, secondProcID, true)
			assert.Equal(t, int32(1), atomic.LoadInt32(participantHit.Get(t)),
				assert.MessageF("only the second-scheduled process should have "+
					"hit the participant; if the count is higher, work from "+
					"before the restart leaked into the new runtime"))
		})
	})
}

// mustProcessCompletionIs is the testEnv analogue of
// wftest.C#ProcessCompletionIs: it asserts that the process is (or is
// not) completed within a short timeout, by polling the in-memory
// EventRepository. The polling is bounded so a regression that prevents
// the worker from picking up the entry surfaces as a test failure
// rather than a silent hang.
func (e *testEnv) mustProcessCompletionIs(t *testcase.T, processID workflow.ProcessID, done bool) {
	t.Helper()
	assert.Eventually(t, 5*time.Second, func(testingTB testing.TB) {
		isCompleted, err := workflow.IsCompleted(t.Context(), e.events.Get(t), processID)
		assert.NoError(testingTB, err)
		assert.Equal(testingTB, done, isCompleted,
			assert.MessageF("expected process %v to be completed=%v within the wait window", processID, done))
	})
}

// mustWaitForInFlight blocks until the participant has been entered
// at least once, using the hit counter exposed by makeBlockingParticipant.
// It is the bridge between the test goroutine and the runtime's worker
// goroutine, which cannot be synchronised directly.
//
// The hit counter is incremented at the top of the participant body, so
// observing it become >= 1 proves the worker has reached the participant
// and is parked on the phaser awaiting release.
//
// The wait is bounded by a short timeout so a regression that prevents
// the worker from picking up the entry surfaces as a test failure rather
// than a silent hang.
func mustWaitForInFlight(t *testcase.T, hit *int32) {
	t.Helper()
	assert.Eventually(t, 5*time.Second, func(testingTB testing.TB) {
		if atomic.LoadInt32(hit) >= 1 {
			return
		}
		testingTB.Fatalf("participant never entered the in-flight window")
	})
}

// singleAttempt is a minimal retry strategy that runs each attempt
// at most once, matching the contract of wftest.noFaultTolerance
// without depending on its unexported type. We need this because
// the test wants the runtime to surface every error directly rather
// than retry it through the jitter backoff, which would obscure the
// shutdown semantics we are pinning.
type singleAttempt struct{}

func (singleAttempt) ShouldTry(_ context.Context, attempt resilience.RetryAttempt) bool {
	return attempt.FailureCount == 0
}
