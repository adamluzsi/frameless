package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
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

func setVar(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID, key workflow.VarName, val any) {
	tb.Helper()
	ctx := rt.Context(context.Background())
	vars := workflow.Vars{EventsRepository: rt.Events, ProcessID: pid}
	assert.NoError(tb, vars.Set(ctx, key, val))
}

func LetVars(s *testcase.Spec, rt testcase.Var[workflow.Runtime], processID testcase.Var[workflow.ProcessID]) testcase.Var[workflow.Vars] {
	return let.Var(s, func(t *testcase.T) workflow.Vars {
		return getVars(t, rt.Get(t), processID.Get(t))
	})
}

func getVars(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID) workflow.Vars {
	tb.Helper()
	assert.NotNil(tb, rt.Events)
	assert.NotEmpty(tb, pid)
	return workflow.Vars{EventsRepository: rt.Events, ProcessID: pid}
}

func getVar(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID, key workflow.VarName) any {
	tb.Helper()
	val, err := getVars(tb, rt, pid).Get(tb.Context(), key)
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
