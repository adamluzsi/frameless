package workflow_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TestEventError_participant expresses the behavioural contract that
// when a workflow participant fails with an error during execution,
// the workflow records an EventError event attributed to that participant.
//
// A new EventError is recorded for every individual failure occurrence,
// so that an entire history of failures can be reviewed later per participant.
//
// Signals and definitions returned from participants are not considered errors
// and therefore must not produce an EventError event.
func TestEventError_participant(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		c   = wftest.LetC(s)
		pid = wftest.LetParticipantID(s)
	)

	var (
		callCount = let.VarOf(s, 0)
		// failUntil defines how many times the participant should fail before it
		// returns successfully. Used by tests that exercise the "fails N times
		// then passes" pattern, so the test can cleanly observe N distinct
		// EventError events AND a successful final attempt (with its downstream
		// side effects, e.g. output variable values) in a single spec.
		failUntil = let.VarOf(s, 0)
		// expErr is the error the participant returns when it fails.
		// let.Error(s) yields a fresh random error per test, so the spec
		// exercises the runtime's error classification behaviour against a
		// variety of generic error values rather than pinning the contract
		// to any single concrete error type.
		expErr = let.Error(s)
	)

	// lastOutput captures the string value returned by the participant on its
	// final (successful) attempt, so tests can assert that downstream values
	// are written into the output variable after the eventual success.
	lastOutput := let.VarOf(s, "")

	participant := wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
		return func(ctx context.Context, in string) (string, error) {
			n := callCount.Get(t) + 1
			callCount.Set(t, n)

			if n <= failUntil.Get(t) {
				return "", expErr.Get(t)
			}

			out := "ok-" + t.Random.UUID()
			lastOutput.Set(t, out)
			return out, nil
		}
	})

	var (
		inKey = let.As[workflow.VarName](let.UUID(s))
		inVal = let.UUID(s)

		input = let.Var(s, func(t *testcase.T) []workflow.VarName {
			return []workflow.VarName{inKey.Get(t)}
		})

		outKey = let.As[workflow.VarName](let.UUID(s))
		output = let.Var(s, func(t *testcase.T) []workflow.VarName {
			return []workflow.VarName{outKey.Get(t)}
		})
	)

	subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
		return &workflow.ExecuteParticipant{
			ID:     pid.Get(t),
			Input:  input.Get(t),
			Output: output.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)

			processID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := c.ProcessID.Super(t)
				setVar(t, c.Runtime.Get(t), p, inKey.Get(t), inVal.Get(t))
				return p
			})
		)

		// act executes the participant once. Tests assert on the resulting event history.
		act := let.Act(func(t *testcase.T) error {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			return subject.Get(t).Execute(execCTX, processID.Get(t))
		})

		// errorsOf collects every EventError recorded for the process under test.
		errorsOf := func(t *testcase.T) []workflow.EventError {
			t.Helper()
			var out []workflow.EventError
			for _, ev := range c.ProcessEvents(t, processID.Get(t)) {
				ee, ok := ev.(workflow.EventError)
				if !ok {
					continue
				}
				out = append(out, ee)
			}
			return out
		}

		// Happy path: a single participant failure is recorded as exactly one EventError.
		s.Then("a single participant failure is recorded as a single EventError", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), expErr.Get(t))

			errors := errorsOf(t)
			first, ok := slicekit.First(errors)
			assert.True(t, ok, assert.Message("an EventError must be recorded for the failed participant"))

			assert.Equal(t, first.ParticipantID, pid.Get(t),
				assert.Message("the EventError must attribute the failure to the participant that produced it"))
			assert.Equal(t, first.ProcessID, processID.Get(t))
			assert.Equal(t, first.Error, expErr.Get(t).Error(),
				assert.Message("the EventError must carry the error message that the participant returned"))
			assert.NotEmpty(t, first.EventID)
			assert.False(t, first.Timestamp.IsZero())
			assert.NotNil(t, first.Path)
		})

		// Failure-then-success: every failure is recorded as its own EventError,
		// the eventual successful attempt produces no further EventError,
		// and downstream values (output variable) arrive correctly.
		s.When("the participant fails multiple times before eventually succeeding", func(s *testcase.Spec) {
			failUntil.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(2, 5)
			})

			s.Before(func(t *testcase.T) {
				// Drive the participant through all its failing attempts and the
				// final successful one. Mirrors the TestExecuteParticipant_rollback
				// pattern: each call to act() is one execution attempt, and we
				// keep going until the participant finally succeeds.
				expectedAttempts := failUntil.Get(t) + 1
				for i := 1; i <= expectedAttempts; i++ {
					err := act(t)
					if i <= failUntil.Get(t) {
						assert.ErrorIs(t, err, expErr.Get(t),
							assert.MessageF("attempt #%d must fail with the expected error", i))
						continue
					}
					assert.NoError(t, err,
						assert.MessageF("attempt #%d must succeed (after %d failures)", i, failUntil.Get(t)))
				}
			})

			s.Then("each failure occasion is recorded as its own EventError", func(t *testcase.T) {
				errors := errorsOf(t)
				assert.Equal(t, failUntil.Get(t), len(errors),
					assert.Message("one EventError must be recorded per failure occasion"))

				for i, ee := range errors {
					assert.Equal(t, ee.ParticipantID, pid.Get(t),
						assert.MessageF("EventError #%d must be attributed to the participant", i+1))
					assert.Equal(t, ee.Error, expErr.Get(t).Error(),
						assert.MessageF("EventError #%d must carry the error message", i+1))
					assert.NotEmpty(t, ee.EventID,
						assert.MessageF("EventError #%d must have a non-empty EventID", i+1))
					assert.False(t, ee.Timestamp.IsZero(),
						assert.MessageF("EventError #%d must have a timestamp", i+1))
				}

				// Each occurrence must be a distinct event, so the audit trail
				// can be inspected on a per-failure basis.
				seenIDs := make(map[workflow.EventID]struct{}, len(errors))
				for _, ee := range errors {
					_, dup := seenIDs[ee.EventID]
					assert.False(t, dup,
						assert.MessageF("EventError %s must be unique across occurrences", ee.EventID))
					seenIDs[ee.EventID] = struct{}{}
				}
			})

			s.Then("the eventual successful attempt produces no further EventError", func(t *testcase.T) {
				// Even after the participant succeeds, no extra EventError is recorded
				// beyond the N failures we already accounted for.
				errors := errorsOf(t)
				assert.Equal(t, failUntil.Get(t), len(errors),
					assert.Message("no EventError must be recorded for the successful attempt"))
			})

			s.Then("the participant's eventual success delivers its output value", func(t *testcase.T) {
				// Downstream side effect: the successful attempt must persist its
				// output via the output variable mapping, so consumers can observe it.
				got, ok := getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)).(string)
				assert.True(t, ok,
					assert.Message("the successful participant attempt must write its output variable"))
				assert.Equal(t, lastOutput.Get(t), got,
					assert.Message("the output variable must hold the value returned by the successful attempt"))
			})
		})

		// When the participant returns no error, no EventError is recorded.
		s.When("the participant completes without an error", func(s *testcase.Spec) {
			failUntil.LetValue(s, 0)

			s.Then("no EventError is recorded", func(t *testcase.T) {
				assert.NoError(t, act(t))

				errors := errorsOf(t)
				assert.Empty(t, errors,
					assert.Message("a successful participant execution must not record an EventError"))
			})
		})

		// Signals (e.g. Terminate) are not failures: they must not produce an EventError.
		s.When("the participant returns a workflow signal rather than an error", func(s *testcase.Spec) {
			failUntil.Let(s, func(t *testcase.T) int { return 1 })
			participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
				return func(ctx context.Context, in string) (string, error) {
					callCount.Set(t, callCount.Get(t)+1)
					return "", workflow.Terminate{}
				}
			})

			s.Then("the signal is forwarded but no EventError is recorded", func(t *testcase.T) {
				_ = act(t) // may or may not error, depending on signal handling policy

				errors := errorsOf(t)
				assert.Empty(t, errors,
					assert.Message("signals returned by a participant must not be classified as errors"))
			})
		})

		// Definition-like values returned from a participant are not failures either.
		s.When("the participant returns a workflow definition value rather than an error", func(s *testcase.Spec) {
			failUntil.Let(s, func(t *testcase.T) int { return 1 })
			participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
				return func(ctx context.Context, in string) (string, error) {
					callCount.Set(t, callCount.Get(t)+1)
					return "", workflow.SetVar{Name: "noop", Value: 1}
				}
			})

			s.Then("no EventError is recorded", func(t *testcase.T) {
				_ = act(t)

				errors := errorsOf(t)
				assert.Empty(t, errors,
					assert.Message("definitions returned by a participant must not be classified as errors"))
			})
		})

		// Non-fatal generic errors (e.g. io.EOF) must still surface as EventError.
		s.When("the error is a non-fatal generic error such as io.EOF", func(s *testcase.Spec) {
			failUntil.Let(s, func(t *testcase.T) int { return 1 })
			expErr.Let(s, func(t *testcase.T) error { return io.EOF })

			s.Then("it is recorded as an EventError attributed to the participant", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), io.EOF)

				errors := errorsOf(t)
				first, ok := slicekit.First(errors)
				assert.True(t, ok)
				assert.Equal(t, first.ParticipantID, pid.Get(t))
				assert.Equal(t, first.Error, io.EOF.Error())
			})
		})

		// Sanity guard: error implementations that are NOT workflow signals
		// or definitions must still be treated as errors and recorded.
		s.When("the error is a plain error value that wraps no signal/definition", func(s *testcase.Spec) {
			// Captured in a local var rather than via LetValue, because LetValue
			// rejects *errors.errorString pointers to keep tests independent.
			plainErr := errors.New("boom")
			failUntil.Let(s, func(t *testcase.T) int { return 1 })
			expErr.Let(s, func(t *testcase.T) error {
				return plainErr
			})

			s.Then("it is recorded as an EventError", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), plainErr)

				errors := errorsOf(t)
				first, ok := slicekit.First(errors)
				assert.True(t, ok)
				assert.Equal(t, first.ParticipantID, pid.Get(t))
				assert.Equal(t, first.Error, "boom")
			})
		})
	})
}
