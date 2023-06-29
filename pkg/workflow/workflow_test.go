package workflow_test

import (
	"context"
	wf "github.com/adamluzsi/frameless/pkg/workflow"
)


var SampleProcessDefinition = wf.ProcessDefinition{
	ID: "test-this-out",
	Task: wf.Sequence{
		wf.UseParticipant{ID: "step-1", ParticipantID: wf.ParticipantID("42")},
		wf.If{
			Cond: func(ctx context.Context) (bool, error) {
				return vars.Foo == "42", nil
			},
			Then: nil,
			Else: nil,
		},
		wf.While{
			Cond: func(ctx context.Context) (bool, error) {
				return vars.Foo == "42", nil
			},
			Then: nil,
			Else: nil,
		},
	},
}

type SampleVariables struct {
	Foo string
	Bar string
	Baz string
}

var SamplePRocessDefinitionExp = wf.ProcessDefinition[SampleVariables]{
	ID: "test-process-def",
	Participant: wf.If[SampleVariables](func(ctx context.Context, vs SampleVariables) (bool, error) {

	}),
}

func ExampleParticipant() {

}
