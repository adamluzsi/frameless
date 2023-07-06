package workflow_test

import (
	wf "github.com/adamluzsi/frameless/pkg/workflow"
)

var SampleProcessDefinition = wf.ProcessDefinition{
	Task: wf.Sequence{
		wf.UseParticipant{ParticipantID: wf.ParticipantID("42")},
		wf.If{
			Cond: wf.Template(`.x != 42`),
			Then: nil,
		},
		wf.If{
			Cond: wf.Comparison{
				Left:      wf.ConstValue{Value: 42},
				Right:     wf.RefValue{Key: "x"},
				Operation: "!=",
			},
			Then: nil,
			Else: nil,
		},
		wf.While{
			Cond:  wf.Template(`.x != 42`),
			Block: wf.UseParticipant{ParticipantID: "X"},
		},
	},
}
