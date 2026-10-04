package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
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

// conditionEvents returns the condition answers recorded in the Process event history.
func conditionEvents(tb testing.TB, repo workflow.EventRepository, pid workflow.ProcessID) []workflow.EventCondition {
	tb.Helper()
	var out []workflow.EventCondition
	for _, event := range getProcessEvents(tb, repo, pid) {
		if ec, ok := event.(workflow.EventCondition); ok {
			out = append(out, ec)
		}
	}
	return out
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

// undeclared is what peekVar records of a variable that is not visible from where it peeks.
const undeclared = "<undeclared>"

// peekVar makes a step which records into seen the value of a variable, as it is visible from where the step runs,
// or undeclared when no variable of that name is visible from there.
//
// It is deliberately not an idempotent step, so it peeks again on every execution.
func peekVar(t *testcase.T, seen testcase.Var[[]any], name workflow.VarName) workflow.Definition {
	return wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
		vars, err := workflow.GetVars(ctx)
		if err != nil {
			return err
		}
		v, ok, err := vars.Lookup(ctx, name)
		if err != nil {
			return err
		}
		if !ok {
			v = undeclared
		}
		testcase.Append(t, seen, v)
		return nil
	}}
}

// copyVar makes a step which assigns the value of a variable to another one, like `to := from`.
//
// It is not an idempotent step, so it only suits a Process which is executed once.
func copyVar(from, to workflow.VarName) workflow.Definition {
	return wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
		vars, err := workflow.GetVars(ctx)
		if err != nil {
			return err
		}
		v, err := vars.Get(ctx, from)
		if err != nil {
			return err
		}
		return vars.Set(ctx, to, v)
	}}
}

// execProcess binds the definition to a new Process, and executes it.
func execProcess(t *testcase.T, rt workflow.Runtime, def workflow.Definition) (workflow.ProcessID, error) {
	t.Helper()
	pid := wftest.MakeProcessID(t)
	assert.Must(t).NoError(rt.Bind(t.Context(), pid, def))
	return pid, rt.Execute(t.Context(), pid)
}
