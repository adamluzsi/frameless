package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TestReplace is the dedicated behavioural specification for the
// workflow.Replace RuntimeSignal.
//
// Replace is a "happy" error that a participant may return from its function:
// it instructs the workflow runtime to abandon the in-flight definition and
// resume execution from a brand new definition. This test describes the
// expected semantics through testcase.Spec scenarios — what an end-user of the
// workflow package can rely on when they choose to use Replace.
//
// The behaviour described here is intentionally specified first (top-down),
// independently of how the runtime currently implements Replace, so the
// production code can be checked against it.
func TestReplace(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	subject := let.Var(s, func(t *testcase.T) workflow.Replace {
		return workflow.Replace{}
	})

	_, replacePID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return subject.Get(t)
		}
	})

	var fooN = let.VarOf(s, 0)
	_, fooPID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			fooN.Set(t, fooN.Get(t)+1)
			return nil
		}
	})

	var barN = let.VarOf(s, 0)
	_, barPID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			barN.Set(t, barN.Get(t)+1)
			return nil
		}
	})

	var bazN = let.VarOf(s, 0)
	_, bazPID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			bazN.Set(t, bazN.Get(t)+1)
			return nil
		}
	})

	act := let.Act(func(t *testcase.T) error {
		return c.ActExecute(t)
	})

	subject.Let(s, func(t *testcase.T) workflow.Replace {
		t.Log("replace occurs midway of a definition")
		return workflow.Replace{
			Definition: workflow.Sequence{
				workflow.ExecuteParticipant{ID: fooPID.Get(t)},
				workflow.ExecuteParticipant{ID: bazPID.Get(t)},
			},
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.ExecuteParticipant{ID: fooPID.Get(t)},
			workflow.ExecuteParticipant{ID: replacePID.Get(t)},
			workflow.ExecuteParticipant{ID: barPID.Get(t)},
		}
	})

	s.Then("execution is interrupted by replace and workflow execution proceeds on the definition", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, fooN.Get(t), 2)
		assert.Equal(t, barN.Get(t), 0)
		assert.Equal(t, bazN.Get(t), 1)
	})

	s.Then("upon completion, the workflow marked as completed", func(t *testcase.T) {
		assert.NoError(t, act(t))

		done, err := workflow.Complete{ProcessID: c.ProcessID.Get(t)}.IsCompleted(t.Context(), c.EventRepository.Get(t))
		assert.NoError(t, err)
		assert.True(t, done)
	})

	s.Then("execution is idempotent and can be repeated", func(t *testcase.T) {
		t.Random.Repeat(3, 7, func() {
			assert.NoError(t, act(t))
		})

		assert.Equal(t, fooN.Get(t), 2)
		assert.Equal(t, barN.Get(t), 0)
		assert.Equal(t, bazN.Get(t), 1)
	})
}
