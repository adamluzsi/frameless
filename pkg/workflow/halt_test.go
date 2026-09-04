package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TestHalt specifies workflow.Halt, the RuntimeSignal that ends the current
// execution of a Process without marking it complete and without rescheduling
// it for further execution.
//
// Halt is the no-reschedule cousin of Suspend. Both stop the current execution
// and neither is recorded as a step outcome, but Suspend asks the runtime to
// come back later on its own schedule, whereas Halt asks the runtime to stop
// asking. The Process is left inert in the event log: not completed, not
// retried, not requeued. Resuming the Process is the caller's job, by calling
// Schedule again.
func TestHalt(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	// signal is what the participant raises in place of returning nil. It is
	// parameterised so the wrapped-Halt rainy path can drive it without
	// depending on any concrete wrapper the workflow package provides.
	signal := let.Var(s, func(t *testcase.T) workflow.RuntimeSignal {
		return workflow.Halt{}
	})

	s.Describe("#Raise", func(s *testcase.Spec) {
		var participantCalls = let.VarOf(s, 0)

		_, participantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				participantCalls.Set(t, participantCalls.Get(t)+1)
				return signal.Get(t)
			}
		})

		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.ExecuteParticipant{ID: participantID.Get(t)}
		})

		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("Runtime#Execute returns the Halt, unmodified", func(t *testcase.T) {
			err := act(t)
			assert.True(t, errors.Is(err, workflow.Halt{}),
				assert.MessageF("the signal is the contract; "+
					"Runtime#Execute must surface it so the scheduler can recognise it. "+
					"swallowing it here would force callers to invent a side channel "+
					"to tell the difference between a halted run and a normal completion. "+
					"got %#v", err))
		})

		s.Then("the participant that raised Halt is left out of the event history", func(t *testcase.T) {
			_ = act(t)

			assert.Empty(t, participantEventsOf(t, c),
				assert.MessageF("a RuntimeSignal is dynamic control flow, not a step result; "+
					"recording the call would freeze one execution's decision and let a "+
					"replay skip the step that decides whether to still halt. "+
					"See TestRuntime_Execute_participantRuntimeSignal for the Suspend case"))
		})

		s.Then("the Process is not marked complete", func(t *testcase.T) {
			_ = act(t)

			completed, err := workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), c.ProcessID.Get(t))
			assert.NoError(t, err)
			assert.False(t, completed,
				assert.MessageF("Halt is not completion; "+
					"workflow.IsCompleted must stay false so a Halted Process is "+
					"distinguishable from one that ran to its natural end"))
		})

		s.When("the participant wraps the Halt in another error", func(s *testcase.Spec) {
			// The runtime's signal contract is unwrapped signals only —
			// dispatch and isRuntimeSignal both rely on the bare type
			// assertion / errors.Is. A wrapped Halt is no longer control
			// flow, on purpose; pinning that here keeps any future "be
			// liberal in what you accept" change visible.
			//
			// runtimeSignalWrapper is deliberately opaque: it does NOT
			// implement Unwrap, so errors.Is cannot see through it. If it
			// did, the test would silently pass for the wrong reason.
			signal.Let(s, func(t *testcase.T) workflow.RuntimeSignal {
				return runtimeSignalWrapper{inner: workflow.Halt{}}
			})

			s.Then("the runtime treats it as a failure, not a Halt", func(t *testcase.T) {
				err := act(t)
				assert.True(t, err != nil,
					assert.MessageF("a wrapped Halt is no longer control flow; "+
						"the runtime's dispatch and withRetry both rely on the "+
						"unwrapped identity. got %#v", err))
				assert.False(t, errors.Is(err, workflow.Halt{}),
					assert.MessageF("errors.Is must not see through the wrapper — "+
						"the contract is unwrapped signals only"))
			})
		})
	})

	s.Context("as part of scheduling", func(s *testcase.Spec) {
		var (
			participantID    = wftest.LetParticipantID(s)
			participantCalls = let.VarOf(s, 0)
		)

		_ = wftest.LetParticipantWithID(s, participantID, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				participantCalls.Set(t, participantCalls.Get(t)+1)
				return workflow.Halt{}
			}
		})

		subject := c.Runtime.Bind(s)

		var (
			Context   = let.Context(s)
			process   = wftest.LetProcessID(s)
			startTime = let.Var(s, func(t *testcase.T) time.Time {
				return time.Time{}
			})
		)

		// processWithDefinition mirrors the helper from TestRuntime_scheduling.
		processWithDefinition := func(mk func(t *testcase.T) workflow.Definition) func(t *testcase.T) workflow.ProcessID {
			return func(t *testcase.T) workflow.ProcessID {
				var p = process.Super(t)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), p, mk(t)))
				return p
			}
		}

		process.Let(s, processWithDefinition(func(t *testcase.T) workflow.Definition {
			return workflow.ExecuteParticipant{ID: participantID.Get(t)}
		}))

		// scheduleAct is the public-API surface: it asks the runtime to
		// schedule a Process for execution. It is the call the user makes,
		// not the queue subscriber.
		scheduleAct := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Schedule(Context.Get(t), process.Get(t), func(s *workflow.ProcessExecution) {
				s.StartTime = startTime.Get(t)
			})
		})

		// startRun runs the Runtime worker for the lifetime of the test.
		// The worker consumes the queue and dispatches each entry to
		// Runtime#Execute, which is where runSignalHandler sees a Halt and
		// decides whether to ACK-and-drop or to requeue.
		startRun := func(t *testcase.T) {
			t.Go(func(ctx context.Context) error {
				return subject.Get(t).Run(ctx)
			})
		}

		// isCompleted reports whether the scheduled process has reached a
		// completed state, as determined by its event history. For a Halted
		// process this must stay false, which is the headline difference
		// from Suspend.
		isCompleted := func(t *testcase.T) bool {
			completed, err := workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), process.Get(t))
			return err == nil && completed
		}

		s.When("a Process Halted on its only run", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				startRun(t)
			})

			s.Then("the participant runs once and the Process is not marked complete", func(t *testcase.T) {
				assert.NoError(t, scheduleAct(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, 1, participantCalls.Get(t),
						assert.MessageF("the participant must be called at least once"))
				})

				assert.False(t, isCompleted(t),
					assert.MessageF("Halt is not completion; "+
						"workflow.IsCompleted must stay false so the Process is "+
						"distinguishable from one that ran to its natural end"))
			})

			s.Then("the scheduler does not re-execute the Process on its own", func(t *testcase.T) {
				assert.NoError(t, scheduleAct(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, 1, participantCalls.Get(t))
				})

				// Wait past the runtime's normal backoff window. A
				// Suspended process would be picked up again in this
				// window. A Halted one must not.
				time.Sleep(deadline)

				assert.Equal(t, 1, participantCalls.Get(t),
					assert.MessageF("a Halted Process must not be re-executed by "+
						"the scheduler; the queue entry is acknowledged and dropped. "+
						"if it is being re-executed, the participant will be called "+
						"again and participantCalls will grow past 1"))
				assert.False(t, isCompleted(t),
					assert.MessageF("a Halted Process never reaches completed"))
			})

			s.When("the Process is rescheduled externally", func(s *testcase.Spec) {
				s.Then("the participant is asked again", func(t *testcase.T) {
					// First Schedule: the queue subscriber picks up the
					// entry, the participant Halt-s, runSignalHandler
					// ACKs and drops. participantCalls goes to 1.
					assert.NoError(t, scheduleAct(t))

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, 1, participantCalls.Get(t))
					})

					// Second Schedule: the same queue subscriber picks
					// up the fresh entry, replays the Process, the
					// participant is asked again. participantCalls goes
					// to 2.
					assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), process.Get(t)))

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, 2, participantCalls.Get(t),
							assert.MessageF("a re-Scheduled Halted Process must re-execute "+
								"from the start of the Definition, asking the Halt-raising "+
								"step again — that's the only way out of a Halt"))
					})
				})

				s.Then("the second Halt is also not completion", func(t *testcase.T) {
					assert.NoError(t, scheduleAct(t))

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, 1, participantCalls.Get(t))
					})

					assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), process.Get(t)))

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, 2, participantCalls.Get(t))
					})

					assert.False(t, isCompleted(t),
						assert.MessageF("the second Halt is also not completion; "+
							"the contract is the same on every external Schedule"))
				})
			})
		})
	})
}

// runtimeSignalWrapper is a minimal error that wraps a RuntimeSignal so the
// TestHalt wrapped-signal rainy path can drive it without depending on any
// concrete wrapper the workflow package already provides.
//
// Crucially, it does NOT implement Unwrap — errors.Is must not see through
// it, or the wrapped-signal rainy path passes for the wrong reason. If a
// future change to the runtime starts unwrapping signals, this is the test
// that catches it.
type runtimeSignalWrapper struct {
	inner error
}

func (w runtimeSignalWrapper) Error() string { return "wrapped: " + w.inner.Error() }

func (w runtimeSignalWrapper) RuntimeSignalExecute(ctx context.Context, rt workflow.Runtime, id workflow.ProcessID) error {
	return w
}

func ExampleHalt() {
	// A participant asks the runtime to stop processing this Process without
	// marking it complete and without rescheduling it. Resuming the Process
	// is the caller's responsibility, by re-Scheduling the same ProcessID.
	_ = workflow.ExecuteParticipant{ID: "review-gate"}
}
