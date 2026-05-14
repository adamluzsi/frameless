package workflow_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
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

func TestScheduler(t *testing.T) {
	s := testcase.NewSpec(t)
	c := letC(s)

	var (
		pid  = LetParticipantID(s)
		stub = c.LetStub(s, pid)
	)

	subject := c.Scheduler.Bind(s)

	s.Describe("#Schedule and #Run", func(s *testcase.Spec) {
		var (
			Context = let.Context(s)
			process = LetProcess(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Schedule(Context.Get(t), process.Get(t))
		})

		s.When("process's definition succeeds without an issue", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("then upon scheduling, eventually Schedule#Run will process the process task", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
				})
			})
		})

		s.When("process has no ID", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.ID = workflow.ProcessID{} // zero value
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("a new ID is generated and assigned to the process", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.False(t, process.Get(t).ID.IsZero())
				})
			})
		})

		s.When("process is scheduled multiple times", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("scheduling remains idempotent and the participant is called only once", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
				})
			})
		})

		s.When("process definition is a sequence", func(s *testcase.Spec) {
			var (
				pid2  = LetParticipantID(s)
				stub2 = c.LetStub(s, pid2)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
					workflow.ExecuteParticipant{ID: pid2.Get(t)},
				}
				return p
			})

			s.Then("all participants in the sequence are executed", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
					assert.Equal(t, stub2.Get(t).CallCount(), 1)
				})
			})
		})

		s.When("process definition is an If with true condition", func(s *testcase.Spec) {
			var (
				thenPid  = LetParticipantID(s)
				elsePid  = LetParticipantID(s)
				thenStub = c.LetStub(s, thenPid)
				elseStub = c.LetStub(s, elsePid)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = &workflow.If{
					Cond: wftemplate.Condition("true"),
					Then: workflow.ExecuteParticipant{ID: thenPid.Get(t)},
					Else: workflow.ExecuteParticipant{ID: elsePid.Get(t)},
				}
				return p
			})

			s.Then("the Then branch is executed", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, thenStub.Get(t).CallCount(), 1)
					assert.Equal(t, elseStub.Get(t).CallCount(), 0)
				})
			})
		})

		s.When("process definition is an If with false condition", func(s *testcase.Spec) {
			var (
				thenPid  = LetParticipantID(s)
				elsePid  = LetParticipantID(s)
				thenStub = c.LetStub(s, thenPid)
				elseStub = c.LetStub(s, elsePid)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = &workflow.If{
					Cond: wftemplate.Condition("false"),
					Then: workflow.ExecuteParticipant{ID: thenPid.Get(t)},
					Else: workflow.ExecuteParticipant{ID: elsePid.Get(t)},
				}
				return p
			})

			s.Then("the Else branch is executed", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, thenStub.Get(t).CallCount(), 0)
					assert.Equal(t, elseStub.Get(t).CallCount(), 1)
				})
			})
		})

		s.When("process definition suspends", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
					workflow.Suspend{While: wftemplate.Condition("true")},
				}
				return p
			})

			s.Then("the participant is executed but process remains incomplete", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
					assert.False(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("context is cancelled during scheduling", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // cancel immediately
				return ctx
			})

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("scheduling fails with context cancellation error", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, context.Canceled)
			})
		})
	})

	s.Describe("#Run", func(s *testcase.Spec) {
		var (
			Context = c.LetContext(s)
			process = LetProcess(s)
		)

		s.When("a process is scheduled and processed", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("the process is executed and marked as completed", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("a process suspends during execution", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
					workflow.Suspend{While: wftemplate.Condition("true")},
				}
				return p
			})

			s.Then("the participant is executed but process remains suspended", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
					assert.False(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("multiple processes are scheduled concurrently", func(s *testcase.Spec) {
			var (
				pid2  = LetParticipantID(s)
				stub2 = c.LetStub(s, pid2)
				pid3  = LetParticipantID(s)
				stub3 = c.LetStub(s, pid3)
			)

			process1 := LetProcess(s)
			process2 := LetProcess(s)
			process3 := LetProcess(s)

			s.Before(func(t *testcase.T) {
				process1.Get(t).Definition = workflow.ExecuteParticipant{ID: pid.Get(t)}
				process2.Get(t).Definition = workflow.ExecuteParticipant{ID: pid2.Get(t)}
				process3.Get(t).Definition = workflow.ExecuteParticipant{ID: pid3.Get(t)}

				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process1.Get(t)))
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process2.Get(t)))
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process3.Get(t)))
			})

			s.Then("all processes are eventually executed", func(t *testcase.T) {
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
					assert.Equal(t, stub2.Get(t).CallCount(), 1)
					assert.Equal(t, stub3.Get(t).CallCount(), 1)

					assert.True(t, workflow.IsCompleted(process1.Get(t)))
					assert.True(t, workflow.IsCompleted(process2.Get(t)))
					assert.True(t, workflow.IsCompleted(process3.Get(t)))
				})
			})
		})

		s.When("process definition fails with a non-suspend error", func(s *testcase.Spec) {
			expErr := let.Error(s)

			stub.Let(s, func(t *testcase.T) *StubParticipant {
				stub := stub.Super(t)
				stub.Err = expErr.Get(t)
				return stub
			})

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("the error is propagated and process remains incomplete", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stub.Get(t).CallCount(), 1)
					assert.False(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("process definition contains SetVar", func(s *testcase.Spec) {
			var (
				key   = let.As[workflow.VariableKey](let.String(s))
				value = let.Int(s)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.SetVar{Key: key.Get(t), Value: value.Get(t)},
					workflow.ExecuteParticipant{ID: pid.Get(t)},
				}
				return p
			})

			s.Then("the variable is set and participant can access it", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					got, ok := process.Get(t).Var().Lookup(key.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, value.Get(t))
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("process definition is empty", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = nil
				return p
			})

			s.Then("the process completes immediately without error", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("process definition is a complex nested structure", func(s *testcase.Spec) {
			var (
				pidA  = LetParticipantID(s)
				stubA = c.LetStub(s, pidA)
				pidB  = LetParticipantID(s)
				stubB = c.LetStub(s, pidB)
				pidC  = LetParticipantID(s)
				stubC = c.LetStub(s, pidC)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = &workflow.If{
					Cond: wftemplate.Condition("true"),
					Then: workflow.Sequence{
						workflow.ExecuteParticipant{ID: pidA.Get(t)},
						&workflow.If{
							Cond: wftemplate.Condition("false"),
							Then: workflow.ExecuteParticipant{ID: pidB.Get(t)},
							Else: workflow.ExecuteParticipant{ID: pidC.Get(t)},
						},
					},
				}
				return p
			})

			s.Then("the complex definition is executed correctly", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, stubA.Get(t).CallCount(), 1)
					assert.Equal(t, stubB.Get(t).CallCount(), 0)
					assert.Equal(t, stubC.Get(t).CallCount(), 1)
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("process is scheduled with context that has custom values", func(s *testcase.Spec) {
			type ctxKey struct{}
			ctxValue := let.String(s)

			Context.Let(s, func(t *testcase.T) context.Context {
				return context.WithValue(context.Background(), ctxKey{}, ctxValue.Get(t))
			})

			var stubCtx = let.VarOf[context.Context](s, nil)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{ID: pid.Get(t)}
				return p
			})

			s.Then("the context values are propagated to the participant", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					gotCtx, ok := stub.Get(t).Last()
					assert.True(t, ok)
					assert.NotNil(t, gotCtx)
					gotValue, ok := gotCtx.Value(ctxKey{}).(string)
					assert.True(t, ok)
					assert.Equal(t, ctxValue.Get(t), gotValue)
				})
			})
		})

		s.Context("validation", func(s *testcase.Spec) {
			s.When("scheduler is nil", func(s *testcase.Spec) {
				var nilScheduler *workflow.Scheduler

				s.Then("Schedule returns a validation error", func(t *testcase.T) {
					err := nilScheduler.Schedule(context.Background(), &workflow.Process{})
					assert.ErrorIs(t, err, workflow.ErrFatal)
				})
			})

			s.When("ProcessSignalQueue is missing", func(s *testcase.Spec) {
				incompleteScheduler := let.Var(s, func(t *testcase.T) *workflow.Scheduler {
					return &workflow.Scheduler{
						Runtime: pointer.Of(c.Runtime.Get(t)),
						// ProcessSignalQueue intentionally nil
					}
				})

				s.Then("Schedule returns a validation error", func(t *testcase.T) {
					err := incompleteScheduler.Get(t).Schedule(context.Background(), &workflow.Process{})
					assert.ErrorIs(t, err, workflow.ErrFatal)
				})
			})

			s.When("ProcessRepository is missing", func(s *testcase.Spec) {
				incompleteScheduler := let.Var(s, func(t *testcase.T) *workflow.Scheduler {
					return &workflow.Scheduler{
						Runtime:            pointer.Of(c.Runtime.Get(t)),
						ProcessSignalQueue: c.ProcessSignalQueue.Get(t),
						// ProcessRepository intentionally nil
					}
				})

				s.Then("Schedule returns a validation error", func(t *testcase.T) {
					err := incompleteScheduler.Get(t).Schedule(context.Background(), &workflow.Process{})
					assert.ErrorIs(t, err, workflow.ErrFatal)
				})
			})
		})
	})

	s.Context("integration scenarios", func(s *testcase.Spec) {
		s.When("a workflow with multiple sequential steps completes successfully", func(s *testcase.Spec) {
			var (
				step1Pid  = LetParticipantID(s)
				step1Stub = c.LetStub(s, step1Pid)
				step2Pid  = LetParticipantID(s)
				step2Stub = c.LetStub(s, step2Pid)
				step3Pid  = LetParticipantID(s)
				step3Stub = c.LetStub(s, step3Pid)
			)

			var callOrder = let.Var(s, func(t *testcase.T) []workflow.ParticipantID {
				return make([]workflow.ParticipantID, 0, 3)
			})

			// Override stubs to track call order
			c.Participants.Let(s, func(t *testcase.T) workflow.Participants {
				ps := make(workflow.Participants)
				ps[step1Pid.Get(t)] = func(ctx context.Context) error {
					testcase.Append(t, callOrder, step1Pid.Get(t))
					return step1Stub.Get(t).Func(ctx)
				}
				ps[step2Pid.Get(t)] = func(ctx context.Context) error {
					testcase.Append(t, callOrder, step2Pid.Get(t))
					return step2Stub.Get(t).Func(ctx)
				}
				ps[step3Pid.Get(t)] = func(ctx context.Context) error {
					testcase.Append(t, callOrder, step3Pid.Get(t))
					return step3Stub.Get(t).Func(ctx)
				}
				return ps
			})

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: step1Pid.Get(t)},
					workflow.ExecuteParticipant{ID: step2Pid.Get(t)},
					workflow.ExecuteParticipant{ID: step3Pid.Get(t)},
				}
				return p
			})

			s.Then("all steps are executed in order", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					expectedOrder := []workflow.ParticipantID{
						step1Pid.Get(t),
						step2Pid.Get(t),
						step3Pid.Get(t),
					}
					assert.Equal(t, expectedOrder, callOrder.Get(t))
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("a conditional workflow executes the correct branch", func(s *testcase.Spec) {
			var (
				truePid   = LetParticipantID(s)
				trueStub  = c.LetStub(s, truePid)
				falsePid  = LetParticipantID(s)
				falseStub = c.LetStub(s, falsePid)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = &workflow.If{
					Cond: wftemplate.Condition("true"),
					Then: workflow.Sequence{
						workflow.ExecuteParticipant{ID: truePid.Get(t)},
					},
					Else: workflow.Sequence{
						workflow.ExecuteParticipant{ID: falsePid.Get(t)},
					},
				}
				return p
			})

			s.Then("only the Then branch is executed", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, trueStub.Get(t).CallCount(), 1)
					assert.Equal(t, falseStub.Get(t).CallCount(), 0)
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("a suspended process is resumed", func(s *testcase.Spec) {
			var (
				beforePid  = LetParticipantID(s)
				beforeStub = c.LetStub(s, beforePid)
				afterPid   = LetParticipantID(s)
				afterStub  = c.LetStub(s, afterPid)
			)

			suspendCount := let.VarOf(s, 0)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: beforePid.Get(t)},
					workflow.Suspend{While: wftemplate.Condition("true")},
					workflow.ExecuteParticipant{ID: afterPid.Get(t)},
				}
				return p
			})

			s.Then("the first participant executes but the second waits", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, beforeStub.Get(t).CallCount(), 1)
					assert.Equal(t, afterStub.Get(t).CallCount(), 0)
					assert.False(t, workflow.IsCompleted(process.Get(t)))
				})
			})

			s.Context("after resuming the suspended process", func(s *testcase.Spec) {
				var resumedProcess = let.Var(s, func(t *testcase.T) *workflow.Process {
					p := process.Super(t)
					// Remove the suspend condition to allow continuation
					p.Definition = workflow.Sequence{
						workflow.ExecuteParticipant{ID: beforePid.Get(t)},
						workflow.ExecuteParticipant{ID: afterPid.Get(t)},
					}
					return p
				})

				s.Then("the remaining steps are executed", func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), resumedProcess.Get(t)))

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, beforeStub.Get(t).CallCount(), 1) // idempotent
						assert.Equal(t, afterStub.Get(t).CallCount(), 1)
						assert.True(t, workflow.IsCompleted(resumedProcess.Get(t)))
					})
				})
			})
		})

		s.When("a workflow with variables executes correctly", func(s *testcase.Spec) {
			var (
				key1 = let.As[workflow.VariableKey](let.String(s))
				val1 = let.Int(s)
				key2 = let.As[workflow.VariableKey](let.String(s))
				val2 = let.String(s)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.SetVar{Key: key1.Get(t), Value: val1.Get(t)},
					workflow.SetVar{Key: key2.Get(t), Value: val2.Get(t)},
					workflow.ExecuteParticipant{ID: pid.Get(t)},
				}
				return p
			})

			s.Then("variables are set and process completes", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					got1, ok1 := process.Get(t).Var().Lookup(key1.Get(t))
					got2, ok2 := process.Get(t).Var().Lookup(key2.Get(t))
					assert.True(t, ok1)
					assert.True(t, ok2)
					assert.Equal[any](t, got1, val1.Get(t))
					assert.Equal[any](t, got2, val2.Get(t))
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("a workflow with nested If statements executes correctly", func(s *testcase.Spec) {
			var (
				outerTruePid   = LetParticipantID(s)
				outerTrueStub  = c.LetStub(s, outerTruePid)
				innerTruePid   = LetParticipantID(s)
				innerTrueStub  = c.LetStub(s, innerTruePid)
				innerFalsePid  = LetParticipantID(s)
				innerFalseStub = c.LetStub(s, innerFalsePid)
			)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = &workflow.If{
					Cond: wftemplate.Condition("true"),
					Then: workflow.Sequence{
						workflow.ExecuteParticipant{ID: outerTruePid.Get(t)},
						&workflow.If{
							Cond: wftemplate.Condition("false"),
							Then: workflow.ExecuteParticipant{ID: innerTruePid.Get(t)},
							Else: workflow.ExecuteParticipant{ID: innerFalsePid.Get(t)},
						},
					},
				}
				return p
			})

			s.Then("nested conditions are evaluated correctly", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, outerTrueStub.Get(t).CallCount(), 1)
					assert.Equal(t, innerTrueStub.Get(t).CallCount(), 0)
					assert.Equal(t, innerFalseStub.Get(t).CallCount(), 1)
					assert.True(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})

		s.When("a workflow with error handling executes correctly", func(s *testcase.Spec) {
			var (
				successPid  = LetParticipantID(s)
				successStub = c.LetStub(s, successPid)
				failPid     = LetParticipantID(s)
				failErr     = let.Error(s)
				failStub    = c.LetStub(s, failPid)
			)

			failStub.Let(s, func(t *testcase.T) *StubParticipant {
				stub := failStub.Super(t)
				stub.Err = failErr.Get(t)
				return stub
			})

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: successPid.Get(t)},
					workflow.ExecuteParticipant{ID: failPid.Get(t)},
				}
				return p
			})

			s.Then("the error stops the sequence and process remains incomplete", func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Schedule(Context.Get(t), process.Get(t)))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, successStub.Get(t).CallCount(), 1)
					assert.Equal(t, failStub.Get(t).CallCount(), 1)
					assert.False(t, workflow.IsCompleted(process.Get(t)))
				})
			})
		})
	})
}
