package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestDefer(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		eventRepository = let.Var(s, func(t *testcase.T) workflow.EventRepository {
			return &memory.WorkflowEventRepository{}
		})
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		def = let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.Execute{Participant: workflow.ParticipantID(t.Random.UUID())}
		})
		subject = let.Var(s, func(t *testcase.T) workflow.Defer {
			return workflow.Defer{Definition: def.Get(t)}
		})
	)

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			// stepContext makes the context a step receives during the execution of the process.
			stepContext = func(t *testcase.T) context.Context {
				return workflow.Runtime{Events: eventRepository.Get(t)}.Context(t.Context())
			}
			ctx = let.Var(s, func(t *testcase.T) context.Context {
				c := workflow.WithName(stepContext(t), t.Random.UUID())
				return workflow.WithVarScope(c, t.Random.UUID())
			})
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), processID.Get(t))
		}

		hooksOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventDefer {
			t.Helper()
			return deferHooksOf(t, eventRepository.Get(t), pid)
		}

		s.Then("the definition is registered as a deferred hook in the process's event log", func(t *testcase.T) {
			assert.NoError(t, act(t))

			hooks := hooksOf(t, processID.Get(t))
			assert.Equal(t, len(hooks), 1)
			hook := hooks[0]
			assert.NotEmpty(t, hook.EventID)
			assert.Equal(t, hook.ProcessID, processID.Get(t))
			assert.NotEmpty(t, hook.Timestamp)
			assert.Equal(t, hook.Definition, def.Get(t))
		})

		s.Then("the hook records the location of the Defer step: its own path segment, and the variable scope it runs in", func(t *testcase.T) {
			assert.NoError(t, act(t))

			hooks := hooksOf(t, processID.Get(t))
			assert.Equal(t, len(hooks), 1)
			expPath := append(workflow.Path{}, workflow.CurrentPath(ctx.Get(t))...)
			expPath = append(expPath, "defer")
			assert.Equal(t, hooks[0].Path, expPath)
			assert.Equal(t, hooks[0].VarScope, workflow.CurrentVarScope(ctx.Get(t)))
		})

		s.Then("the hook is not registered to other processes", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Empty(t, hooksOf(t, mustProcessID(t)))
		})

		s.Then("repeating the step is idempotent, so a replay doesn't register the hook twice", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))

			assert.Equal(t, len(hooksOf(t, processID.Get(t))), 1)
		})

		s.When("a hook is already registered by another step", func(s *testcase.Spec) {
			othDef := let.Var(s, func(t *testcase.T) workflow.Definition {
				return workflow.Execute{Participant: workflow.ParticipantID(t.Random.UUID())}
			})
			s.Before(func(t *testcase.T) {
				othCtx := workflow.WithName(stepContext(t), t.Random.UUID())
				assert.Must(t).NoError(workflow.Defer{Definition: othDef.Get(t)}.Execute(othCtx, processID.Get(t)))
			})

			s.Then("both hooks are kept, in the order of their registration", func(t *testcase.T) {
				assert.NoError(t, act(t))

				hooks := hooksOf(t, processID.Get(t))
				assert.Equal(t, len(hooks), 2)
				assert.Equal(t, hooks[0].Definition, othDef.Get(t))
				assert.Equal(t, hooks[1].Definition, def.Get(t))
			})
		})

		s.When("the same step is re-entered under a different variable scope, like a loop iteration", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				sibling := workflow.WithVarScope(ctx.Get(t), t.Random.UUID())
				assert.Must(t).NoError(subject.Get(t).Execute(sibling, processID.Get(t)))
			})

			s.Then("each scope registers its own hook", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, len(hooksOf(t, processID.Get(t))), 2)
			})
		})

		s.When("the definition is nil", func(s *testcase.Spec) {
			def.LetValue(s, nil)

			s.Then("there is nothing to defer, so nothing is registered", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Empty(t, hooksOf(t, processID.Get(t)))
			})
		})

		s.When("the context is canceled", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				c, cancel := context.WithCancel(ctx.Super(t))
				cancel()
				return c
			})

			s.Then("it returns the context error and registers nothing", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), context.Canceled)

				assert.Empty(t, hooksOf(t, processID.Get(t)))
			})
		})

		s.When("the context has no runtime", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				return workflow.WithName(t.Context(), t.Random.UUID())
			})

			s.Then("it fails with the missing runtime error, as there is no event log to register the hook to", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrNoContextRuntime)

				assert.Empty(t, hooksOf(t, processID.Get(t)))
			})
		})
	})

	// Sequence{A, Defer{B}, C} runs as A, C, B:
	// the sequence runs its elements, and B, deferred on the way, runs once the process ends.
	s.Context("smoke: a Defer between the steps of a sequence", func(s *testcase.Spec) {
		c := wftest.LetC(s)

		// calls records the order in which the participants run.
		calls := let.Var(s, func(t *testcase.T) []string {
			return nil
		})
		recording := func(name string) func(t *testcase.T) func(context.Context) error {
			return func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					calls.Set(t, append(calls.Get(t), name))
					return nil
				}
			}
		}
		_, participantA := wftest.LetParticipant(s, recording("A"))
		_, participantB := wftest.LetParticipant(s, recording("B"))
		_, participantC := wftest.LetParticipant(s, recording("C"))

		sequence := let.Var(s, func(t *testcase.T) workflow.Sequence {
			return workflow.Sequence{
				workflow.Execute{Participant: participantA.Get(t)},
				workflow.Defer{Definition: workflow.Execute{Participant: participantB.Get(t)}},
				workflow.Execute{Participant: participantC.Get(t)},
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return sequence.Get(t)
		})

		s.Describe("Sequence#Execute", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				ctx := c.Runtime.Get(t).Context(t.Context())
				return sequence.Get(t).Execute(ctx, c.ProcessID.Get(t))
			}

			s.Then("the elements of the sequence run, while the deferred definition is only registered, as it is deferred to the end of the process, not of the sequence", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, calls.Get(t), []string{"A", "C"})
				assert.Equal(t, len(deferHooksOf(t, c.EventRepository.Get(t), c.ProcessID.Get(t))), 1)
			})
		})

		s.Describe("Runtime#Execute", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
			}

			s.Then("the elements of the sequence run first, then the deferred definition runs after them, as the process completes", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, calls.Get(t), []string{"A", "C", "B"})
				assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
			})
		})
	})

	// A spawned child is a Process of its own, the way a goroutine runs a function call of its own,
	// so what the child defers runs when the child ends, and not when its parent does:
	//
	//	defer see("parent-cleanup")
	//	go func() { // joined below
	//		defer see("child-cleanup")
	//		see("child")
	//	}()
	//	join()
	//	see("parent")
	s.Context("smoke: a Defer in a spawned child", func(s *testcase.Spec) {
		c := wftest.LetC(s)

		calls := let.VarOf[[]string](s, nil)
		recording := func(name string) func(t *testcase.T) func(context.Context) error {
			return func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					testcase.Append(t, calls, name)
					return nil
				}
			}
		}
		childBusy := let.Phaser(s)
		_, childStep := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				childBusy.Get(t).Wait()
				testcase.Append(t, calls, "child")
				return nil
			}
		})
		_, childCleanup := wftest.LetParticipant(s, recording("child-cleanup"))
		_, parentStep := wftest.LetParticipant(s, recording("parent"))
		_, parentCleanup := wftest.LetParticipant(s, recording("parent-cleanup"))

		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.Defer{Definition: workflow.Execute{Participant: parentCleanup.Get(t)}},
				workflow.Spawn{
					Name: "child",
					Definition: workflow.Sequence{
						workflow.Defer{Definition: workflow.Execute{Participant: childCleanup.Get(t)}},
						workflow.Execute{Participant: childStep.Get(t)},
					},
				},
				workflow.Join{Spawn: "child"},
				workflow.Execute{Participant: parentStep.Get(t)},
			}
		})

		s.Describe("Runtime#Execute", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
			}
			// untilCompleted resumes the parent until the joined child let it complete.
			untilCompleted := func(t *testcase.T) {
				t.Helper()
				t.Eventually(func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
			}

			s.Then("the child runs its deferred definition as the child ends, before the parent that joins it goes on", func(t *testcase.T) {
				childBusy.Get(t).Finish()
				untilCompleted(t)

				assert.Equal(t, calls.Get(t), []string{"child", "child-cleanup", "parent", "parent-cleanup"})
			})

			s.Then("each Process holds the deferred definitions of its own", func(t *testcase.T) {
				childBusy.Get(t).Finish()
				untilCompleted(t)

				parentHooks := deferHooksOf(t, c.EventRepository.Get(t), c.ProcessID.Get(t))
				assert.Equal(t, len(parentHooks), 1)
				assert.Equal[workflow.Definition](t, parentHooks[0].Definition, workflow.Execute{Participant: parentCleanup.Get(t)})

				var children int
				for childID := range wftest.IterChildren(t, c.EventRepository.Get(t), c.ProcessID.Get(t)) {
					children++
					childHooks := deferHooksOf(t, c.EventRepository.Get(t), childID)
					assert.Equal(t, len(childHooks), 1)
					assert.Equal[workflow.Definition](t, childHooks[0].Definition, workflow.Execute{Participant: childCleanup.Get(t)})
				}
				assert.Equal(t, children, 1)
			})

			s.When("the child is still busy", func(s *testcase.Spec) {
				// childBusy is left unfinished, so the child's step keeps waiting on it.
				s.Before(func(t *testcase.T) {
					t.Cleanup(childBusy.Get(t).Finish)
				})

				s.Then("neither Process has ended, so none of the deferred definitions runs yet", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})

					c.WaitForSpawn(t, c.ProcessID.Get(t))
					assert.Empty(t, calls.Get(t))
				})
			})
		})
	})

	s.Context("unwinding, as the process ends", func(s *testcase.Spec) {
		c := wftest.LetC(s)

		// deferredExecution records one execution of a deferred Definition.
		type deferredExecution struct {
			Name string
			// Ended tells whether the process already had its outcome on record
			// at the time the deferred Definition ran.
			Ended bool
		}
		var executions = let.Var(s, func(t *testcase.T) []deferredExecution {
			return nil
		})
		record := func(t *testcase.T, ctx context.Context, name string) error {
			completed, err := workflow.IsCompleted(ctx, c.EventRepository.Get(t), c.ProcessID.Get(t))
			if err != nil {
				return err
			}
			terminated, err := workflow.IsTerminated(ctx, c.EventRepository.Get(t), c.ProcessID.Get(t))
			if err != nil {
				return err
			}
			executions.Set(t, append(executions.Get(t), deferredExecution{
				Name:  name,
				Ended: completed || terminated,
			}))
			return nil
		}
		executed := func(t *testcase.T) []string {
			t.Helper()
			var names []string
			for _, e := range executions.Get(t) {
				names = append(names, e.Name)
			}
			return names
		}
		// deferred makes a Definition that records every one of its executions.
		// It is deliberately not an idempotent step,
		// so a repeated execution can't hide behind a replay from the event log.
		deferred := func(t *testcase.T, name string) workflow.Definition {
			return wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				return record(t, ctx, name)
			}}
		}
		thenTheyRanBeforeTheOutcome := func(t *testcase.T) {
			t.Helper()
			assert.NotEmpty(t, executions.Get(t))
			for _, e := range executions.Get(t) {
				assert.False(t, e.Ended, assert.MessageF(
					"the %q deferred definition ran after the process's outcome was already on record; "+
						"an outcome must mean that nothing is left to run for the process", e.Name))
			}
		}
		isTerminated := func(t *testcase.T) bool {
			t.Helper()
			ok, err := workflow.IsTerminated(t.Context(), c.EventRepository.Get(t), c.ProcessID.Get(t))
			assert.NoError(t, err)
			return ok
		}

		var (
			firstDeferred = let.Var(s, func(t *testcase.T) workflow.Definition {
				return deferred(t, "first")
			})
			secondDeferred = let.Var(s, func(t *testcase.T) workflow.Definition {
				return deferred(t, "second")
			})
			// ending is what the last step of the process returns.
			ending = let.VarOf[error](s, nil)
		)
		_, last := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				return ending.Get(t)
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.Defer{Definition: firstDeferred.Get(t)},
				workflow.Defer{Definition: secondDeferred.Get(t)},
				workflow.Execute{Participant: last.Get(t)},
			}
		})

		s.Describe("Runtime#Execute", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
			}

			s.Then("once the process completes, its deferred definitions run in the reverse order of their registration (LIFO)", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, executed(t), []string{"second", "first"})
				assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
			})

			s.Then("they run before the completion is recorded", func(t *testcase.T) {
				assert.NoError(t, act(t))

				thenTheyRanBeforeTheOutcome(t)
			})

			s.Then("re-executing the completed process doesn't run them again", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				assert.Equal(t, executed(t), []string{"second", "first"})
			})

			s.When("the execution suspends before the process ends", func(s *testcase.Spec) {
				ending.Let(s, func(t *testcase.T) error {
					return workflow.Suspend{}
				})

				s.Then("the deferred definitions are kept for later, as the process hasn't ended yet", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})

					assert.Empty(t, executions.Get(t))
					assert.Equal(t, len(deferHooksOf(t, c.EventRepository.Get(t), c.ProcessID.Get(t))), 2)
				})
			})

			s.When("a step completes the process early with workflow.Complete", func(s *testcase.Spec) {
				ending.Let(s, func(t *testcase.T) error {
					return workflow.Complete{}
				})

				s.Then("the deferred definitions run in LIFO order, before the completion is recorded", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
					thenTheyRanBeforeTheOutcome(t)
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})
			})

			s.When("a step terminates the process with workflow.Terminate", func(s *testcase.Spec) {
				ending.Let(s, func(t *testcase.T) error {
					return workflow.Terminate{}
				})

				s.Then("the deferred definitions still run in LIFO order, before the termination is recorded", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
					thenTheyRanBeforeTheOutcome(t)
					assert.True(t, isTerminated(t))
				})

				s.Then("re-executing the terminated process doesn't run them again", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
				})
			})

			s.When("a participant defers a cleanup by returning a Defer as its follow-up", func(s *testcase.Spec) {
				_, reserve := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
					return func(ctx context.Context) error {
						return workflow.Defer{Definition: firstDeferred.Get(t)}
					}
				})
				c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.Execute{Participant: reserve.Get(t)},
						workflow.Defer{Definition: secondDeferred.Get(t)},
						workflow.Execute{Participant: last.Get(t)},
					}
				})

				s.Then("the cleanup is registered along with the participant's call, and unwinds with the rest, in LIFO order", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
					thenTheyRanBeforeTheOutcome(t)
				})
			})

			s.When("a deferred definition registers a deferred definition of its own", func(s *testcase.Spec) {
				secondDeferred.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						deferred(t, "second"),
						workflow.Defer{Definition: deferred(t, "third")},
					}
				})

				s.Then("it runs right after the one that registered it, like a nested defer in Go", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "third", "first"})
					thenTheyRanBeforeTheOutcome(t)
				})
			})

			s.When("a deferred definition ends itself with workflow.Complete or workflow.Terminate", func(s *testcase.Spec) {
				signal := let.Var(s, func(t *testcase.T) error {
					if t.Random.Bool() {
						return workflow.Complete{}
					}
					return workflow.Terminate{}
				})
				secondDeferred.Let(s, func(t *testcase.T) workflow.Definition {
					return wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						if err := record(t, ctx, "second"); err != nil {
							return err
						}
						return signal.Get(t)
					}}
				})

				s.Then("it only ends that deferred definition, so the rest still run and the process keeps its outcome", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})
			})

			s.When("a deferred definition fails", func(s *testcase.Spec) {
				var (
					expErr = let.Error(s)
					// failures is how many more times the first deferred definition fails.
					failures = let.VarOf(s, 1)
				)
				_, recordSecond := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
					return func(ctx context.Context) error {
						return record(t, ctx, "second")
					}
				})
				_, recordFirst := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
					return func(ctx context.Context) error {
						if err := record(t, ctx, "first"); err != nil {
							return err
						}
						if 0 < failures.Get(t) {
							failures.Set(t, failures.Get(t)-1)
							return expErr.Get(t)
						}
						return nil
					}
				})
				// This time, the deferred definitions are idempotent steps,
				// just like the ones a participant would normally defer.
				secondDeferred.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Execute{Participant: recordSecond.Get(t)}
				})
				firstDeferred.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Execute{Participant: recordFirst.Get(t)}
				})

				s.Then("the failure is returned, and the completion is not recorded", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))

					assert.False(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})

				s.Then("the next attempt resumes the unwinding, retrying the failed step without repeating the finished ones", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first", "first"})
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})
			})
		})

		s.Describe("Runtime#Terminate", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return c.Runtime.Get(t).Terminate(t.Context(), c.ProcessID.Get(t))
			}

			// the process registers its deferred definitions, then waits at its last step
			ending.Let(s, func(t *testcase.T) error {
				return workflow.Suspend{}
			})
			s.Before(func(t *testcase.T) {
				assert.Must(t).ErrorIs(c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t)), workflow.Suspend{})
				assert.Must(t).Empty(executions.Get(t))
			})

			s.Then("the deferred definitions run in the reverse order of their registration (LIFO)", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, executed(t), []string{"second", "first"})
				assert.True(t, isTerminated(t))
			})

			s.Then("they run before the termination is recorded", func(t *testcase.T) {
				assert.NoError(t, act(t))

				thenTheyRanBeforeTheOutcome(t)
			})

			s.Then("terminating the process again doesn't run them again", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				assert.Equal(t, executed(t), []string{"second", "first"})
			})

			s.When("a deferred definition fails", func(s *testcase.Spec) {
				expErr := let.Error(s)
				secondDeferred.Let(s, func(t *testcase.T) workflow.Definition {
					return wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						return expErr.Get(t)
					}}
				})

				s.Then("the failure is returned, and the termination is not recorded, so it can be retried", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))

					assert.False(t, isTerminated(t))
				})
			})

			s.When("the process has already completed", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					ending.Set(t, nil)
					assert.Must(t).NoError(c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t)))
				})

				s.Then("its deferred definitions, which ran at the completion, don't run again", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"second", "first"})
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})
			})
		})

		// The idiom of Defer, like Go's `defer f.Close()` right after a successful os.Open:
		// act first, then defer the cleanup of the act.
		s.Context("a cleanup deferred right after the step whose effect it cleans up", func(s *testcase.Spec) {
			var (
				commitErr = let.Error(s)
				// outage makes every commit of the event log fail while it is set.
				outage = let.Var(s, func(t *testcase.T) *bool { return new(bool) })
				// outages is how many more times the event log goes down right after the reservation took effect.
				outages = let.VarOf(s, 0)
				// followUp makes the step defer its cleanup itself, by returning the Defer as its follow-up,
				// instead of having a Defer step right after it.
				followUp = let.VarOf(s, false)
				release  = let.Var(s, func(t *testcase.T) workflow.Definition {
					return deferred(t, "release")
				})
			)
			c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := c.Runtime.Super(t)
				rt.Events = failingCommitEventRepository{
					WorkflowEventRepository: c.EventRepository.Get(t),
					Err:                     commitErr.Get(t),
					Armed:                   outage.Get(t),
				}
				return rt
			})
			// reserve is the step that acts: its reservation takes effect every time it is called.
			_, reserve := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					if err := record(t, ctx, "reserve"); err != nil {
						return err
					}
					// the immediate undo, in case this call fails to be recorded
					if err := txkit.OnRollback(ctx, func() {
						executions.Set(t, append(executions.Get(t), deferredExecution{Name: "undo reserve"}))
					}); err != nil {
						return err
					}
					if 0 < outages.Get(t) {
						outages.Set(t, outages.Get(t)-1)
						*outage.Get(t) = true
					}
					if followUp.Get(t) {
						return workflow.Defer{Definition: release.Get(t)}
					}
					return nil
				}
			})
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				if followUp.Get(t) {
					return workflow.Sequence{
						workflow.Execute{Participant: reserve.Get(t)},
						workflow.Execute{Participant: last.Get(t)},
					}
				}
				return workflow.Sequence{
					workflow.Execute{Participant: reserve.Get(t)},
					workflow.Defer{Definition: release.Get(t)},
					workflow.Execute{Participant: last.Get(t)},
				}
			})
			// theCallFailsToBeRecorded arranges a first attempt in which the reservation takes effect,
			// but the event log goes down right after, so the call is never recorded.
			theCallFailsToBeRecorded := func(s *testcase.Spec) {
				outages.LetValue(s, 1)
				s.Before(func(t *testcase.T) {
					assert.Must(t).ErrorIs(c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t)), commitErr.Get(t))
					*outage.Get(t) = false // the event log recovers
				})
			}

			s.Describe("Runtime#Execute", func(s *testcase.Spec) {
				act := func(t *testcase.T) error {
					return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
				}

				s.Then("the cleanup is registered once the reservation took effect, and it runs when the process completes", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"reserve", "release"})
					thenTheyRanBeforeTheOutcome(t)
					assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
				})
			})

			s.Describe("Runtime#Terminate", func(s *testcase.Spec) {
				act := func(t *testcase.T) error {
					return c.Runtime.Get(t).Terminate(t.Context(), c.ProcessID.Get(t))
				}

				// the process makes the reservation, defers its cleanup, then waits at its last step
				ending.Let(s, func(t *testcase.T) error {
					return workflow.Suspend{}
				})
				s.Before(func(t *testcase.T) {
					assert.Must(t).ErrorIs(c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t)), workflow.Suspend{})
				})

				s.Then("the reservation is cleaned up", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, executed(t), []string{"reserve", "release"})
					thenTheyRanBeforeTheOutcome(t)
					assert.True(t, isTerminated(t))
				})
			})

			s.When("the reservation takes effect, but its call fails to be recorded", func(s *testcase.Spec) {
				theCallFailsToBeRecorded(s)

				s.Describe("Runtime#Terminate", func(s *testcase.Spec) {
					act := func(t *testcase.T) error {
						return c.Runtime.Get(t).Terminate(t.Context(), c.ProcessID.Get(t))
					}

					s.Then("no cleanup runs, as the process never got past the unrecorded step; txkit.OnRollback undid the reservation instead", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, executed(t), []string{"reserve", "undo reserve"})
						assert.Empty(t, deferHooksOf(t, c.EventRepository.Get(t), c.ProcessID.Get(t)))
						assert.True(t, isTerminated(t))
					})
				})

				s.Describe("Runtime#Execute", func(s *testcase.Spec) {
					act := func(t *testcase.T) error {
						return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
					}

					s.Then("the step is retried, and once its call is on record, its cleanup is registered and runs when the process completes", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, executed(t), []string{"reserve", "undo reserve", "reserve", "release"})
						thenTheyRanBeforeTheOutcome(t)
						assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
					})
				})
			})

			s.When("the step defers the cleanup itself, as its follow-up, but its call fails to be recorded", func(s *testcase.Spec) {
				followUp.LetValue(s, true)
				theCallFailsToBeRecorded(s)

				s.Describe("Runtime#Terminate", func(s *testcase.Spec) {
					act := func(t *testcase.T) error {
						return c.Runtime.Get(t).Terminate(t.Context(), c.ProcessID.Get(t))
					}

					s.Then("the registration is lost along with the call it was committed with, so no cleanup runs; txkit.OnRollback undid the reservation instead", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, executed(t), []string{"reserve", "undo reserve"})
						assert.Empty(t, deferHooksOf(t, c.EventRepository.Get(t), c.ProcessID.Get(t)))
						assert.True(t, isTerminated(t))
					})
				})

				s.Describe("Runtime#Execute", func(s *testcase.Spec) {
					act := func(t *testcase.T) error {
						return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
					}

					s.Then("the step is retried, and its recorded call registers the cleanup, which runs when the process completes", func(t *testcase.T) {
						assert.NoError(t, act(t))

						assert.Equal(t, executed(t), []string{"reserve", "undo reserve", "reserve", "release"})
						thenTheyRanBeforeTheOutcome(t)
						assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
					})
				})
			})
		})
	})
}

