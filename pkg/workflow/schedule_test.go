package workflow_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

const waitTime = time.Second / 8
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
				workflow.Execute{
					ParticipantID: pid.Get(t),
					Input: []workflow.VarName{
						workflow.VarName("input"),
					},
				},
			},
		}
		assert.NoError(t, c.EventRepository.Get(t).Create(rtCtx, &ev))

		waitTime := time.Hour * 24
		target := clock.Now().Add(waitTime)

		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID, func(s *workflow.ExecutionRequest) {
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
				workflow.Execute{
					ParticipantID: pid.Get(t),
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
			return subject.Get(t).Schedule(Context.Get(t), process.Get(t), func(s *workflow.ExecutionRequest) {
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
				return workflow.Execute{ParticipantID: pid.Get(t)}
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
				return workflow.Execute{ParticipantID: pid.Get(t)}
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
						assert.NoError(t, scheduler.Schedule(ctx, p, func(s *workflow.ExecutionRequest) {
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
					workflow.Execute{ParticipantID: pid.Get(t)},
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
			workflow.Execute{ParticipantID: pid.Get(t)}))
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
			workflow.Execute{ParticipantID: pid.Get(t)}))

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
				workflow.Execute{ParticipantID: pid.Get(t)}))

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
				workflow.Execute{ParticipantID: pid.Get(t)}))
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

// TestRuntime_Run_faultyDoesNotStarveHealthy pins that a Process which never
// recovers (a faulty participant that always errors) cannot starve a healthy
// Process scheduled on the same single-worker runtime.
//
// The runtime has one queue subscriber and orders entries by StartTime ASC.
// When a faulty Process fails, Runtime#runSignalHandler re-queues it with a
// StartTime pushed forward by Runtime#WaitTime, so it lands behind any
// healthy entry that was already queued. Without that deferral, a single
// worker would loop on the faulty entry forever and the healthy Process
// would never run.
//
// The test floods the queue with several faulty Processes alongside a single
// healthy one. The faulty ones are enough to keep retry pressure high; the
// healthy one is the canary whose completion proves the scheduler actually
// defers the faulty retries instead of hot-looping on them.
func TestRuntime_Run_faultyDoesNotStarveHealthy(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	// One worker is deliberate. With a larger pool the healthy Process
	// would be served by a free worker in parallel, and the test would
	// pass even if the faulty Processes were retried in a tight loop.
	// The single-worker setting is what makes this a regression test
	// rather than a happy accident — it pins that the scheduling
	// contract protects the healthy Process on its own.
	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		var rt = c.Runtime.Super(t)
		rt.NumQueueSubscriber = 1
		return rt
	})

	var (
		// faultyPID is registered with a participant that always returns
		// an error, so every Process bound to it is doomed to retry.
		faultyPID = wftest.LetParticipantID(s)
		_         = wftest.LetParticipantWithID(s, faultyPID, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				return errors.New("faulty participant never recovers")
			}
		})

		// healthyPID is registered with a participant that always
		// returns nil, so its Process completes on the first attempt.
		healthyPID = wftest.LetParticipantID(s)
		_          = wftest.LetParticipantWithID(s, healthyPID, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				return nil
			}
		})
	)

	// Flood the queue with faulty Processes so the worker has plenty
	// of failed entries to loop over if the deferral ever breaks.
	numFaulty := let.VarOf(s, 5)

	faultyProcesses := let.Var(s, func(t *testcase.T) []workflow.ProcessID {
		var pids []workflow.ProcessID
		for range numFaulty.Get(t) {
			pids = append(pids, mustProcessID(t))
		}
		return pids
	})

	healthyProcess := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		return mustProcessID(t)
	})

	s.Before(func(t *testcase.T) {
		for _, pid := range faultyProcesses.Get(t) {
			assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), pid,
				workflow.Execute{ParticipantID: faultyPID.Get(t)}))
		}
		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), healthyProcess.Get(t),
			workflow.Execute{ParticipantID: healthyPID.Get(t)}))
	})

	s.Test("a healthy Process reaches completion while faulty Processes are retrying", func(t *testcase.T) {
		// Schedule all the faulty ones first so they dominate the queue,
		// then schedule the healthy one. If the runtime ever forgets to
		// defer a faulty's retry, the worker will burn its budget on
		// the faulty entries and the healthy one will not complete.
		for _, pid := range faultyProcesses.Get(t) {
			assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), pid))
		}
		assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), healthyProcess.Get(t)))

		c.ProcessCompletionIs(t, healthyProcess.Get(t), true)
	})
}

