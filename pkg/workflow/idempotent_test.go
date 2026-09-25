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
// life-cycle. The call is persisted with the Definition attached: replay resumes
// that follow-up without calling its producer again.
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
		return workflow.Execute{ParticipantID: participantID.Get(t)}
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

				assert.ErrorIs(t, act(t), workflow.Suspend{})

				assert.Equal(t, 1, participantCalls.Get(t),
					assert.MessageF("a suspended process is resumed by re-executing it, "+
						"so the already-performed participant call must be replayed, not repeated"))
			})
		})

		s.When("the participant returns a sequence waiting before required work", func(s *testcase.Spec) {
			var ready = let.VarOf(s, false)
			var calls = let.VarOf(s, []string(nil))
			_, requiredID := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, calls, "required")
					return nil
				}
			})
			_, afterID := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, calls, "after")
					return nil
				}
			})
			followUp.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.Sleep{Until: wftest.Stub{
						StubEvaluate: func(context.Context, workflow.ProcessID) (bool, error) {
							return ready.Get(t), nil
						},
					}},
					workflow.Execute{ParticipantID: requiredID.Get(t)},
				}
			})
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.Execute{ParticipantID: participantID.Get(t)},
					workflow.Execute{ParticipantID: afterID.Get(t)},
				}
			})

			s.Then("every replay stays suspended without rerunning the producer or starting outer work", func(t *testcase.T) {
				for range 3 {
					assert.ErrorIs(t, act(t), workflow.Suspend{})
					assert.False(t, c.IsCompleted(t, c.ProcessID.Get(t)))
					assert.Empty(t, calls.Get(t))
					assert.Equal(t, 1, participantCalls.Get(t))
					assert.Equal(t, 1, len(participantEventsOf(t, c)))
				}
			})

			s.Then("resuming runs required work before outer work and completes without rerunning the producer", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				ready.Set(t, true)

				assert.NoError(t, act(t))
				assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				assert.Equal(t, []string{"required", "after"}, calls.Get(t))
				assert.Equal(t, 1, participantCalls.Get(t))
				assert.Equal(t, 3, len(participantEventsOf(t, c)))

				assert.NoError(t, act(t))
				assert.Equal(t, []string{"required", "after"}, calls.Get(t))
				assert.Equal(t, 1, participantCalls.Get(t))
			})

			s.And("the producer has an output mapping but returns a follow-up definition", func(s *testcase.Spec) {
				var outputKey = let.As[workflow.VarName](let.UUID(s))
				_, producerID := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) (string, error) {
					return func(context.Context) (string, error) {
						participantCalls.Set(t, participantCalls.Get(t)+1)
						return t.Random.String(), followUp.Get(t)
					}
				})
				c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.Execute{ParticipantID: producerID.Get(t), Output: []workflow.VarName{outputKey.Get(t)}},
						workflow.Execute{ParticipantID: afterID.Get(t)},
					}
				})

				s.Then("keeps the producer cached through suspension and completion without fabricating its mapped output", func(t *testcase.T) {
					assertOutputIgnored := func() {
						t.Helper()
						vars := workflow.Vars{ProcessID: c.ProcessID.Get(t), EventsRepository: c.EventRepository.Get(t)}
						_, found, err := vars.Lookup(t.Context(), outputKey.Get(t))
						assert.NoError(t, err)
						assert.False(t, found, "returning a definition does not assign the producer's output")
						for _, event := range participantEventsOf(t, c) {
							if event.ParticipantID == producerID.Get(t) {
								assert.Nil(t, event.Output)
								assert.NotNil(t, event.Definition)
							}
						}
					}

					for range 3 {
						assert.ErrorIs(t, act(t), workflow.Suspend{})
						assert.False(t, c.IsCompleted(t, c.ProcessID.Get(t)))
						assert.Empty(t, calls.Get(t))
						assert.Equal(t, 1, participantCalls.Get(t))
						assert.Equal(t, 1, len(participantEventsOf(t, c)))
						assertOutputIgnored()
					}

					ready.Set(t, true)
					assert.NoError(t, act(t))
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
					assert.Equal(t, []string{"required", "after"}, calls.Get(t))
					assert.Equal(t, 1, participantCalls.Get(t))
					assert.Equal(t, 3, len(participantEventsOf(t, c)))
					assertOutputIgnored()

					assert.NoError(t, act(t))
					assert.Equal(t, 1, participantCalls.Get(t))
					assert.Equal(t, []string{"required", "after"}, calls.Get(t))
					assertOutputIgnored()
				})
			})

			s.And("the follow-up suspends again after required work", func(s *testcase.Spec) {
				var finish = let.VarOf(s, false)
				followUp.Let(s, func(t *testcase.T) workflow.Definition {
					seq := followUp.Super(t).(workflow.Sequence)
					return append(seq, workflow.Sleep{Until: wftest.Stub{
						StubEvaluate: func(context.Context, workflow.ProcessID) (bool, error) {
							return finish.Get(t), nil
						},
					}})
				})

				s.Then("completed follow-up steps remain cached across further suspensions and eventual completion", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})
					ready.Set(t, true)

					for range 3 {
						assert.ErrorIs(t, act(t), workflow.Suspend{})
						assert.False(t, c.IsCompleted(t, c.ProcessID.Get(t)))
						assert.Equal(t, []string{"required"}, calls.Get(t))
						assert.Equal(t, 1, participantCalls.Get(t))
						assert.Equal(t, 2, len(participantEventsOf(t, c)))
					}

					finish.Set(t, true)
					assert.NoError(t, act(t))
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
					assert.Equal(t, []string{"required", "after"}, calls.Get(t))
					assert.Equal(t, 1, participantCalls.Get(t))
					assert.Equal(t, 3, len(participantEventsOf(t, c)))
				})
			})
		})
	})
}

