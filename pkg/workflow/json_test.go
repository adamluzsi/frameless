package workflow_test

import (
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/frameless/pkg/workflow/workflowcontracts"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestJSON(t *testing.T) {
	workflowcontracts.Encoding(wf.MarshalJSON, wf.UnmarshalJSON).Test(t)
}

func TestJSON_smoke(t *testing.T) {
	pdef := wf.ProcessDefinition{
		ID: "42",
		Task: wf.Seq(
			wf.Template(`var "x"`),
		),
	}

	data, err := wf.MarshalJSON(pdef)
	assert.NoError(t, err)

	var gotPDef wf.ProcessDefinition
	assert.NoError(t, wf.UnmarshalJSON(data, &gotPDef))

	assert.Equal(t, pdef, gotPDef)
}
