package workflow_test

import (
	"context"
	"sync"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestExecuteParticipant(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var pid = wftest.LetParticipantID(s)

	var (
		callCount = let.VarOf(s, 0)
		lastCTX   = let.VarOf[context.Context](s, nil)
		// lastCTXErr records the liveness of the context AT CALL TIME.
		// The context handed to a participant is transaction scoped, and the
		// memory EventLog cancels it once that transaction finishes, so it
		// can only be meaningfully inspected while the participant runs.
		lastCTXErr = let.VarOf[error](s, nil)
		lastOut    = let.VarOf[string](s, "")
	)
	participant := wftest.LetParticipantWithID(s, c, pid, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
		return func(ctx context.Context, in string) (string, error) {
			lastCTX.Set(t, ctx)
			lastCTXErr.Set(t, ctx.Err())
			callCount.Set(t, callCount.Get(t)+1)
			out := t.Random.UUID()
			lastOut.Set(t, out)
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
			ctx       = let.Context(s)
			processID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := c.ProcessID.Super(t)
				setVar(t, c.Runtime.Get(t), p, inKey.Get(t), inVal.Get(t))
				return p
			})
		)
		act := let.Act(func(t *testcase.T) error {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			return subject.Get(t).Execute(execCTX, processID.Get(t))
		})

		s.Then("participant is looked up by its ID and executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, callCount.Get(t), 1)

			gotCTX := lastCTX.Get(t)
			assert.NotNil(t, gotCTX)
			assert.NoError(t, lastCTXErr.Get(t),
				"the participant must be called with a live context")
		})

		s.Then("the execution event has a timestamp", func(t *testcase.T) {
			assert.NoError(t, act(t))

			for _, event := range mustHistory(t, c.Runtime.Get(t), processID.Get(t)) {
				executionEvent, ok := event.(workflow.EventParticipant)
				if !ok {
					continue
				}
				assert.False(t, executionEvent.Timestamp.IsZero())
				return
			}
			t.Fatal("missing ExecuteParticipantEvent from process history")
		})

		s.When("the ExecuteParticipant.ID (participant ID) is invalid", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *workflow.ExecuteParticipant {
				randomPID := workflow.ParticipantID(random.Unique(t.Random.String, string(pid.Get(t))))
				ep := subject.Super(t)
				ep.ID = randomPID
				return ep
			})

			s.Then("we get back a validation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{ID: subject.Get(t).ID})
			})
		})

		s.When("the referenced participant has an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (string, error) {
				return func(ctx context.Context, in string) (string, error) {
					return "", expErr.Get(t)
				}
			})

			s.Then("error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})
		})

		s.When("the participant was executed already", func(s *testcase.Spec) {
			var firstOut = let.Var[string](s, nil)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
				firstOut.Set(t, getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), outKey.Get(t)).(string))
				assert.Equal(t, callCount.Get(t), 1)
			})

			s.Then("calling it again will not execute the participant function to ensure idempotent behaviour", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, 1, callCount.Get(t))
			})

			s.And("even if the function would return back always unique values for the same input", func(s *testcase.Spec) {
				var lastIn = let.VarOf[string](s, "")

				participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
					return func(ctx context.Context, in string) (out string, _ error) {
						lastIn.Set(t, in)
						callCount.Set(t, 1+callCount.Get(t))
						out = t.Random.UUID()
						lastOut.Set(t, out)
						return out, nil
					}
				})

				s.Then("the execution remains idempotent and the result don't change", func(t *testcase.T) {
					t.Random.Repeat(1, 7, func() {
						assert.NoError(t, act(t))
						gotOut := getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)).(string)
						assert.Equal(t, firstOut.Get(t), gotOut)
						assert.Equal(t, 1, callCount.Get(t))
					})
				})

				s.Context("but if the input argument changes AFTER the last execution", func(s *testcase.Spec) {
					var newIn = let.UUID(s)
					s.Before(func(t *testcase.T) {
						setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), newIn.Get(t))

						event, ok := slicekit.Last(mustHistory(t, c.Runtime.Get(t), processID.Get(t)))
						assert.True(t, ok)
						ve, ok := event.(workflow.EventSetVar)
						assert.True(t, ok)

						assert.Equal(t, ve.Name, inKey.Get(t))
						assert.Equal[any](t, ve.Value, newIn.Get(t))
					})

					s.Then("the execution won't reoccur, because historically, at the position of the original execution, the variables are still the same", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, 1, callCount.Get(t), "expected that execution count remained the same")
					})
				})

				s.Context("but if the original input argument modified (external manual tampering of the event log)", func(s *testcase.Spec) {
					var newIn = let.UUID(s)
					s.Before(func(t *testcase.T) {
						setVar(t, c.Runtime.Get(t), processID.Get(t), inKey.Get(t), newIn.Get(t))

						// history rewrite: retroactively change the value originally
						// recorded for the input variable.
						events := mustHistory(t, c.Runtime.Get(t), processID.Get(t))
						for _, e := range events {
							ve, ok := e.(workflow.EventSetVar)
							if !ok {
								continue
							}
							if ve.Name == inKey.Get(t) {
								ve.Value = newIn.Get(t)

								var event workflow.Event = ve
								assert.NoError(t, c.EventRepository.Get(t).Update(t.Context(), &event))
								break
							}
						}
					})

					s.Then("the due to this change, the execution gets repeated", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, 2, callCount.Get(t))
						assert.Equal(t, lastOut.Get(t), getVar(t, c.Runtime.Get(t), processID.Get(t), outKey.Get(t)).(string))
						assert.Equal(t, lastIn.Get(t), newIn.Get(t))
					})
				})
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Context("idempotency", func(s *testcase.Spec) {
			s.Test("same repetition don't execute participants twice", func(t *testcase.T) {

				var (
					fooOut = t.Random.String()
					barOut = t.Random.Int()
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				participants := workflow.Participants{
					"foo": func(ctx context.Context) (string, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (int, error) {
						inc("bar")
						assert.Equal(t, in, fooOut)
						return barOut, nil
					},
					"baz": func(ctx context.Context, s string, n int) error {
						inc("baz")
						assert.Equal(t, fooOut, s)
						assert.Equal(t, barOut, n)
						return nil
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{
						ID:     "foo",
						Output: []workflow.VarName{"foo-val"},
					},
					&workflow.ExecuteParticipant{
						ID:     "bar",
						Input:  []workflow.VarName{"foo-val"},
						Output: []workflow.VarName{"bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID:    "baz",
						Input: []workflow.VarName{"foo-val", "bar-val"},
					},
				}

				r := workflow.Runtime{
					Participants: participants,
					Events:       &memory.WorkflowEventRepository{},
				}

				pid := mustProcessID(t)
				p := pid
				// Process is stateless — bind the definition via the event history.
				rtCtx := r.Context(t.Context())
				var ev workflow.Event = workflow.EventUseDefinition{
					EventID:    mustEventID(t),
					ProcessID:  pid,
					Timestamp:  clock.Now(),
					Definition: pdef,
				}
				assert.NoError(t, r.Events.Create(rtCtx, &ev))

				assert.NoError(t, r.Execute(t.Context(), p))
				assert.NotEmpty(t, mustHistory(t, r, p))
				eventsAfterTheFirstExecution := mustHistory(t, r, p)

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, r.Execute(t.Context(), p))

					assert.Equal(t, ranCount["foo"], 1)
					assert.Equal(t, ranCount["bar"], 1)
					assert.Equal(t, ranCount["baz"], 1)

					assert.Equal[any](t, getVar(t, r, p, "foo-val"), fooOut)
					assert.Equal[any](t, getVar(t, r, p, "bar-val"), barOut)

					assert.Equal(t, mustHistory(t, r, p), eventsAfterTheFirstExecution)
				})
			})

			s.Test("repeating the same participant execution at definition level is supported", func(t *testcase.T) {
				var ran int

				participants := workflow.Participants{
					"foo": func(ctx context.Context) error {
						ran++
						return nil
					},
				}

				var def workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{ID: "foo"},
					&workflow.ExecuteParticipant{ID: "foo"},
					&workflow.ExecuteParticipant{ID: "foo"},
				}

				r := workflow.Runtime{
					Participants: participants,
					Events:       &memory.WorkflowEventRepository{},
				}

				p := mustProcessID(t)

				assert.NoError(t, def.Execute(r.Context(t.Context()), p))
				assert.NotEmpty(t, mustHistory(t, r, p))
				eventsAfterTheFirstExecution := mustHistory(t, r, p)

				assert.Equal(t, ran, 3, "expected that the 3 individual foo participant call will all execute, since they are referenced multiple times in the definition")

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, def.Execute(r.Context(t.Context()), p))
					assert.Equal(t, mustHistory(t, r, p), eventsAfterTheFirstExecution)
					assert.Equal(t, ran, 3, "after the initial call, the execution should remain idempotent")
				})
			})

			s.Test("upon failure, restarting the execution will continue from the last successful point", func(t *testcase.T) {
				var (
					fooOut = t.Random.String()
					barOut = t.Random.Int()

					expectedFlakyErr = t.Random.Error()
					failOnce         sync.Once
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				participants := workflow.Participants{
					"foo": func(ctx context.Context) (string, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (int, error) {
						inc("bar")
						assert.Equal(t, in, fooOut)
						return barOut, nil
					},
					"baz": func(ctx context.Context, s string, n int) error {
						inc("baz")
						assert.Equal(t, fooOut, s)
						assert.Equal(t, barOut, n)
						return nil
					},
					"flaky": func(ctx context.Context) (err error) {
						inc("flaky")
						failOnce.Do(func() {
							err = expectedFlakyErr
						})
						return err
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{
						ID:     "foo",
						Output: []workflow.VarName{"foo-val"},
					},
					&workflow.ExecuteParticipant{
						ID:     "bar",
						Input:  []workflow.VarName{"foo-val"},
						Output: []workflow.VarName{"bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID:    "baz",
						Input: []workflow.VarName{"foo-val", "bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID: "flaky",
						//TODO: retry integration maybe?
					},
				}

				r := workflow.Runtime{
					Participants: participants,
					Events:       &memory.WorkflowEventRepository{},
				}

				p := mustProcessID(t)

				assert.ErrorIs(t, expectedFlakyErr, pdef.Execute(r.Context(t.Context()), p))
				assert.NotEmpty(t, mustHistory(t, r, p))

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), p))
				assert.Equal[any](t, getVar(t, r, p, "foo-val"), fooOut)
				assert.Equal[any](t, getVar(t, r, p, "bar-val"), barOut)
				assert.Equal(t, ranCount["foo"], 1)
				assert.Equal(t, ranCount["bar"], 1)
				assert.Equal(t, ranCount["baz"], 1)
				assert.Equal(t, ranCount["flaky"], 2)
			})
		})
	})
}

