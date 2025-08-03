package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestIf(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		Cond = let.VarOf[workflow.Condition](s, nil)
		Then = let.VarOf[workflow.ProcessDefinition](s, nil)
		Else = let.VarOf[workflow.ProcessDefinition](s, nil)
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.If {
		return &workflow.If{
			Cond: Cond.Get(t),
			Then: Then.Get(t),
			Else: Else.Get(t),
		}
	})

	stubCallCount := let.VarOf[int](s, 0)

	stub := let.Var(s, func(t *testcase.T) func(ctx context.Context, s *workflow.State) error {
		return func(ctx context.Context, s *workflow.State) error {
			return nil
		}
	})

	CTX := let.Var(s, func(t *testcase.T) context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		t.Defer(cancel)

		ctx = workflow.ContextWithParticipants(ctx, workflow.Participants{
			"stub": workflow.ParticipantFunc(func(ctx context.Context, s *workflow.State) error {
				stubCallCount.Set(t, stubCallCount.Get(t)+1)
				return stub.Get(t)(ctx, s)
			}),
		})
		return ctx
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = CTX.Bind(s)
			state = let.Var(s, func(t *testcase.T) *workflow.State {
				return workflow.NewState()
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Test("on missing fields, we get a validation error", func(t *testcase.T) {
			err := act(t)
			assert.Error(t, err)

			got, ok := errorkit.As[validate.Error](err)
			assert.True(t, ok)
			assert.Error(t, got.Cause)
		})

		s.When("condition is true", func(s *testcase.Spec) {
			Cond.Let(s, func(t *testcase.T) workflow.Condition {
				return workflow.NewConditionTemplate("true")
			})

			s.And("Then is supplied", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.ProcessDefinition {
					return workflow.PID("stub")
				})

				s.Then("If/Then is called", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, stubCallCount.Get(t), 1)
				})
			})

			s.And("Then is absent", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.ProcessDefinition {
					return nil
				})

				s.Then("validation error returned", func(t *testcase.T) {
					err := act(t)
					assert.Error(t, err)

					got, ok := errorkit.As[validate.Error](err)
					assert.True(t, ok)
					assert.NotNil(t, got.Cause)
					assert.Contains(t, got.Cause.Error(), "if.then")
				})
			})
		})
	})
}