func TestEventDefer(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		eventRepository = let.Var(s, func(t *testcase.T) workflow.EventRepository {
			return &memory.WorkflowEventRepository{}
		})
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		declaredPath = let.Var(s, func(t *testcase.T) workflow.Path {
			return workflow.Path{t.Random.UUID(), t.Random.UUID(), t.Random.UUID()}
		})
		declaredScope = let.Var(s, func(t *testcase.T) workflow.VarScope {
			return workflow.VarScope{t.Random.UUID(), t.Random.UUID()}
		})
		subject = let.Var(s, func(t *testcase.T) workflow.EventDefer {
			id, err := workflow.MakeEventID()
			assert.Must(t).NoError(err)
			return workflow.EventDefer{
				EventID:   id,
				ProcessID: processID.Get(t),
				Path:      declaredPath.Get(t),
				VarScope:  declaredScope.Get(t),
			}
		})
	)

	s.Describe("#Context", func(s *testcase.Spec) {
		ctx := let.Var(s, func(t *testcase.T) context.Context {
			return t.Context()
		})
		act := func(t *testcase.T) context.Context {
			return subject.Get(t).Context(ctx.Get(t))
		}

		s.Then("the hooked definition runs under the declaration's path, in a segment named after the hook's event id", func(t *testcase.T) {
			exp := append(workflow.Path{}, declaredPath.Get(t)...)
			exp = append(exp, subject.Get(t).EventID.String())
			assert.Equal(t, workflow.CurrentPath(act(t)), exp)
		})

		s.Then("the hooked definition runs under the declaration's variable scope, in a scope named after the hook's event id", func(t *testcase.T) {
			exp := append(workflow.VarScope{}, declaredScope.Get(t)...)
			exp = append(exp, subject.Get(t).EventID.String())
			assert.Equal(t, workflow.CurrentVarScope(act(t)), exp)
		})

		s.Then("the hook's own path and variable scope are left untouched", func(t *testcase.T) {
			_ = act(t)
			assert.Equal(t, subject.Get(t).Path, declaredPath.Get(t))
			assert.Equal(t, subject.Get(t).VarScope, declaredScope.Get(t))
		})

		s.When("multiple hooks are declared at the same location", func(s *testcase.Spec) {
			othHook := let.Var(s, func(t *testcase.T) workflow.EventDefer {
				id, err := workflow.MakeEventID()
				assert.Must(t).NoError(err)
				h := subject.Get(t)
				h.EventID = id
				return h
			})

			s.Then("each hook runs on a distinct path, so their idempotent steps don't collide", func(t *testcase.T) {
				assert.NotEqual(t, workflow.CurrentPath(act(t)), workflow.CurrentPath(othHook.Get(t).Context(ctx.Get(t))))
			})

			s.Then("each hook runs in a distinct variable scope, so their local variables don't collide", func(t *testcase.T) {
				assert.NotEqual(t, workflow.CurrentVarScope(act(t)), workflow.CurrentVarScope(othHook.Get(t).Context(ctx.Get(t))))
			})
		})

		s.When("the context already carries a path and a variable scope, like the one where the process finishes", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				c := workflow.WithName(t.Context(), t.Random.UUID())
				return workflow.WithVarScope(c, t.Random.UUID())
			})

			s.Then("they are replaced by the declaration's location rather than nested under it", func(t *testcase.T) {
				assert.Equal(t, workflow.CurrentPath(act(t))[:len(declaredPath.Get(t))], declaredPath.Get(t))
				assert.Equal(t, len(workflow.CurrentPath(act(t))), len(declaredPath.Get(t))+1)
				assert.Equal(t, workflow.CurrentVarScope(act(t))[:len(declaredScope.Get(t))], declaredScope.Get(t))
				assert.Equal(t, len(workflow.CurrentVarScope(act(t))), len(declaredScope.Get(t))+1)
			})

			s.And("the hook was declared in the process's root variable scope", func(s *testcase.Spec) {
				declaredScope.LetValue(s, nil)

				s.Then("the hooked definition's scope is nested directly under the root", func(t *testcase.T) {
					assert.Equal(t, workflow.CurrentVarScope(act(t)), workflow.VarScope{subject.Get(t).EventID.String()})
				})
			})
		})

		s.Test("a hook registered by a workflow.Defer step runs within the location of that step", func(t *testcase.T) {
			declCtx := workflow.Runtime{Events: eventRepository.Get(t)}.Context(t.Context())
			for _, name := range declaredPath.Get(t) {
				declCtx = workflow.WithName(declCtx, name)
			}
			for _, name := range declaredScope.Get(t) {
				declCtx = workflow.WithVarScope(declCtx, name)
			}
			assert.NoError(t, workflow.Defer{Definition: workflow.Sequence{}}.Execute(declCtx, processID.Get(t)))

			hooks := deferHooksOf(t, eventRepository.Get(t), processID.Get(t))
			assert.Equal(t, len(hooks), 1)
			runCtx := hooks[0].Context(t.Context())

			exp := append(workflow.Path{}, declaredPath.Get(t)...)
			exp = append(exp, "defer", hooks[0].EventID.String())
			assert.Equal(t, workflow.CurrentPath(runCtx), exp)

			expScope := append(workflow.VarScope{}, declaredScope.Get(t)...)
			expScope = append(expScope, hooks[0].EventID.String())
			assert.Equal(t, workflow.CurrentVarScope(runCtx), expScope)
		})

		s.Test("the hooked definition sees the variables of its declaration, but its own writes stay local to the hook", func(t *testcase.T) {
			vars := workflow.Vars{ProcessID: processID.Get(t), EventsRepository: eventRepository.Get(t)}

			declCtx := t.Context()
			for _, name := range declaredScope.Get(t) {
				declCtx = workflow.WithVarScope(declCtx, name)
			}
			var (
				shared = workflow.VarName(t.Random.UUID())
				local  = workflow.VarName(t.Random.UUID())
				value  = t.Random.String()
			)
			assert.NoError(t, vars.Set(declCtx, shared, value))

			runCtx := act(t)
			got, ok, err := vars.Lookup(runCtx, shared)
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal[any](t, got, value)

			assert.NoError(t, vars.Set(runCtx, local, t.Random.String()))
			_, ok, err = vars.Lookup(declCtx, local)
			assert.NoError(t, err)
			assert.False(t, ok, assert.MessageF("a variable first assigned by the hook must not leak into its declaration's scope"))
		})
	})
}

func deferHooksOf(tb testing.TB, repo workflow.EventRepository, pid workflow.ProcessID) []workflow.EventDefer {
	tb.Helper()
	events, err := iterkit.CollectE(repo.FindByProcessID(context.Background(), pid))
	assert.Must(tb).NoError(err)
	var hooks []workflow.EventDefer
	for _, e := range events {
		if h, ok := e.(workflow.EventDefer); ok {
			hooks = append(hooks, h)
		}
	}
	return hooks
}