// TestExecuteParticipant_rollback pins the transactional boundary around a
// participant call.
//
// A participant may mutate process variables through workflow.GetVars before it
// fails. Those mutations belong to the attempt, not to the process: a failed
// attempt records no execution event, so the next pass re-runs the participant
// from scratch. If the mutations outlived the failure, that retry would observe
// state left behind by an execution which, as far as the event history is
// concerned, never happened — and the participant would no longer be idempotent
// in the only sense that matters, "same starting state, same behaviour".
func TestExecuteParticipant_rollback(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		varName = let.As[workflow.VarName](let.UUID(s))
		expErr  = let.Error(s)
		// callCount counts how many times the participant body ran.
		callCount = let.VarOf(s, 0)
		// visibleOnEntry records, per call, whether the variable the
		// participant is about to write was already visible when it started.
		visibleOnEntry = let.VarOf[[]bool](s, nil)
	)

	// The participant mutates a variable and only afterwards decides whether it
	// can finish. The first attempt fails after the mutation, later ones pass.
	_, participantID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			vars, err := workflow.GetVars(ctx)
			if err != nil {
				return err
			}

			_, ok, err := vars.Lookup(ctx, varName.Get(t))
			if err != nil {
				return err
			}
			visibleOnEntry.Set(t, append(visibleOnEntry.Get(t), ok))

			if err := vars.Set(ctx, varName.Get(t), t.Random.UUID()); err != nil {
				return err
			}

			callCount.Set(t, callCount.Get(t)+1)
			if callCount.Get(t) == 1 {
				return expErr.Get(t)
			}
			return nil
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.ExecuteParticipant{ID: participantID.Get(t)}
	})

	act := let.Act(func(t *testcase.T) error {
		return c.ActExecute(t)
	})

	s.Then("the variable written by the failed attempt is not visible afterwards", func(t *testcase.T) {
		assert.ErrorIs(t, act(t), expErr.Get(t))
		assert.Equal(t, callCount.Get(t), 1)

		_, ok, err := getVars(t, c.Runtime.Get(t), c.ProcessID.Get(t)).
			Lookup(t.Context(), varName.Get(t))
		assert.NoError(t, err)
		assert.False(t, ok,
			"the failed attempt recorded no execution event, so it must not leave a variable behind either")
	})

	s.Then("the failed attempt leaves no variable mutation in the event history", func(t *testcase.T) {
		assert.ErrorIs(t, act(t), expErr.Get(t))

		for _, e := range mustHistory(t, c.Runtime.Get(t), c.ProcessID.Get(t)) {
			ve, ok := e.(workflow.EventSetVar)
			if !ok {
				continue
			}
			assert.NotEqual(t, ve.Name, varName.Get(t),
				"an EventSetVar from the rolled back attempt is still in the history")
		}
	})

	s.Then("the retry starts from the same state as the failed attempt did", func(t *testcase.T) {
		assert.ErrorIs(t, act(t), expErr.Get(t))
		assert.NoError(t, act(t))

		assert.Equal(t, callCount.Get(t), 2)
		assert.Equal(t, visibleOnEntry.Get(t), []bool{false, false},
			"the retry must not observe the variable written by the failed attempt")
	})

	s.Then("a successful attempt keeps its variable mutation", func(t *testcase.T) {
		assert.ErrorIs(t, act(t), expErr.Get(t))
		assert.NoError(t, act(t))

		_, ok, err := getVars(t, c.Runtime.Get(t), c.ProcessID.Get(t)).
			Lookup(t.Context(), varName.Get(t))
		assert.NoError(t, err)
		assert.True(t, ok,
			"rolling back a failed attempt must not cost us the mutations of the successful one")
	})
}
