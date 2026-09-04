package wftemplate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// setVar seeds a workflow variable into the Process' event history.
//
// wftemplate has no access to the unexported test helpers of the workflow
// package, and going through workflow.Vars directly is also the most honest
// arrangement here: it is exactly how a Participant or a SetVar definition
// would have recorded the variable at runtime.
func setVar(tb testing.TB, rt workflow.Runtime, pid workflow.ProcessID, name workflow.VarName, val any) {
	tb.Helper()
	vars := workflow.Vars{ProcessID: pid, EventsRepository: rt.Events}
	assert.NoError(tb, vars.Set(rt.Context(context.Background()), name, val))
}

func TestCondition(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	// varName is fixed rather than randomised, because a template field
	// reference (".name") must be a valid template identifier, which a random
	// UUID is not. The value is randomised instead, so the assertions still
	// prove that the real stored value is read back, not a coincidence.
	const varName workflow.VarName = "name"

	var (
		// storedValue is what the Process actually holds in varName.
		storedValue = let.UUID(s)
		// comparedValue is what the template expression compares varName
		// against. Keeping the two apart is what lets a single ACT cover both
		// the matching and the non-matching case.
		comparedValue = let.Var(s, func(t *testcase.T) string {
			return storedValue.Get(t)
		})
	)

	subject := let.Var(s, func(t *testcase.T) wftemplate.Condition {
		return wftemplate.Condition(fmt.Sprintf("eq .%s %q", varName, comparedValue.Get(t)))
	})

	s.Describe("#Evaluate", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)

			// processID is the harness Process, seeded with varName before the
			// condition ever runs.
			processID = c.ProcessID.Let(s, func(t *testcase.T) workflow.ProcessID {
				pid := c.ProcessID.Super(t)
				setVar(t, c.Runtime.Get(t), pid, varName, storedValue.Get(t))
				return pid
			})

			// execCTX is the context the Condition is evaluated with. It is a
			// variable of its own so that contexts which lack a Runtime, or
			// which carry a FuncMap, can be arranged without a second ACT.
			execCTX = let.Var(s, func(t *testcase.T) context.Context {
				return c.Runtime.Get(t).Context(ctx.Get(t))
			})
		)

		act := let.Act2(func(t *testcase.T) (bool, error) {
			return subject.Get(t).Evaluate(execCTX.Get(t), processID.Get(t))
		})

		s.Then("the process variable is read back and the expression evaluates to true", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.True(t, got, assert.MessageF("expected .%s to resolve to %q", varName, storedValue.Get(t)))
		})

		s.When("the expression compares the variable against a different value", func(s *testcase.Spec) {
			comparedValue.Let(s, func(t *testcase.T) string {
				return t.Random.UUID()
			})

			s.Then("it evaluates to false", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.False(t, got)
			})
		})

		s.When("the expression references a variable that was never set", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) wftemplate.Condition {
				return wftemplate.Condition(fmt.Sprintf("eq .not_set_%s %q", t.Random.StringNC(8, "abcdefghijklmnopqrstuvwxyz"), comparedValue.Get(t)))
			})

			// An absent key on the variable map yields the zero value, which
			// compares unequal rather than blowing up. Pinning this keeps an
			// unset variable a benign "false" instead of a process failure.
			s.Then("it evaluates to false without an error", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.False(t, got)
			})
		})

		s.When("the expression is a constant that references no variable", func(s *testcase.Spec) {
			subject.LetValue(s, wftemplate.Condition("true"))

			s.Then("it evaluates to true", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.True(t, got)
			})

			s.And("the constant is false", func(s *testcase.Spec) {
				subject.LetValue(s, wftemplate.Condition("false"))

				s.Then("it evaluates to false", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.False(t, got)
				})
			})
		})

		s.When("the variable holds a non string value", func(s *testcase.Spec) {
			const numVarName workflow.VarName = "count"

			var num = let.Var(s, func(t *testcase.T) int {
				return t.Random.IntBetween(1, 100)
			})

			processID.Let(s, func(t *testcase.T) workflow.ProcessID {
				pid := c.ProcessID.Super(t)
				setVar(t, c.Runtime.Get(t), pid, numVarName, num.Get(t))
				return pid
			})

			subject.Let(s, func(t *testcase.T) wftemplate.Condition {
				return wftemplate.Condition(fmt.Sprintf("lt .%s %d", numVarName, num.Get(t)+1))
			})

			s.Then("the template operates on the value's own type", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.True(t, got)
			})
		})

		s.When("a custom function is supplied through the context", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				return wftemplate.ContextWith(ctx.Super(t), wftemplate.FuncMap{
					"hasPrefix": strings.HasPrefix,
				})
			})

			subject.Let(s, func(t *testcase.T) wftemplate.Condition {
				prefix := storedValue.Get(t)[:8]
				return wftemplate.Condition(fmt.Sprintf("hasPrefix .%s %q", varName, prefix))
			})

			s.Then("the function is callable and receives the variable's value", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.True(t, got)
			})

			s.And("a further function is registered on top of it", func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context {
					return wftemplate.ContextWith(ctx.Super(t), wftemplate.FuncMap{
						"hasSuffix": strings.HasSuffix,
					})
				})

				subject.Let(s, func(t *testcase.T) wftemplate.Condition {
					value := storedValue.Get(t)
					return wftemplate.Condition(fmt.Sprintf("and (hasPrefix .%s %q) (hasSuffix .%s %q)",
						varName, value[:8], varName, value[len(value)-8:]))
				})

				s.Then("both functions remain callable", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.True(t, got, "ContextWith is expected to merge with the already registered FuncMap")
				})
			})
		})

		s.When("the context has no workflow runtime in it", func(s *testcase.Spec) {
			execCTX.Let(s, func(t *testcase.T) context.Context {
				return ctx.Get(t) // deliberately not passed through Runtime#Context
			})

			s.Then("it reports that the runtime is missing", func(t *testcase.T) {
				got, err := act(t)
				assert.ErrorIs(t, err, workflow.ErrNoContextRuntime)
				assert.False(t, got)
			})
		})

		s.When("the expression is malformed", func(s *testcase.Spec) {
			subject.LetValue(s, wftemplate.Condition("eq (.name"))

			s.Then("the error is returned instead of a result", func(t *testcase.T) {
				got, err := act(t)
				assert.Error(t, err)
				assert.False(t, got)
			})
		})
	})

	s.Describe("#Validate", func(s *testcase.Spec) {
		var ctx = let.Context(s)

		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Validate(ctx.Get(t))
		})

		s.Then("a parseable expression is accepted", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("the expression is not parseable", func(s *testcase.Spec) {
			// An unclosed paren fails at parse time. Note that arity mistakes
			// such as `eq .name` do parse cleanly and only fail once executed,
			// so Validate is a syntax gate, not a full correctness check.
			subject.LetValue(s, wftemplate.Condition("eq (.name"))

			s.Then("the parse error is returned", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})

		s.When("the expression calls an unregistered function", func(s *testcase.Spec) {
			subject.LetValue(s, wftemplate.Condition(`hasPrefix .name "x"`))

			// Template functions are resolved at parse time, so Validate can
			// reject an unknown function before the Process is ever started.
			s.Then("the unknown function is rejected", func(t *testcase.T) {
				assert.Error(t, act(t))
			})

			s.And("the function is registered on the context", func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context {
					return wftemplate.ContextWith(ctx.Super(t), wftemplate.FuncMap{
						"hasPrefix": strings.HasPrefix,
					})
				})

				s.Then("the expression is accepted", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
			})
		})
	})
}
