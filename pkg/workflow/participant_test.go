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
	participant := wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
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

	// #Execute with input argument type conversion pins the contract that
	// values stored in process variables can flow into a participant function
	// whose declared argument type differs from the variable's stored type, as
	// long as Go's reflect rules deem the conversion valid (e.g. a `string`
	// value can be passed where the participant expects a
	// `workflow.ParticipantID`).
	s.Describe("#Execute with input argument type conversion", func(s *testcase.Spec) {
		var convPID = wftest.LetParticipantID(s)

		var (
			convCallCount = let.VarOf(s, 0)
			// lastIn records the value the participant actually received, as
			// the participant sees it (typed as workflow.ParticipantID).
			lastIn = let.VarOf[workflow.ParticipantID](s, "")
		)

		convInKey := let.As[workflow.VarName](let.UUID(s))
		// convInVal is intentionally a plain string so it is convertible to
		// workflow.ParticipantID (a string-based named type) but is NOT the
		// same type.
		convInVal := let.Var(s, func(t *testcase.T) string {
			return t.Random.String()
		})
		convInput := let.Var(s, func(t *testcase.T) []workflow.VarName {
			return []workflow.VarName{convInKey.Get(t)}
		})

		// The participant declares its argument as workflow.ParticipantID. The
		// variable the workflow writes holds a plain string. If the conversion
		// contract holds, the participant receives the string as a
		// workflow.ParticipantID without an error.
		wftest.LetParticipantWithID(s, convPID, func(t *testcase.T) func(ctx context.Context, in workflow.ParticipantID) error {
			return func(ctx context.Context, in workflow.ParticipantID) error {
				lastIn.Set(t, in)
				convCallCount.Set(t, convCallCount.Get(t)+1)
				return nil
			}
		})

		convSubject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
			return &workflow.ExecuteParticipant{
				ID:    convPID.Get(t),
				Input: convInput.Get(t),
			}
		})

		var (
			ctx           = let.Context(s)
			convProcessID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := c.ProcessID.Super(t)
				setVar(t, c.Runtime.Get(t), p, convInKey.Get(t), convInVal.Get(t))
				return p
			})
		)

		act := let.Act(func(t *testcase.T) error {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			return convSubject.Get(t).Execute(execCTX, convProcessID.Get(t))
		})

		// Happy path: a string-typed variable flowing into a
		// workflow.ParticipantID-typed argument must be converted.
		s.Then("the variable's value is converted to the participant's argument type before being passed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, 1, convCallCount.Get(t),
				"the participant must be called exactly once when its argument type matches after conversion")

			assert.Equal(t, workflow.ParticipantID(convInVal.Get(t)), lastIn.Get(t),
				"the participant must receive the variable's string value as a workflow.ParticipantID")
		})

		// The conversion contract is not specific to ParticipantID: any pair
		// where the variable's stored Go type is convertible (per
		// reflect.Value.ConvertibleTo) to the participant's declared parameter
		// type must flow through. Each When block below exercises a different
		// convertible pair to pin the general rule.
		s.When("the variable is convertible to a different string-based named type", func(s *testcase.Spec) {
			// The participant declares its parameter as workflow.ConditionID,
			// which is also a string-based named type (see type ConditionID
			// string in pkg/workflow/workflow.go). The conversion rule must
			// apply uniformly, not just to ParticipantID.
			var (
				condPID   = wftest.LetParticipantID(s)
				condCalls = let.VarOf(s, 0)
				condLast  = let.VarOf[workflow.ConditionID](s, "")
			)
			wftest.LetParticipantWithID(s, condPID,
				func(t *testcase.T) func(ctx context.Context, in workflow.ConditionID) error {
					return func(ctx context.Context, in workflow.ConditionID) error {
						condLast.Set(t, in)
						condCalls.Set(t, condCalls.Get(t)+1)
						return nil
					}
				})

			var (
				condInKey = let.As[workflow.VarName](let.UUID(s))
				condInVal = convInVal // reuse the parent's string value
				condInput = let.Var(s, func(t *testcase.T) []workflow.VarName {
					return []workflow.VarName{condInKey.Get(t)}
				})
				condSubject = let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
					return &workflow.ExecuteParticipant{
						ID:    condPID.Get(t),
						Input: condInput.Get(t),
					}
				})
				condProcessID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
					p := c.ProcessID.Super(t)
					setVar(t, c.Runtime.Get(t), p, condInKey.Get(t), condInVal.Get(t))
					return p
				})
			)

			actCond := let.Act(func(t *testcase.T) error {
				execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
				return condSubject.Get(t).Execute(execCTX, condProcessID.Get(t))
			})

			s.Then("the string-typed variable is converted to a ConditionID", func(t *testcase.T) {
				assert.NoError(t, actCond(t))

				assert.Equal(t, 1, condCalls.Get(t),
					"the ConditionID-taking participant must be called exactly once")

				assert.Equal(t, workflow.ConditionID(condInVal.Get(t)), condLast.Get(t),
					"the participant must receive the variable's string value as a workflow.ConditionID")
			})
		})

		s.When("the variable's type already equals the participant's parameter type", func(s *testcase.Spec) {
			// No conversion needed: the participant declares its parameter as
			// plain string and the variable holds a string. The conversion
			// branch at participant.go must be skipped cleanly without
			// changing the value.
			var (
				strPID   = wftest.LetParticipantID(s)
				strCalls = let.VarOf(s, 0)
				strLast  = let.VarOf[string](s, "")
			)
			wftest.LetParticipantWithID(s, strPID,
				func(t *testcase.T) func(ctx context.Context, in string) error {
					return func(ctx context.Context, in string) error {
						strLast.Set(t, in)
						strCalls.Set(t, strCalls.Get(t)+1)
						return nil
					}
				})

			var (
				strInKey = let.As[workflow.VarName](let.UUID(s))
				strInput = let.Var(s, func(t *testcase.T) []workflow.VarName {
					return []workflow.VarName{strInKey.Get(t)}
				})
				strSubject = let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
					return &workflow.ExecuteParticipant{
						ID:    strPID.Get(t),
						Input: strInput.Get(t),
					}
				})
				strProcessID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
					p := c.ProcessID.Super(t)
					setVar(t, c.Runtime.Get(t), p, strInKey.Get(t), convInVal.Get(t))
					return p
				})
			)

			actStr := let.Act(func(t *testcase.T) error {
				execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
				return strSubject.Get(t).Execute(execCTX, strProcessID.Get(t))
			})

			s.Then("the value is passed through unchanged", func(t *testcase.T) {
				assert.NoError(t, actStr(t))

				assert.Equal(t, 1, strCalls.Get(t),
					"the string-taking participant must be called exactly once when no conversion is needed")

				assert.Equal(t, convInVal.Get(t), strLast.Get(t),
					"the participant must receive the variable's value unchanged")
			})
		})

		s.When("the variable is a bool convertible to a custom bool named type", func(s *testcase.Spec) {
			// Demonstrate the rule on a non-string underlying kind: a bool
			// JSON value is convertible to any named type whose underlying is
			// bool, because both share the bool underlying type.
			type featureFlag bool
			var (
				boolPID   = wftest.LetParticipantID(s)
				boolCalls = let.VarOf(s, 0)
				boolLast  = let.VarOf[featureFlag](s, false)
			)
			wftest.LetParticipantWithID(s, boolPID,
				func(t *testcase.T) func(ctx context.Context, in featureFlag) error {
					return func(ctx context.Context, in featureFlag) error {
						boolLast.Set(t, in)
						boolCalls.Set(t, boolCalls.Get(t)+1)
						return nil
					}
				})

			boolInVal := let.Var(s, func(t *testcase.T) bool {
				return t.Random.Bool()
			})
			var (
				boolInKey = let.As[workflow.VarName](let.UUID(s))
				boolInput = let.Var(s, func(t *testcase.T) []workflow.VarName {
					return []workflow.VarName{boolInKey.Get(t)}
				})
				boolSubject = let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
					return &workflow.ExecuteParticipant{
						ID:    boolPID.Get(t),
						Input: boolInput.Get(t),
					}
				})
				boolProcessID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
					p := c.ProcessID.Super(t)
					setVar(t, c.Runtime.Get(t), p, boolInKey.Get(t), boolInVal.Get(t))
					return p
				})
			)

			actBool := let.Act(func(t *testcase.T) error {
				execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
				return boolSubject.Get(t).Execute(execCTX, boolProcessID.Get(t))
			})

			s.Then("the bool-typed variable is converted to the custom bool named type", func(t *testcase.T) {
				assert.NoError(t, actBool(t))

				assert.Equal(t, 1, boolCalls.Get(t),
					"the featureFlag-taking participant must be called exactly once")

				assert.Equal(t, featureFlag(boolInVal.Get(t)), boolLast.Get(t),
					"the participant must receive the variable's bool value as a featureFlag")
			})
		})
	})

	// #Execute with follow-up Definition return value pins the contract from
	// idempotent.go#handleResultDefinition for the participant-execution
	// surface. When a participant function returns a workflow.Definition instead
	// of nil, the runtime must:
	//
	//   - persist an EventParticipant whose Definition field carries that
	//     returned Definition, so a replay can short-circuit without re-running
	//     the follow-up,
	//   - dispatch the follow-up Definition exactly once,
	//   - propagate the follow-up Definition's outcome (e.g. a RuntimeSignal)
	//     back through Execute.
	//
	// The runtime does not read EventParticipant.Definition on replay; this
	// field exists so an external observer (debugger, analytics, the v1 wire
	// codec) can see "which follow-up Definition this participant triggered"
	// without re-executing anything. The contract pinned here makes that
	// observable.
	s.Describe("#Execute with follow-up Definition return value", func(s *testcase.Spec) {
		var (
			participantCalls = let.VarOf(s, 0)
			followUpCalls    = let.VarOf(s, 0)
			// followUp is the Definition the participant function hands back to
			// the runtime. The default stub increments followUpCalls so
			// follow-up dispatch can be observed by counting; When blocks
			// override StubExecute to test alternative outcomes (suspend).
			followUp = let.Var(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						followUpCalls.Set(t, followUpCalls.Get(t)+1)
						return nil
					},
				}
			})
		)

		pid := wftest.LetParticipantID(s)
		wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				participantCalls.Set(t, participantCalls.Get(t)+1)
				// Returning the Definition as an error is the documented
				// "happy error" pathway: the runtime type-asserts it to
				// workflow.Definition and dispatches it via handleResultDefinition.
				return followUp.Get(t)
			}
		})

		subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
			return &workflow.ExecuteParticipant{ID: pid.Get(t)}
		})

		ctx := let.Context(s)

		act := let.Act(func(t *testcase.T) error {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			return subject.Get(t).Execute(execCTX, c.ProcessID.Get(t))
		})

		// Happy path: the follow-up Definition is recorded on the persisted
		// event, and the follow-up's Execute is dispatched once.
		s.Then("the follow-up Definition is persisted on EventParticipant.Definition", func(t *testcase.T) {
			assert.NoError(t, act(t))

			events := participantEventsOf(t, c)
			assert.Equal(t, 1, len(events),
				"exactly one EventParticipant must be persisted per Execute call")

			assert.Equal(t, followUp.Get(t), events[0].Definition,
				"the persisted EventParticipant must carry the follow-up Definition the participant returned")
		})

		s.Then("the follow-up Definition's Execute is invoked exactly once", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, 1, participantCalls.Get(t))
			assert.Equal(t, 1, followUpCalls.Get(t),
				"the runtime must dispatch the follow-up Definition once on first execution")
		})

		// Idempotency: a replay must not re-invoke the participant function or
		// re-dispatch the follow-up Definition. The cache hit short-circuits
		// before handleResultDefinition, so the cached EventParticipant's
		// Definition is what stops the second dispatch.
		s.When("Execute is called again on the same logical step", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("the participant function is not re-invoked", func(t *testcase.T) {
				assert.Equal(t, 1, participantCalls.Get(t))

				assert.NoError(t, act(t))
				assert.Equal(t, 1, participantCalls.Get(t),
					"replaying the same logical step must not re-invoke the participant function")
			})

			s.Then("the follow-up Definition is not re-dispatched", func(t *testcase.T) {
				assert.Equal(t, 1, followUpCalls.Get(t))

				assert.NoError(t, act(t))
				assert.Equal(t, 1, followUpCalls.Get(t),
					"the follow-up Definition is part of the cached step outcome and must not be re-executed")
			})
		})

		// Signal propagation: a follow-up Definition that raises a
		// RuntimeSignal must surface that signal through Execute, while the
		// participant call itself stays recorded (suspension is an expected
		// outcome, not a failure of the call).
		s.When("the follow-up Definition raises a runtime signal", func(s *testcase.Spec) {
			followUp.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						followUpCalls.Set(t, followUpCalls.Get(t)+1)
						return workflow.Suspend{}
					},
				}
			})

			s.Then("the signal is propagated back through Execute", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
			})

			s.Then("the participant execution is still recorded in the event history", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})

				events := participantEventsOf(t, c)
				assert.Equal(t, 1, len(events),
					"a participant call that returned a Definition is a step outcome; suspension must not erase it")
				assert.NotNil(t, events[0].Definition,
					"the cached EventParticipant must retain the follow-up Definition even when its execution suspended")
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
					Locks:        &memory.WorkflowProcessLocks{},
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
					Locks:        &memory.WorkflowProcessLocks{},
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
					Locks:        &memory.WorkflowProcessLocks{},
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
	_, participantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
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