func TestRuntime_Run_schedulePolicy(t *testing.T) {
	s := testcase.NewSpec(t)
	e := bindEnv(s)
	var (
		process = wftest.LetProcessID(s)
		queue   = let.Var(s, func(t *testcase.T) *scheduleObservedQueue {
			return &scheduleObservedQueue{
				availabilityQueue: availabilityQueue{Queue: e.queue.Get(t)},
				deliveries:        make(chan workflow.ExecutionRequest, 64),
				settlements:       make(chan scheduleSettlement, 64),
			}
		})
		calls           = let.Var(s, func(t *testcase.T) *atomic.Int64 { return new(atomic.Int64) })
		outcome         = let.Var[error](s, func(t *testcase.T) error { return nil })
		warningInterval = let.VarOf(s, 0)
		warnings        = let.Var(s, func(t *testcase.T) chan logging.Fields {
			ch := make(chan logging.Fields, 64)
			logger.Stub(t, func(l *logging.Logger) {
				l.Hijack = func(ctx context.Context, level logging.Level, msg string, fields logging.Fields) {
					if level == logging.LevelWarn {
						ch <- fields
					}
				}
			})
			return ch
		})
	)
	subject := e.runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := e.runtime.Super(t)
		rt.Queue = queue.Get(t)
		rt.NumQueueSubscriber = 1
		rt.WaitTime = time.Hour
		rt.ParticipantWarningInterval = warningInterval.Get(t)
		rt.RetryStrategy = scheduleRetryAttempts(3)
		return rt
	})
	s.Before(func(t *testcase.T) {
		timecop.Travel(t, clock.Now(), timecop.Freeze)
		warnings.Get(t)
		count, err := calls.Get(t), outcome.Get(t)
		assert.Must(t).NoError(subject.Get(t).Bind(t.Context(), process.Get(t), wftest.Stub{
			StubExecute: func(context.Context, workflow.ProcessID) error {
				count.Add(1)
				return err
			},
		}))
	})

	s.Describe("#Run", func(s *testcase.Spec) {
		act := func(t *testcase.T) { t.Go(subject.Get(t).Run) }

		s.Test("executes a due entry and acknowledges it", func(t *testcase.T) {
			assert.NoError(t, subject.Get(t).Schedule(t.Context(), process.Get(t)))
			act(t)
			settled := awaitScheduleValue(t, queue.Get(t).settlements)
			assert.True(t, settled.acked)
			assert.Equal(t, settled.request.ProcessID, process.Get(t))
			assert.Equal(t, calls.Get(t).Load(), int64(1))
		})

		s.When("execution fails fatally", func(s *testcase.Spec) {
			outcome.Let(s, func(t *testcase.T) error { return workflow.ErrFatal.F("permanent execution failure") })

			checkFatal := func(t *testcase.T) {
				rt := subject.Get(t)
				assert.NoError(t, rt.Schedule(t.Context(), process.Get(t)))
				act(t)
				settled := awaitScheduleValue(t, queue.Get(t).settlements)
				assert.True(t, settled.acked)
				assert.Equal(t, len(queue.Get(t).Requests()), 1, "fatal errors must not publish a retry")
				assert.Equal(t, calls.Get(t).Load(), int64(1))
				warning := awaitScheduleValue(t, warnings.Get(t))
				assert.Equal[any](t, warning["process_id"], process.Get(t).String())
				assert.NotEmpty(t, warning["error"])
				completed, err := workflow.IsCompleted(t.Context(), rt.Events, process.Get(t))
				assert.NoError(t, err)
				assert.False(t, completed)
				terminated, err := workflow.IsTerminated(t.Context(), rt.Events, process.Get(t))
				assert.NoError(t, err)
				assert.False(t, terminated)

				canary := mustProcessID(t)
				assert.NoError(t, rt.Spawn(t.Context(), canary, workflow.Sequence{}))
				assert.Equal(t, awaitScheduleValue(t, queue.Get(t).settlements).request.ProcessID, canary)
				assert.NoError(t, rt.Schedule(t.Context(), process.Get(t)))
				assert.Equal(t, awaitScheduleValue(t, queue.Get(t).settlements).request.ProcessID, process.Get(t))
				assert.Equal(t, calls.Get(t).Load(), int64(2), "explicit scheduling can retry an inert process")
			}
			s.Then("warns and drops only the scheduling entry without completing the process or stopping the worker", checkFatal)
			s.And("the fatal error is joined with a participant signature mismatch", func(s *testcase.Spec) {
				outcome.Let(s, func(t *testcase.T) error {
					return errors.Join(workflow.ErrFatal, workflow.ErrParticipantSignatureMismatch{
						ID: workflow.ParticipantID(t.Random.UUID()), Cause: workflow.ErrInvalidParticipantFunc,
					})
				})
				s.Then("drops the entry instead of requeueing for participant availability", checkFatal)
			})
			s.And("the error is classified as fatal without wrapping ErrFatal", func(s *testcase.Spec) {
				outcome.Let(s, func(t *testcase.T) error {
					return errors.Join(errors.New("invalid workflow"), workflow.ErrInvalidDefinition)
				})
				s.Then("also drops only the scheduling entry and keeps serving other processes", checkFatal)
			})
		})

		for _, availability := range []struct {
			name      string
			makeError func(workflow.ParticipantID) error
		}{
			{"missing", func(id workflow.ParticipantID) error { return workflow.ErrParticipantNotFound{ID: id} }},
			{"signature mismatch", func(id workflow.ParticipantID) error {
				return workflow.ErrParticipantSignatureMismatch{ID: id, Cause: workflow.ErrParticipantFuncMappingMismatch}
			}},
			{"pointer signature mismatch", func(id workflow.ParticipantID) error {
				return &workflow.ErrParticipantSignatureMismatch{ID: id, Cause: workflow.ErrParticipantFuncMappingMismatch}
			}},
			{"wrapped signature mismatch", func(id workflow.ParticipantID) error {
				return errors.Join(errors.New("participant lookup"), workflow.ErrParticipantSignatureMismatch{ID: id, Cause: workflow.ErrInvalidParticipantFunc})
			}},
		} {
			s.When("the participant is "+availability.name, func(s *testcase.Spec) {
				outcome.Let(s, func(t *testcase.T) error { return availability.makeError(workflow.ParticipantID(t.Random.UUID())) })

				checkRequeue := func(t *testcase.T) {
					rt, q := subject.Get(t), queue.Get(t)
					initialFailures := t.Random.IntBetween(1, 4)
					assert.NoError(t, rt.Schedule(t.Context(), process.Get(t), func(req *workflow.ExecutionRequest) { req.FailureCount = initialFailures }))
					initial := q.Requests()[0]
					act(t)
					assert.Equal(t, awaitScheduleValue(t, q.deliveries), initial)
					interval := warningInterval.Get(t)
					if interval <= 0 {
						interval = 5
					}
					for attempt := 1; attempt <= 2*interval+1; attempt++ {
						assert.True(t, awaitScheduleValue(t, q.settlements).acked)
						retry := awaitScheduleValue(t, q.deliveries)
						assert.Equal(t, retry.ProcessID, initial.ProcessID)
						assert.Equal(t, retry.CreatedAt, initial.CreatedAt)
						assert.Equal(t, retry.StartTime, clock.Now().Add(rt.WaitTime))
						assert.Equal(t, retry.FailureCount, initialFailures+attempt)
						assert.Equal(t, calls.Get(t).Load(), int64(attempt), "availability errors must not consume tight local retries")
						if retry.FailureCount%interval == 0 {
							warning := awaitScheduleValue(t, warnings.Get(t))
							assert.Equal[any](t, warning["process_id"], initial.ProcessID.String())
							assert.Equal[any](t, warning["failure_count"], retry.FailureCount)
							assert.Equal[any](t, warning["error"], outcome.Get(t).Error())
						}
						assert.Empty(t, warnings.Get(t), "warn only on interval boundaries")
						completed, err := workflow.IsCompleted(t.Context(), rt.Events, process.Get(t))
						assert.NoError(t, err)
						assert.False(t, completed)
						awaitScheduleListener(t, rt)
						if attempt <= 2*interval {
							timecop.Travel(t, retry.StartTime, timecop.Freeze)
						}
					}
					assert.Equal(t, len(q.Requests()), 2*interval+2, "keep requeueing beyond warning thresholds, without hard drop")
				}

				s.Then("counts delayed retries and warns every five failures without dropping the process", checkRequeue)
				s.And("a custom warning interval is configured", func(s *testcase.Spec) {
					warningInterval.LetValue(s, 3)
					s.Then("warns only at the configured failure interval", checkRequeue)
				})
				s.And("a negative warning interval is configured", func(s *testcase.Spec) {
					warningInterval.LetValue(s, -1)
					s.Then("uses the default warning interval", checkRequeue)
				})
			})
		}

		s.When("execution suspends after earlier failures", func(s *testcase.Spec) {
			outcome.LetValue(s, workflow.Suspend{})
			warningInterval.LetValue(s, 1)

			s.Then("delays the next attempt without increasing failures or warning", func(t *testcase.T) {
				rt, q := subject.Get(t), queue.Get(t)
				failures := t.Random.IntBetween(1, 20)
				assert.NoError(t, rt.Schedule(t.Context(), process.Get(t), func(req *workflow.ExecutionRequest) { req.FailureCount = failures }))
				act(t)
				initial := awaitScheduleValue(t, q.deliveries)
				assert.True(t, awaitScheduleValue(t, q.settlements).acked)
				retry := awaitScheduleValue(t, q.deliveries)
				assert.Equal(t, retry.FailureCount, failures)
				assert.Equal(t, retry.CreatedAt, initial.CreatedAt)
				assert.Equal(t, retry.StartTime, clock.Now().Add(rt.WaitTime))
				assert.Equal(t, calls.Get(t).Load(), int64(1))
				assert.Empty(t, warnings.Get(t))
			})
		})

		s.When("a future entry has already been delivered", func(s *testcase.Spec) {
			request := let.Var(s, func(t *testcase.T) workflow.ExecutionRequest {
				return workflow.ExecutionRequest{ProcessID: process.Get(t), StartTime: clock.Now().Add(24 * time.Hour), CreatedAt: clock.Now(), FailureCount: t.Random.IntBetween(1, 20)}
			})
			s.Before(func(t *testcase.T) {
				// Publish without a wake-up: readiness is established by the probe below.
				assert.Must(t).NoError(subject.Get(t).Queue.Publish(t.Context(), request.Get(t)))
			})

			s.Then("reselects an immediate job on notification and retains the future entry until its deadline", func(t *testcase.T) {
				act(t)
				q, rt := queue.Get(t), subject.Get(t)
				assert.Equal(t, awaitScheduleValue(t, q.deliveries), request.Get(t))
				awaitScheduleListener(t, rt)

				canary := mustProcessID(t)
				assert.NoError(t, rt.Spawn(t.Context(), canary, workflow.Sequence{}))
				deferred := awaitScheduleValue(t, q.settlements)
				assert.False(t, deferred.acked, "release the future delivery, do not drop it")
				assert.Equal(t, deferred.request, request.Get(t))
				assert.Equal(t, awaitScheduleValue(t, q.deliveries).ProcessID, canary)
				assert.True(t, awaitScheduleValue(t, q.settlements).acked)
				assert.Equal(t, awaitScheduleValue(t, q.deliveries), request.Get(t))
				awaitScheduleListener(t, rt)
				assert.Equal(t, calls.Get(t).Load(), int64(0))
				assert.Empty(t, q.deliveries, "reselection must wait rather than redeliver in a busy loop")
				assert.Equal(t, len(q.Requests()), 2, "NACK must not publish a replacement or another wake-up")

				timecop.Travel(t, request.Get(t).StartTime, timecop.Freeze)
				settled := awaitScheduleValue(t, q.settlements)
				assert.True(t, settled.acked)
				assert.Equal(t, settled.request, request.Get(t))
				assert.Equal(t, calls.Get(t).Load(), int64(1))
			})
		})
	})
}

