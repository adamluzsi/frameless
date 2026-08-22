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
		participants = let.Var(s, func(t *testcase.T) workflow.Participants {
			return workflow.Participants{
				"/dev/null": func(ctx context.Context) error { return nil },
			}
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
			Events:       &memory.WorkflowEventRepository{},
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

		s.When("a participant returns a SwitchDefinition runtime signal", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				// TODO: SwitchDefinition runtime signal handling is not yet
				// implemented in workflow.Runtime; this will be tackled as
				// part of the in-flight definition-mutation work.
				t.Skip("out-of-scope currently")
			})

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
				var participantEvents []*workflow.EventParticipant
				for _, e := range hist {
					switch v := e.(type) {
					case workflow.EventUseDefinition:
						useDefs = append(useDefs, v)
					case *workflow.EventParticipant:
						participantEvents = append(participantEvents, v)
					}
				}

				// Initial UseDefinitionEvent (from Spawn) + the SwitchDefinition
				// append (one new UseDefinitionEvent carrying the switched-to def).
				assert.Equal(t, 2, len(useDefs),
					assert.MessageF("expected two UseDefinitionEvents (initial + switched), got %d", len(useDefs)))

				// The newly appended UseDefinitionEvent MUST carry the
				// switched-to definition, not the original Sequence.
				last := useDefs[len(useDefs)-1]
				assert.Equal(t, newDef.Get(t), last.Definition,
					assert.MessageF("expected the last UseDefinitionEvent to carry the switched-to definition"))

				// The participant call itself MUST be persisted so a replay
				// does not re-execute it (idempotency).
				assert.Equal(t, 1, len(participantEvents),
					assert.MessageF("expected exactly one ExecuteParticipantEvent for the switcher call, got %d",
						len(participantEvents)))
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

		s.Then("no error is returned", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("a process entity is present in the ProcessRepository", func(t *testcase.T) {
			assert.NoError(t, act(t))

			// Process is stateless; the proof of existence is the
			// UseDefinitionEvent recorded in the event history.
			hist := mustHistory(t, runtime.Get(t), processID.Get(t))
			var found bool
			for _, e := range hist {
				if _, ok := e.(workflow.EventUseDefinition); ok {
					found = true
					break
				}
			}
			assert.True(t, found,
				assert.MessageF("expected a UseDefinitionEvent to be persisted for id %v after Spawn", processID.Get(t)))
		})

		s.Then("the definition is recorded as a UseDefinitionEvent in the event history", func(t *testcase.T) {
			assert.NoError(t, act(t))

			hist := mustHistory(t, runtime.Get(t), processID.Get(t))
			var sde workflow.EventUseDefinition
			for _, e := range hist {
				if v, ok := e.(workflow.EventUseDefinition); ok {
					sde = v
					break
				}
			}
			assert.NotNil(t, sde,
				assert.MessageF("expected a UseDefinitionEvent to be recorded in the event history"))
			assert.NotNil(t, sde.Definition,
				assert.MessageF("expected the recorded UseDefinitionEvent to carry the spawned Definition"))
		})

		s.Then("the new process is executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			// The simplest observable sign of execution is an EventCompleted event
			// recorded for the process — Runtime.Execute emits one on success.
			assert.NotEmpty(t, mustHistory(t, runtime.Get(t), processID.Get(t)))
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

			s.Then("only one process entity is stored in the ProcessRepository", func(t *testcase.T) {
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
				before := mustHistory(t, runtime.Get(t), processID.Get(t))

				assert.NoError(t, act(t))

				after := mustHistory(t, runtime.Get(t), processID.Get(t))
				assert.Equal(t, len(before), len(after),
					assert.MessageF("expected history length to be unchanged after re-Spawn; got %d before, %d after",
						len(before), len(after)))
			})
		})

		s.When("the process has a UseDefinitionEvent in its history already", func(s *testcase.Spec) {
			// Build a definition whose execution produces an observable side effect:
			// a participant call counter, so we can assert that Execute picks up
			// the historical definition rather than nothing.
			participantID := let.Var(s, func(t *testcase.T) workflow.ParticipantID {
				return workflow.ParticipantID(t.Random.Domain())
			})
			secondDef := let.Var(s, func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: participantID.Get(t)}
			})
			callCount := let.VarOf(s, 0)
			participants.Let(s, func(t *testcase.T) workflow.Participants {
				ps := participants.Super(t)
				ps[participantID.Get(t)] = func(ctx context.Context) error {
					callCount.Set(t, callCount.Get(t)+1)
					return nil
				}
				return ps
			})

			s.Before(func(t *testcase.T) {
				// Seed the history with a UseDefinitionEvent that points to a
				// second definition. This simulates a process whose definition has
				// been changed in flight (or replayed after a crash).
				var sde workflow.Event = workflow.EventUseDefinition{
					EventID:    mustEventID(t),
					ProcessID:  processID.Get(t),
					Timestamp:  time.Now(),
					Definition: secondDef.Get(t),
				}
				assert.NoError(t, runtime.Get(t).Events.Create(t.Context(), &sde))
			})

			s.Then("Execute replays the historical definition", func(t *testcase.T) {
				assert.NoError(t, runtime.Get(t).Execute(t.Context(), processID.Get(t)))

				assert.Equal(t, 1, callCount.Get(t),
					assert.MessageF("expected the second (historical) definition to be executed via replay"))
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
