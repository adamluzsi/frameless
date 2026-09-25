package deprecated_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/deprecated"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestExecuteParticipant(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	var (
		pid   = wftest.LetParticipantID(s)
		calls = let.VarOf(s, 0)
	)
	wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(context.Context) error {
		return func(context.Context) error {
			calls.Set(t, calls.Get(t)+1)
			return nil
		}
	})
	subject := let.Var(s, func(t *testcase.T) deprecated.ExecuteParticipant {
		return deprecated.ExecuteParticipant{ID: pid.Get(t)}
	})
	// replacement is the workflow.Execute that took over the deprecated definition.
	replacement := let.Var(s, func(t *testcase.T) workflow.Execute {
		return workflow.Execute{ParticipantID: pid.Get(t)}
	})
	ctx := let.Var(s, func(t *testcase.T) context.Context {
		return c.Runtime.Get(t).Context(t.Context())
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), c.ProcessID.Get(t))
		}

		s.Then("the registered participant is called, and the call is recorded the same way as by workflow.Execute", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, calls.Get(t), 1)
			events := participantEvents(t, c)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ParticipantID, pid.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{"participant", string(pid.Get(t))})
		})

		s.Then("the recorded call is replayed by the replacement workflow.Execute", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, replacement.Get(t).Execute(ctx.Get(t), c.ProcessID.Get(t)))

			assert.Equal(t, calls.Get(t), 1)
		})

		s.When("the replacement workflow.Execute already executed the step", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(replacement.Get(t).Execute(ctx.Get(t), c.ProcessID.Get(t)))
			})

			s.Then("its recorded call is replayed", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, calls.Get(t), 1)
			})
		})
	})

	s.Describe("#ExecuteWith", func(s *testcase.Spec) {
		var (
			executions = let.VarOf(s, 0)
			executeFn  = let.Var(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) error {
				return func(context.Context, workflow.ProcessID) error {
					executions.Set(t, executions.Get(t)+1)
					return nil
				}
			})
		)

		act := func(t *testcase.T) error {
			return subject.Get(t).ExecuteWith(ctx.Get(t), c.ProcessID.Get(t), executeFn.Get(t))
		}

		s.Then("the execute function runs in place of the registered participant, and is replayed like with workflow.Execute#ExecuteWith under the ID as its name", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, workflow.Execute{}.ExecuteWith(ctx.Get(t), c.ProcessID.Get(t), pid.Get(t), executeFn.Get(t)))

			assert.Equal(t, executions.Get(t), 1)
			assert.Equal(t, calls.Get(t), 0)
			assert.Equal(t, len(participantEvents(t, c)), 1)
		})
	})
}

func TestExecuteCondition(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	var (
		cid    = let.Var(s, func(t *testcase.T) workflow.ConditionID { return workflow.ConditionID(t.Random.UUID()) })
		answer = let.Var(s, func(t *testcase.T) bool { return t.Random.Bool() })
		calls  = let.VarOf(s, 0)
	)
	c.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
		return workflow.Conditions{cid.Get(t): func(context.Context) (bool, error) {
			calls.Set(t, calls.Get(t)+1)
			return answer.Get(t), nil
		}}
	})
	subject := let.Var(s, func(t *testcase.T) deprecated.ExecuteCondition {
		return deprecated.ExecuteCondition{ID: cid.Get(t)}
	})
	// replacement is the workflow.Execute that took over the deprecated definition.
	replacement := let.Var(s, func(t *testcase.T) workflow.Execute {
		return workflow.Execute{ConditionID: cid.Get(t)}
	})
	ctx := let.Var(s, func(t *testcase.T) context.Context {
		return c.Runtime.Get(t).Context(t.Context())
	})

	s.Describe("#Evaluate", func(s *testcase.Spec) {
		act := func(t *testcase.T) (bool, error) {
			return subject.Get(t).Evaluate(ctx.Get(t), c.ProcessID.Get(t))
		}

		s.Then("the registered condition is asked, and the answer is recorded the same way as by workflow.Execute", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, answer.Get(t))

			events := conditionEvents(t, c)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ConditionID, cid.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{string(cid.Get(t))})
		})

		s.Then("the recorded answer is replayed by the replacement workflow.Execute", func(t *testcase.T) {
			first, err := act(t)
			assert.NoError(t, err)
			answer.Set(t, !first)

			got, err := replacement.Get(t).Evaluate(ctx.Get(t), c.ProcessID.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, got, first)
			assert.Equal(t, calls.Get(t), 1)
		})
	})

	s.Describe("#EvaluateWith", func(s *testcase.Spec) {
		var (
			evaluations = let.VarOf(s, 0)
			evaluateFn  = let.Var(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) (bool, error) {
				return func(context.Context, workflow.ProcessID) (bool, error) {
					evaluations.Set(t, evaluations.Get(t)+1)
					return answer.Get(t), nil
				}
			})
		)

		act := func(t *testcase.T) (bool, error) {
			return subject.Get(t).EvaluateWith(ctx.Get(t), c.ProcessID.Get(t), evaluateFn.Get(t))
		}

		s.Then("the evaluate function answers in place of the registered condition, and is replayed like with workflow.Execute#EvaluateWith under the ID as its name", func(t *testcase.T) {
			first, err := act(t)
			assert.NoError(t, err)
			answer.Set(t, !first)

			got, err := workflow.Execute{}.EvaluateWith(ctx.Get(t), c.ProcessID.Get(t), cid.Get(t), evaluateFn.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, got, first)
			assert.Equal(t, evaluations.Get(t), 1)
			assert.Equal(t, calls.Get(t), 0)
		})
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), c.ProcessID.Get(t))
		}

		s.Then("the condition is asked as a step, and its answer is recorded under a workflow::condition path segment", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, calls.Get(t), 1)
			events := conditionEvents(t, c)
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].Path, workflow.Path{"workflow::condition", string(cid.Get(t))})
		})

		s.Then("a later execution at the same position replays the answer", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))

			assert.Equal(t, calls.Get(t), 1)
		})
	})
}

func participantEvents(t *testcase.T, c wftest.C) []workflow.EventParticipant {
	t.Helper()
	var out []workflow.EventParticipant
	for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
		if e, ok := event.(workflow.EventParticipant); ok {
			out = append(out, e)
		}
	}
	return out
}

func conditionEvents(t *testcase.T, c wftest.C) []workflow.EventCondition {
	t.Helper()
	var out []workflow.EventCondition
	for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
		if e, ok := event.(workflow.EventCondition); ok {
			out = append(out, e)
		}
	}
	return out
}