type scheduleRetryAttempts int

func (n scheduleRetryAttempts) ShouldTry(ctx context.Context, attempt resilience.RetryAttempt) bool {
	return ctx.Err() == nil && attempt.FailureCount < int(n)
}

type scheduleSettlement struct {
	request workflow.ExecutionRequest
	acked   bool
}

// Observe real queue delivery and settlement without changing ordering or durability.
type scheduleObservedQueue struct {
	availabilityQueue
	deliveries  chan workflow.ExecutionRequest
	settlements chan scheduleSettlement
}

func (q *scheduleObservedQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ExecutionRequest] {
	return func(yield func(pubsub.Message[workflow.ExecutionRequest], error) bool) {
		for msg, err := range q.Queue.Subscribe(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			req := msg.Data()
			wrapped := scheduleObservedMessage{Message: msg, ctx: ctx, settlements: q.settlements}
			select {
			case q.deliveries <- req:
			case <-ctx.Done():
				return
			}
			if !yield(wrapped, nil) {
				return
			}
		}
	}
}

type scheduleObservedMessage struct {
	pubsub.Message[workflow.ExecutionRequest]
	ctx         context.Context
	settlements chan<- scheduleSettlement
}

func (msg scheduleObservedMessage) ACK() error {
	err := msg.Message.ACK()
	msg.record(true)
	return err
}

