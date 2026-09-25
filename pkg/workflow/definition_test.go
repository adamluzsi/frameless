package workflow_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

func ExampleIf() {
	var _ workflow.Definition = workflow.If{
		Cond: wftemplate.Condition(".X == .Y"),
		Then: workflow.Execute{ParticipantID: "run-on-true"},
		Else: workflow.Execute{ParticipantID: "run-on-false"},
	}
}

func TestIf(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var (
		Cond = let.Var(s, func(t *testcase.T) workflow.Condition {
			return wftemplate.Condition(strconv.FormatBool(t.Random.Bool()))
		})
		Then = let.Var(s, func(t *testcase.T) workflow.Definition {
			return nil
		})
		Else = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return nil
		})
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.If {
		return &workflow.If{
			Cond: Cond.Get(t),
			Then: Then.Get(t),
			Else: Else.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Test("on a valid If, no error expected from execution", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("condition is missing", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *workflow.If {
				d := subject.Super(t)
				d.Cond = nil
				return d
			})

			s.Then("we get back a fatal workflow error", func(t *testcase.T) {
				assert.ErrorIs(t, workflow.ErrFatal, act(t))
			})
		})

		s.When("condition is true", func(s *testcase.Spec) {
			Cond.Let(s, func(t *testcase.T) workflow.Condition {
				return wftemplate.Condition("true")
			})

			s.And("Then is supplied", func(s *testcase.Spec) {
				var (
					pid   = wftest.LetParticipantID(s)
					count = let.VarOf(s, 0)
					_     = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
						return func(ctx context.Context) error {
							count.Set(t, count.Get(t)+1)
							return nil
						}
					})
				)
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Execute{ParticipantID: pid.Get(t)}
				})

				s.Then("If/Then is called", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, 1, count.Get(t))
				})
			})

			s.And("Then is absent", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return nil
				})

				s.Then("no action is taken", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
			})
		})

		s.When("condition is false", func(s *testcase.Spec) {
			Cond.Let(s, func(t *testcase.T) workflow.Condition {
				return wftemplate.Condition("false")
			})

			s.And("Else is supplied", func(s *testcase.Spec) {
				var (
					pid   = wftest.LetParticipantID(s)
					count = let.VarOf(s, 0)
					_     = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
						return func(ctx context.Context) error {
							count.Set(t, count.Get(t)+1)
							return nil
						}
					})
				)
				Else.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Execute{ParticipantID: pid.Get(t)}
				})

				s.Then("Else path is executed", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, 1, count.Get(t))
				})
			})

			s.And("Else is absent", func(s *testcase.Spec) {
				Else.Let(s, func(t *testcase.T) workflow.Definition {
					return nil
				})

				s.Then("no action is taken", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
			})
		})

		s.Test("is idempotent", func(t *testcase.T) {

			pdef := workflow.If{
				Cond: wftemplate.Condition(strconv.FormatBool(t.Random.Bool())),
				Then: workflow.Execute{ParticipantID: "then"},
				Else: workflow.Execute{ParticipantID: "else"},
			}

			var count int
			rt := workflow.Runtime{
				Participants: workflow.Participants{
					"then": func(ctx context.Context) error {
						count++
						return ctx.Err()
					},
					"else": func(ctx context.Context) error {
						count++
						return ctx.Err()
					},
				},
				Events: &memory.WorkflowEventRepository{},
				Locks:  &memory.WorkflowProcessLocks{},
			}

			// Allocate the ProcessID and seed the UseDefinitionEvent so the
			// runtime can find the definition when Execute runs. Process is
			// stateless; the runtime reads the definition from the history.
			pid := mustProcessID(t)

			rtCtx := rt.Context(t.Context())
			var ev workflow.Event = workflow.EventUseDefinition{
				EventID:    mustEventID(t),
				ProcessID:  pid,
				Timestamp:  clock.Now(),
				Definition: pdef,
			}
			assert.NoError(t, rt.Events.Create(rtCtx, &ev))

			t.Random.Repeat(3, 7, func() {
				// a fresh dedicated context for each execution is expected
				assert.NoError(t, rt.Execute(t.Context(), pid))
			})
			assert.Equal(t, count, 1,
				"Process contains the event log of changes,",
				"hence executing the definition twice",
				"with the same process results in the same result,",
				"with no repeated calls.")

		})
	})
}

func ExampleSequence() {
	_ = workflow.Sequence{
		workflow.Execute{ParticipantID: "foo"},
		workflow.Execute{ParticipantID: "bar"},
		workflow.Execute{ParticipantID: "baz"},
	}
}

