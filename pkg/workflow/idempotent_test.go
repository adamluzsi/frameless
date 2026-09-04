package workflow_test

import (
	"context"
	"errors"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TestRuntime_Execute_participantFollowUpDefinition pins the contract documented
// on workflow.Definition and on idempotentExecutor#handleResultDefinition:
//
// a Participant may return a Definition instead of nil to extend its own
// life-cycle, and that call "is persisted with the Definition attached, making a
// replay of the same logical step a no-op".
//
// The persistence has to survive the follow-up Definition's own outcome. A
// follow-up that suspends is an ordinary, expected result — workflow.Sleep is
// built on exactly that — and it must not erase the record of the participant
// call that produced it.
func TestRuntime_Execute_participantFollowUpDefinition(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var participantCalls = let.VarOf(s, 0)

	// followUp is the Definition the participant hands back to the runtime,
	// returned as a "happy error" (see the workflow.Definition doc comment).
	var followUp = let.Var(s, func(t *testcase.T) workflow.Definition {
		return wftest.Stub{
			StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				return nil
			},
		}
	})

	_, participantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			participantCalls.Set(t, participantCalls.Get(t)+1)
			return followUp.Get(t)
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.ExecuteParticipant{ID: participantID.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("the participant call is recorded in the event history", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, 1, len(participantEventsOf(t, c)),
				assert.MessageF("expected the participant execution to be persisted"))
		})

		s.Then("a replay does not call the participant again", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, 1, participantCalls.Get(t))

			assert.NoError(t, act(t))
			assert.Equal(t, 1, participantCalls.Get(t),
				assert.MessageF("expected the recorded execution to be replayed from the event history"))
		})

		s.When("the follow-up Definition suspends the process", func(s *testcase.Spec) {
			followUp.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						return workflow.Suspend{}
					},
				}
			})

			s.Then("the process is suspended", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
			})

			s.Then("the participant call is still recorded in the event history", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})

				assert.Equal(t, 1, len(participantEventsOf(t, c)),
					assert.MessageF("suspension is an expected outcome, not a failure; "+
						"the participant execution that produced it must stay persisted"))
			})

			s.Then("a replay does not call the participant again", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, 1, participantCalls.Get(t))

				_ = act(t)

				assert.Equal(t, 1, participantCalls.Get(t),
					assert.MessageF("a suspended process is resumed by re-executing it, "+
						"so the already-performed participant call must be replayed, not repeated"))
			})
		})
	})
}

// TestRuntime_Execute_nestedStepSignal pins that a step nested inside another
// step reports its outcome to the runtime unaltered.
//
// A Participant can hand a follow-up Definition back to the runtime, and that
// Definition can itself run further steps — so one participant execution ends up
// containing another. When the inner step signals the runtime (here: suspend),
// the signal has to arrive at workflow.Runtime#execute as the RuntimeSignal it
// is, because the runtime dispatches on that concrete type. Anything the inner
// step does to its own bookkeeping must stay invisible to the outer one.
func TestRuntime_Execute_nestedStepSignal(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	// inner asks the runtime to suspend the process.
	_, innerID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return workflow.Suspend{}
		}
	})

	// outer hands back a follow-up Definition, and that Definition is what runs
	// inner — which is what nests one participant execution inside another.
	_, outerID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return workflow.Sequence{
				workflow.ExecuteParticipant{ID: innerID.Get(t)},
			}
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.ExecuteParticipant{ID: outerID.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("the suspension signal reaches the caller", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Suspend{})
		})

		s.Then("the signal is not accompanied by a context cancellation", func(t *testcase.T) {
			err := act(t)

			assert.False(t, errors.Is(err, context.Canceled),
				assert.MessageF("the inner step must not cancel the context of the step that contains it; got %v", err))
		})

		s.Then("the signal keeps its RuntimeSignal type", func(t *testcase.T) {
			err := act(t)

			_, ok := err.(workflow.RuntimeSignal)
			assert.True(t, ok,
				assert.MessageF("workflow.Runtime#execute dispatches with err.(RuntimeSignal), "+
					"so a signal that gets wrapped into another error is silently dropped; got %#v", err))
		})
	})
}

