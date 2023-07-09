package wfencode_test

import (
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/frameless/pkg/workflow/wfencode"
	"github.com/adamluzsi/frameless/pkg/workflow/wfencode/wfencodecontracts"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestJSON(t *testing.T) {
	wfencodecontracts.Encoding(
		wfencode.MarshalJSON, wfencode.UnmarshalJSON).
		Test(t)
}

func TestJSON_smoke(t *testing.T) {
	pdef := wf.ProcessDefinition{
		ID: "42",
		Task: wf.Sequence{
			wf.Template(`var "x"`),
		},
		EntryPoint: nil,
	}

	data, err := wfencode.MarshalJSON(pdef)
	assert.NoError(t, err)

	var gotPDef wf.ProcessDefinition
	assert.NoError(t, wfencode.UnmarshalJSON(data, &gotPDef))

	assert.Equal(t, pdef, gotPDef)
}
