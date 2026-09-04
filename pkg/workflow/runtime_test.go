package workflow_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
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
		participants = wftest.Participants.Let(s, func(t *testcase.T) workflow.Participants {
			return workflow.Participants{
				"/dev/null": func(ctx context.Context) error { return nil },
			}
		})
		conditions = wftest.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
			return workflow.Conditions{}
		})
		contextSetup = let.Var(s, func(t *testcase.T) []func(context.Context) context.Context {
			return nil
		})
	)
	runtime := wftest.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants: participants.Get(t),
			Conditions:   conditions.Get(t),
			ContextSetup: contextSetup.Get(t),
			Events:       &memory.WorkflowEventRepository{},
			Queue:        &memory.WorkflowProcessExecutionQueue{},
			Changes:      &memory.WorkflowProcessChangeBroadcast{},
			Locks:        &memory.WorkflowProcessLocks{},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = let.Context(s)
			process = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return runtime.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.When("process doesn't have definition", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				return p
			})

			s.Then("it will return with an error of missing definition", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrNoProcessDefinition)
			})
		})

		s.When("definition is provided in the process", func(s *testcase.Spec) {
			defRan := let.VarOf(s, false)
			defCtx := let.VarOf[context.Context](s, nil)

			definition := let.Var(s, func(t *testcase.T) workflow.Definition {
				return &wftest.Stub{StubExecute: func(ctx context.Context, p workflow.ProcessID) error {
					defCtx.Set(t, ctx)
					assert.NotNil(t, ctx)
					defRan.Set(t, true)
					return ctx.Err()
				}}
			})
			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				if p.IsZero() {
					p = mustProcessID(t)
				}
				assert.NoError(t, runtime.Get(t).Bind(t.Context(), p, definition.Get(t)))
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

		s.Context("during a participant execution", func(s *testcase.Spec) {
			participantID := wftest.LetParticipantID(s)
			participantN := let.VarOf(s, 0)

			lastContext := let.VarOf[context.Context](s, nil)

			participants.Let(s, func(t *testcase.T) workflow.Participants {
				ps := participants.Super(t)
				assert.NotNil(t, ps)
				ps[participantID.Get(t)] = func(ctx context.Context) error {
					participantN.Set(t, participantN.Get(t)+1)
					lastContext.Set(t, ctx)
					return nil
				}
				return ps
			})

			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				assert.NotEmpty(t, p)
				assert.NoError(t, runtime.Get(t).Bind(t.Context(), p, workflow.ExecuteParticipant{ID: participantID.Get(t)}))
				return p
			})

			s.Then("participant has access to the current workflow execution process id", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := workflow.ProcessIDFromContext(lastContext.Get(t))
				assert.True(t, ok)
				assert.Equal(t, got, process.Get(t))
			})
		})

		s.When("ContextSetup is supplied", func(s *testcase.Spec) {
			receivedContext := let.VarOf[context.Context](s, nil)

			runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := runtime.Super(t)
				rt.ContextSetup = append(rt.ContextSetup, func(ctx context.Context) context.Context {
					receivedContext.Set(t, ctx)
					return ctx
				})
				return rt
			})

			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				assert.NotEmpty(t, p)
				assert.NoError(t, runtime.Get(t).Bind(t.Context(), p, workflow.Sequence{}))
				return p
			})

			s.Then("processID is available in the context during a workflow execution triggered contest setup", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := workflow.ProcessIDFromContext(receivedContext.Get(t))
				assert.True(t, ok)
				assert.Equal(t, got, process.Get(t))
			})
		})

		s.When("a participant returns a Replace runtime signal", func(s *testcase.Spec) {

			switcherID := let.Var(s, func(t *testcase.T) workflow.ParticipantID {
				return workflow.ParticipantID(t.Random.Domain())
			})
			switcherCalls := let.VarOf(s, 0)

			// The replacement definition that the switcher hands to the runtime.
			// We use a Stub so we can observe whether the runtime actually
			// picked up the new definition and executed it.
			newDefRan := let.VarOf(s, false)
			newDef := let.Var(s, func(t *testcase.T) workflow.Definition {
				return &wftest.Stub{StubExecute: func(ctx context.Context, p workflow.ProcessID) error {
					newDefRan.Set(t, true)
					return nil
				}}
			})

			participants.Let(s, func(t *testcase.T) workflow.Participants {
				ps := participants.Super(t)
				assert.NotNil(t, ps)
				ps[switcherID.Get(t)] = func(ctx context.Context) error {
					switcherCalls.Set(t, switcherCalls.Get(t)+1)
					return workflow.Replace{Definition: newDef.Get(t)}
				}
				return ps
			})

			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				if p.IsZero() {
					p = mustProcessID(t)
				}
				assert.NoError(t, runtime.Get(t).Bind(t.Context(), p, workflow.Sequence{
					workflow.SetVar{Name: "warmup", Value: "ready"},
					workflow.ExecuteParticipant{ID: switcherID.Get(t)},
				}))
				return p
			})

			s.Then("Runtime.Execute returns without an error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("the switcher participant was called exactly once (idempotency preserved across the signal)", func(t *testcase.T) {
				act(t)
				assert.Equal(t, 1, switcherCalls.Get(t),
					assert.MessageF("expected the switcher to be called once; "+
						"the SwitchDefinition signal must be persisted so a replay does not re-execute it"))
			})

			s.Then("the switched-to definition is picked up and executed", func(t *testcase.T) {
				act(t)
				assert.True(t, newDefRan.Get(t),
					assert.MessageF("expected the new (switched-to) definition to have been executed by the runtime"))
			})

			s.Then("the event history records the SwitchDefinition signal as a UseDefinitionEvent append", func(t *testcase.T) {
				act(t)

				hist := mustHistory(t, runtime.Get(t), process.Get(t))

				var useDefs []workflow.EventUseDefinition
				var participantEvents []workflow.EventParticipant
				for _, e := range hist {
					switch v := e.(type) {
					case workflow.EventUseDefinition:
						useDefs = append(useDefs, v)
					case workflow.EventParticipant:
						participantEvents = append(participantEvents, v)
					}
				}

				t.LogPretty(hist)

				// Initial UseDefinitionEvent (from Spawn) + the SwitchDefinition
				// append (one new UseDefinitionEvent carrying the switched-to def).
				assert.Equal(t, 2, len(useDefs),
					assert.MessageF("expected two UseDefinitionEvents (initial + switched), got %d", len(useDefs)))

				// The newly appended UseDefinitionEvent MUST carry the
				// switched-to definition, not the original Sequence.
				last := useDefs[len(useDefs)-1]
				assert.Equal(t, newDef.Get(t), last.Definition,
					assert.MessageF("expected the last UseDefinitionEvent to carry the switched-to definition"))

				// The switcher call itself is deliberately NOT persisted. A
				// RuntimeSignal is dynamic control flow rather than a step
				// result, so it is raised afresh on every execution instead of
				// being replayed from the event history.
				// See TestRuntime_Execute_participantRuntimeSignal.
				assert.Empty(t, participantEvents,
					assert.MessageF("expected the signalling participant to stay out of the "+
						"event history, got %d record(s)", len(participantEvents)))
			})
		})

		s.When("the Process has already reached a terminal state", func(s *testcase.Spec) {
			defRan := let.VarOf(s, false)

			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				if p.IsZero() {
					p = mustProcessID(t)
				}
				// Bind a Definition that records whether it was invoked, so the
				// short-circuit can be observed: a Process whose outcome is
				// already on record must not be re-executed. Re-running the
				// Definition would replay steps the runtime has already
				// committed to, and the idempotent executor would happily
				// re-emit every EventParticipant for steps that already ran.
				assert.NoError(t, runtime.Get(t).Bind(t.Context(), p, &wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						defRan.Set(t, true)
						return nil
					},
				}))
				return p
			})

			s.And("the Process is already Completed", func(s *testcase.Spec) {

				s.Before(func(t *testcase.T) {
					var completed workflow.Event = workflow.EventCompleted{
						EventID:   mustEventID(t),
						ProcessID: process.Get(t),
						Timestamp: clock.Now(),
					}
					assert.NoError(t, runtime.Get(t).Events.Create(t.Context(), &completed))
				})

				s.Then("Execute returns without an error", func(t *testcase.T) {
					assert.NoError(t, act(t),
						assert.MessageF("a Process whose outcome is already on record "+
							"is a no-op for Runtime#Execute; any error here means we "+
							"are re-running work that has already been done"))
				})

				s.Then("the Definition is not invoked", func(t *testcase.T) {
					act(t)

					assert.False(t, defRan.Get(t),
						assert.MessageF("re-running the Definition would replay steps "+
							"the runtime has already committed to; the idempotent "+
							"executor would happily re-emit every EventParticipant "+
							"for steps that already ran"))
				})

				s.Then("no new events are appended to the event history", func(t *testcase.T) {
					var before = mustHistory(t, runtime.Get(t), process.Get(t))

					assert.NoError(t, act(t))

					var after = mustHistory(t, runtime.Get(t), process.Get(t))
					assert.Equal(t, len(before), len(after),
						assert.MessageF("the event log is append-only: re-executing a "+
							"terminal Process must not grow a second EventCompleted, "+
							"or the log would later read the same Process as completed "+
							"twice"))

					var completedBefore = len(filterEvents[workflow.EventCompleted](t, before))
					var completedAfter = len(filterEvents[workflow.EventCompleted](t, after))
					assert.Equal(t, completedBefore, completedAfter,
						assert.MessageF("the existing completion is left exactly as it was"))

					isCompleted, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, process.Get(t))
					assert.NoError(t, err)
					assert.True(t, isCompleted,
						assert.MessageF("the Process still reads as completed; a "+
							"second EventCompleted over the first would not change "+
							"this answer, but having one is the whole point of the "+
							"no-op contract"))
				})
			})

			s.And("the Process is already Terminated", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					var terminated workflow.Event = workflow.EventTerminated{
						EventID:   mustEventID(t),
						ProcessID: process.Get(t),
						Timestamp: clock.Now(),
					}
					assert.NoError(t, runtime.Get(t).Events.Create(t.Context(), &terminated))
				})

				s.Then("Execute returns without an error", func(t *testcase.T) {
					assert.NoError(t, act(t),
						assert.MessageF("a Process that has been called off is a no-op "+
							"for Runtime#Execute; any error here means we are "+
							"re-running work the caller already decided to abort"))
				})

				s.Then("no new events are appended to the event history", func(t *testcase.T) {
					var before = mustHistory(t, runtime.Get(t), process.Get(t))

					assert.NoError(t, act(t))

					var after = mustHistory(t, runtime.Get(t), process.Get(t))
					assert.Equal(t, len(before), len(after),
						assert.MessageF("the event log is append-only: re-executing a "+
							"terminal Process must not grow a second EventTerminated, "+
							"or the log would later read the same Process as called "+
							"off twice"))

					var terminatedBefore = len(filterEvents[workflow.EventTerminated](t, before))
					var terminatedAfter = len(filterEvents[workflow.EventTerminated](t, after))
					assert.Equal(t, terminatedBefore, terminatedAfter,
						assert.MessageF("the existing termination is left exactly as it was"))

					isTerminated, err := workflow.IsTerminated(t.Context(), runtime.Get(t).Events, process.Get(t))
					assert.NoError(t, err)
					assert.True(t, isTerminated,
						assert.MessageF("the Process still reads as terminated; a "+
							"second EventTerminated over the first would not change "+
							"this answer, but having one is the whole point of the "+
							"no-op contract"))
				})

				s.Then("no contradicting terminal event is written over the existing one", func(t *testcase.T) {
					var before = mustHistory(t, runtime.Get(t), process.Get(t))
					var completedBefore = len(filterEvents[workflow.EventCompleted](t, before))

					assert.NoError(t, act(t))

					var after = mustHistory(t, runtime.Get(t), process.Get(t))
					var completedAfter = len(filterEvents[workflow.EventCompleted](t, after))
					assert.Equal(t, completedBefore, completedAfter,
						assert.MessageF("Runtime#Execute must not promote a Terminated "+
							"Process to Completed by writing an EventCompleted over "+
							"the existing EventTerminated, even when the recorded "+
							"reason for termination is unknown (e.g. the Process "+
							"was called off mid-execution, with state that may not "+
							"be safe to retry)"))
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

			receivedContext := let.VarOf[context.Context](s, nil)

			ContextSetup := let.Var(s, func(t *testcase.T) func(ctx context.Context) context.Context {
				return func(ctx context.Context) context.Context {
					receivedContext.Set(t, ctx)
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

	s.Describe("#Spawn", func(s *testcase.Spec) {
		var (
			processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})
			definition = let.Var(s, func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: "/dev/null"}
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return runtime.Get(t).Spawn(t.Context(), processID.Get(t), definition.Get(t))
		})

		s.Before(func(t *testcase.T) {
			t.Go(func(ctx context.Context) error {
				return runtime.Get(t).Run(ctx)
			})
		})

		s.Then("no error is returned", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("a workflow definition is associated with the process id (#Bind)", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var hist = mustHistory(t, runtime.Get(t), processID.Get(t))
			assert.OneOf(t, hist, func(tb testing.TB, e workflow.Event) {
				got, ok := e.(workflow.EventUseDefinition)
				assert.True(tb, ok)
				assert.Equal(t, definition.Get(t), got.Definition)
			})
		})

		s.Then("the new process is eventually executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Eventually(t, deadline, func(tb testing.TB) {
				// The simplest observable sign of execution is an EventCompleted event
				// recorded for the process — Runtime.Execute emits one on success.
				var events = mustHistory(tb, runtime.Get(t), processID.Get(t))
				assert.NotEmpty(tb, events)
				assert.OneOf(tb, events, func(tb testing.TB, e workflow.Event) {
					_, ok := e.(workflow.EventCompleted)
					assert.True(tb, ok)
				})
			})
		})

		// Spawn must not block on definition execution. If it did, an
		// in-process definition would deadlock the caller — Spawn is only
		// useful if it returns as soon as the process is bound and queued,
		// letting the runtime worker handle the actual execution.
		s.Then("the spawn doesn't block and wait for execution", func(t *testcase.T) {
			assert.Within(t, waitTime, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})
		})

		s.When("definition would take time to finish up", func(s *testcase.Spec) {
			// The spawned definition blocks on a phaser before completing.
			// This lets the test observe three observable moments in order:
			//
			//   1. Spawn has returned (caller is no longer blocked), but the
			//      definition is still parked on the phaser.
			//   2. While the phaser is still holding the definition, the
			//      process is observably NOT yet complete.
			//   3. After the phaser is finished, the process eventually
			//      completes — proving Spawn queued the work rather than
			//      blocking on it.
			phaser := let.Phaser(s)

			// Force eager initialization of the phaser in the test goroutine
			// so its t.Cleanup(phaser.Finish) is registered before the test
			// body returns. Without this, when the test body completes faster
			// than the Runtime worker goroutine reaches the participant, the
			// phaser init runs inside the worker — after t.teardown.Finish()
			// has already drained its queue — and the Finish cleanup is
			// silently lost. The worker then parks on phaser.Wait forever
			// and t.g.Wait() hangs. See TestRuntime_phaserLazyInitRace.
			s.Before(func(t *testcase.T) {
				phaser.Get(t)
			})

			blockingParticipantID := wftest.LetParticipantID(s)
			_ = wftest.LetParticipantWithID(s, blockingParticipantID, func(t *testcase.T) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					phaser.Get(t).Wait()
					return nil
				}
			})

			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: blockingParticipantID.Get(t)}
			})

			s.Then("Spawn returns before the definition has finished", func(t *testcase.T) {
				assert.Within(t, waitTime, func(ctx context.Context) {
					assert.NoError(t, act(t))
				})

				// While the participant is still parked on the phaser, the
				// process cannot be complete. If Spawn synchronously waited
				// for execution, the assertion would have deadlocked inside
				// the assert.Within above.
				compl, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, processID.Get(t))
				assert.NoError(t, err)
				assert.False(t, compl,
					assert.MessageF("expected Spawn to queue the work without blocking on it, "+
						"but the process is already complete while the participant is still blocked"))
			})

			s.Then("the process eventually completes after the definition is unblocked", func(t *testcase.T) {
				assert.Within(t, waitTime, func(ctx context.Context) {
					assert.NoError(t, act(t))
				})

				compl, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, processID.Get(t))
				assert.NoError(t, err)
				assert.False(t, compl)

				phaser.Get(t).Finish()

				assert.Eventually(t, deadline, func(tb testing.TB) {
					compl, err := workflow.IsCompleted(tb.Context(), runtime.Get(t).Events, processID.Get(t))
					assert.NoError(tb, err)
					assert.True(tb, compl,
						assert.MessageF("expected the process to complete after the participant is unblocked"))
				})
			})
		})

		s.When("Spawn was already called multiple times with the same id and definition", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				for range t.Random.IntBetween(2, 7) {
					assert.NoError(t, act(t))
				}
			})

			s.Then("it does not return an error on subsequent calls", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("only one process entity is stored in process event history", func(t *testcase.T) {
				assert.NoError(t, act(t))

				// Process is stateless; the proof of (single) existence is that
				// exactly one UseDefinitionEvent is recorded in the event history
				// for the given ProcessID.
				hist := mustHistory(t, runtime.Get(t), processID.Get(t))
				var seen int
				for _, e := range hist {
					if _, ok := e.(workflow.EventUseDefinition); ok {
						seen++
					}
				}
				assert.Equal(t, 1, seen,
					assert.MessageF("expected exactly one UseDefinitionEvent to remain after repeated Spawn calls, got %d", seen))
			})

			s.Then("the process is not re-executed on subsequent calls", func(t *testcase.T) {
				t.Eventually(func(t *testcase.T) {
					compl, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, processID.Get(t))
					assert.NoError(t, err)
					assert.True(t, compl)
				})

				before := mustHistory(t, runtime.Get(t), processID.Get(t))

				assert.NoError(t, act(t))

				t.Random.Repeat(3, 7, func() {
					time.Sleep(time.Millisecond)

					after := mustHistory(t, runtime.Get(t), processID.Get(t))

					assert.Equal(t, len(before), len(after),
						assert.MessageF("expected history length to be unchanged after re-Spawn; got %d before, %d after",
							len(before), len(after)))
				})

			})
		})

		// Spawn owns the choice of *executing* a process, but the *identity* of
		// the process must be supplied by the caller.
		//
		// Why? The sender is the one that decides "I asked for this work". If
		// the sender ever retries after an error (network blip, timeout, panic
		// recovery, anything), it must produce the SAME ProcessID, so the second
		// Spawn is idempotent with the first. If Spawn silently minted a fresh
		// ID for a zero-value ProcessID, two retries would create two distinct
		// processes — exactly the failure mode the idempotency contract is
		// designed to prevent.
		s.When("a zero ProcessID is supplied", func(s *testcase.Spec) {
			processID.Let(s, func(t *testcase.T) workflow.ProcessID {
				var zero workflow.ProcessID
				return zero
			})

			s.Then("Spawn returns an error rather than silently generating an id", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err,
					assert.MessageF("expected Spawn to reject a zero ProcessID, "+
						"because the caller must own process identity for safe retry semantics"))
			})

			s.Then("no events are recorded on behalf of the rejected Spawn", func(t *testcase.T) {
				beforeN, err := iterkit.CountE(runtime.Get(t).Events.FindAll(t.Context()))
				assert.NoError(t, err)

				assert.Error(t, act(t))

				afterN, err := iterkit.CountE(runtime.Get(t).Events.FindAll(t.Context()))
				assert.NoError(t, err)

				assert.Equal(t, beforeN, afterN,
					"expected the EventsRepository to be unchanged after a rejected Spawn")
			})

			s.Then("the error message mentions ProcessID so callers can diagnose the misuse", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "ProcessID",
					assert.MessageF("expected the error to reference ProcessID, got: %q", err.Error()))
			})
		})
	})

	s.Describe("#Execute with UseDefinitionEvent history", func(s *testcase.Spec) {
		// UseDefinitionEvents let a process dynamically change its definition.
		// The runtime replays from the event history. This block exercises the
		// fast-forward vs NoFastForward replay semantics.

		processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})

		// First and second definitions are distinct ExecuteParticipants; the
		// participant counters let us assert that each round executed the
		// intended definition.
		firstParticipant := wftest.LetParticipantID(s)
		secondParticipant := wftest.LetParticipantID(s)

		firstCalls := let.VarOf(s, 0)
		secondCalls := let.VarOf(s, 0)

		participants.Let(s, func(t *testcase.T) workflow.Participants {
			ps := participants.Super(t)
			ps[firstParticipant.Get(t)] = func(ctx context.Context) error {
				firstCalls.Set(t, firstCalls.Get(t)+1)
				return nil
			}
			ps[secondParticipant.Get(t)] = func(ctx context.Context) error {
				secondCalls.Set(t, secondCalls.Get(t)+1)
				return nil
			}
			return ps
		})

		seedHistory := func(t *testcase.T) {
			// Use clock.Now() so the timestamps are deterministic across runs.
			// time.Now() can produce identical timestamps under fast machines
			// (µs resolution), which would scramble the timestamp-based
			// ordering of the two UseDefinitionEvent entries, breaking the
			// assertion that the first participant ran first.
			base := clock.Now()
			eventsRepo := runtime.Get(t).Events
			first := workflow.Event(workflow.EventUseDefinition{
				EventID:    mustEventID(t),
				ProcessID:  processID.Get(t),
				Timestamp:  base,
				Definition: workflow.ExecuteParticipant{ID: firstParticipant.Get(t)},
			})
			assert.NoError(t, eventsRepo.Create(t.Context(), &first))
			second := workflow.Event(workflow.EventUseDefinition{
				EventID:    mustEventID(t),
				ProcessID:  processID.Get(t),
				Timestamp:  base.Add(time.Millisecond),
				Definition: workflow.ExecuteParticipant{ID: secondParticipant.Get(t)},
			})
			assert.NoError(t, eventsRepo.Create(t.Context(), &second))
		}

		s.Test("fast-forward replays only the most recent UseDefinitionEvent", func(t *testcase.T) {
			seedHistory(t)

			assert.NoError(t, runtime.Get(t).Execute(t.Context(), processID.Get(t)))

			assert.Equal(t, 0, firstCalls.Get(t),
				assert.MessageF("expected the first definition to be skipped in fast-forward mode"))
			assert.Equal(t, 1, secondCalls.Get(t),
				assert.MessageF("expected only the latest definition to be executed"))
		})
	})

	s.Describe("#Terminate", func(s *testcase.Spec) {
		var (
			ctx       = let.Context(s)
			processID = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return runtime.Get(t).Terminate(ctx.Get(t), processID.Get(t))
		})

		s.Then("it marks the process terminated", func(t *testcase.T) {
			var before = mustHistory(t, runtime.Get(t), processID.Get(t))

			assert.NoError(t, act(t))

			var after = mustHistory(t, runtime.Get(t), processID.Get(t))
			assert.Equal(t, len(before)+1, len(after),
				assert.MessageF("the event log is append-only: terminating adds exactly "+
					"one entry and never removes any. before=%d, after=%d",
					len(before), len(after)))

			isTerminated, err := workflow.IsTerminated(t.Context(), runtime.Get(t).Events, processID.Get(t))
			assert.NoError(t, err)
			assert.True(t, isTerminated,
				assert.MessageF("raising the signal and asking the question are two halves "+
					"of one contract; whatever Runtime#Terminate writes, IsTerminated must read"))
		})

		s.Then("the recorded termination names the Process", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var found bool
			for _, event := range mustHistory(t, runtime.Get(t), processID.Get(t)) {
				if terminated, ok := event.(workflow.EventTerminated); ok {
					assert.True(t, terminated.ProcessID.Equal(processID.Get(t)),
						assert.MessageF("every entry in a Process' history belongs to that Process"))
					found = true
				}
			}
			assert.True(t, found, assert.MessageF("expected EventTerminated in the history"))
		})

		s.When("the Process was scheduled", func(s *testcase.Spec) {
			var (
				isParticipantStarted = let.VarOf(s, false)
				isCancelled          = let.VarOf(s, false)
			)

			_, pid := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					isParticipantStarted.Set(t, true)
					select {
					case <-ctx.Done():
						isCancelled.Set(t, true)
					case <-time.After(time.Minute):
						// safety net so a missed cancellation cannot wedge the
						// Runtime worker goroutine past the test's lifetime
					}
					return nil
				}
			})

			s.Before(func(t *testcase.T) {
				t.Go(runtime.Get(t).Run)

				def := workflow.ExecuteParticipant{ID: pid.Get(t)}
				assert.NoError(t, runtime.Get(t).Spawn(t.Context(), processID.Get(t), def))

				t.Eventually(func(t *testcase.T) {
					assert.True(t, isParticipantStarted.Get(t))
				})
			})

			s.Then("the in-flight participant is still recorded as terminated", func(t *testcase.T) {
				var before = mustHistory(t, runtime.Get(t), processID.Get(t))

				assert.NoError(t, act(t))

				var after = mustHistory(t, runtime.Get(t), processID.Get(t))
				assert.Equal(t, len(before)+1, len(after),
					assert.MessageF("scheduling an entry does not get removed when the "+
						"Process is terminated; the event log is append-only"))

				isTerminated, err := workflow.IsTerminated(t.Context(), runtime.Get(t).Events, processID.Get(t))
				assert.NoError(t, err)
				assert.True(t, isTerminated,
					assert.MessageF("the Process reads as terminated after the call"))
			})

			s.Then("the in-flight process is cancelled", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.True(t, isCancelled.Get(t))
				})
			})
		})

		s.When("the Process has already completed", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				var completed workflow.Event = workflow.EventCompleted{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
				}
				assert.NoError(t, runtime.Get(t).Events.Create(t.Context(), &completed))
			})

			s.Then("the completion is preserved and no termination is appended", func(t *testcase.T) {
				var before = mustHistory(t, runtime.Get(t), processID.Get(t))
				var beforeCompletions = len(filterEvents[workflow.EventCompleted](t, before))

				assert.NoError(t, act(t))

				var after = mustHistory(t, runtime.Get(t), processID.Get(t))
				assert.Equal(t, len(before), len(after),
					assert.MessageF("a completed Process has its outcome on record and the "+
						"signal must not append a contradicting event over it"))

				assert.Equal(t, beforeCompletions, len(filterEvents[workflow.EventCompleted](t, after)),
					assert.MessageF("the existing completion is left exactly as it was"))

				isCompleted, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, processID.Get(t))
				assert.NoError(t, err)
				assert.True(t, isCompleted,
					assert.MessageF("the Process still reads as completed; terminating it "+
						"after the fact would lose the distinction between 'finished' and "+
						"'called off'"))
			})
		})

		s.When("the Process has already been terminated", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				var terminated workflow.Event = workflow.EventTerminated{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
				}
				assert.NoError(t, runtime.Get(t).Events.Create(t.Context(), &terminated))
			})

			s.Then("no second termination is appended", func(t *testcase.T) {
				var before = mustHistory(t, runtime.Get(t), processID.Get(t))

				assert.NoError(t, act(t))

				var after = mustHistory(t, runtime.Get(t), processID.Get(t))
				assert.Equal(t, len(before), len(after),
					assert.MessageF("terminating an already terminated Process is a no-op; "+
						"a duplicate call must not grow a second EventTerminated, or the "+
						"log stops being a reliable answer to when it was stopped"))
			})
		})

		s.When("the Runtime passes no ProcessID", func(s *testcase.Spec) {
			processID.Let(s, func(t *testcase.T) workflow.ProcessID {
				return workflow.ProcessID{}
			})

			s.Then("nothing is terminated and the fault is reported", func(t *testcase.T) {
				assert.Error(t, act(t),
					assert.MessageF("without a ProcessID there is no Process to stop, "+
						"and the call has to fail loudly rather than be silently dropped"))
			})
		})
	})
}

