package workflow_test

import (
	"context"
	wf "github.com/adamluzsi/frameless/pkg/workflow"
)

var SampleProcessDefinition = wf.ProcessDefinition{
	Task: wf.Sequence{
		wf.UseParticipant{ParticipantID: wf.ParticipantID("42")},
		wf.If{
			Cond: wf.CondTemplate(`.x != 42`),
			Then: nil,
		},
		wf.While{
			Cond: func(ctx context.Context) (bool, error) {
				return true, nil
			},
			Block: wf.UseParticipant{ParticipantID: "X"},
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
