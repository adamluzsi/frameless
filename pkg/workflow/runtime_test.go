package workflow_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftesting"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleRuntime() {
	_ = workflow.Runtime{
		Participants: workflow.Participants{
			"foo": func(ctx context.Context) (int, error) {
				return 42, nil
			},
			"bar": func(ctx context.Context) (int, error) {
				return 24, nil
			},
			"baz": func(ctx context.Context) error {
				return nil
			},
			"qux": func(ctx context.Context) error {
				return nil
			},
		},
		Conditions: workflow.Conditions{
			"question": func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
		},
		ContextSetup: []func(context.Context) context.Context{
			func(ctx context.Context) context.Context {
				return logging.ContextWith(ctx, logging.Field("workflow", "context"))
			},
		},
	}
}

func TestRuntime(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		participants = let.Var(s, func(t *testcase.T) workflow.Participants {
			return workflow.Participants{}
		})
		conditions = let.Var(s, func(t *testcase.T) workflow.Conditions {
			return workflow.Conditions{}
		})
		contextSetup = let.Var(s, func(t *testcase.T) []func(context.Context) context.Context {
			return nil
		})
	)
	runtime := let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants: participants.Get(t),
			Conditions:   conditions.Get(t),
			ContextSetup: contextSetup.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = let.Context(s)
			process = let.Var(s, func(t *testcase.T) *workflow.Process {
				return &workflow.Process{}
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return runtime.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.When("process doesn't have definition", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = nil
				return p
			})

			s.Then("it will return without any error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("process will be flagged as completed", func(t *testcase.T) {
				act(t)

				assert.True(t, workflow.IsCompleted(process.Get(t)))
			})
		})

		s.When("definition is provided in the process", func(s *testcase.Spec) {
			defRan := let.VarOf(s, false)
			defCtx := let.VarOf[context.Context](s, nil)

			definition := let.Var(s, func(t *testcase.T) workflow.Definition {
				return &wftesting.Stub{StubExecute: func(ctx context.Context, p *workflow.Process) error {
					defCtx.Set(t, ctx)
					assert.NotNil(t, ctx)
					defRan.Set(t, true)
					return ctx.Err()
				}}
			})
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = definition.Get(t)
				return p
			})

			s.Then("the definition is executed", func(t *testcase.T) {
				act(t)

				assert.True(t, defRan.Get(t))
			})

			s.And("context contains values", func(s *testcase.Spec) {
				type ctxKey struct{}
				ctxValue := let.String(s)

				ctx.Let(s, func(t *testcase.T) context.Context {
					return context.WithValue(ctx.Super(t), ctxKey{}, ctxValue.Get(t))
				})

				s.Then("context with its values is passed through", func(t *testcase.T) {
					act(t)

					assert.NotNil(t, defCtx.Get(t))
					got, ok := defCtx.Get(t).Value(ctxKey{}).(string)
					assert.True(t, ok)
					assert.Equal(t, ctxValue.Get(t), got)
				})
			})
		})
	})

	s.Describe("#Context", func(s *testcase.Spec) {
		var (
			key         = let.String(s)
			value       = let.String(s)
			baseContext = let.Var(s, func(t *testcase.T) context.Context {
				return context.WithValue(context.Background(), key.Get(t), value.Get(t))
			})
		)

		act := let.Act(func(t *testcase.T) context.Context {
			return runtime.Get(t).Context(baseContext.Get(t))
		})

		s.Then("a valid context is returned", func(t *testcase.T) {
			got := act(t)
			assert.NotNil(t, got)
			assert.NoError(t, got.Err())
			assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
				select {
				case <-got.Done():
				case <-ctx.Done():
				}
			})
		})

		s.Then("it contains the values from the base context", func(t *testcase.T) {
			got := act(t)
			assert.NotNil(t, got)
			gotValue, ok := got.Value(key.Get(t)).(string)
			assert.True(t, ok, "expected string value")
			assert.Equal(t, gotValue, value.Get(t))
		})

		s.Then("runtime is retrievable from the runtime context", func(t *testcase.T) {
			rt, ok := workflow.RuntimeFromContext(act(t))
			assert.True(t, ok)
			assert.Equal(t, rt, runtime.Get(t))
		})

		s.When("context setup is provided", func(s *testcase.Spec) {
			var (
				csKey   = let.String(s)
				csValue = let.String(s)

				runtimeFoundInContextSetup = let.VarOf(s, false)
			)

			ContextSetup := let.Var(s, func(t *testcase.T) func(ctx context.Context) context.Context {
				return func(ctx context.Context) context.Context {
					if _, ok := workflow.RuntimeFromContext(ctx); ok {
						runtimeFoundInContextSetup.Set(t, true)
					}
					return context.WithValue(ctx, csKey.Get(t), csValue.Get(t))
				}
			})
			runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := runtime.Super(t)
				rt.ContextSetup = append(rt.ContextSetup, ContextSetup.Get(t))
				return rt
			})

			s.Then("context setup is used to set up the runtime context", func(t *testcase.T) {
				got := act(t)

				assert.NotNil(t, got)
				gotVal, ok := got.Value(csKey.Get(t)).(string)
				assert.True(t, ok, "string value expected")
				assert.Equal(t, csValue.Get(t), gotVal)
			})

			s.Then("runtime is retrievable during the context setup already", func(t *testcase.T) {
				_ = act(t)

				assert.True(t, runtimeFoundInContextSetup.Get(t))
			})
		})

		s.When("base context is cancelled", func(s *testcase.Spec) {
			baseContext.Let(s, func(t *testcase.T) context.Context {
				ctx := baseContext.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx
			})

			s.Then("runtime context is cancelled too", func(t *testcase.T) {
				got := act(t)
				assert.Error(t, got.Err())
				assert.Within(t, time.Millisecond, func(ctx context.Context) {
					select {
					case <-got.Done():
					case <-ctx.Done():
					}
				})
			})
		})
	})
}
