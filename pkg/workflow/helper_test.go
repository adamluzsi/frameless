package workflow_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase/assert"
)

// mustHistory returns the Process event history, failing the test on error.
func mustHistory(tb testing.TB, p *workflow.Process) []workflow.Event {
	tb.Helper()
	events, err := p.History()
	assert.NoError(tb, err)
	return events
}
