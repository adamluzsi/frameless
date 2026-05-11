package workflow_test

import (
	"context"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftesting"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleIf() {
	var _ workflow.Definition = workflow.If{
		Cond: wftemplate.Condition(".X == .Y"),
		Then: workflow.ExecuteParticipant{ID: "run-on-true"},
		Else: workflow.ExecuteParticipant{ID: "run-on-false"},
	}
}

func TestIf(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

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
			process = LetProcess(s)
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
					pid  = LetParticipantID(s)
					stub = c.LetStub(s, pid)
				)
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.ExecuteParticipant{ID: pid.Get(t)}
				})

				s.Then("If/Then is called", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, 1, stub.Get(t).CallCount())
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
					pid  = LetParticipantID(s)
					stub = c.LetStub(s, pid)
				)
				Else.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.ExecuteParticipant{ID: pid.Get(t)}
				})

				s.Then("Else path is executed", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, 1, stub.Get(t).CallCount())
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
				Then: workflow.ExecuteParticipant{ID: "then"},
				Else: workflow.ExecuteParticipant{ID: "else"},
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
			}

			var p workflow.Process
			p.Definition = pdef
			t.Random.Repeat(3, 7, func() {
				// a fresh dedicated context for each execution is expected
				assert.NoError(t, rt.Execute(t.Context(), &p))
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
		workflow.ExecuteParticipant{ID: "foo"},
		workflow.ExecuteParticipant{ID: "bar"},
		workflow.ExecuteParticipant{ID: "baz"},
	}
}

func TestSequence(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	sequence := let.Var(s, func(t *testcase.T) workflow.Sequence {
		return workflow.Sequence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = LetProcess(s)
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
			var pid = LetParticipantID(s)
			var stub = c.LetStub(s, pid)

			sequence.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
				}
			})

			s.Then("it should execute the given element", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, stub.Get(t).CallCount(), 1)
			})

			s.And("the element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				stub.Let(s, func(t *testcase.T) *StubParticipant {
					stub := stub.Super(t)
					stub.Err = expErr.Get(t)
					return stub
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})
			})
		})

		s.When("it has multiple elements", func(s *testcase.Spec) {
			var (
				fooPid = LetParticipantID(s)
				barPid = LetParticipantID(s)
				bazPid = LetParticipantID(s)
			)

			var callOrder = let.Var(s, func(t *testcase.T) []workflow.ParticipantID {
				return make([]workflow.ParticipantID, 0, 3)
			})

			LetParticipant(s, c, fooPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, fooPid.Get(t))
					return nil
				}
			})
			mid := LetParticipant(s, c, barPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, barPid.Get(t))
					return nil
				}
			})
			LetParticipant(s, c, bazPid, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, callOrder, bazPid.Get(t))
					return nil
				}
			})

			sequence.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					&workflow.ExecuteParticipant{ID: fooPid.Get(t)},
					&workflow.ExecuteParticipant{ID: barPid.Get(t)},
					&workflow.ExecuteParticipant{ID: bazPid.Get(t)},
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

func TestSuspend(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		while = let.VarOf[workflow.Condition](s, nil)
		until = let.VarOf[workflow.Condition](s, nil)
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.Suspend {
		return &workflow.Suspend{
			While: while.Get(t),
			Until: until.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = let.Context(s)
			process = LetProcess(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.When("While condition is true (Continue=false)", func(s *testcase.Spec) {
			while.Let(s, func(t *testcase.T) workflow.Condition {
				return wftesting.Stub{
					StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return true, nil
					},
				}
			})

			s.Test("Suspend error expected", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, workflow.Suspend{})
			})
		})

		s.When("While condition is false (Continue=true)", func(s *testcase.Spec) {
			while.Let(s, func(t *testcase.T) workflow.Condition {
				return wftesting.Stub{
					StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return false, nil
					},
				}
			})

			s.Test("no error expected", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("Until condition is true (Continue=true)", func(s *testcase.Spec) {
			until.Let(s, func(t *testcase.T) workflow.Condition {
				return wftesting.Stub{
					StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return true, nil
					},
				}
			})

			s.Test("no error expected", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("Until condition is false (Continue=false)", func(s *testcase.Spec) {
			until.Let(s, func(t *testcase.T) workflow.Condition {
				return wftesting.Stub{
					StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return false, nil
					},
				}
			})

			s.Test("Suspend error expected", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
			})
		})

		s.When("condition evaluation fails", func(s *testcase.Spec) {
			expErr := let.Error(s)

			s.Before(func(t *testcase.T) {
				faultyCondition := wftesting.Stub{
					StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return false, expErr.Get(t)
					},
				}
				if t.Random.Bool() {
					while.Set(t, faultyCondition)
				} else {
					until.Set(t, faultyCondition)
				}
			})

			s.Then("fault propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})
		})
	})
}

