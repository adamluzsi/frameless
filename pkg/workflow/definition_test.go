package workflow_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleIf() {
	ifd := &workflow.If{
		Cond: wftemplate.NewCondition(".X == .Y"),
		Then: &workflow.ExecuteParticipant{ID: "run-on-true"},
		Else: &workflow.ExecuteParticipant{ID: "run-on-false"},
	}
	_, _, _, _ = ifd, ifd.Cond, ifd.Then, ifd.Else
}

// func TestIf(t *testing.T) {
// 	s := testcase.NewSpec(t)

// 	var (
// 		c        = letC(s)
// 		thenStub = c.LetStub(s, "then")
// 		elseStub = c.LetStub(s, "else")
// 	)

// 	var (
// 		Cond = let.Var(s, func(t *testcase.T) workflow.Condition {
// 			return wftemplate.NewCondition(strconv.FormatBool(t.Random.Bool()))
// 		})
// 		Then = let.Var(s, func(t *testcase.T) workflow.Definition {
// 			return &workflow.ExecuteParticipant{ID: "then"}
// 		})
// 		Else = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
// 			return &workflow.ExecuteParticipant{ID: "else"}
// 		})
// 	)
// 	subject := let.Var(s, func(t *testcase.T) *workflow.If {
// 		return &workflow.If{
// 			Cond: Cond.Get(t),
// 			Then: Then.Get(t),
// 			Else: Else.Get(t),
// 		}
// 	})

// 	ctx := let.Var(s, func(t *testcase.T) context.Context {
// 		ctx := t.Context()
// 		ctx = workflow.ContextWithParticipants(ctx)

// 	})

// 	s.Describe("#Execute", func(s *testcase.Spec) {
// 		var (
// 			ctx   = c.Context.Bind(s)
// 			state = c.Process.Bind(s)
// 		)
// 		act := let.Act(func(t *testcase.T) error {
// 			return subject.Get(t).Execute(ctx.Get(t), state.Get(t))
// 		})

// 		s.Test("on a valid If, no error expected from execution", func(t *testcase.T) {
// 			assert.NoError(t, act(t))
// 		})

// 		s.When("fields are missing", func(s *testcase.Spec) {
// 			subject.Let(s, func(t *testcase.T) *workflow.If {
// 				return &workflow.If{}
// 			})

// 			s.Test("on missing fields, we get a validation error", func(t *testcase.T) {
// 				err := act(t)
// 				assert.Error(t, err)

// 				got, ok := errorkit.As[validate.Error](err)
// 				assert.True(t, ok)
// 				assert.Error(t, got.Cause)
// 			})
// 		})

// 		s.When("condition is true", func(s *testcase.Spec) {
// 			Cond.Let(s, func(t *testcase.T) workflow.Condition {
// 				return wftemplate.NewCondition("true")
// 			})

// 			s.And("Then is supplied", func(s *testcase.Spec) {
// 				Then.Let(s, func(t *testcase.T) workflow.Definition {
// 					return workflow.PID("stub")
// 				})

// 				s.Then("If/Then is called", func(t *testcase.T) {
// 					n := t.Random.Repeat(1, 3, func() {
// 						assert.NoError(t, act(t))
// 					})

// 					assert.Equal(t, thenStub.Get(t).CallCount, n)
// 					gotCtx, gotState, ok := thenStub.Get(t).LastExecutedWith()
// 					assert.True(t, ok)
// 					assert.Equal(t, ctx.Get(t), gotCtx)
// 					assert.Equal(t, state.Get(t), gotState)
// 				})
// 			})

// 			s.And("Then is absent", func(s *testcase.Spec) {
// 				Then.Let(s, func(t *testcase.T) workflow.Definition {
// 					return nil
// 				})

// 				s.Then("validation error returned", func(t *testcase.T) {
// 					err := act(t)
// 					assert.Error(t, err)

