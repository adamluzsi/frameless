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
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

func TestExecuteParticipant(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	var fooPID = LetParticipantID(s)

	var fooP = LetParticipant(s, c, fooPID, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return nil
		}
	})

	var (
		args   = let.VarOf[[]workflow.VariableKey](s, nil)
		output = let.VarOf[[]workflow.VariableKey](s, nil)
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
		return &workflow.ExecuteParticipant{
			ID:     fooPID.Get(t),
			Input:  args.Get(t),
			Output: output.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.Context.Bind(s)
			process = c.Process.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			sub := subject.Get(t)
			pp.PP(sub)
			ctxV := ctx.Get(t)
			pp.PP(ctxV)
			process.Get(t)
			pp.PP()
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("participant is looked up by its PID", func(t *testcase.T) {
			n := t.Random.Repeat(3, 7, func() {
				assert.NoError(t, act(t))
			})

			assert.Equal(t, stub.Get(t).CallCount, n)
			gotCtx, ok := stub.Get(t).LastExecutedWith()
			assert.True(t, ok)
			assert.Equal(t, ctx.Get(t), gotCtx)
		})

		s.When("the pid is invalid in the given context", func(s *testcase.Spec) {
			fooPID.Let(s, func(t *testcase.T) workflow.ParticipantID {
				validPID := fooPID.Super(t)
				randomPID := random.Unique(t.Random.String, string(validPID))
				return workflow.ParticipantID(randomPID)
			})

			s.Then("we get back a validation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{ID: fooPID.Get(t)})
			})
		})

		s.When("the referenced participant has an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			stub.Let(s, func(t *testcase.T) *StubParticipant {
				stub := stub.Super(t)
				stub.Stub = func(ctx context.Context, p *workflow.Process) error {
					return expErr.Get(t)
				}
				return stub
			})

			s.Then("error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
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

				assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
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
