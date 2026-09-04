package workflow_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TestRuntime_phaserLazyInitRace is a regression test for a hang in the
// workflow test suite caused by a race between the Runtime worker
// goroutine and the testcase framework's test teardown.
//
// The pattern at issue:
//
//   - A test registers a phaser via let.Phaser(s) at the When-block level.
//   - The phaser is accessed only from inside a participant function,
//     which runs in a Runtime worker goroutine spawned by t.Go in a
//     Before hook.
//   - The test body completes quickly and returns without ever touching
//     phaser.Get(t) directly.
//
// Because let.Phaser(s) is backed by testcase.Let, its init function
// runs the first time phaser.Get(t) is called. The cleanup
// registration (t.Cleanup(p.Finish)) is part of that init. When
// phaser.Get(t) is called only from a worker goroutine that reaches
// the participant AFTER the test body returns, the cleanup is
// registered AFTER t.teardown.Finish() has already drained its
// queue. The phaser.Finish cleanup is silently lost, the worker
// goroutine parks on phaser.Wait forever, and t.g.Wait() blocks until
// the Go test timeout panic fires.
//
// The fix is to force eager initialization of the phaser inside the
// test goroutine so its cleanup is registered before the test body
// returns. This regression test pins that contract: a test body that
// spawns work via t.Go and returns immediately must not hang on
// teardown, even when the goroutine accesses a let-bound phaser.
func TestRuntime_phaserLazyInitRace(t *testing.T) {
	const fastFailBudget = 5 * time.Second

	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	phaser := let.Phaser(s).
		// Force eager initialization of the phaser in the test goroutine.
		// Without this line, the phaser is initialized lazily by the
		// participant goroutine, which can race with teardown:
		// t.teardown.Finish() runs before the worker has a chance to
		// register t.Cleanup(p.Finish), so the cleanup is silently
		// dropped and the worker hangs on phaser.Wait.
		EagerLoading(s)

	pid := wftest.LetProcessID(s)
	participantID := wftest.LetParticipantID(s)
	_ = wftest.LetParticipantWithID(s, participantID, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			// Access the phaser ONLY from this goroutine. If
			// teardown fires before this line runs, the
			// t.Cleanup registration inside let.Phaser's init
			// is lost and we hang on phaser.Wait.
			phaser.Get(t).Wait()
			return nil
		}
	})

	// Override c's definition so it executes the blocking participant.
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.ExecuteParticipant{ID: participantID.Get(t)}
	})

	s.Before(func(t *testcase.T) {
		t.Go(func(ctx context.Context) error {
			return c.Runtime.Get(t).Run(ctx)
		})
	})

	s.Test("a fast test body that parks a worker on the phaser must clean up within budget", func(t *testcase.T) {

		// The body completes immediately. The Runtime worker goroutine
		// will be parked on phaser.Wait when this body returns.
		// The race window is the gap between body return and
		// teardown.Finish(): if phaser.Get(t) has not yet been
		// called, the phaser.Finish cleanup is not registered.
		assert.NoError(t, c.Runtime.Get(t).Spawn(t.Context(), pid.Get(t),
			workflow.ExecuteParticipant{ID: participantID.Get(t)}))
	})

	assert.Within(t, fastFailBudget, func(ctx context.Context) {
		s.Finish()
	})
}