// filterEvents returns the events in events that are of type E.
// Used to assert append-only log behaviour without having to read whole event
// types back through type assertions at the call site.
func filterEvents[E workflow.Event](t *testcase.T, events []workflow.Event) []E {
	t.Helper()
	var out []E
	for _, event := range events {
		if e, ok := event.(E); ok {
			out = append(out, e)
		}
	}
	return out
}

// TestRuntime_missingMandatoryDependencyFailsFast is a regression test for a
// production bug where Runtime#Schedule (and any Runtime method that delegates
// through withRetry) would burn the full retry budget on a configuration
// error rather than failing fast. The root cause was Runtime#Validate
// returning plain fmt.Errorf("missing ...") values, which withRetry did not
// classify as fatal and therefore retried up to the default 5 attempts of
// resilience.Jitter{} — each attempt sleeping up to 5s of jitter, so a
// misconfigured Runtime#Schedule could block for ~20 seconds.
//
//   - The error must come back within a small budget (well under the default
//     retry budget) so the caller observes a fail, not a hang.
//   - The error must be classified as fatal via workflow.ErrIsFatal so
//     withRetry short-circuits without retrying.
func TestRuntime_missingMandatoryDependencyFailsFast(t *testing.T) {
	const fastFailBudget = 2 * time.Second // < one retry-jitter cycle

	rt := workflow.Runtime{
		// Events is set so the runtime does not bail on the first Validate
		// check, exposing the ProcessExecutionQueue dependency path.
		Events: &memory.WorkflowEventRepository{},
		// ProcessExecutionQueue intentionally left nil.
		// ProcessChangeBroadcast intentionally left nil.
	}

	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	var gotErr error
	assert.Within(t, fastFailBudget, func(ctx context.Context) {
		gotErr = rt.Schedule(ctx, pid)
	})

	assert.Error(t, gotErr)
	assert.True(t, workflow.ErrIsFatal(gotErr),
		assert.MessageF("missing-dependency errors must be classified as fatal so withRetry short-circuits; otherwise a misconfigured Runtime#Schedule hangs for the full retry budget"))
}