func (msg scheduleObservedMessage) NACK() error {
	err := msg.Message.NACK()
	msg.record(false)
	return err
}

func (msg scheduleObservedMessage) record(acked bool) {
	select {
	case msg.settlements <- scheduleSettlement{request: msg.Data(), acked: acked}:
	case <-msg.ctx.Done():
	}
}

func awaitScheduleValue[T any](tb testing.TB, ch <-chan T) T {
	tb.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		tb.Fatal("scheduler did not reach the expected synchronization point")
	}
	return *new(T)
}

// The unrelated cancellation is a harmless barrier: NotificationType is called
// by the worker, not the fan-out forwarders, once it is listening to notifications.
type scheduleListenerProbe struct {
	seen chan struct{}
	once sync.Once
}

func (p *scheduleListenerProbe) GetProcessID() workflow.ProcessID { return workflow.ProcessID{} }
func (p *scheduleListenerProbe) NotificationType() workflow.NotificationType {
	p.once.Do(func() { close(p.seen) })
	return (workflow.ProcessCancel{}).NotificationType()
}
func awaitScheduleListener(t *testcase.T, rt workflow.Runtime) {
	t.Helper()
	probe := &scheduleListenerProbe{seen: make(chan struct{})}
	t.Eventually(func(t *testcase.T) {
		assert.NoError(t, rt.Notifications.Publish(t.Context(), probe))
		select {
		case <-probe.seen:
		default:
			t.FailNow()
		}
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