func TestSequence(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	sequence := let.Var(s, func(t *testcase.T) workflow.Sequence {
		return workflow.Sequence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return sequence.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Test("a valid sequence should yield no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			sequence.Let(s, func(t *testcase.T) workflow.Sequence {
				var seq workflow.Sequence
				if t.Random.Bool() {
					seq = make(workflow.Sequence, 0)
				}
				return seq
			})

			s.Then("practically do nothing", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("it has an element", func(s *testcase.Spec) {
			var (
				pid   = wftest.LetParticipantID(s)
				count = let.VarOf(s, 0)
				part  = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						count.Set(t, count.Get(t)+1)
						return nil
					}
				})
			)

			sequence.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					workflow.Execute{ParticipantID: pid.Get(t)},
				}
			})

			s.Then("it should execute the given element", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, count.Get(t), 1)
			})

			s.And("the element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				part.Let(s, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						return expErr.Get(t)
					}
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})
			})
		})

		s.When("it has multiple elements", func(s *testcase.Spec) {
			var (
				fooPid = wftest.LetParticipantID(s)
				barPid = wftest.LetParticipantID(s)
				bazPid = wftest.LetParticipantID(s)
			)

			var callOrder = let.Var(s, func(t *testcase.T) []workflow.ParticipantID {
				return make([]workflow.ParticipantID, 0, 3)
			})

			wftest.LetParticipantWithID(s, fooPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, fooPid.Get(t))
					return nil
				}
			})
			mid := wftest.LetParticipantWithID(s, barPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, barPid.Get(t))
					return nil
				}
			})
			wftest.LetParticipantWithID(s, bazPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, bazPid.Get(t))
					return nil
				}
			})

			sequence.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					&workflow.Execute{ParticipantID: fooPid.Get(t)},
					&workflow.Execute{ParticipantID: barPid.Get(t)},
					&workflow.Execute{ParticipantID: bazPid.Get(t)},
				}
			})

			s.Then("it should execute all the elements", func(t *testcase.T) {
				assert.NoError(t, act(t))

				expectedOrder := []workflow.ParticipantID{
					fooPid.Get(t),
					barPid.Get(t),
					bazPid.Get(t),
				}
				assert.Equal(t, expectedOrder, callOrder.Get(t))
			})

			s.And("an element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				mid.Let(s, func(t *testcase.T) func(context.Context) error {
					prev := mid.Super(t)
					return func(ctx context.Context) error {
						prev(ctx)
						return expErr.Get(t)
					}
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})

				s.Then("sequence execution is interrupted by the error", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					assert.NotContains(t, callOrder.Get(t), bazPid.Get(t))
				})
			})
		})
	})
}

func ExampleSleep() {
	_ = workflow.Sleep{
		While: workflow.Execute{ConditionID: "wait-while-something"},
	}
}

