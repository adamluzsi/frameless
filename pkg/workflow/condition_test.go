package workflow_test

import (
	"context"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestExecuteCondition(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	var cid = LetConditionID(s)

	var (
		callCount = let.VarOf(s, 0)
		lastCTX   = let.VarOf[context.Context](s, nil)
		lastOut   = let.VarOf[bool](s, false)
	)
	condition := LetCondition(s, c, cid, func(t *testcase.T) func(ctx context.Context, in string) (out bool, _ error) {
		return func(ctx context.Context, in string) (bool, error) {
			lastCTX.Set(t, ctx)
			callCount.Set(t, callCount.Get(t)+1)
			out := t.Random.Bool()
			lastOut.Set(t, out)
			return out, nil
		}
	})

	var (
		inKey = let.As[workflow.VariableKey](let.UUID(s))
		inVal = let.UUID(s)
		input = let.Var(s, func(t *testcase.T) []workflow.VariableKey {
			return []workflow.VariableKey{inKey.Get(t)}
		})
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteCondition {
		return &workflow.ExecuteCondition{
			ID:    cid.Get(t),
			Input: input.Get(t),
		}
	})

	s.Describe("#Evaluate", func(s *testcase.Spec) {
		var (
			ctx     = let.Context(s)
			process = c.Process.Let(s, func(t *testcase.T) *workflow.Process {
				p := c.Process.Super(t)
				p.Variables.Set(inKey.Get(t), inVal.Get(t))
				return p
			})
		)
		act := let.Act(func(t *testcase.T) error {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			_, err := subject.Get(t).Evaluate(execCTX, process.Get(t))
			return err
		})

		getResult := func(t *testcase.T) bool {
			execCTX := c.Runtime.Get(t).Context(ctx.Get(t))
			result, _ := subject.Get(t).Evaluate(execCTX, process.Get(t))
			return result
		}

		s.Then("condition is looked up by its ID and executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, callCount.Get(t), 1)

			gotCTX := lastCTX.Get(t)
			assert.NotNil(t, gotCTX)
			assert.NoError(t, gotCTX.Err())

			gotOut := getResult(t)
			assert.Equal(t, gotOut, lastOut.Get(t))
		})

		s.When("the ExecuteCondition.ID (condition ID) is invalid", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *workflow.ExecuteCondition {
				randomCID := workflow.ConditionID(random.Unique(t.Random.String, string(cid.Get(t))))
				ep := subject.Super(t)
				ep.ID = randomCID
				return ep
			})

			s.Then("we get back a validation error", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, workflow.ErrConditionNotFound{ID: subject.Get(t).ID})
			})
		})

		s.When("the referenced condition has an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			condition.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (bool, error) {
				return func(ctx context.Context, in string) (bool, error) {
					return false, expErr.Get(t)
				}
			})

			s.Then("error is propagated back", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, expErr.Get(t))
			})
		})

		s.When("the condition was executed already", func(s *testcase.Spec) {
			var firstOut = let.VarOf[bool](s, false)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
				firstOut.Set(t, getResult(t))
				assert.Equal(t, callCount.Get(t), 1)
			})

			s.Then("calling it again will not execute the condition function to ensure idempotent behaviour", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
					gotOut := getResult(t)
					assert.Equal(t, firstOut.Get(t), gotOut)
				})

				assert.Equal(t, 1, callCount.Get(t))
			})

			s.And("even if the function would return back always unique values for the same input", func(s *testcase.Spec) {
				var lastIn = let.VarOf[string](s, "")

				condition.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (bool, error) {
					return func(ctx context.Context, in string) (bool, error) {
						lastIn.Set(t, in)
						callCount.Set(t, 1+callCount.Get(t))
						result := t.Random.Bool()
						lastOut.Set(t, result)
						return result, nil
					}
				})

				s.Then("the execution remains idempotent and the result don't change", func(t *testcase.T) {
					t.Random.Repeat(1, 7, func() {
						assert.NoError(t, act(t))
						gotOut := getResult(t)
						assert.Equal(t, firstOut.Get(t), gotOut)
						assert.Equal(t, 1, callCount.Get(t))
					})
				})

				s.Context("but if the input argument changes since the last execution", func(s *testcase.Spec) {
					var newIn = let.UUID(s)
					s.Before(func(t *testcase.T) {
						c.Process.Get(t).Variables.Set(inKey.Get(t), newIn.Get(t))
					})

					s.Then("the execution will occur once again but with the new input", func(t *testcase.T) {
						assert.NoError(t, act(t))
						gotOut := getResult(t)

						assert.Equal(t, 3, callCount.Get(t))
						assert.Equal(t, lastOut.Get(t), gotOut)
						assert.Equal(t, lastIn.Get(t), newIn.Get(t))
					})
				})
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Context("idempotency", func(s *testcase.Spec) {
			s.Test("same repeation don't execute conditions twice", func(t *testcase.T) {

				var (
					fooOut = t.Random.Bool()
					barOut = t.Random.Bool()
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				triggerVal := t.Random.String()

				conditions := workflow.Conditions{
					"foo": func(ctx context.Context) (bool, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (bool, error) {
						inc("bar")
						assert.Equal(t, in, triggerVal)
						return barOut, nil
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteCondition{
						ID: "foo",
					},
					&workflow.ExecuteCondition{
						ID:    "bar",
						Input: []workflow.VariableKey{"trigger-val"},
					},
				}

				r := workflow.Runtime{
					Conditions: conditions,
				}

				var p workflow.Process
				p.Variables.Set("trigger-val", triggerVal)

				assert.NoError(t, r.Execute(t.Context(), pdef, &p))
				assert.NotEmpty(t, p.Events)
				eventsAfterTheFirstExecution := slicekit.Clone(p.Events)

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
					assert.Equal(t, p.Events, eventsAfterTheFirstExecution)

					assert.Equal(t, ranCount["foo"], 1)
					assert.Equal(t, ranCount["bar"], 1)
				})

			})

			s.Test("repeating the same condition execution at definition level is supported", func(t *testcase.T) {
				var ran int

				conditions := workflow.Conditions{
					"foo": func(ctx context.Context) (bool, error) {
						ran++
						return true, nil
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteCondition{ID: "foo"},
					&workflow.ExecuteCondition{ID: "foo"},
					&workflow.ExecuteCondition{ID: "foo"},
				}

				r := workflow.Runtime{
					Conditions: conditions,
				}

				var p workflow.Process

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
				assert.NotEmpty(t, p.Events)
				eventsAfterTheFirstExecution := slicekit.Clone(p.Events)

				assert.Equal(t, ran, 3, "expected that the 3 individiual foo condition call will all execute, since they are referenced multiple times in the definition")

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
					assert.Equal(t, p.Events, eventsAfterTheFirstExecution)
					assert.Equal(t, ran, 3, "after the initial call, the execution should remain idempotent")
				})
			})

			s.Test("upon failure, restarting the execution will continue from the last successful point", func(t *testcase.T) {
				var (
					fooOut = t.Random.Bool()
					barOut = t.Random.Bool()

					expectedFlakyErr = t.Random.Error()
					failOnce         sync.Once
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				triggerVal := t.Random.String()

				conditions := workflow.Conditions{
					"foo": func(ctx context.Context) (bool, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (bool, error) {
						inc("bar")
						assert.Equal(t, in, triggerVal)
						return barOut, nil
					},
					"flaky": func(ctx context.Context) (bool, error) {
						inc("flaky")
						var err error
						failOnce.Do(func() {
							err = expectedFlakyErr
						})
						return false, err
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteCondition{
						ID: "foo",
					},
					&workflow.ExecuteCondition{
						ID:    "bar",
						Input: []workflow.VariableKey{"trigger-val"},
					},
					&workflow.ExecuteCondition{
						ID: "flaky",
					},
				}

				r := workflow.Runtime{
					Conditions: conditions,
				}

				var p workflow.Process
				p.Variables.Set("trigger-val", triggerVal)

				assert.ErrorIs(t, expectedFlakyErr, pdef.Execute(r.Context(t.Context()), &p))
				assert.NotEmpty(t, p.Events)

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
				assert.Equal(t, ranCount["foo"], 1)
				assert.Equal(t, ranCount["bar"], 1)
				assert.Equal(t, ranCount["flaky"], 2)
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

func LetCondition[Func any](s *testcase.Spec, c C, cid testcase.Var[workflow.ConditionID], mk func(t *testcase.T) Func) testcase.Var[Func] {
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
