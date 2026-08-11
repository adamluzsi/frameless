package workflow_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

const waitTime = time.Second / 4
const deadline = time.Second

func TestRuntime_Schedule_E2E(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	lastV := let.Var(s, func(t *testcase.T) string {
		return ""
	})
	participantN := let.VarOf(s, 0)

	pid := wftest.LetParticipantID(s)
	_ = wftest.LetParticipantWithID(s, c, pid, func(t *testcase.T) func(ctx context.Context, v string) error {
		return func(ctx context.Context, v string) error {
			lastV.Set(t, v)
			participantN.Set(t, participantN.Get(t)+1)
			return nil
		}
	})

	s.Test("scheduled process, eventually runs", func(t *testcase.T) {
		processID := uuid.Must(uuid.MakeV7)

		inputVal := t.Random.String()

		// Process is stateless — bind the definition via the event history
		// so the runtime can pick it up at execute time.
		rtCtx := c.Runtime.Get(t).Context(t.Context())
		var ev workflow.Event = workflow.EventUseDefinition{
			EventID:   mustEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now(),
			Definition: workflow.Sequence{
				workflow.SetVar{
					Key:   workflow.VarKey("input"),
					Value: inputVal,
				},
				workflow.ExecuteParticipant{
					ID: pid.Get(t),
					Input: []workflow.VarKey{
						workflow.VarKey("input"),
					},
				},
			},
		}
		assert.NoError(t, c.EventRepository.Get(t).Create(rtCtx, &ev))

		waitTime := time.Hour * 24
		target := clock.Now().Add(waitTime)

		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID, func(s *workflow.ProcessExecution) {
			s.StartTime = target
		}))

		t.Random.Repeat(3, 7, func() {
			runtime.Gosched()
			completed, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), c.Runtime.Get(t).Events)
			assert.NoError(t, err)
			assert.False(t, completed)
		})

		timecop.Travel(t, waitTime+time.Second)

		t.Eventually(func(t *testcase.T) {
			completed, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), c.Runtime.Get(t).Events)
			assert.NoError(t, err)
			assert.True(t, completed)
		})

		assert.Equal(t, lastV.Get(t), inputVal)
	})

	s.Test("same process, multiple reschedule, runs once", func(t *testcase.T) {
		inputVal := t.Random.String()

		// Allocate the ID ahead of Schedule so we can seed the event history
		// with a UseDefinitionEvent (Process is stateless — the runtime reads
		// the definition from the history, not from the Process struct).
		processID := uuid.Must(uuid.MakeV7)

		rtCtx := c.Runtime.Get(t).Context(t.Context())
		var ev workflow.Event = workflow.EventUseDefinition{
			EventID:   mustEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now(),
			Definition: workflow.Sequence{
				workflow.SetVar{
					Key:   workflow.VarKey("input"),
					Value: inputVal,
				},
				workflow.ExecuteParticipant{
					ID: pid.Get(t),
					Input: []workflow.VarKey{
						workflow.VarKey("input"),
					},
				},
			},
		}
		assert.NoError(t, c.EventRepository.Get(t).Create(rtCtx, &ev))

		t.Random.Repeat(3, 7, func() {
			assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))
		})

		assert.NotEmpty(t, processID)

		t.Eventually(func(t *testcase.T) {
			completed, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), c.Runtime.Get(t).Events)
			assert.NoError(t, err)
			assert.True(t, completed)
		})

		assert.Equal(t, 1, participantN.Get(t),
			"expected that the participant is only called once",
			"even though multiple times the same process were rescheduled")
	})
}