// TestRuntime_Execute_participantRuntimeSignal pins that a Participant which
// concludes by raising a workflow.RuntimeSignal is not recorded in the event
// history as a performed step.
//
// A RuntimeSignal is dynamic control flow rather than a result. Whether a step
// still wants to suspend the process — or, in the future, halt it — is only
// answerable at the moment it is asked, so it has to be asked again on every
// execution. Recording the call would freeze one execution's answer into the
// event history and replay that answer from then on.
//
// This is what separates a signal from the other "happy error" a Participant may
// return. A follow-up workflow.Definition is a result: the participant finished
// and produced the next stage of the workflow, so that call is recorded and
// replayed (see TestRuntime_Execute_participantFollowUpDefinition).
func TestRuntime_Execute_participantRuntimeSignal(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var participantCalls = let.VarOf(s, 0)

	// signal is what the participant raises in place of returning nil.
	//
	// Replace is the default because the runtime recovers from it and carries on,
	// which is where an unwanted recorded step would be hardest to notice: the
	// replacement definition no longer contains the step, so nothing ever reaches
	// it again to reveal that it had been cached.
	var signal = let.Var(s, func(t *testcase.T) workflow.RuntimeSignal {
		return workflow.Replace{
			Definition: wftest.Stub{
				StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
					return nil
				},
			},
		}
	})

	participant, participantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			participantCalls.Set(t, participantCalls.Get(t)+1)
			return signal.Get(t)
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.ExecuteParticipant{ID: participantID.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("the participant call is left out of the event history", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, 1, participantCalls.Get(t))

			assert.Empty(t, participantEventsOf(t, c),
				assert.MessageF("raising a signal is not a step outcome to replay; "+
					"an EventParticipant here would let a later execution skip the very call "+
					"that decides whether the signal still applies"))
		})

		s.When("the signal is one the runtime propagates instead of acting on", func(s *testcase.Spec) {
			signal.Let(s, func(t *testcase.T) workflow.RuntimeSignal {
				return workflow.Suspend{}
			})

			s.Then("the participant call is still left out of the event history", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})

				assert.Empty(t, participantEventsOf(t, c))
			})

			s.Then("resuming the process asks the participant again", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, 1, participantCalls.Get(t))

				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, 2, participantCalls.Get(t),
					assert.MessageF("a suspended step is resumed by asking it again, "+
						"not by replaying the answer it gave last time"))
			})

			s.And("the participant stops signalling once its wait is over", func(s *testcase.Spec) {
				participant.Let(s, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						participantCalls.Set(t, participantCalls.Get(t)+1)
						if 1 < participantCalls.Get(t) {
							return nil
						}
						return signal.Get(t)
					}
				})

				s.Then("the process moves past the step and completes", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})
					assert.NoError(t, act(t))

					done, err := workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), c.ProcessID.Get(t))
					assert.NoError(t, err)
					assert.True(t, done,
						assert.MessageF("what a participant signals is a live decision; "+
							"once it stops signalling, the process has to be able to move on"))
				})

				s.Then("the call that did not signal is recorded", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})
					assert.NoError(t, act(t))

					assert.Equal(t, 1, len(participantEventsOf(t, c)),
						assert.MessageF("only the call that returned an actual result is a step "+
							"outcome; the signalling one before it is not"))
				})
			})
		})
	})
}

func participantEventsOf(t *testcase.T, c wftest.C) []workflow.EventParticipant {
	t.Helper()
	var out []workflow.EventParticipant
	for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
		if pe, ok := event.(workflow.EventParticipant); ok {
			out = append(out, pe)
		}
	}
	return out
}
