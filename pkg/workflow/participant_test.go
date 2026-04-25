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

func TestExecuteParticipant(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	var pid = LetParticipantID(s)

	var (
		callCount = let.VarOf(s, 0)
		lastCTX   = let.VarOf[context.Context](s, nil)
		lastOut   = let.VarOf[string](s, "")
	)
	participant := LetParticipant(s, c, pid, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
		return func(ctx context.Context, in string) (string, error) {
			lastCTX.Set(t, ctx)
			callCount.Set(t, callCount.Get(t)+1)
			out := t.Random.UUID()
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

		outKey = let.As[workflow.VariableKey](let.UUID(s))
		output = let.Var(s, func(t *testcase.T) []workflow.VariableKey {
			return []workflow.VariableKey{outKey.Get(t)}
		})
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
		return &workflow.ExecuteParticipant{
			ID:     pid.Get(t),
			Input:  input.Get(t),
			Output: output.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
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
			return subject.Get(t).Execute(execCTX, process.Get(t))
		})

		s.Then("participant is looked up by its ID and executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, callCount.Get(t), 1)

			gotCTX := lastCTX.Get(t)
			assert.NotNil(t, gotCTX)
			assert.NoError(t, gotCTX.Err())
		})

		s.When("the ExecuteParticipant.ID (pariticipant ID) is invalid", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *workflow.ExecuteParticipant {
				randomPID := workflow.ParticipantID(random.Unique(t.Random.String, string(pid.Get(t))))
				ep := subject.Super(t)
				ep.ID = randomPID
				return ep
			})

			s.Then("we get back a validation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{ID: subject.Get(t).ID})
			})
		})

		s.When("the referenced participant has an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (string, error) {
				return func(ctx context.Context, in string) (string, error) {
					return "", expErr.Get(t)
				}
			})

			s.Then("error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})
		})

		s.When("the participant was executed already", func(s *testcase.Spec) {
			var firstOut = let.Var[string](s, nil)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
				firstOut.Set(t, c.Process.Get(t).Variables.Get(outKey.Get(t)).(string))
				assert.Equal(t, callCount.Get(t), 1)
			})

			s.Then("calling it again will not execute the participant function to ensure idempotent behaviour", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, 1, callCount.Get(t))
			})

			s.And("even if the function would return back always unique values for the same input", func(s *testcase.Spec) {
				var lastIn = let.VarOf[string](s, "")

				participant.Let(s, func(t *testcase.T) func(ctx context.Context, in string) (out string, _ error) {
					return func(ctx context.Context, in string) (out string, _ error) {
						lastIn.Set(t, in)
						callCount.Set(t, 1+callCount.Get(t))
						out = t.Random.UUID()
						lastOut.Set(t, out)
						return out, nil
					}
				})

				s.Then("the execution remains idempotent and the result don't change", func(t *testcase.T) {
					t.Random.Repeat(1, 7, func() {
						assert.NoError(t, act(t))
						gotOut := c.Process.Get(t).Variables.Get(outKey.Get(t)).(string)
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

						assert.Equal(t, 2, callCount.Get(t))
						assert.Equal(t, lastOut.Get(t), c.Process.Get(t).Variables.Get(outKey.Get(t)).(string))
						assert.Equal(t, lastIn.Get(t), newIn.Get(t))
					})
				})
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Context("idempotency", func(s *testcase.Spec) {
			s.Test("same repeation don't execute participants twice", func(t *testcase.T) {

				var (
					fooOut = t.Random.String()
					barOut = t.Random.Int()
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				participants := workflow.Participants{
					"foo": func(ctx context.Context) (string, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (int, error) {
						inc("bar")
						assert.Equal(t, in, fooOut)
						return barOut, nil
					},
					"baz": func(ctx context.Context, s string, n int) error {
						inc("baz")
						assert.Equal(t, fooOut, s)
						assert.Equal(t, barOut, n)
						return nil
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{
						ID:     "foo",
						Output: []workflow.VariableKey{"foo-val"},
					},
					&workflow.ExecuteParticipant{
						ID:     "bar",
						Input:  []workflow.VariableKey{"foo-val"},
						Output: []workflow.VariableKey{"bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID:    "baz",
						Input: []workflow.VariableKey{"foo-val", "bar-val"},
					},
				}

				r := workflow.Runtime{
					Participants: participants,
				}

				var p workflow.Process

				assert.NoError(t, r.Execute(t.Context(), pdef, &p))
				assert.NotEmpty(t, p.Events)
				eventsAfterTheFirstExecution := slicekit.Clone(p.Events)

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
					assert.Equal(t, p.Events, eventsAfterTheFirstExecution)

					assert.Equal[any](t, p.Variables.Get("foo-val"), fooOut)
					assert.Equal[any](t, p.Variables.Get("bar-val"), barOut)
					assert.Equal(t, ranCount["foo"], 1)
					assert.Equal(t, ranCount["bar"], 1)
					assert.Equal(t, ranCount["baz"], 1)
				})

			})

			s.Test("repeating the same participant execution at definition level is supported", func(t *testcase.T) {
				var ran int

				participants := workflow.Participants{
					"foo": func(ctx context.Context) error {
						ran++
						return nil
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{ID: "foo"},
					&workflow.ExecuteParticipant{ID: "foo"},
					&workflow.ExecuteParticipant{ID: "foo"},
				}

				r := workflow.Runtime{
					Participants: participants,
				}

				var p workflow.Process

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
				assert.NotEmpty(t, p.Events)
				eventsAfterTheFirstExecution := slicekit.Clone(p.Events)

				assert.Equal(t, ran, 3, "expected that the 3 individiual foo participant call will all execute, since they are referenced multiple times in the definition")

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
					assert.Equal(t, p.Events, eventsAfterTheFirstExecution)
					assert.Equal(t, ran, 3, "after the initial call, the execution should remain idempotent")
				})
			})

			s.Test("upon failure, restarting the execution will continue from the last successful point", func(t *testcase.T) {
				var (
					fooOut = t.Random.String()
					barOut = t.Random.Int()

					expectedFlakyErr = t.Random.Error()
					failOnce         sync.Once
				)

				var ranCount = map[string]int{}
				var inc = func(n string) {
					ranCount[n] = ranCount[n] + 1
				}

				participants := workflow.Participants{
					"foo": func(ctx context.Context) (string, error) {
						inc("foo")
						return fooOut, nil
					},
					"bar": func(ctx context.Context, in string) (int, error) {
						inc("bar")
						assert.Equal(t, in, fooOut)
						return barOut, nil
					},
					"baz": func(ctx context.Context, s string, n int) error {
						inc("baz")
						assert.Equal(t, fooOut, s)
						assert.Equal(t, barOut, n)
						return nil
					},
					"flaky": func(ctx context.Context) (err error) {
						inc("flaky")
						failOnce.Do(func() {
							err = expectedFlakyErr
						})
						return err
					},
				}

				var pdef workflow.Definition = &workflow.Sequence{
					&workflow.ExecuteParticipant{
						ID:     "foo",
						Output: []workflow.VariableKey{"foo-val"},
					},
					&workflow.ExecuteParticipant{
						ID:     "bar",
						Input:  []workflow.VariableKey{"foo-val"},
						Output: []workflow.VariableKey{"bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID:    "baz",
						Input: []workflow.VariableKey{"foo-val", "bar-val"},
					},
					&workflow.ExecuteParticipant{
						ID: "flaky",
						//TODO: retry integration maybe?
					},
				}

				r := workflow.Runtime{
					Participants: participants,
				}

				var p workflow.Process

				assert.ErrorIs(t, expectedFlakyErr, pdef.Execute(r.Context(t.Context()), &p))
				assert.NotEmpty(t, p.Events)

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
				assert.Equal[any](t, p.Variables.Get("foo-val"), fooOut)
				assert.Equal[any](t, p.Variables.Get("bar-val"), barOut)
				assert.Equal(t, ranCount["foo"], 1)
				assert.Equal(t, ranCount["bar"], 1)
				assert.Equal(t, ranCount["baz"], 1)
				assert.Equal(t, ranCount["flaky"], 2)
			})
		})
	})
}
