package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestConditions_reflection(t *testing.T) {
	s := testcase.NewSpec(t)

	fn := let.Var[any](s, func(t *testcase.T) any {
		return func(context.Context) (bool, error) { return true, nil }
	})
	subject := let.Var(s, func(t *testcase.T) workflow.Condition {
		condition, found, err := (workflow.Conditions{"condition": fn.Get(t)}).FindByID(t.Context(), "condition")
		assert.Must(t).NoError(err)
		assert.Must(t).True(found)
		return condition
	})

	s.Describe("#Evaluate directly from the registry", func(s *testcase.Spec) {
		act := func(t *testcase.T) (bool, error) { return subject.Get(t).Evaluate(t.Context(), workflow.ProcessID{}) }
		s.Then("evaluates a context-only condition", func(t *testcase.T) {
			result, err := act(t)
			assert.NoError(t, err)
			assert.True(t, result)
		})
		s.When("the condition returns a result alongside an operational error", func(s *testcase.Spec) {
			expected := let.Error(s)
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context) (bool, error) { return true, expected.Get(t) }
			})
			s.Then("preserves both return values for direct callers", func(t *testcase.T) {
				result, err := act(t)
				assert.True(t, result)
				assert.ErrorIs(t, err, expected.Get(t))
			})
		})
		s.When("the condition implementation panics", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context) (bool, error) { panic("condition panic") }
			})
			s.Then("propagates the application panic unchanged", func(t *testcase.T) {
				assert.Equal[any](t, assert.Panic(t, func() { _, _ = act(t) }), "condition panic")
			})
		})
		s.When("the condition requires mapped input", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(context.Context, bool) (bool, error) { panic("must not be called") }
			})
			s.Then("returns a fatal mapping error rather than panicking", func(t *testcase.T) {
				result, err := act(t)
				assert.ErrorIs(t, err, workflow.ErrConditionFuncMappingMismatch)
				assert.True(t, workflow.ErrIsFatal(err))
				assert.False(t, result)
			})
		})
		s.When("the condition only requires optional input", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) any {
				return func(_ context.Context, values ...bool) (bool, error) { return len(values) == 0, nil }
			})
			s.Then("evaluates with zero optional arguments", func(t *testcase.T) {
				result, err := act(t)
				assert.NoError(t, err)
				assert.True(t, result)
			})
		})
	})
}

func LetConditionID(s *testcase.Spec) testcase.Var[workflow.ConditionID] {
	return let.Var(s, func(t *testcase.T) workflow.ConditionID {
		cid := random.Unique(func() workflow.ConditionID {
			return workflow.ConditionID(t.Random.Domain())
		})
		return cid
	})
}

func LetCondition[Func any](s *testcase.Spec, c wftest.C, cid testcase.Var[workflow.ConditionID], mk func(t *testcase.T) Func) testcase.Var[Func] {
	p := let.Var(s, func(t *testcase.T) Func {
		return mk(t)
	})
	c.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
		cs := c.Conditions.Super(t)
		if cs == nil {
			cs = make(workflow.Conditions)
		}
		cs[cid.Get(t)] = p.Get(t)
		return cs
	})
	return p
}