func TestIsCompleted(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		process = LetProcess(s)
	)
	act := let.Act(func(t *testcase.T) bool {
		return workflow.IsCompleted(process.Get(t))
	})

	s.When("when events are empty", func(s *testcase.Spec) {
		process.Let(s, func(t *testcase.T) *workflow.Process {
			p := process.Super(t)
			if t.Random.Bool() {
				p.Events = nil
			} else {
				p.Events = []workflow.Event{}
			}
			return p
		})

		s.Then("event is considered not completed", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("process has events but nothing related to process completion", func(s *testcase.Spec) {
		process.Let(s, func(t *testcase.T) *workflow.Process {
			p := process.Super(t)
			p.Events = workflow.Events{
				workflow.VariableEvent{
					Operation: workflow.SetVariableEventOperation,
					Key:       "foo",
					Value:     42,
				},
				workflow.ExecuteParticipantEvent{
					ParticipantID: "foo",
				},
			}
			return p
		})

		s.Then("it is considered not completed", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("EventCompleted event is present", func(s *testcase.Spec) {
		process.Let(s, func(t *testcase.T) *workflow.Process {
			p := process.Super(t)
			p.Events = append(p.Events, workflow.EventCompleted{})
			return p
		})

		s.Then("it is considered completed", func(t *testcase.T) {
			assert.True(t, act(t))
		})

		s.And("other events as well present in the event history", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				var events []workflow.Event
				events = append(events,
					workflow.VariableEvent{
						Operation: workflow.SetVariableEventOperation,
						Key:       "foo",
						Value:     42,
					},
					workflow.ExecuteParticipantEvent{
						ParticipantID: "foo",
					})
				slicekit.Unshift(&p.Events, events...)
				return p
			})

			s.Then("it is considered completed", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})
}

func TestSetVar(t *testing.T) {
	s := testcase.NewSpec(t)
	c := letC(s)

	var (
		key   = let.As[workflow.VariableKey](let.String(s))
		value = let.Int(s)
	)
	subject := let.Var(s, func(t *testcase.T) workflow.SetVar {
		return workflow.SetVar{
			Key:   key.Get(t),
			Value: value.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			Context = let.Context(s)
			process = LetProcessWithDefinition(s, subject)
		)
		act := let.Act(func(t *testcase.T) error {
			return c.Runtime.Get(t).Execute(Context.Get(t), process.Get(t))
		})

		s.Then("I expect that the process will have the variable set", func(t *testcase.T) {
			act(t)

			assert.Equal[any](t, process.Get(t).Var().Get(key.Get(t)), value.Get(t))
		})

		s.Then("execution is idempotent with runtime", func(t *testcase.T) {
			assert.NoError(t, act(t)) // first pass

			firstPassEvents := slicekit.Clone(process.Get(t).Events)

			t.Random.Repeat(3, 7, func() {
				assert.NoError(t, act(t))

				assert.Equal(t, process.Get(t).Events, firstPassEvents)
			})
		})
	})
}

func ExampleSpawn() {
	_ = workflow.Sequence{
		workflow.Spawn{
			Definition: workflow.Sequence{
				workflow.SetVar{Key: "foo", Value: "bar"},
			},
		},
	}
}

func TestSpawn(t *testing.T) {
	s := testcase.NewSpec(t)
	c := letC(s)

	var (
		blocking = let.VarOf(s, false)
	)
	subject := let.Var(s, func(t *testcase.T) workflow.Spawn {
		return workflow.Spawn{
			Blocking: blocking.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			Context = let.Context(s)
			process = LetProcessWithDefinition(s, subject)
		)
		act := let.Act(func(t *testcase.T) error {
			return c.Runtime.Get(t).Execute(Context.Get(t), process.Get(t))
		})

		s.Then("I expect that the process will have the variable set", func(t *testcase.T) {
			act(t)

		})

		s.Then("execution is idempotent with runtime", func(t *testcase.T) {
			assert.NoError(t, act(t)) // first pass

			firstPassEvents := slicekit.Clone(process.Get(t).Events)

			t.Random.Repeat(3, 7, func() {
				assert.NoError(t, act(t))

				assert.Equal(t, process.Get(t).Events, firstPassEvents)
			})
		})
	})
}
