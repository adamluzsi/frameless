package workflow_test

import (
	"context"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"

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

					assert.Equal(t, 1, stub.Get(t).CallCount)
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

					assert.Equal(t, 1, stub.Get(t).CallCount)
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

				assert.Equal(t, stub.Get(t).CallCount, 1)
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