func TestRuntime_Execute_participantSignatureMismatch(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)
	var (
		parentID       = wftest.LetParticipantID(s)
		completedID    = wftest.LetParticipantID(s)
		requiredID     = wftest.LetParticipantID(s)
		parentCalls    = let.VarOf(s, 0)
		completedCalls = let.VarOf(s, 0)
		requiredCalls  = let.VarOf(s, 0)
		value          = let.String(s)
		followUp       = let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.Execute{ParticipantID: completedID.Get(t), Output: []workflow.VarName{"result"}},
				workflow.Execute{ParticipantID: requiredID.Get(t), Input: []workflow.VarName{"result"}},
			}
		})
	)
	subject := let.Var(s, func(t *testcase.T) workflow.Runtime {
		rt := c.Runtime.Get(t)
		rt.Participants = workflow.Participants{
			parentID.Get(t): func(context.Context) error {
				parentCalls.Set(t, parentCalls.Get(t)+1)
				return followUp.Get(t)
			},
			completedID.Get(t): func(context.Context) (string, error) {
				completedCalls.Set(t, completedCalls.Get(t)+1)
				return value.Get(t), nil
			},
			requiredID.Get(t): func(context.Context, bool) error {
				requiredCalls.Set(t, requiredCalls.Get(t)+1)
				return nil
			},
		}
		return rt
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Execute{ParticipantID: parentID.Get(t)}
	})
	s.Before(func(t *testcase.T) {
		assert.Must(t).NoError(subject.Get(t).Bind(t.Context(), c.ProcessID.Get(t), c.Definition.Get(t)))
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
		}
		assertNoFailures := func(t *testcase.T) {
			t.Helper()
			for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
				_, failed := event.(workflow.EventError)
				assert.False(t, failed, "a signature mismatch is availability, not a failed participant call")
			}
		}

		s.Then("preserves the parent and completed nested stage while the signature remains incompatible", func(t *testcase.T) {
			for range 3 {
				err := act(t)
				assert.ErrorIs(t, err, workflow.ErrParticipantSignatureMismatch{ID: requiredID.Get(t)})
				assert.ErrorIs(t, err, workflow.ErrParticipantFuncMappingMismatch)
				assert.False(t, workflow.ErrIsFatal(err))
				assert.False(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				assert.Equal(t, 1, parentCalls.Get(t))
				assert.Equal(t, 1, completedCalls.Get(t))
				assert.Equal(t, 0, requiredCalls.Get(t))
				assert.Equal(t, 2, len(participantEventsOf(t, c)))
				assertNoFailures(t)
			}
		})

		s.Then("a compatible registry resumes the follow-up without rerunning its producer or completed stage", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrParticipantSignatureMismatch{ID: requiredID.Get(t)})
			rt := subject.Get(t)
			rt.Participants = workflow.Participants{
				requiredID.Get(t): func(_ context.Context, got string) error {
					assert.Equal(t, value.Get(t), got)
					requiredCalls.Set(t, requiredCalls.Get(t)+1)
					return nil
				},
			}
			subject.Set(t, rt)

			assert.NoError(t, act(t))
			assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
			assert.Equal(t, 1, parentCalls.Get(t))
			assert.Equal(t, 1, completedCalls.Get(t))
			assert.Equal(t, 1, requiredCalls.Get(t))
			assert.Equal(t, 3, len(participantEventsOf(t, c)))
			assertNoFailures(t)
		})

		s.When("the incompatible participant is invoked outside a parent transaction", func(s *testcase.Spec) {
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return followUp.Get(t)
			})

			s.Then("does not record an EventError for the availability mismatch", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantSignatureMismatch{ID: requiredID.Get(t)})
				assert.Equal(t, 0, requiredCalls.Get(t))
				assertNoFailures(t)
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
				workflow.Execute{ParticipantID: innerID.Get(t)},
			}
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Execute{ParticipantID: outerID.Get(t)}
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
		return workflow.Execute{ParticipantID: participantID.Get(t)}
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
