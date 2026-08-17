package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase/assert"
)

// mustHistory returns the Process event history from the Runtime's
// EventsRepository, failing the test on error.
//
// The event log is the single source of truth about a Process; it is read by
// looking up the Runtime from the context and querying its EventsRepository.
func mustHistory(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID) []workflow.Event {
	tb.Helper()
	assert.NotNil(tb, rt.Events)
	return getProcessEvents(tb, rt.Events, pid)
}

func getProcessEvents(tb testing.TB, repo workflow.EventRepository, pid workflow.ProcessID) []workflow.Event {
	tb.Helper()
	events, err := iterkit.CollectE(repo.FindByProcessID(tb.Context(), pid))
	assert.NoError(tb, err)
	return events
}

func setVar(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID, key workflow.VarKey, val any) {
	tb.Helper()
	ctx := rt.Context(context.Background())
	vars := workflow.Vars{EventsRepository: rt.Events, ProcessID: pid}
	assert.NoError(tb, vars.Set(ctx, key, val))
}

func getVar(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID, key workflow.VarKey) any {
	tb.Helper()
	ctx := rt.Context(context.Background())
	vars := workflow.Vars{EventsRepository: rt.Events, ProcessID: pid}
	val, err := vars.Get(ctx, key)
	assert.NoError(tb, err)
	return val
}

func mustEventID(tb testing.TB) workflow.EventID {
	tb.Helper()
	id, err := workflow.MakeEventID()
	assert.NoError(tb, err)
	return id
}

func mustProcessID(tb testing.TB) workflow.ProcessID {
	tb.Helper()
	id, err := workflow.MakeProcessID()
	assert.NoError(tb, err)
	return id
}
