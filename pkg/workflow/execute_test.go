package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestExecute(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	var (
		pid = wftest.LetParticipantID(s)
		cid = LetConditionID(s)
	)

	// The participant registered under pid.
	// The context it receives is transaction scoped and finishes together with the execution,
	// so its liveness can only be observed at call time.
	var (
		participantCalls  = let.VarOf(s, 0)
		participantCTXErr = let.VarOf[error](s, nil)
		participantIn     = let.VarOf(s, "")
		participantOut    = let.VarOf(s, "")
	)
	participant := wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(context.Context, string) (string, error) {
		return func(ctx context.Context, in string) (string, error) {
			participantCalls.Set(t, participantCalls.Get(t)+1)
			participantCTXErr.Set(t, ctx.Err())
			participantIn.Set(t, in)
			out := t.Random.UUID()
			participantOut.Set(t, out)
			return out, nil
		}
	})

	// The condition registered under cid.
	var (
		conditionCalls  = let.VarOf(s, 0)
		conditionCTXErr = let.VarOf[error](s, nil)
		conditionIn     = let.VarOf(s, "")
		conditionAnswer = let.Var(s, func(t *testcase.T) bool { return t.Random.Bool() })
	)
	condition := LetCondition(s, c, cid, func(t *testcase.T) func(context.Context, string) (bool, error) {
		return func(ctx context.Context, in string) (bool, error) {
			conditionCalls.Set(t, conditionCalls.Get(t)+1)
			conditionCTXErr.Set(t, ctx.Err())
			conditionIn.Set(t, in)
			return conditionAnswer.Get(t), nil
		}
	})

	var (
		inKey = let.As[workflow.VarName](let.UUID(s))
		inVal = let.UUID(s)
		input = let.Var(s, func(t *testcase.T) []workflow.VarName {
			return []workflow.VarName{inKey.Get(t)}
		})
		outKey = let.As[workflow.VarName](let.UUID(s))
		output = let.VarOf[[]workflow.VarName](s, nil)

		participantID = let.VarOf[workflow.ParticipantID](s, "")
		conditionID   = let.VarOf[workflow.ConditionID](s, "")
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.Execute {
		return &workflow.Execute{
			ParticipantID: participantID.Get(t),
			ConditionID:   conditionID.Get(t),
			Input:         input.Get(t),
			Output:        output.Get(t),
		}
	})

	var (
		ctx       = let.Context(s)
		processID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
			p := c.ProcessID.Super(t)
			setVar(t, c.Runtime.Get(t), p, inKey.Get(t), inVal.Get(t))
			return p
		})
	)
	execCTX := func(t *testcase.T) context.Context {
		return c.Runtime.Get(t).Context(ctx.Get(t))
	}

	// executionEvents are the events an Execute records: calls, answers and failures.
	executionEvents := func(t *testcase.T) []workflow.Event {
		var out []workflow.Event
		for _, event := range c.ProcessEvents(t, processID.Get(t)) {
			switch event.(type) {
			case workflow.EventParticipant, workflow.EventCondition, workflow.EventError:
				out = append(out, event)
			}
		}
		return out
	}
	failuresOf := func(t *testcase.T) []workflow.EventError {
		var out []workflow.EventError
		for _, event := range c.ProcessEvents(t, processID.Get(t)) {
			if ee, ok := event.(workflow.EventError); ok {
				out = append(out, ee)
			}
		}
		return out
	}
	answersOf := func(t *testcase.T) []workflow.EventCondition {
		return conditionEvents(t, c.EventRepository.Get(t), processID.Get(t))
	}
	thenNothingIsExecuted := func(t *testcase.T) {
		t.Helper()
		assert.Equal(t, participantCalls.Get(t), 0, "the registered participant must not be called")
		assert.Equal(t, conditionCalls.Get(t), 0, "the registered condition must not be asked")
		assert.Empty(t, executionEvents(t), "nothing must be recorded")
	}
	thenInvalidDefinition := func(t *testcase.T, err error) {
		t.Helper()
		assert.ErrorIs(t, err, workflow.ErrInvalidDefinition)
		assert.True(t, workflow.ErrIsFatal(err))
		thenNothingIsExecuted(t)
	}

	// IDs selects which of ParticipantID and ConditionID is set.
	type IDs struct{ Participant, Condition bool }
	letIDs := func(s *testcase.Spec, ids IDs) {
		participantID.Let(s, func(t *testcase.T) workflow.ParticipantID {
			if ids.Participant {
				return pid.Get(t)
			}
			return ""
		})
		conditionID.Let(s, func(t *testcase.T) workflow.ConditionID {
			if ids.Condition {
				return cid.Get(t)
			}
			return ""
		})
	}
	// notAStep and notACondition are the ID combinations which don't select the role of the method under test.
	var (
		notAStep = []struct {
			Name string
			IDs  IDs
		}{
			{"the ConditionID is set as well", IDs{Participant: true, Condition: true}},
			{"only the ConditionID is set", IDs{Condition: true}},
			{"neither ID is set", IDs{}},
		}
		notACondition = []struct {
			Name string
			IDs  IDs
		}{
			{"the ParticipantID is set as well", IDs{Participant: true, Condition: true}},
			{"only the ParticipantID is set", IDs{Participant: true}},
			{"neither ID is set", IDs{}},
		}
		// notWithID are the ID combinations which ExecuteWith and EvaluateWith reject,
		// since they identify the execution by their id argument instead.
		notWithID = []struct {
			Name string
			IDs  IDs
		}{
			{"the ParticipantID is set", IDs{Participant: true}},
			{"the ConditionID is set", IDs{Condition: true}},
			{"both IDs are set", IDs{Participant: true, Condition: true}},
		}
	)

	s.Describe("#Execute", func(s *testcase.Spec) {
		letIDs(s, IDs{Participant: true})
		output.Let(s, func(t *testcase.T) []workflow.VarName {
			return []workflow.VarName{outKey.Get(t)}
		})

		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(execCTX(t), processID.Get(t))
		}

		s.Then("the participant registered under the ParticipantID is called with the Input values and a live context", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, participantCalls.Get(t), 1)
			assert.Equal(t, participantIn.Get(t), inVal.Get(t))
			assert.NoError(t, participantCTXErr.Get(t),
				"the participant must be called with a live context")
		})

		s.Then("the participant's results are stored in the Output variables", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)), participantOut.Get(t))
		})

		s.Then("the call is recorded at the participant/<ParticipantID> path, with its input, output and timestamp", func(t *testcase.T) {
			assert.NoError(t, act(t))

			events := participantEventsOf(t, c)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ParticipantID, pid.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{"participant", string(pid.Get(t))})
			assert.Equal(t, events[0].Input, []any{inVal.Get(t)})
			assert.Equal(t, events[0].Output, []any{participantOut.Get(t)})
			assert.False(t, events[0].Timestamp.IsZero())
			assert.Nil(t, events[0].Definition)
		})

		s.When("the participant is not registered on this node", func(s *testcase.Spec) {
			participantID.Let(s, func(t *testcase.T) workflow.ParticipantID {
				return workflow.ParticipantID(random.Unique(t.Random.UUID, string(pid.Get(t))))
			})

			s.Then("it fails with a nonfatal ErrParticipantNotFound, so another node can take over", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, workflow.ErrParticipantNotFound{ID: participantID.Get(t)})
				assert.False(t, workflow.ErrIsFatal(err))
			})

			s.Then("it is neither recorded as a call nor as a failure", func(t *testcase.T) {
				assert.Error(t, act(t))
				assert.Empty(t, executionEvents(t))
			})
		})

		s.When("the participant fails", func(s *testcase.Spec) {
			failure := let.Error(s)
			participant.Let(s, func(t *testcase.T) func(context.Context, string) (string, error) {
				return func(ctx context.Context, in string) (string, error) {
					participantCalls.Set(t, participantCalls.Get(t)+1)
					return "", failure.Get(t)
				}
			})

			s.Then("the error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), failure.Get(t))
			})

			s.Then("the failure is recorded as an EventError at the position of the call", func(t *testcase.T) {
				assert.Error(t, act(t))

				failures := failuresOf(t)
				assert.Must(t).Equal(len(failures), 1)
				assert.Equal(t, failures[0].ParticipantID, pid.Get(t))
				assert.Equal(t, failures[0].Path, workflow.Path{"participant", string(pid.Get(t))})
				assert.Equal(t, failures[0].Error, failure.Get(t).Error())
			})

			s.Then("the failure is not recorded as a call, so the next execution calls the participant again", func(t *testcase.T) {
				assert.Error(t, act(t))
				assert.Error(t, act(t))

				assert.Equal(t, participantCalls.Get(t), 2)
				assert.Empty(t, participantEventsOf(t, c))
			})
		})

		// A participant may set process variables before it fails.
		// Those belong to the attempt, not to the process:
		// the failed attempt records no call, so the retry has to start from the same state.
		s.When("the participant fails after setting process variables", func(s *testcase.Spec) {
			var (
				setKey         = let.As[workflow.VarName](let.UUID(s))
				failure        = let.Var(s, func(t *testcase.T) error { return t.Random.Error() })
				visibleOnEntry = let.Var(s, func(t *testcase.T) []bool { return nil })
			)
			participant.Let(s, func(t *testcase.T) func(context.Context, string) (string, error) {
				return func(ctx context.Context, in string) (string, error) {
					participantCalls.Set(t, participantCalls.Get(t)+1)
					repo, err := workflow.LookupEventsRepository(ctx)
					if err != nil {
						return "", err
					}
					vars := workflow.Vars{ProcessID: processID.Get(t), EventsRepository: repo}
					_, ok, err := vars.Lookup(ctx, setKey.Get(t))
					if err != nil {
						return "", err
					}
					testcase.Append(t, visibleOnEntry, ok)
					if err := vars.Set(ctx, setKey.Get(t), t.Random.UUID()); err != nil {
						return "", err
					}
					return t.Random.UUID(), failure.Get(t)
				}
			})

			s.Then("the variables it set are discarded together with the failed attempt", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), failure.Get(t))

				_, ok, err := getVars(t, c.Runtime.Get(t), processID.Get(t)).Lookup(t.Context(), setKey.Get(t))
				assert.NoError(t, err)
				assert.False(t, ok)
				for _, event := range c.ProcessEvents(t, processID.Get(t)) {
					if ve, ok := event.(workflow.EventSetVar); ok {
						assert.NotEqual(t, ve.Name, setKey.Get(t),
							"an EventSetVar of the failed attempt is still in the history")
					}
				}
			})

			s.Then("the retry starts from the same state as the failed attempt, and a successful attempt keeps its variables", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), failure.Get(t))
				failure.Set(t, nil)
				assert.NoError(t, act(t))

				assert.Equal(t, visibleOnEntry.Get(t), []bool{false, false})
				_, ok, err := getVars(t, c.Runtime.Get(t), processID.Get(t)).Lookup(t.Context(), setKey.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok)
			})
		})

		// The routing participant pattern: returning a Definition extends the
		// step with a recorded follow-up, instead of editing the bound definition.
		s.When("the participant returns a workflow.Definition as its error", func(s *testcase.Spec) {
			var (
				followUpCalls = let.VarOf(s, 0)
				followUpErr   = let.VarOf[error](s, nil)
				followUp      = let.Var(s, func(t *testcase.T) workflow.Definition {
					return wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							followUpCalls.Set(t, followUpCalls.Get(t)+1)
							return followUpErr.Get(t)
						},
					}
				})
			)
			participant.Let(s, func(t *testcase.T) func(context.Context, string) (string, error) {
				return func(ctx context.Context, in string) (string, error) {
					participantCalls.Set(t, participantCalls.Get(t)+1)
					return "", followUp.Get(t)
				}
			})

			s.Then("the definition is recorded with the call, and runs in place of the step", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := participantEventsOf(t, c)
				assert.Must(t).Equal(len(events), 1)
				assert.Equal(t, events[0].Definition, followUp.Get(t))
				assert.Equal(t, followUpCalls.Get(t), 1)
			})

			s.Then("a replay walks the recorded definition again without calling the participant", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				assert.Equal(t, participantCalls.Get(t), 1)
				assert.Equal(t, followUpCalls.Get(t), 2,
					"the recorded follow-up must be walked again to resume unfinished work")
			})

			s.And("the definition yields with a runtime signal", func(s *testcase.Spec) {
				followUpErr.LetValue(s, workflow.Suspend{})

				s.Then("the signal is propagated, and the call stays recorded with its definition", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})

					events := participantEventsOf(t, c)
					assert.Must(t).Equal(len(events), 1)
					assert.NotNil(t, events[0].Definition)
				})
			})
		})

		s.When("the participant was already executed at the same position", func(s *testcase.Spec) {
			firstOut := let.VarOf(s, "")

			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(act(t))
				assert.Must(t).Equal(participantCalls.Get(t), 1)
				firstOut.Set(t, participantOut.Get(t))
			})

			s.Then("the recorded call is replayed without calling the participant again, even though it answers differently each time", func(t *testcase.T) {
				t.Random.Repeat(2, 5, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, participantCalls.Get(t), 1)
				assert.Equal(t, len(participantEventsOf(t, c)), 1)
				assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)), firstOut.Get(t))
			})

			s.And("an Input variable changed since", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), t.Random.UUID())
				})

				s.Then("the call is still replayed, because at the position of the recorded call the input is unchanged", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, participantCalls.Get(t), 1)
				})
			})

			s.And("the recorded input was tampered with in the event history", func(s *testcase.Spec) {
				newIn := let.UUID(s)

				s.Before(func(t *testcase.T) {
					setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), newIn.Get(t))
					// history rewrite: retroactively change the value originally recorded for the input variable.
					for _, event := range c.ProcessEvents(t, processID.Get(t)) {
						ve, ok := event.(workflow.EventSetVar)
						if !ok || ve.Name != inKey.Get(t) {
							continue
						}
						ve.Value = newIn.Get(t)
						var e workflow.Event = ve
						assert.Must(t).NoError(c.EventRepository.Get(t).Update(t.Context(), &e))
						break
					}
				})

				s.Then("the call is repeated with the current input", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, participantCalls.Get(t), 2)
					assert.Equal(t, participantIn.Get(t), newIn.Get(t))
					assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)), participantOut.Get(t))
				})
			})

			s.And("the next execution happens at a different path", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					ctx.Set(t, workflow.WithName(ctx.Get(t), t.Random.UUID()))
				})

				s.Then("it is executed independently", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, participantCalls.Get(t), 2)
				})
			})
		})

		s.When("an Input variable's type is convertible to the participant's parameter type", func(s *testcase.Spec) {
			type featureFlag bool
			var (
				flagKey = let.As[workflow.VarName](let.UUID(s))
				flagVal = let.Var(s, func(t *testcase.T) bool { return t.Random.Bool() })
				got     = let.VarOf[[]any](s, nil)
			)
			input.Let(s, func(t *testcase.T) []workflow.VarName {
				return []workflow.VarName{inKey.Get(t), flagKey.Get(t)}
			})
			s.Before(func(t *testcase.T) {
				setVar(t, c.Runtime.Get(t), processID.Get(t), flagKey.Get(t), flagVal.Get(t))
			})
			wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(context.Context, workflow.ConditionID, featureFlag) (string, error) {
				return func(ctx context.Context, id workflow.ConditionID, flag featureFlag) (string, error) {
					got.Set(t, []any{id, flag})
					return t.Random.UUID(), nil
				}
			})

			s.Then("the value is converted to the declared parameter type", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, got.Get(t), []any{workflow.ConditionID(inVal.Get(t)), featureFlag(flagVal.Get(t))})
			})
		})

		for _, tc := range notAStep {
			s.When(tc.Name, func(s *testcase.Spec) {
				letIDs(s, tc.IDs)

				s.Then("it fails with a fatal ErrInvalidDefinition, before anything is executed or recorded", func(t *testcase.T) {
					thenInvalidDefinition(t, act(t))
				})
			})
		}
	})

	// ExecuteWith identifies the call by its id argument, so neither ID is set.
	s.Describe("#ExecuteWith", func(s *testcase.Spec) {
		var (
			// id is the ID of the participant registered under pid,
			// so the specs can tell that the registered participant is not called.
			id         = let.Var(s, func(t *testcase.T) workflow.ParticipantID { return pid.Get(t) })
			executions = let.VarOf(s, 0)
			executeErr = let.VarOf[error](s, nil)
			gotCTXErr  = let.VarOf[error](s, nil)
			gotPID     = let.Var(s, func(t *testcase.T) workflow.ProcessID { return workflow.ProcessID{} })

			executeFn = let.Var(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) error {
				return func(ctx context.Context, pid workflow.ProcessID) error {
					executions.Set(t, executions.Get(t)+1)
					gotCTXErr.Set(t, ctx.Err())
					gotPID.Set(t, pid)
					return executeErr.Get(t)
				}
			})
		)

		act := func(t *testcase.T) error {
			return subject.Get(t).ExecuteWith(execCTX(t), processID.Get(t), id.Get(t), executeFn.Get(t))
		}

		s.Then("the execute function runs with the process ID and a live context, and a participant registered under the same ID is not called", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, executions.Get(t), 1)
			assert.Equal(t, participantCalls.Get(t), 0,
				"the participant registered under the ID must not be called")
			assert.Equal(t, gotPID.Get(t), processID.Get(t))
			assert.NoError(t, gotCTXErr.Get(t),
				"the execute function must be called with a live context")
		})

		s.Then("the call is recorded as a participant call with the ID as its ParticipantID, together with the input", func(t *testcase.T) {
			assert.NoError(t, act(t))

			events := participantEventsOf(t, c)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ParticipantID, id.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{"participant", string(id.Get(t))})
			assert.Equal(t, events[0].Input, []any{inVal.Get(t)})
			assert.Empty(t, events[0].Output)
			assert.Nil(t, events[0].Definition)
		})

		s.When("no participant is registered under the ID", func(s *testcase.Spec) {
			id.Let(s, func(t *testcase.T) workflow.ParticipantID {
				return workflow.ParticipantID(random.Unique(t.Random.UUID, string(pid.Get(t))))
			})

			s.Then("it runs without requiring a participant registration", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, executions.Get(t), 1)
				assert.Equal(t, len(participantEventsOf(t, c)), 1)
			})
		})

		s.When("the execute function sets process variables", func(s *testcase.Spec) {
			produced := let.UUID(s)
			executeFn.Let(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) error {
				return func(ctx context.Context, pid workflow.ProcessID) error {
					executions.Set(t, executions.Get(t)+1)
					repo, err := workflow.LookupEventsRepository(ctx)
					if err != nil {
						return err
					}
					vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}
					if err := vars.Set(ctx, outKey.Get(t), produced.Get(t)); err != nil {
						return err
					}
					return executeErr.Get(t)
				}
			})

			s.Then("the variables become part of the process, in place of an Output mapping", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)), produced.Get(t))
			})

			s.And("then it fails", func(s *testcase.Spec) {
				executeErr.Let(s, func(t *testcase.T) error { return t.Random.Error() })

				s.Then("the variables it set are discarded together with the failed attempt", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), executeErr.Get(t))

					_, ok, err := getVars(t, c.Runtime.Get(t), processID.Get(t)).Lookup(t.Context(), outKey.Get(t))
					assert.NoError(t, err)
					assert.False(t, ok)
				})
			})
		})

		s.When("the step was already executed at the same position", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(act(t))
				assert.Must(t).Equal(executions.Get(t), 1)
			})

			s.Then("the recorded call is replayed without running the execute function again", func(t *testcase.T) {
				t.Random.Repeat(2, 5, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, executions.Get(t), 1)
				assert.Equal(t, len(participantEventsOf(t, c)), 1)
			})

			s.And("the next execution happens at a different path", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					ctx.Set(t, workflow.WithName(ctx.Get(t), t.Random.UUID()))
				})

				s.Then("it is executed independently", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executions.Get(t), 2)
				})
			})

			s.And("the next execution uses a different ID", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					id.Set(t, workflow.ParticipantID(random.Unique(t.Random.UUID, string(id.Get(t)))))
				})

				s.Then("it is executed independently", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executions.Get(t), 2)
				})
			})
		})

		// Moving a participant's logic into a custom definition, under the participant's ID,
		// must not repeat the calls the registered participant already made.
		s.When("the step was already executed at the same position through the participant registered under the ID", func(s *testcase.Spec) {
			registeredCalls := let.VarOf(s, 0)
			wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(context.Context, string) error {
				return func(ctx context.Context, in string) error {
					registeredCalls.Set(t, registeredCalls.Get(t)+1)
					return nil
				}
			})
			// recorded is the step as it was executed through the registered participant.
			recorded := let.Var(s, func(t *testcase.T) workflow.Execute {
				r := *subject.Get(t)
				r.ParticipantID = id.Get(t)
				return r
			})

			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(recorded.Get(t).Execute(execCTX(t), processID.Get(t)))
				assert.Must(t).Equal(registeredCalls.Get(t), 1)
			})

			s.Then("the recorded call is replayed without running the execute function", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, executions.Get(t), 0)
				assert.Equal(t, registeredCalls.Get(t), 1)
				assert.Equal(t, len(participantEventsOf(t, c)), 1)
			})

			// Output is part of a recorded call's cache identity,
			// and an execute function maps nothing onto Output.
			s.And("the registered participant's results were mapped onto Output", func(s *testcase.Spec) {
				wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(context.Context, string) (string, error) {
					return func(ctx context.Context, in string) (string, error) {
						registeredCalls.Set(t, registeredCalls.Get(t)+1)
						return t.Random.UUID(), nil
					}
				})
				recorded.Let(s, func(t *testcase.T) workflow.Execute {
					r := recorded.Super(t)
					r.Output = []workflow.VarName{outKey.Get(t)}
					return r
				})

				s.Then("the recorded call is not replayed, and the execute function runs", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executions.Get(t), 1)
					assert.Equal(t, registeredCalls.Get(t), 1)
					assert.Equal(t, len(participantEventsOf(t, c)), 2)
				})
			})
		})

		s.When("the execute function fails", func(s *testcase.Spec) {
			executeErr.Let(s, func(t *testcase.T) error { return t.Random.Error() })

			s.Then("the error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), executeErr.Get(t))
			})

			s.Then("the failure is recorded as an EventError under the ID, the same way as a registered participant's", func(t *testcase.T) {
				assert.Error(t, act(t))

				failures := failuresOf(t)
				assert.Must(t).Equal(len(failures), 1)
				assert.Equal(t, failures[0].ParticipantID, id.Get(t))
				assert.Equal(t, failures[0].Path, workflow.Path{"participant", string(id.Get(t))})
				assert.Equal(t, failures[0].Error, executeErr.Get(t).Error())
			})

			s.Then("the failure is not recorded as a call, so the next execution runs it again", func(t *testcase.T) {
				assert.Error(t, act(t))
				assert.Empty(t, participantEventsOf(t, c))

				executeErr.Set(t, nil)
				assert.NoError(t, act(t))
				assert.Equal(t, executions.Get(t), 2)
				assert.Equal(t, len(participantEventsOf(t, c)), 1)
			})
		})

		s.When("the execute function yields with a runtime signal", func(s *testcase.Spec) {
			executeErr.LetValue(s, workflow.Suspend{})

			s.Then("the signal is propagated, and no call is recorded, so the next execution runs it again", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Empty(t, participantEventsOf(t, c))

				executeErr.Set(t, nil)
				assert.NoError(t, act(t))
				assert.Equal(t, executions.Get(t), 2)
			})
		})

		s.When("the execute function returns a workflow.Definition as its error", func(s *testcase.Spec) {
			var (
				followUpCalls = let.VarOf(s, 0)
				followUp      = let.Var(s, func(t *testcase.T) workflow.Definition {
					return wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							followUpCalls.Set(t, followUpCalls.Get(t)+1)
							return nil
						},
					}
				})
			)
			executeErr.Let(s, func(t *testcase.T) error { return followUp.Get(t) })

			s.Then("the definition is recorded with the call and runs in place of the step", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := participantEventsOf(t, c)
				assert.Must(t).Equal(len(events), 1)
				assert.Equal(t, events[0].Definition, followUp.Get(t))
				assert.Equal(t, followUpCalls.Get(t), 1)
			})

			s.Then("a replay walks the recorded definition again without running the execute function", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				assert.Equal(t, executions.Get(t), 1)
				assert.Equal(t, followUpCalls.Get(t), 2)
			})
		})

		s.When("an Input variable is missing from the process", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []workflow.VarName {
				return append(input.Super(t), workflow.VarName(t.Random.UUID()))
			})

			s.Then("it fails with a fatal error without running the execute function", func(t *testcase.T) {
				assert.True(t, workflow.ErrIsFatal(act(t)))

				assert.Equal(t, executions.Get(t), 0)
				assert.Empty(t, participantEventsOf(t, c))
			})
		})

		s.When("Output variables are mapped", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []workflow.VarName {
				return []workflow.VarName{outKey.Get(t)}
			})

			s.Then("it fails with a fatal ErrInvalidDefinition without running the execute function, since it returns no values to map", func(t *testcase.T) {
				thenInvalidDefinition(t, act(t))

				assert.Equal(t, executions.Get(t), 0)
			})
		})

		s.When("the execute function is nil", func(s *testcase.Spec) {
			executeFn.Let(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) error {
				return nil
			})

			s.Then("it fails with a fatal error without recording a call", func(t *testcase.T) {
				assert.True(t, workflow.ErrIsFatal(act(t)))

				thenNothingIsExecuted(t)
			})
		})

		s.When("the ID is empty", func(s *testcase.Spec) {
			id.LetValue(s, "")

			s.Then("it fails with a fatal error, before anything is executed or recorded", func(t *testcase.T) {
				assert.True(t, workflow.ErrIsFatal(act(t)))

				assert.Equal(t, executions.Get(t), 0)
				thenNothingIsExecuted(t)
			})
		})

		for _, tc := range notWithID {
			s.When(tc.Name, func(s *testcase.Spec) {
				letIDs(s, tc.IDs)

				s.Then("it fails with a fatal ErrInvalidDefinition, since the id argument identifies the call, before anything is executed or recorded", func(t *testcase.T) {
					thenInvalidDefinition(t, act(t))

					assert.Equal(t, executions.Get(t), 0)
				})
			})
		}
	})

	s.Describe("#Evaluate", func(s *testcase.Spec) {
		letIDs(s, IDs{Condition: true})

		act := func(t *testcase.T) (bool, error) {
			return subject.Get(t).Evaluate(execCTX(t), processID.Get(t))
		}

		s.Then("the condition registered under the ConditionID is asked with the Input values and a live context", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, conditionAnswer.Get(t))

			assert.Equal(t, conditionCalls.Get(t), 1)
			assert.Equal(t, conditionIn.Get(t), inVal.Get(t))
			assert.NoError(t, conditionCTXErr.Get(t),
				"the condition must be called with a live context")
		})

		s.Then("the answer is recorded at the <ConditionID> path, with its input and timestamp", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			events := answersOf(t)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ConditionID, cid.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{string(cid.Get(t))})
			assert.Equal(t, events[0].Input, []any{inVal.Get(t)})
			assert.Equal(t, events[0].Answer, got)
			assert.False(t, events[0].Timestamp.IsZero())
		})

		s.When("the condition is not registered", func(s *testcase.Spec) {
			conditionID.Let(s, func(t *testcase.T) workflow.ConditionID {
				return workflow.ConditionID(random.Unique(t.Random.UUID, string(cid.Get(t))))
			})

			s.Then("it fails with a fatal ErrConditionNotFound", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, workflow.ErrConditionNotFound{ID: conditionID.Get(t)})
				assert.True(t, workflow.ErrIsFatal(err))
			})
		})

		s.When("the condition fails", func(s *testcase.Spec) {
			failure := let.Var(s, func(t *testcase.T) error { return t.Random.Error() })
			condition.Let(s, func(t *testcase.T) func(context.Context, string) (bool, error) {
				return func(ctx context.Context, in string) (bool, error) {
					conditionCalls.Set(t, conditionCalls.Get(t)+1)
					return conditionAnswer.Get(t), failure.Get(t)
				}
			})

			s.Then("the error is propagated back", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, failure.Get(t))
			})

			s.Then("the failure is not recorded as an answer, so the next evaluation asks again", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)
				assert.Empty(t, answersOf(t))

				failure.Set(t, nil)
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, conditionAnswer.Get(t))
				assert.Equal(t, conditionCalls.Get(t), 2)
			})
		})

		s.When("the condition returns a workflow.Definition as its error", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) func(context.Context, string) (bool, error) {
				return func(ctx context.Context, in string) (bool, error) {
					return false, workflow.Break{}
				}
			})

			s.Then("it is rejected as a fatal error without recording an answer, since a condition can only answer with a bool", func(t *testcase.T) {
				_, err := act(t)
				assert.True(t, workflow.ErrIsFatal(err))
				assert.Empty(t, answersOf(t))
			})
		})

		s.When("the condition was already asked at the same position", func(s *testcase.Spec) {
			firstAnswer := let.VarOf(s, false)

			s.Before(func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).NoError(err)
				firstAnswer.Set(t, got)
				// the live answer changes, so a replay can be told apart from a re-evaluation
				conditionAnswer.Set(t, !got)
			})

			s.Then("the recorded answer is replayed without asking the condition again", func(t *testcase.T) {
				t.Random.Repeat(2, 5, func() {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, firstAnswer.Get(t))
				})

				assert.Equal(t, conditionCalls.Get(t), 1)
				assert.Equal(t, len(answersOf(t)), 1)
			})

			s.And("an Input variable changed since", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), t.Random.UUID())
				})

				s.Then("the answer is still replayed, because at the position of the recorded answer the input is unchanged", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, firstAnswer.Get(t))
					assert.Equal(t, conditionCalls.Get(t), 1)
				})
			})

			s.And("the recorded input was tampered with in the event history", func(s *testcase.Spec) {
				newIn := let.UUID(s)

				s.Before(func(t *testcase.T) {
					setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), newIn.Get(t))
					// history rewrite: retroactively change the value originally recorded for the input variable.
					for _, event := range c.ProcessEvents(t, processID.Get(t)) {
						ve, ok := event.(workflow.EventSetVar)
						if !ok || ve.Name != inKey.Get(t) {
							continue
						}
						ve.Value = newIn.Get(t)
						var e workflow.Event = ve
						assert.Must(t).NoError(c.EventRepository.Get(t).Update(t.Context(), &e))
						break
					}
				})

				s.Then("the condition is asked again with the current input", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, !firstAnswer.Get(t))
					assert.Equal(t, conditionCalls.Get(t), 2)
					assert.Equal(t, conditionIn.Get(t), newIn.Get(t))
				})
			})

			s.And("the next evaluation happens at a different path", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					ctx.Set(t, workflow.WithName(ctx.Get(t), t.Random.UUID()))
				})

				s.Then("it is answered independently", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, !firstAnswer.Get(t))
					assert.Equal(t, conditionCalls.Get(t), 2)
				})
			})
		})

		s.When("an Input variable's type is convertible to the condition's parameter type", func(s *testcase.Spec) {
			type featureFlag bool
			var (
				flagKey = let.As[workflow.VarName](let.UUID(s))
				flagVal = let.Var(s, func(t *testcase.T) bool { return t.Random.Bool() })
				got     = let.VarOf[[]any](s, nil)
			)
			input.Let(s, func(t *testcase.T) []workflow.VarName {
				return []workflow.VarName{inKey.Get(t), flagKey.Get(t)}
			})
			s.Before(func(t *testcase.T) {
				setVar(t, c.Runtime.Get(t), processID.Get(t), flagKey.Get(t), flagVal.Get(t))
			})
			LetCondition(s, c, cid, func(t *testcase.T) func(context.Context, workflow.ParticipantID, featureFlag) (bool, error) {
				return func(ctx context.Context, id workflow.ParticipantID, flag featureFlag) (bool, error) {
					got.Set(t, []any{id, flag})
					return t.Random.Bool(), nil
				}
			})

			s.Then("the value is converted to the declared parameter type", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)

				assert.Equal(t, got.Get(t), []any{workflow.ParticipantID(inVal.Get(t)), featureFlag(flagVal.Get(t))})
			})
		})

		s.When("Output variables are mapped", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []workflow.VarName {
				return []workflow.VarName{outKey.Get(t)}
			})

			s.Then("it fails with a fatal ErrInvalidDefinition, since an answer is not stored in variables", func(t *testcase.T) {
				_, err := act(t)
				thenInvalidDefinition(t, err)
			})
		})

		for _, tc := range notACondition {
			s.When(tc.Name, func(s *testcase.Spec) {
				letIDs(s, tc.IDs)

				s.Then("it fails with a fatal ErrInvalidDefinition, before anything is asked or recorded", func(t *testcase.T) {
					got, err := act(t)
					assert.False(t, got)
					thenInvalidDefinition(t, err)
				})
			})
		}
	})

	// EvaluateWith identifies the answer by its id argument, so neither ID is set.
	s.Describe("#EvaluateWith", func(s *testcase.Spec) {
		var (
			// id is the ID of the condition registered under cid,
			// so the specs can tell that the registered condition is not asked.
			id          = let.Var(s, func(t *testcase.T) workflow.ConditionID { return cid.Get(t) })
			answer      = let.Var(s, func(t *testcase.T) bool { return t.Random.Bool() })
			answerErr   = let.VarOf[error](s, nil)
			evaluations = let.VarOf(s, 0)
			gotCTXErr   = let.VarOf[error](s, nil)
			gotPID      = let.Var(s, func(t *testcase.T) workflow.ProcessID { return workflow.ProcessID{} })

			evaluateFn = let.Var(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) (bool, error) {
				return func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					evaluations.Set(t, evaluations.Get(t)+1)
					gotCTXErr.Set(t, ctx.Err())
					gotPID.Set(t, pid)
					return answer.Get(t), answerErr.Get(t)
				}
			})
		)

		act := func(t *testcase.T) (bool, error) {
			return subject.Get(t).EvaluateWith(execCTX(t), processID.Get(t), id.Get(t), evaluateFn.Get(t))
		}

		s.Then("the evaluate function answers with the process ID and a live context, and a condition registered under the same ID is not asked", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, answer.Get(t))

			assert.Equal(t, evaluations.Get(t), 1)
			assert.Equal(t, conditionCalls.Get(t), 0,
				"the condition registered under the ID must not be asked")
			assert.Equal(t, gotPID.Get(t), processID.Get(t))
			assert.NoError(t, gotCTXErr.Get(t),
				"the evaluate function must be called with a live context")
		})

		s.Then("the answer is recorded as a condition answer with the ID as its ConditionID, together with the input", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			events := answersOf(t)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ConditionID, id.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{string(id.Get(t))})
			assert.Equal(t, events[0].Input, []any{inVal.Get(t)})
			assert.Equal(t, events[0].Answer, got)
		})

		s.When("no condition is registered under the ID", func(s *testcase.Spec) {
			id.Let(s, func(t *testcase.T) workflow.ConditionID {
				return workflow.ConditionID(random.Unique(t.Random.UUID, string(cid.Get(t))))
			})

			s.Then("it answers without requiring a condition registration", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, answer.Get(t))

				assert.Equal(t, evaluations.Get(t), 1)
				assert.Equal(t, len(answersOf(t)), 1)
			})
		})

		s.When("the evaluate function reads process variables", func(s *testcase.Spec) {
			evaluateFn.Let(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) (bool, error) {
				return func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					repo, err := workflow.LookupEventsRepository(ctx)
					if err != nil {
						return false, err
					}
					vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}
					val, err := vars.Get(ctx, inKey.Get(t))
					if err != nil {
						return false, err
					}
					return val == any(inVal.Get(t)), nil
				}
			})

			s.Then("the process variables are reachable through the context it receives", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.True(t, got)
			})
		})

		s.When("the condition was already answered at the same position", func(s *testcase.Spec) {
			firstAnswer := let.VarOf(s, false)

			s.Before(func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).NoError(err)
				firstAnswer.Set(t, got)
				// the live answer changes, so a replay can be told apart from a re-evaluation
				answer.Set(t, !got)
			})

			s.Then("the recorded answer is replayed without calling the evaluate function again", func(t *testcase.T) {
				t.Random.Repeat(2, 5, func() {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, firstAnswer.Get(t))
				})

				assert.Equal(t, evaluations.Get(t), 1)
				assert.Equal(t, len(answersOf(t)), 1)
			})

			s.And("the next evaluation happens at a different path", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					ctx.Set(t, workflow.WithName(ctx.Get(t), t.Random.UUID()))
				})

				s.Then("it is answered independently", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, !firstAnswer.Get(t))
					assert.Equal(t, evaluations.Get(t), 2)
				})
			})

			s.And("the next evaluation uses a different ID", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					id.Set(t, workflow.ConditionID(random.Unique(t.Random.UUID, string(id.Get(t)))))
				})

				s.Then("it is answered independently", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, got, !firstAnswer.Get(t))
					assert.Equal(t, evaluations.Get(t), 2)
				})
			})
		})

		// Moving a condition's logic into a custom Condition, under the condition's ID,
		// must keep the answers the registered condition already gave.
		s.When("the condition was already answered at the same position through the condition registered under the ID", func(s *testcase.Spec) {
			firstAnswer := let.VarOf(s, false)
			// recorded is the condition as it was evaluated through the registered condition.
			recorded := let.Var(s, func(t *testcase.T) workflow.Execute {
				r := *subject.Get(t)
				r.ConditionID = id.Get(t)
				return r
			})

			s.Before(func(t *testcase.T) {
				got, err := recorded.Get(t).Evaluate(execCTX(t), processID.Get(t))
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(conditionCalls.Get(t), 1)
				firstAnswer.Set(t, got)
				answer.Set(t, !got)
			})

			s.Then("the recorded answer is replayed without calling the evaluate function", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, firstAnswer.Get(t))

				assert.Equal(t, evaluations.Get(t), 0)
				assert.Equal(t, len(answersOf(t)), 1)
			})
		})

		s.When("the evaluate function fails", func(s *testcase.Spec) {
			answerErr.Let(s, func(t *testcase.T) error { return t.Random.Error() })

			s.Then("the error is propagated back", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, answerErr.Get(t))
			})

			s.Then("the failure is not recorded as an answer, so the next evaluation asks again", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)
				assert.Empty(t, answersOf(t))

				answerErr.Set(t, nil)
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, answer.Get(t))
				assert.Equal(t, evaluations.Get(t), 2)
			})
		})

		s.When("the evaluate function declines to answer with a runtime signal", func(s *testcase.Spec) {
			answerErr.LetValue(s, workflow.Suspend{})

			s.Then("the signal is propagated, and no answer is recorded, so the next evaluation asks again", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, workflow.Suspend{})
				assert.Empty(t, answersOf(t))

				answerErr.Set(t, nil)
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, answer.Get(t))
				assert.Equal(t, evaluations.Get(t), 2)
			})
		})

		s.When("the evaluate function returns a workflow.Definition as its error", func(s *testcase.Spec) {
			// a Definition error is a happy follow-up for a step,
			// but a condition only answers with a bool.
			answerErr.LetValue(s, workflow.Break{})

			s.Then("it is rejected as a fatal error without recording an answer", func(t *testcase.T) {
				_, err := act(t)
				assert.True(t, workflow.ErrIsFatal(err))
				assert.Empty(t, answersOf(t))
			})
		})

		s.When("evaluated inside a caller transaction", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				repo := c.EventRepository.Get(t)
				tx, err := repo.BeginTx(ctx.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = repo.RollbackTx(tx) })
				ctx.Set(t, tx)
			})

			s.Then("the answer is part of the caller's transaction and discarded with its rollback", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)
				assert.Empty(t, answersOf(t), "the answer must not be visible outside of the transaction")

				assert.NoError(t, c.EventRepository.Get(t).RollbackTx(ctx.Get(t)))
				ctx.Set(t, t.Context())

				_, err = act(t)
				assert.NoError(t, err)
				assert.Equal(t, evaluations.Get(t), 2)
				assert.Equal(t, len(answersOf(t)), 1)
			})
		})

		s.When("an Input variable is missing from the process", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []workflow.VarName {
				return append(input.Super(t), workflow.VarName(t.Random.UUID()))
			})

			s.Then("it fails with a fatal error without calling the evaluate function", func(t *testcase.T) {
				_, err := act(t)
				assert.True(t, workflow.ErrIsFatal(err))

				assert.Equal(t, evaluations.Get(t), 0)
				assert.Empty(t, answersOf(t))
			})
		})

		s.When("Output variables are mapped", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []workflow.VarName {
				return []workflow.VarName{outKey.Get(t)}
			})

			s.Then("it fails with a fatal ErrInvalidDefinition without calling the evaluate function, since an answer is not stored in variables", func(t *testcase.T) {
				_, err := act(t)
				thenInvalidDefinition(t, err)

				assert.Equal(t, evaluations.Get(t), 0)
			})
		})

		s.When("the evaluate function is nil", func(s *testcase.Spec) {
			evaluateFn.Let(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) (bool, error) {
				return nil
			})

			s.Then("it fails with a fatal error without recording an answer", func(t *testcase.T) {
				_, err := act(t)
				assert.True(t, workflow.ErrIsFatal(err))

				thenNothingIsExecuted(t)
			})
		})

		s.When("the ID is empty", func(s *testcase.Spec) {
			id.LetValue(s, "")

			s.Then("it fails with a fatal error, before anything is asked or recorded", func(t *testcase.T) {
				got, err := act(t)
				assert.False(t, got)
				assert.True(t, workflow.ErrIsFatal(err))

				assert.Equal(t, evaluations.Get(t), 0)
				thenNothingIsExecuted(t)
			})
		})

		for _, tc := range notWithID {
			s.When(tc.Name, func(s *testcase.Spec) {
				letIDs(s, tc.IDs)

				s.Then("it fails with a fatal ErrInvalidDefinition, since the id argument identifies the answer, before anything is asked or recorded", func(t *testcase.T) {
					got, err := act(t)
					assert.False(t, got)
					thenInvalidDefinition(t, err)

					assert.Equal(t, evaluations.Get(t), 0)
				})
			})
		}
	})

	// Steps are told apart by their position in the definition,
	// so the same participant or condition can be referenced any number of times.
	s.Describe("as steps of a definition", func(s *testcase.Spec) {
		var (
			out2Key    = let.As[workflow.VarName](let.UUID(s))
			flakyID    = wftest.LetParticipantID(s)
			flakyErr   = let.VarOf[error](s, nil)
			flakyCalls = let.VarOf(s, 0)
		)
		wftest.LetParticipantWithID(s, flakyID, func(t *testcase.T) func(context.Context) error {
			return func(context.Context) error {
				flakyCalls.Set(t, flakyCalls.Get(t)+1)
				return flakyErr.Get(t)
			}
		})
		definition := let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.Execute{ParticipantID: pid.Get(t), Input: input.Get(t), Output: []workflow.VarName{outKey.Get(t)}},
				workflow.Execute{ParticipantID: pid.Get(t), Input: []workflow.VarName{outKey.Get(t)}, Output: []workflow.VarName{out2Key.Get(t)}},
				workflow.If{Cond: workflow.Execute{ConditionID: cid.Get(t), Input: []workflow.VarName{out2Key.Get(t)}}},
				workflow.Execute{ParticipantID: flakyID.Get(t)},
			}
		})

		act := func(t *testcase.T) error {
			return definition.Get(t).Execute(execCTX(t), processID.Get(t))
		}

		s.Then("each step executes once, and a step's Output can be the Input of a later step", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, participantCalls.Get(t), 2)
			assert.Equal(t, conditionCalls.Get(t), 1)
			assert.Equal(t, flakyCalls.Get(t), 1)
			assert.Equal[any](t, participantIn.Get(t), getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)))
			assert.Equal[any](t, conditionIn.Get(t), getVar(t, c.Runtime.Get(t), processID.Get(t), out2Key.Get(t)))
		})

		s.Then("executing it again replays every step without calling them or changing the history", func(t *testcase.T) {
			assert.NoError(t, act(t))
			history := c.ProcessEvents(t, processID.Get(t))

			t.Random.Repeat(2, 5, func() {
				assert.NoError(t, act(t))
			})

			assert.Equal(t, participantCalls.Get(t), 2)
			assert.Equal(t, conditionCalls.Get(t), 1)
			assert.Equal(t, flakyCalls.Get(t), 1)
			assert.Equal(t, c.ProcessEvents(t, processID.Get(t)), history)
		})

		s.When("a step fails", func(s *testcase.Spec) {
			flakyErr.Let(s, func(t *testcase.T) error { return t.Random.Error() })

			s.Then("executing it again continues from the failed step", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), flakyErr.Get(t))
				flakyErr.Set(t, nil)
				assert.NoError(t, act(t))

				assert.Equal(t, participantCalls.Get(t), 2)
				assert.Equal(t, conditionCalls.Get(t), 1)
				assert.Equal(t, flakyCalls.Get(t), 2)
			})
		})
	})
}

