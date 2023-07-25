package wfdto_test

import (
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/frameless/pkg/workflow/wfdto"
	"github.com/adamluzsi/frameless/pkg/workflow/workflowcontracts"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/pp"
	"testing"
)

func TestJSON(t *testing.T) {
	workflowcontracts.Encoding(wfdto.MarshalJSON, wfdto.UnmarshalJSON).Test(t)
}

func TestJSON_smoke(t *testing.T) {
	pdef := wf.ProcessDefinition{
		ID: "42",
		Task: wf.Seq(
			wf.Template(`var "x"`),
		),
	}
	
	data, err := wfdto.MarshalJSON(pdef)
	assert.NoError(t, err)
	
	pp.PP(data)
	return

	var gotPDef wf.ProcessDefinition
	assert.NoError(t, wfdto.UnmarshalJSON(data, &gotPDef))

	assert.Equal(t, pdef, gotPDef)
}
