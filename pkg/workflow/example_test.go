package workflow_test

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
)

func Example() {
	// the system designers provide participant domain logic to be used by definitions
	// plus initialise the workflow.Runtime with their own dependencies.
	// (currently this is a blank init, without any dependencies)
	rt := workflow.Runtime{
		Participants: workflow.Participants{
			"foo": func(ctx context.Context) (int, error) {
				return 42, nil
			},
			"bar": func(ctx context.Context) (int, error) {
				return 24, nil
			},
			"baz": func(ctx context.Context) error {
				return nil
			},
			"qux": func(ctx context.Context) error {
				return nil
			},
		},
	}

	// someone builds a workflow definition
	definition := &workflow.Sequence{
		workflow.ExecuteParticipant{ID: "foo",
			Output: []workflow.VarName{"foo"}},
		workflow.ExecuteParticipant{ID: "bar",
			Output: []workflow.VarName{"bar"}},
		workflow.If{
			Cond: wftemplate.Condition(".foo <= .bar"),   // (42 < 24) == false
			Then: workflow.ExecuteParticipant{ID: "baz"}, //
			Else: workflow.ExecuteParticipant{ID: "qux"}, // will run
		},
	}

	pid, err := workflow.MakeProcessID()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// to bind the pid and definition in the system,
	// and prepare for its execution/scheduling
	_ = rt.Bind(ctx, pid, definition)

	// to execute it right-away
	_ = rt.Execute(ctx, pid)

	// or if you want to schedule it for execution later on, and let the system handle suspend signals and such
	_ = rt.Schedule(ctx, pid)
}

func ExampleDefinition_sequence() {
	_ = workflow.Sequence{
		workflow.SetVar{Name: "topic", Value: "go.llib.dev/frameless"},
		workflow.ExecuteParticipant{
			ID:     "summarise",
			Input:  []workflow.VarName{"topic"},
			Output: []workflow.VarName{"summary", "found"},
		},
		workflow.If{
			Cond: wftemplate.Condition(`eq .found true`),
			Then: workflow.ExecuteParticipant{
				ID:    "publish",
				Input: []workflow.VarName{"summary"},
			},
		},
	}
}
