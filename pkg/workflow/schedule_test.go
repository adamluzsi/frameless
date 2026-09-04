package workflow_test

import (
	"context"
	"runtime"
	"sync"
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
	_ = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context, v string) error {
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
					Name:  workflow.VarName("input"),
					Value: inputVal,
				},
				workflow.ExecuteParticipant{
					ID: pid.Get(t),
					Input: []workflow.VarName{
						workflow.VarName("input"),
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
			completed, err := workflow.IsCompleted(t.Context(), c.Runtime.Get(t).Events, processID)
			assert.NoError(t, err)
			assert.False(t, completed)
		})

		timecop.Travel(t, waitTime+time.Second)

		t.Eventually(func(t *testcase.T) {
			completed, err := workflow.IsCompleted(t.Context(), c.Runtime.Get(t).Events, processID)
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
					Name:  workflow.VarName("input"),
					Value: inputVal,
				},
				workflow.ExecuteParticipant{
					ID: pid.Get(t),
					Input: []workflow.VarName{
						workflow.VarName("input"),
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
			completed, err := workflow.IsCompleted(t.Context(), c.Runtime.Get(t).Events, processID)
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
		_         = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
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
				completed, err := workflow.IsCompleted(context.Background(), c.Runtime.Get(t).Events, process.Get(t))
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
		_            = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				participantN.Set(t, participantN.Get(t)+1)
				return nil
			}
		})
	)

	s.Test("smoke", func(t *testcase.T) {
		def := workflow.SetVar{Name: "foo", Value: "bar"}
		procID, err := workflow.MakeProcessID()
		assert.NoError(t, err)

		t.Log("given we schedule the process id for execution")
		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), procID))

		t.Random.Repeat(3, 7, func() {
			c.ProcessCompletionIs(t, procID, false)
		})

		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), procID, def))

		assert.Eventually(t, time.Minute, func(tb testing.TB) {
			done, err := workflow.IsCompleted(t.Context(), c.Runtime.Get(t).Events, procID)
			assert.NoError(tb, err)
			assert.True(tb, done)
		})
	})
}

// TestRuntime_Run_scheduledWithoutDefinition pins how a worker copes with an
// orphaned schedule entry: a ProcessID that was queued for execution but never
// had a Definition bound to it.
//
// This state is reachable, not hypothetical. Schedule and Bind are separate
// operations, so a crash, a rolled back transaction, or a plain forgotten Bind
// between the two leaves an entry in the queue that can never execute — which
// is why ErrNoProcessDefinition's own message guesses that "workflow.Runtime#Bind
// is forgotten". Runtime.Execute answers such an entry with that error, and
// ErrIsFatal classifies it as non-retryable.
//
// The entry is worthless, but it has to stay HARMLESS. The execution queue is
// shared by every process on the node, so one unbound ProcessID must not be
// able to stop the worker from serving everything else.
func TestRuntime_Run_scheduledWithoutDefinition(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		pid = wftest.LetParticipantID(s)
		_   = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error { return nil }
		})
	)

	// bindGracePeriod is how long the runtime keeps hoping that the Definition of
	// a scheduled Process is still on its way. It is set to almost nothing here,
	// so that the orphan below runs out of grace right away.
	bindGracePeriod := let.VarOf(s, time.Millisecond)

	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		var rt = c.Runtime.Super(t)
		rt.BindGracePeriod = bindGracePeriod.Get(t)
		return rt
	})

	// orphan is scheduled for execution but deliberately never bound, so every
	// attempt to execute it answers with ErrNoProcessDefinition.
	orphan := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		return mustProcessID(t)
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), orphan.Get(t)))
	})

	// executeCanary hands the worker an ordinary, fully bound process and
	// requires it to be executed. It is the proof that the worker is still
	// serving the queue at all.
	var executeCanary = func(t *testcase.T) {
		t.Helper()
		canary := mustProcessID(t)
		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), canary,
			workflow.ExecuteParticipant{ID: pid.Get(t)}))
		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), canary))
		c.ProcessCompletionIs(t, canary, true)
	}

	s.Then("the worker keeps serving the other processes on the queue", func(t *testcase.T) {
		// The worker only reacts to the orphan once it picks it up, so give it
		// the chance to do so before judging whether it survived the encounter.
		time.Sleep(waitTime)

		t.Random.Repeat(3, 7, func() {
			executeCanary(t)
		})
	})

	s.Then("the process is given up on, and a definition bound afterwards no longer revives it", func(t *testcase.T) {
		// Let the entry outlive its grace period and be dropped from the queue.
		time.Sleep(waitTime)

		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), orphan.Get(t),
			workflow.ExecuteParticipant{ID: pid.Get(t)}))

		// Dropping the entry is what keeps the worker alive, and this is its
		// price: nothing in the queue points at the process anymore, so a late
		// Bind is not enough on its own, the process has to be scheduled again.
		time.Sleep(waitTime)
		isCompleted, err := workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), orphan.Get(t))
		assert.NoError(t, err)
		assert.False(t, isCompleted)
	})

	s.When("the process still has time left to receive its definition", func(s *testcase.Spec) {
		bindGracePeriod.LetValue(s, time.Hour)

		s.Then("the worker keeps serving the other processes on the queue", func(t *testcase.T) {
			time.Sleep(waitTime)

			t.Random.Repeat(3, 7, func() {
				executeCanary(t)
			})
		})

		s.Then("binding the definition later still gets the process executed", func(t *testcase.T) {
			assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), orphan.Get(t),
				workflow.ExecuteParticipant{ID: pid.Get(t)}))

			c.ProcessCompletionIs(t, orphan.Get(t), true)
		})
	})
}