func TestRuntime_multipleDefinitionStage(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		fooN = let.VarOf(s, 0)
		barN = let.VarOf(s, 0)
		bazN = let.VarOf(s, 0)
		quxN = let.VarOf(s, 0)
	)

	participants := let.Var(s, func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"foo": func(ctx context.Context) error {
				fooN.Set(t, fooN.Get(t)+1)
				return nil
			},
			"bar": func(ctx context.Context) error {
				barN.Set(t, barN.Get(t)+1)
				return nil
			},
			"baz": func(ctx context.Context) error {
				bazN.Set(t, bazN.Get(t)+1)
				return nil
			},
			"qux": func(ctx context.Context) error {
				quxN.Set(t, quxN.Get(t)+1)
				return nil
			},

			"dynamic": func(ctx context.Context) error {
				return workflow.Sequence{
					workflow.ExecuteParticipant{ID: "foo"},
					workflow.ExecuteParticipant{ID: "bar"},
				}
			},

			"replace": func(ctx context.Context) error {
				return workflow.Replace{
					Definition: workflow.Sequence{
						workflow.ExecuteParticipant{ID: "foo"},
						workflow.ExecuteParticipant{ID: "bar"},
					},
				}
			},
		}
	})

	runtime := c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := c.Runtime.Super(t)
		rt.ContextSetup = append(rt.ContextSetup, func(ctx context.Context) context.Context {
			return workflow.ContextWithParticipants(ctx, participants.Get(t))
		})
		return rt
	})

	processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		return mustProcessID(t)
	})

	// Each repeat of the spec acts as a fresh sub-test so the random seed
	// can shuffle the variables without affecting the assertion.
	s.Test("workflow.Replace RuntimeSignal used midway of the workflow", func(t *testcase.T) {
		pid := processID.Get(t)
		rt := runtime.Get(t)

		definition := workflow.Sequence{
			workflow.ExecuteParticipant{ID: "foo"},
			workflow.ExecuteParticipant{ID: "bar"},
			workflow.ExecuteParticipant{ID: "baz"},
			workflow.ExecuteParticipant{ID: "replace"},
			workflow.ExecuteParticipant{ID: "qux"}, // will be ignored due to switch
		}

		assert.Within(t, deadline, func(ctx context.Context) {
			assert.NoError(t, rt.Bind(ctx, pid, definition))
			assert.NoError(t, rt.Execute(ctx, pid))
		})

		assert.Equal(t, fooN.Get(t), 2)
		assert.Equal(t, barN.Get(t), 2)
		assert.Equal(t, bazN.Get(t), 1)
		assert.Equal(t, quxN.Get(t), 0,
			"expected that qux participant is never reached due to switch statement")
	})

	s.Test("a participant replies back with a sub-workflow signal", func(t *testcase.T) {
		pid := processID.Get(t)
		rt := runtime.Get(t)

		definition := workflow.Sequence{
			workflow.ExecuteParticipant{ID: "foo"},
			workflow.ExecuteParticipant{ID: "bar"},
			workflow.ExecuteParticipant{ID: "baz"},
			workflow.ExecuteParticipant{ID: "dynamic"},
			workflow.ExecuteParticipant{ID: "qux"},
		}

		assert.NoError(t, rt.Bind(t.Context(), pid, definition))
		assert.NoError(t, rt.Execute(t.Context(), pid))

		assert.Equal(t, fooN.Get(t), 2)
		assert.Equal(t, barN.Get(t), 2)
		assert.Equal(t, bazN.Get(t), 1)
		assert.Equal(t, quxN.Get(t), 1)
	})
}