// TestExecute_reflection pins how the Input values are passed to a registered function,
// and how its signature is checked against the Input and Output mapping.
func TestExecute_reflection(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		processID     = wftest.LetProcessID(s)
		participantID = let.VarOf[workflow.ParticipantID](s, "")
		conditionID   = let.VarOf[workflow.ConditionID](s, "")
		// fn is the function registered under the ID.
		fn = let.Var[any](s, nil)
		// input holds the values of the Input variables.
		input  = let.VarOf[[]any](s, nil)
		output = let.VarOf[[]workflow.VarName](s, nil)
		calls  = let.VarOf(s, 0)
	)
	runtime := let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Events:       &memory.WorkflowEventRepository{},
			Participants: workflow.Participants{participantID.Get(t): fn.Get(t)},
			Conditions:   workflow.Conditions{conditionID.Get(t): fn.Get(t)},
		}
	})
	subject := let.Var(s, func(t *testcase.T) workflow.Execute {
		d := workflow.Execute{
			ParticipantID: participantID.Get(t),
			ConditionID:   conditionID.Get(t),
			Output:        output.Get(t),
		}
		for i, v := range input.Get(t) {
			name := workflow.VarName(fmt.Sprintf("input-%d", i))
			setVar(t, runtime.Get(t), processID.Get(t), name, v)
			d.Input = append(d.Input, name)
		}
		return d
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) int { return t.Random.IntBetween(1, 100) })
		participantID.Let(s, func(t *testcase.T) workflow.ParticipantID {
			return workflow.ParticipantID(t.Random.UUID())
		})
		input.Let(s, func(t *testcase.T) []any { return []any{value.Get(t)} })
		output.Let(s, func(t *testcase.T) []workflow.VarName { return []workflow.VarName{"result"} })
		fn.Let(s, func(t *testcase.T) any {
			return func(_ context.Context, n int) (int, error) {
				calls.Set(t, calls.Get(t)+1)
				return n, nil
			}
		})

		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(runtime.Get(t).Context(t.Context()), processID.Get(t))
		}
		thenResult := func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), 1)
			assert.Equal[any](t, getVar(t, runtime.Get(t), processID.Get(t), "result"), value.Get(t))
		}
		thenMismatch := func(t *testcase.T) {
			err := act(t)
			var mismatch workflow.ErrParticipantSignatureMismatch
			assert.True(t, errors.As(err, &mismatch))
			assert.Equal(t, mismatch.ID, participantID.Get(t))
			assert.ErrorIs(t, err, workflow.ErrParticipantFuncMappingMismatch)
			assert.False(t, workflow.ErrIsFatal(err))
			assert.Equal(t, calls.Get(t), 0)
			assert.NotEmpty(t, err.Error())
		}

		s.Then("passes the input and stores the result", thenResult)

		for _, tc := range []struct {
			name  string
			input []any
		}{
			{"too few inputs", nil},
			{"too many inputs", []any{1, 2}},
			{"an incompatible input", []any{true}},
			{"an untyped nil for a value parameter", []any{nil}},
			{"an incompatible typed nil", []any{(*string)(nil)}},
		} {
			s.When(tc.name, func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return slicekit.Clone(tc.input) })

				s.Then("returns a nonfatal signature mismatch without invoking the participant", thenMismatch)
			})
		}
		for _, count := range []int{0, 2} {
			s.When(fmt.Sprintf("the output mapping has %d entries", count), func(s *testcase.Spec) {
				output.Let(s, func(t *testcase.T) []workflow.VarName { return make([]workflow.VarName, count) })

				s.Then("rejects the output count before invoking the participant", thenMismatch)
			})
		}
		s.When("a conversion depends on the slice length", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []any { return []any{[]int{1}} })
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context, [2]int) (int, error) { calls.Set(t, calls.Get(t)+1); return 0, nil }
			})

			s.Then("rejects an unsafe conversion without panicking", thenMismatch)
		})
		for _, tc := range []struct {
			name string
			fn   any
		}{
			{"a nil registration", nil},
			{"a non-function registration", 42},
			{"a typed nil function", (func(context.Context, int) (int, error))(nil)},
			{"a function without context", func(int) (int, error) { return 0, nil }},
			{"a function without results", func(context.Context, int) {}},
			{"a function without a trailing error", func(context.Context, int) int { return 0 }},
		} {
			s.When(tc.name, func(s *testcase.Spec) {
				fn.Let(s, func(t *testcase.T) any { return tc.fn })

				s.Then("preserves the invalid-function cause in a nonfatal signature mismatch", func(t *testcase.T) {
					err := act(t)
					var mismatch workflow.ErrParticipantSignatureMismatch
					assert.True(t, errors.As(err, &mismatch))
					assert.Equal(t, mismatch.ID, participantID.Get(t))
					assert.ErrorIs(t, err, workflow.ErrInvalidParticipantFunc)
					assert.False(t, workflow.ErrIsFatal(err))
				})
			})
		}
		for _, v := range []any{nil, (*int)(nil)} {
			s.When(fmt.Sprintf("a nil pointer input is supplied as %T", v), func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return []any{v} })
				fn.Let(s, func(t *testcase.T) any {
					return func(_ context.Context, p *int) (int, error) {
						assert.Nil(t, p)
						calls.Set(t, calls.Get(t)+1)
						return value.Get(t), nil
					}
				})

				s.Then("passes a typed nil to the participant", thenResult)
			})
		}
		s.When("the participant accepts an interface", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, v any) (int, error) { calls.Set(t, calls.Get(t)+1); return v.(int), nil }
			})

			s.Then("passes an assignable concrete value", thenResult)
		})
		s.When("the participant has optional variadic arguments", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, n int, extra ...int) (int, error) {
					calls.Set(t, calls.Get(t)+1)
					for _, v := range extra {
						n += v
					}
					return n, nil
				}
			})

			s.Then("accepts zero optional arguments", thenResult)

			s.And("a required argument is missing", func(s *testcase.Spec) {
				input.LetValue(s, nil)

				s.Then("rejects the invocation before reading parameter types", thenMismatch)
			})
			for _, packed := range []bool{false, true} {
				s.And(fmt.Sprintf("optional arguments are packed in a slice: %t", packed), func(s *testcase.Spec) {
					input.Let(s, func(t *testcase.T) []any {
						if packed {
							return []any{value.Get(t) - 3, []int{1, 2}}
						}
						return []any{value.Get(t) - 3, int8(1), 2}
					})

					s.Then("passes all optional arguments in order", thenResult)
				})
			}
			s.And("a packed variadic slice is nil", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return []any{value.Get(t), []int(nil)} })

				s.Then("accepts a nil variadic slice", thenResult)
			})
			s.And("a variadic element is incompatible", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return []any{1, true} })

				s.Then("rejects the element before invoking the participant", thenMismatch)
			})
		})
		s.When("the participant only has variadic input", func(s *testcase.Spec) {
			input.LetValue(s, nil)
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, values ...any) (int, error) {
					assert.Nil(t, values)
					calls.Set(t, calls.Get(t)+1)
					return value.Get(t), nil
				}
			})

			s.Then("passes a nil variadic slice when no input is mapped", thenResult)
		})
		s.When("an untyped nil is mapped to a variadic interface element", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []any { return []any{nil} })
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, values ...any) (int, error) {
					assert.Equal(t, values, []any{nil})
					calls.Set(t, calls.Get(t)+1)
					return value.Get(t), nil
				}
			})

			s.Then("passes one nil element rather than an omitted slice", thenResult)
		})
		s.When("a trailing slice is also assignable to the variadic element", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []any { return []any{[]any{1, "two"}} })
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, values ...any) (int, error) {
					assert.Equal(t, values, []any{1, "two"})
					calls.Set(t, calls.Get(t)+1)
					return value.Get(t), nil
				}
			})

			s.Then("prefers the packed variadic slice interpretation", thenResult)
		})
		s.When("an untyped nil is mapped to an interface", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []any { return []any{nil} })
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, v any) (int, error) {
					assert.Nil(t, v)
					calls.Set(t, calls.Get(t)+1)
					return value.Get(t), nil
				}
			})

			s.Then("passes a nil interface", thenResult)
		})
		s.When("the participant itself panics", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context, int) (int, error) { panic("participant panic") }
			})

			s.Then("does not disguise the application panic as a signature mismatch", func(t *testcase.T) {
				assert.Equal[any](t, assert.Panic(t, func() { _ = act(t) }), "participant panic")
			})
		})
		s.When("the participant returns an operational error", func(s *testcase.Spec) {
			expected := let.Error(s)
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context, int) (int, error) { return 0, expected.Get(t) }
			})

			s.Then("propagates the error without classifying it as a signature mismatch", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, expected.Get(t))
				var mismatch workflow.ErrParticipantSignatureMismatch
				assert.False(t, errors.As(err, &mismatch))
			})
		})
	})

	s.Describe("#Evaluate", func(s *testcase.Spec) {
		conditionID.Let(s, func(t *testcase.T) workflow.ConditionID {
			return workflow.ConditionID(t.Random.UUID())
		})
		input.Let(s, func(t *testcase.T) []any { return []any{true} })
		fn.Let(s, func(t *testcase.T) any {
			return func(_ context.Context, v bool) (bool, error) { calls.Set(t, calls.Get(t)+1); return v, nil }
		})

		act := func(t *testcase.T) (bool, error) {
			return subject.Get(t).Evaluate(runtime.Get(t).Context(t.Context()), processID.Get(t))
		}
		thenResult := func(t *testcase.T) {
			result, err := act(t)
			assert.NoError(t, err)
			assert.True(t, result)
			assert.Equal(t, calls.Get(t), 1)
		}
		thenMismatch := func(t *testcase.T) {
			result, err := act(t)
			assert.ErrorIs(t, err, workflow.ErrConditionFuncMappingMismatch)
			assert.True(t, workflow.ErrIsFatal(err))
			assert.False(t, result)
			assert.Equal(t, calls.Get(t), 0)
			assert.NotEmpty(t, err.Error())
		}

		s.Then("passes the input and returns the condition's answer", thenResult)

		for _, tc := range []struct {
			name  string
			input []any
		}{
			{"too few inputs", nil},
			{"too many inputs", []any{true, false}},
			{"an incompatible input", []any{42}},
			{"an untyped nil for a value parameter", []any{nil}},
			{"an incompatible typed nil", []any{(*string)(nil)}},
		} {
			s.When(tc.name, func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return slicekit.Clone(tc.input) })

				s.Then("returns a fatal mapping error without invoking the condition", thenMismatch)
			})
		}
		s.When("a slice is too short for an array conversion", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) []any { return []any{[]int{1}} })
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context, [2]int) (bool, error) { calls.Set(t, calls.Get(t)+1); return true, nil }
			})

			s.Then("rejects the conversion without panicking", thenMismatch)
		})
		for _, tc := range []struct {
			name string
			fn   any
		}{
			{"a nil registration", nil},
			{"a non-function registration", 42},
			{"a typed nil function", (func(context.Context, bool) (bool, error))(nil)},
			{"a function without context", func(bool) (bool, error) { return true, nil }},
			{"a function without results", func(context.Context, bool) {}},
			{"a function without a trailing error", func(context.Context, bool) bool { return true }},
			{"a function returning a non-boolean result", func(context.Context, bool) (int, error) { return 0, nil }},
			{"a function returning extra results", func(context.Context, bool) (bool, int, error) { return true, 0, nil }},
		} {
			s.When(tc.name, func(s *testcase.Spec) {
				fn.Let(s, func(t *testcase.T) any { return tc.fn })

				s.Then("returns a fatal invalid-function error", func(t *testcase.T) {
					result, err := act(t)
					assert.ErrorIs(t, err, workflow.ErrInvalidConditionFunc)
					assert.True(t, workflow.ErrIsFatal(err))
					assert.False(t, result)
				})
			})
		}
		for _, v := range []any{nil, (*bool)(nil)} {
			s.When(fmt.Sprintf("a nil pointer input is supplied as %T", v), func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return []any{v} })
				fn.Let(s, func(t *testcase.T) any {
					return func(_ context.Context, p *bool) (bool, error) {
						assert.Nil(t, p)
						calls.Set(t, calls.Get(t)+1)
						return true, nil
					}
				})

				s.Then("passes a typed nil to the condition", thenResult)
			})
		}
		s.When("the condition has optional variadic arguments", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, v bool, extra ...bool) (bool, error) {
					calls.Set(t, calls.Get(t)+1)
					for _, item := range extra {
						v = v && item
					}
					return v, nil
				}
			})

			s.Then("accepts zero optional arguments", thenResult)

			s.And("a required argument is missing", func(s *testcase.Spec) {
				input.LetValue(s, nil)

				s.Then("rejects the invocation before reading parameter types", thenMismatch)
			})
			for _, tc := range []struct {
				name  string
				input []any
			}{
				{"expanded optional arguments", []any{true, true, true}},
				{"a packed variadic slice", []any{true, []bool{true, true}}},
				{"a nil variadic slice", []any{true, []bool(nil)}},
			} {
				s.And(tc.name, func(s *testcase.Spec) {
					input.Let(s, func(t *testcase.T) []any { return slicekit.Clone(tc.input) })

					s.Then("passes the optional arguments", thenResult)
				})
			}
			s.And("a variadic element is incompatible", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) []any { return []any{true, 42} })

				s.Then("rejects the element before invoking the condition", thenMismatch)
			})
		})
	})
}