func TestRuntime_scheduling(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		pid = wftest.LetParticipantID(s)

		callCount = let.VarOf(s, 0)
		_         = wftest.LetParticipantWithID(s, c, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				callCount.Set(t, callCount.Get(t)+1)
				return nil
			}
		})
	)

	subject := c.Runtime.Bind(s)

	s.Describe("#Schedule and #Run", func(s *testcase.Spec) {
		var (
			Context   = let.Context(s)
			process   = wftest.LetProcessID(s)
			startTime = let.Var(s, func(t *testcase.T) time.Time {
				return time.Time{}
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Schedule(Context.Get(t), process.Get(t), func(s *workflow.ProcessExecution) {
				s.StartTime = startTime.Get(t)
			})
		})

		// processWithDefinition returns a function compatible with process.Let
		// that wraps the legacy `Process.Definition = ...` pattern by binding
		// the definition via a UseDefinitionEvent in the runtime's event history.
		// Process itself is stateless — the runtime reads the current definition
		// from the history, not from the Process struct.
		processWithDefinition := func(mk func(t *testcase.T) workflow.Definition) func(t *testcase.T) workflow.ProcessID {
			return func(t *testcase.T) workflow.ProcessID {
				var p = process.Super(t)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), p, mk(t)))
				return p
			}
		}

		s.When("process's definition succeeds without an issue", func(s *testcase.Spec) {
			process.Let(s, processWithDefinition(func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: pid.Get(t)}
			}))

			s.Then("then upon scheduling, eventually Schedule#Run will process the process task", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})

			s.And("the start time is somewhere in the future", func(s *testcase.Spec) {
				startTime.Let(s, func(t *testcase.T) time.Time {
					return clock.Now().Add(time.Hour)
				})

				s.Before(func(t *testcase.T) {
					timecop.Travel(t, time.Nanosecond)
				})

				s.Then("execution won't occur until the start time reached", func(t *testcase.T) {
					assert.NoError(t, act(t))

					w := assert.NotWithin(t, deadline, func(ctx context.Context) {
						for callCount.Get(t) == 0 {
							select {
							case <-t.Done():
								return
							default: // OK
							}
						}
					})

					t.Log("but after enough time was waited")
					timecop.Travel(t, time.Hour+time.Minute)

					assert.Within(t, deadline, func(ctx context.Context) {
						w.Wait()
					})
				})
			})
		})

		s.When("process has no ID", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				var zero workflow.ProcessID
				return zero
			})

			s.Then("Schedule refuses a zero ProcessID rather than minting one", func(t *testcase.T) {
				// The caller owns process identity — Schedule must refuse a
				// zero ProcessID so retries remain safe (same ID, same process).
				err := act(t)
				assert.Error(t, err,
					assert.MessageF("expected Schedule to reject a zero ProcessID, "+
						"because the caller must own process identity for safe retry semantics"))
				assert.Contains(t, err.Error(), "ProcessID")
			})
		})

		s.When("process is scheduled multiple times", func(s *testcase.Spec) {
			process.Let(s, processWithDefinition(func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: pid.Get(t)}
			}))

			s.Then("scheduling remains idempotent and the participant is called only once", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})

			s.Then("concurrent execution shares the same idempotency guarantees", func(t *testcase.T) {
				// Resolve the shared testcase variables once, before racing.
				// testcase.Race is meant to exercise the concurrency of the
				// subject (the Scheduler), not testcase's own lazy Var
				// initialisation, which is not designed to be triggered for the
				// first time from several goroutines at once (the let block that
				// builds the *Process would race with the repository reading it).
				// Pre-resolving means every racing goroutine only shares
				// already-initialised, read-only values.
				var (
					ctx       = Context.Get(t)
					p         = process.Get(t)
					st        = startTime.Get(t)
					scheduler = subject.Get(t)
				)

				var schedules []func()
				t.Random.Repeat(3, 7, func() {
					schedules = append(schedules, func() {
						assert.NoError(t, scheduler.Schedule(ctx, p, func(s *workflow.ProcessExecution) {
							s.StartTime = st
						}))
					})
				})

				testcase.Race(schedules...)

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})
		})

		s.When("process definition suspends", func(s *testcase.Spec) {
			shouldSuspend := let.VarOf(s, true)

			process.Let(s, processWithDefinition(func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
					workflow.Sleep{While: wftest.Stub{StubEvaluate: func(ctx context.Context, p workflow.ProcessID) (bool, error) {
						return shouldSuspend.Get(t), nil
					}}},
				}
			}))

			// isProcessCompleted reports whether the scheduled process has reached a
			// completed state, as determined by its event history in the
			// EventsRepository (the single source of truth for completion).
			var isProcessCompleted = func(t *testcase.T) bool {
				completed, err := workflow.Complete{ProcessID: process.Get(t)}.IsCompleted(context.Background(), c.Runtime.Get(t).Events)
				return err == nil && completed
			}

			s.Then("the participant is executed but process remains incomplete until suspend allows it to pass", func(t *testcase.T) {
				assert.NoError(t, act(t))

				// the participant is executed
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})

				// but the process remains incomplete while the suspend condition holds
				assert.NotWithin(t, deadline, func(ctx context.Context) {
					for !isProcessCompleted(t) {
						select {
						case <-t.Done():
							return
						default: // OK
						}
					}
				})

				// until suspend allows it to pass
				shouldSuspend.Set(t, false)

				t.Eventually(func(t *testcase.T) {
					assert.True(t, isProcessCompleted(t))
					// the participant remained idempotent across the suspend re-queues
					assert.Equal(t, callCount.Get(t), 1)
				})
			})
		})

		s.When("context is cancelled during scheduling", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // cancel immediately
				return ctx
			})

			process.Let(s, func(t *testcase.T) workflow.ProcessID {
				p := process.Super(t)
				return p
			})

			s.Then("scheduling fails with context cancellation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), context.Canceled)
			})
		})
	})
}

func TestRuntime_Run_publishBeforeCommitIsSupported(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		// participantN tracks how many times the participant ran, so we can
		// assert that the runtime only fires it once the definition is bound.
		participantN = let.VarOf(s, 0)
		pid          = wftest.LetParticipantID(s)
		_            = wftest.LetParticipantWithID(s, c, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				participantN.Set(t, participantN.Get(t)+1)
				return nil
			}
		})
	)

	s.Test("smoke", func(t *testcase.T) {
		def := workflow.SetVar{Key: "foo", Value: "bar"}
		procID, err := workflow.MakeProcessID()
		assert.NoError(t, err)

		t.Log("given we schedule the process id for execution")
		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), procID))

		t.Random.Repeat(3, 7, func() {
			c.ProcessCompletionIs(t, procID, false)
		})

		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), procID, def))

		assert.Eventually(t, time.Minute, func(tb testing.TB) {
			complete := workflow.Complete{ProcessID: procID}
			done, err := complete.IsCompleted(t.Context(), c.Runtime.Get(t).Events)
			assert.NoError(tb, err)
			assert.True(tb, done)
		})
	})
}