// TestRuntime_Run_numQueueSubscriber pins that Runtime#NumQueueSubscriber
// decides how many workers a runtime node actually runs.
//
// The subscriber count is the concurrency limit of a node. Each subscriber
// takes one entry off the execution queue at a time and executes it to
// completion before reaching for the next one, so the number of subscribers is
// the number of workflow processes the node can have in flight at once.
//
// Operators size that number against what the surroundings can absorb, be it a
// database connection pool, a rate limited third party API or the memory a
// single process needs, which makes a runtime that silently substitutes its own
// default for the configured value able to overrun every one of those budgets.
func TestRuntime_Run_numQueueSubscriber(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	numQueueSubscriber := let.VarOf(s, 1)

	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		var rt = c.Runtime.Super(t)
		rt.NumQueueSubscriber = numQueueSubscriber.Get(t)
		return rt
	})

	// More processes are scheduled than there are workers to run them, so the
	// workers never run out of work, and what limits the observed concurrency is
	// the size of the worker pool alone.
	numScheduledProcess := let.Var(s, func(t *testcase.T) int {
		return numQueueSubscriber.Get(t) + 2
	})

	var (
		// meter observes how many process executions overlap in time.
		meter = let.Var(s, func(t *testcase.T) *concurrencyMeter {
			return &concurrencyMeter{}
		})
		// release frees the participants that are parked in the meter.
		release = let.Var(s, func(t *testcase.T) chan struct{} {
			var ch = make(chan struct{})
			t.Cleanup(func() { close(ch) })
			return ch
		})
	)

	var (
		pid = wftest.LetParticipantID(s)
		_   = wftest.LetParticipantWithID(s, pid, func(t *testcase.T) func(ctx context.Context) error {
			// The meter and the release channel are resolved here, on the test's
			// own goroutine, because the participant itself is called by the
			// workers, and test variables are not meant to be raced upon.
			var (
				meter   = meter.Get(t)
				release = release.Get(t)
			)
			return func(ctx context.Context) error {
				meter.Enter()
				defer meter.Exit()
				// Occupy the worker, so that the number of participants parked
				// here at the same time tells how many workers are running.
				select {
				case <-release:
				case <-ctx.Done():
				}
				return nil
			}
		})
	)

	s.Before(func(t *testcase.T) {
		for range numScheduledProcess.Get(t) {
			processID := mustProcessID(t)
			assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID,
				workflow.ExecuteParticipant{ID: pid.Get(t)}))
			assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))
		}
	})

	s.Then("the node executes as many processes at once as it was configured to", func(t *testcase.T) {
		// Let the node take on all the work it is willing to take on.
		time.Sleep(waitTime)

		assert.Equal(t, meter.Get(t).Peak(), numQueueSubscriber.Get(t))
	})

	s.When("a larger pool is configured", func(s *testcase.Spec) {
		// The count is kept below the default pool size, and off its multiples,
		// so that a runtime running the default instead can't coincidentally
		// match the expectation on a host with few CPUs.
		numQueueSubscriber.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(2, 3)
		})

		s.Then("the node executes as many processes at once as it was configured to", func(t *testcase.T) {
			time.Sleep(waitTime)

			assert.Equal(t, meter.Get(t).Peak(), numQueueSubscriber.Get(t))
		})
	})

	s.When("the pool size is left unconfigured", func(s *testcase.Spec) {
		numQueueSubscriber.LetValue(s, 0)

		s.Then("the node falls back on a default pool that serves every scheduled process", func(t *testcase.T) {
			time.Sleep(waitTime)

			assert.Equal(t, meter.Get(t).Peak(), numScheduledProcess.Get(t))
		})
	})
}

// concurrencyMeter counts how many workflow process executions overlap in time.
type concurrencyMeter struct {
	mutex    sync.Mutex
	inFlight int
	peak     int
}

func (m *concurrencyMeter) Enter() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.inFlight++
	if m.peak < m.inFlight {
		m.peak = m.inFlight
	}
}

func (m *concurrencyMeter) Exit() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.inFlight--
}

// Peak is the highest number of executions that were ever in flight at once.
func (m *concurrencyMeter) Peak() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.peak
}