// 					got, ok := errorkit.As[validate.Error](err)
// 					assert.True(t, ok)
// 					assert.NotNil(t, got.Cause)
// 					assert.Contains(t, got.Cause.Error(), "if.then")
// 				})
// 			})
// 		})

// 		s.When("condition is false", func(s *testcase.Spec) {
// 			Cond.Let(s, func(t *testcase.T) workflow.Condition {
// 				return wftemplate.NewCondition("false")
// 			})

// 			Then.Let(s, func(t *testcase.T) workflow.Definition {
// 				return &workflow.ExecuteParticipant{
// 					ID: "/dev/null",
// 				}
// 			})

// 			s.And("Then is absent", func(s *testcase.Spec) {
// 				Then.Let(s, func(t *testcase.T) workflow.Definition {
// 					return nil
// 				})

// 				s.Then("validation error returned", func(t *testcase.T) {
// 					err := act(t)
// 					assert.Error(t, err)

// 					got, ok := errorkit.As[validate.Error](err)
// 					assert.True(t, ok)
// 					assert.NotNil(t, got.Cause)
// 					assert.Contains(t, got.Cause.Error(), "if.then")
// 				})
// 			})

// 			s.And("Else is supplied", func(s *testcase.Spec) {
// 				Else.Let(s, func(t *testcase.T) workflow.Definition {
// 					return &workflow.ExecuteParticipant{
// 						ID: "stub",
// 					}
// 				})

// 				s.Then("Else path is executed", func(t *testcase.T) {
// 					n := t.Random.Repeat(3, 7, func() {
// 						assert.NoError(t, act(t))
// 					})

// 					assert.Equal(t, thenStub.Get(t).CallCount, n)
// 					gotCtx, gotState, ok := thenStub.Get(t).LastExecutedWith()
// 					assert.True(t, ok)
// 					assert.Equal(t, ctx.Get(t), gotCtx)
// 					assert.Equal(t, state.Get(t), gotState)
// 				})
// 			})

// 			s.And("Else is absent", func(s *testcase.Spec) {
// 				Else.Let(s, func(t *testcase.T) workflow.Definition {
// 					return nil
// 				})

// 				s.Then("no action is taken", func(t *testcase.T) {
// 					assert.NoError(t, act(t))
// 				})
// 			})
// 		})
// 	})
// }

func ExampleSequence() {
	sequence := &workflow.Sequence{
		&workflow.ExecuteParticipant{ID: "foo"},
		&workflow.ExecuteParticipant{ID: "bar"},
		&workflow.ExecuteParticipant{ID: "baz"},
	}

	// a sequence of participants which are executeed after each other.
	_ = sequence
}

func TestSequence(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	seq := let.Var(s, func(t *testcase.T) workflow.Sequence {
		return workflow.Sequence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = c.Context.Bind(s)
			state = c.Process.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return seq.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Test("a valid sequence should yield no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{}
			})

			s.Then("it should yield no error, since nothing can break in it", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("it has an element", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")

			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{&workflow.ExecuteParticipant{ID: "foo"}}
			})

			s.Then("it should execute the given element", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)
			})

			s.And("the element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				foo.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})
			})
		})

		s.When("it has multiple elements", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")
			bar := c.LetStub(s, "bar")
			baz := c.LetStub(s, "baz")

			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					&workflow.ExecuteParticipant{ID: "foo"},
					&workflow.ExecuteParticipant{ID: "bar"},
					&workflow.ExecuteParticipant{ID: "baz"},
				}
			})

			s.Then("it should execute all the elements", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, bar.Get(t).CallCount, n)
				gotCtx, gotState, ok = bar.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, baz.Get(t).CallCount, n)
				gotCtx, gotState, ok = baz.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)
			})

			s.And("an element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				bar.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})

				s.Then("sequence execution is interrupted by the error", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					// baz as being the last in the 3 length sequence, is not reached
					assert.Equal(t, baz.Get(t).CallCount, 0)
				})
			})
		})
	})

}