func TestSleep(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		while = let.VarOf[workflow.Condition](s, nil)
		until = let.VarOf[workflow.Condition](s, nil)
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.Sleep {
		return &workflow.Sleep{
			While: while.Get(t),
			Until: until.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		process := wftest.LetProcessID(s)
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(wftest.Runtime.Get(t).Context(t.Context()), process.Get(t))
		}
		s.Before(func(t *testcase.T) {
			// A Sleep tells its attempts apart by the second they happen in.
			// Each spec starts at the beginning of a second, so its attempts share that second,
			// until the spec moves on to the next one.
			// Time keeps flowing, as a frozen clock would also leave the event IDs unordered.
			timecop.Travel(t, clock.Now().Truncate(time.Second).Add(time.Second))
		})
		nextSecond := func(t *testcase.T) {
			timecop.Travel(t, clock.Now().Truncate(time.Second).Add(time.Second))
		}
		setReady := func(t *testcase.T, v bool) {
			vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: wftest.EventRepository.Get(t)}
			assert.Must(t).NoError(vars.Set(t.Context(), "ready", v))
		}
		completions := func(t *testcase.T) []workflow.EventSleepCompleted {
			return eventsOfType[workflow.EventSleepCompleted](t, wftest.EventRepository.Get(t), process.Get(t))
		}
		condition := let.Var[workflow.Condition](s, func(t *testcase.T) workflow.Condition {
			return wftemplate.Condition(".ready")
		})
		until.Let(s, func(t *testcase.T) workflow.Condition { return condition.Get(t) })
		// wakeUp is the answer of the condition that lets the Sleep wake up.
		wakeUp := let.VarOf(s, true)

		s.Then("it continues once its condition lets it wake up", func(t *testcase.T) {
			setReady(t, wakeUp.Get(t))
			assert.NoError(t, act(t))
		})

		s.Then("it suspends while its condition doesn't let it wake up, and asks again on the next attempt", func(t *testcase.T) {
			setReady(t, !wakeUp.Get(t))
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			nextSecond(t)
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			setReady(t, wakeUp.Get(t))
			nextSecond(t)
			assert.NoError(t, act(t))
		})

		s.Then("once woken up, it is passed on replay, even after its condition changed back", func(t *testcase.T) {
			setReady(t, wakeUp.Get(t))
			assert.NoError(t, act(t))
			setReady(t, !wakeUp.Get(t))
			nextSecond(t)
			assert.NoError(t, act(t))
		})

		s.Then("its wake-up is recorded as its completion, at its position", func(t *testcase.T) {
			setReady(t, !wakeUp.Get(t))
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			assert.Empty(t, completions(t), "a suspended attempt doesn't complete the Sleep")

			setReady(t, wakeUp.Get(t))
			nextSecond(t)
			before := clock.Now()
			assert.NoError(t, act(t))
			after := clock.Now()
			assert.NoError(t, act(t)) // replay

			got := completions(t)
			assert.Must(t).Equal(len(got), 1)
			assert.NotEmpty(t, got[0].EventID)
			assert.Equal(t, got[0].ProcessID, process.Get(t))
			assert.Equal(t, got[0].Path, workflow.Path{"sleep"})
			assert.False(t, got[0].Timestamp.Before(before) || got[0].Timestamp.After(after),
				"the completion is timestamped at the wake-up")
		})

		s.Then("the answers of an attempt are recorded under the second of the attempt", func(t *testcase.T) {
			setReady(t, !wakeUp.Get(t))
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			first := strconv.FormatInt(clock.Now().Unix(), 10)
			nextSecond(t)
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			second := strconv.FormatInt(clock.Now().Unix(), 10)

			events := conditionEvents(t, wftest.EventRepository.Get(t), process.Get(t))
			assert.Must(t).Equal(len(events), 2)
			for i, attempt := range []string{first, second} {
				assert.Must(t).True(2 <= len(events[i].Path))
				assert.Equal(t, events[i].Path[:2], workflow.Path{"sleep", attempt})
			}
		})

		s.Then("attempts within the same second share the answers of its condition, so it wakes up no sooner than the next second", func(t *testcase.T) {
			setReady(t, !wakeUp.Get(t))
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			setReady(t, wakeUp.Get(t))
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			nextSecond(t)
			assert.NoError(t, act(t))
		})

		s.When("While is used instead of Until", func(s *testcase.Spec) {
			until.LetValue(s, nil)
			while.Let(s, func(t *testcase.T) workflow.Condition { return condition.Get(t) })
			wakeUp.LetValue(s, false)

			s.Then("it suspends while its condition holds, and once it doesn't, it is passed for good", func(t *testcase.T) {
				setReady(t, !wakeUp.Get(t))
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				setReady(t, wakeUp.Get(t))
				nextSecond(t)
				assert.NoError(t, act(t))
				setReady(t, !wakeUp.Get(t))
				nextSecond(t)
				assert.NoError(t, act(t))
				assert.Equal(t, len(completions(t)), 1)
			})
		})

		s.When("the condition is an Execute with a registered ConditionID", func(s *testcase.Spec) {
			cid := LetConditionID(s)
			evaluations := let.VarOf(s, 0)
			wftest.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
				return workflow.Conditions{cid.Get(t): func(_ context.Context, ready bool) (bool, error) {
					evaluations.Set(t, evaluations.Get(t)+1)
					return ready, nil
				}}
			})
			condition.Let(s, func(t *testcase.T) workflow.Condition {
				return workflow.Execute{ConditionID: cid.Get(t), Input: []workflow.VarName{"ready"}}
			})

			s.Then("the registry is asked on every attempt until the Sleep wakes up, and no more after", func(t *testcase.T) {
				setReady(t, false)
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				setReady(t, true)
				nextSecond(t)
				assert.NoError(t, act(t))
				setReady(t, false)
				nextSecond(t)
				assert.NoError(t, act(t))
				assert.Equal(t, evaluations.Get(t), 2)
			})

			s.And("an earlier version recorded an answer at the position of the Sleep that kept it asleep", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					setReady(t, true)
					// the input matches the current variables, so this answer would be replayed if it were looked up
					var event workflow.Event = workflow.EventCondition{
						EventID: mustEventID(t), ProcessID: process.Get(t), Timestamp: clock.Now(),
						ConditionID: cid.Get(t),
						Path:        workflow.Path{"sleep", string(cid.Get(t))},
						Input:       []any{true}, Answer: false,
					}
					assert.Must(t).NoError(wftest.EventRepository.Get(t).Create(t.Context(), &event))
				})

				s.Then("the condition is asked again rather than keeping the process asleep for ever", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, evaluations.Get(t), 1)
				})
			})
		})

		s.When("the condition does not record its answers", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) workflow.Condition {
				return wftest.Stub{StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					vars := workflow.Vars{ProcessID: pid, EventsRepository: wftest.EventRepository.Get(t)}
					ready, err := vars.Get(ctx, "ready")
					return ready == true, err
				}}
			})

			s.Then("it is asked on every attempt, even within the same second", func(t *testcase.T) {
				setReady(t, false)
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				setReady(t, true)
				assert.NoError(t, act(t))
			})

			s.Then("once woken up, the Sleep is passed on replay without asking it again", func(t *testcase.T) {
				setReady(t, true)
				assert.NoError(t, act(t))
				setReady(t, false)
				nextSecond(t)
				assert.NoError(t, act(t))
			})
		})

		s.When("the condition is built on another condition that records its answers", func(s *testcase.Spec) {
			cid := LetConditionID(s)
			// the condition inverts .ready, and records its own answer through EvaluateWith
			condition.Let(s, func(t *testcase.T) workflow.Condition {
				inner := wftemplate.Condition(".ready")
				return wftest.Stub{StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					return workflow.Execute{}.EvaluateWith(ctx, pid, cid.Get(t), func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
						ok, err := inner.Evaluate(ctx, pid)
						return !ok, err
					})
				}}
			})

			s.Then("the nested conditions are asked again on the next attempt too", func(t *testcase.T) {
				setReady(t, true)
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				setReady(t, false)
				nextSecond(t)
				assert.NoError(t, act(t))
			})
		})

		s.When("the condition fails", func(s *testcase.Spec) {
			expErr := let.Error(s)
			condition.Let(s, func(t *testcase.T) workflow.Condition {
				return wftest.Stub{StubEvaluate: func(context.Context, workflow.ProcessID) (bool, error) {
					return false, expErr.Get(t)
				}}
			})

			s.Then("the error is returned, and the Sleep is not completed", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
				assert.Empty(t, completions(t))
			})
		})

		s.When("neither While nor Until is given", func(s *testcase.Spec) {
			until.LetValue(s, nil)

			s.Then("it suspends on every attempt, as nothing could wake it up", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				nextSecond(t)
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Empty(t, completions(t))
			})
		})
	})

	s.Describe("resuming a process after its Sleep woke up", func(s *testcase.Spec) {
		process := wftest.LetProcessID(s)
		act := func(t *testcase.T) error {
			return wftest.Runtime.Get(t).Execute(t.Context(), process.Get(t))
		}
		until.LetValue(s, wftemplate.Condition(".ready"))
		suspendCalls := let.VarOf(s, 0)
		wftest.LetParticipantWithID(s, let.VarOf(s, workflow.ParticipantID("suspend-once")), func(t *testcase.T) func(context.Context) error {
			return func(context.Context) error {
				suspendCalls.Set(t, suspendCalls.Get(t)+1)
				if suspendCalls.Get(t) == 1 {
					return workflow.Suspend{}
				}
				return nil
			}
		})
		afterCalls := let.VarOf(s, 0)
		wftest.LetParticipantWithID(s, let.VarOf(s, workflow.ParticipantID("after")), func(t *testcase.T) func(context.Context) error {
			return func(context.Context) error {
				afterCalls.Set(t, afterCalls.Get(t)+1)
				return nil
			}
		})
		s.Before(func(t *testcase.T) {
			vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: wftest.EventRepository.Get(t)}
			assert.Must(t).NoError(vars.Set(t.Context(), "ready", true))
			definition := workflow.Sequence{
				*subject.Get(t),
				// the variable the Sleep waited for changes back before the process suspends
				workflow.SetVar{Name: "ready", Value: false},
				workflow.Execute{ParticipantID: "suspend-once"},
				workflow.Execute{ParticipantID: "after"},
			}
			assert.Must(t).NoError(wftest.Runtime.Get(t).Bind(t.Context(), process.Get(t), definition))
		})

		s.Then("the Sleep does not fall asleep again, and the process continues where it suspended", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			timecop.Travel(t, time.Second) // the process resumes in a later second
			assert.NoError(t, act(t))
			assert.Equal(t, suspendCalls.Get(t), 2)
			assert.Equal(t, afterCalls.Get(t), 1)
		})
	})
}
