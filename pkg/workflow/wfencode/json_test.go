package wfencode_test

import (
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/frameless/pkg/workflow/wfencode"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestJSON_smoke(t *testing.T) {

	pdef := wf.ProcessDefinition{
		ID: "42",
		Task: wf.Sequence{
			wf.If{
				Cond: wf.Comparison{
					Left:      nil,
					Right:     nil,
					Operation: "",
				},
				Then: nil,
				Else: nil,
			},
		},
		EntryPoint: nil,
	}

	data, err := wfencode.MarshalJSON(pdef)
	assert.NoError(t, err)

	var gotPDef wf.ProcessDefinition
	assert.NoError(t, wfencode.UnmarshalJSON(data, &gotPDef))

	assert.Equal(t, pdef, gotPDef)
}
