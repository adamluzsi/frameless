package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase/assert"
)

func Test_smoke(t *testing.T) {
	templateFuncMap := workflow.TemplateFuncMap{
		"isOK": func(v any) bool {
			return true
		},
	}

	participants := workflow.Participants{
		"foo": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"bar": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"baz": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"qux": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
	}

	var pdef workflow.ProcessDefinition = &workflow.If{
		Cond: workflow.NewTemplateCond(`eq .X "foo"`),
		Then: &workflow.Sequence{
			workflow.PID("foo"),
			workflow.PID("bar"),
			&workflow.If{
				Cond: workflow.NewTemplateCond(`isOK .X`),
				Then: workflow.PID("qux"),
			},
		},
		Else: workflow.PID("baz"),
	}

	r := workflow.Runtime{
		Participants:    participants,
		TemplateFuncMap: templateFuncMap,
	}

	assert.NoError(t, pdef.Validate(r.Context(t.Context())), "expected that the process definition is valid")

	p := workflow.Process{
		State: workflow.NewState(),
		PDEF:  pdef,
	}
	assert.NoError(t, r.Execute(t.Context(), p))
}
