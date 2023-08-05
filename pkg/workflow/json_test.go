package workflow_test

import (
	"github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/frameless/pkg/workflow/workflowcontracts"
	"testing"
)

func TestMarshalJSON(t *testing.T) {
	workflowcontracts.Encoding(workflow.MarshalJSON, workflow.UnmarshalJSON)
}
