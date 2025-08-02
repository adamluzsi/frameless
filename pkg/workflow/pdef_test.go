package workflow_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/let"
)

func TestIf(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		Cond = let.Var[workflow.Condition](s, nil)
		Then = let.Var[workflow.ProcessDefinition](s, nil)
		Else = let.Var[workflow.ProcessDefinition](s, nil)
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
			ctx   = let.Context(s)
			state = let.Var(s, func(t *testcase.T) *workflow.State {
				return workflow.NewState()
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

	})
}
