package wfcontract

import (
	"context"
	"iter"
	"sort"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/guard/guardcontract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func EventRepository(subject workflow.EventRepository) contract.Contract {
	s := testcase.NewSpec(nil)
	var c crudcontract.Config[workflow.Event, workflow.EventID]
	c.Init()

	processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		pid, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return pid
	})

	c.MakeEntity = func(tb testing.TB) workflow.Event {
		var tc = testcase.ToT(&tb)
		return MakeEvent(tc, processID.Get(tc))
	}

	c.OnePhaseCommit = subject

	testcase.RunSuite(s,
		crudcontract.Creator(subject, c),
		crudcontract.AllFinder(subject, c),
		crudcontract.ByIDFinder(subject, c),
		crudcontract.OnePhaseCommitProtocol(subject, subject, c),
	)

	type WithDeleteByID interface {
		workflow.EventRepository
		crud.ByIDDeleter[workflow.EventID]
	}
	if sub, ok := subject.(WithDeleteByID); ok {
		testcase.RunSuite(s, crudcontract.ByIDDeleter[workflow.Event, workflow.EventID](sub, c))
	}

	type WithUpdate interface {
		workflow.EventRepository
		crud.Updater[workflow.Event]
	}
	if sub, ok := subject.(WithUpdate); ok {
		testcase.RunSuite(s, crudcontract.Updater[workflow.Event, workflow.EventID](sub, c))
	}

	s.Describe("#Create", func(s *testcase.Spec) {
		varEvent := let.Var(s, func(t *testcase.T) workflow.EventSetVar {
			processID, err := uuid.Parse(t.Random.UUID())
			assert.NoError(t, err)
			eventID, err := uuid.Parse(t.Random.UUID())
			return workflow.EventSetVar{
				EventID:   workflow.EventID(eventID),
				ProcessID: workflow.ProcessID(processID),
				Timestamp: t.Random.Time(),
				Key:       "foo",
				Value:     "bar",
			}
		})
		event := let.Var(s, func(t *testcase.T) workflow.Event {
			return varEvent.Get(t)
		})
		act := let.Act(func(t *testcase.T) error {
			return subject.Create(c.MakeContext(t), pointer.Of(event.Get(t)))
		})

		s.Then("it can persist a valid workflow event", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("EventID is unset", func(s *testcase.Spec) {
			varEvent.Let(s, func(t *testcase.T) workflow.EventSetVar {
				e := varEvent.Super(t)
				var zero workflow.EventID
				e.EventID = zero
				return e
			})

			s.Then("it must yield an error when an event doesn't have timestamp", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})

		s.When("timestamp is missing/zero", func(s *testcase.Spec) {
			varEvent.Let(s, func(t *testcase.T) workflow.EventSetVar {
				e := varEvent.Super(t)
				e.Timestamp = time.Time{}
				return e
			})

			s.Then("it must yield an error when an event doesn't have timestamp", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})

		s.When("processID is missing/zero", func(s *testcase.Spec) {
			varEvent.Let(s, func(t *testcase.T) workflow.EventSetVar {
				e := varEvent.Super(t)
				var zeroProcessID workflow.ProcessID
				e.ProcessID = zeroProcessID
				return e
			})

			s.Then("it must yield an error when an event doesn't have timestamp", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})

	s.Describe("#FindByProcessID", func(s *testcase.Spec) {
		var (
			ctx = let.Var(s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})
			pid = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				id, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				return id
			})
		)
		act := let.Act(func(t *testcase.T) iter.Seq2[workflow.Event, error] {
			return subject.FindByProcessID(ctx.Get(t), pid.Get(t))
		})

		s.Before(func(t *testcase.T) {
			spechelper.TryCleanup(t, c.MakeContext(t), subject)
		})

		crudcontract.QueryMany[workflow.Event, workflow.EventID](subject, "FindByProcessID",
			func(tb testing.TB) crudcontract.QueryManySubject[workflow.Event] {
				tc := testcase.ToT(&tb)
				pid := processID.Get(tc)
				othPID, err := workflow.MakeProcessID()
				assert.NoError(tc, err)
				return crudcontract.QueryManySubject[workflow.Event]{
					Query: func(ctx context.Context) iter.Seq2[workflow.Event, error] {
						return subject.FindByProcessID(ctx, pid)
					},
					IncludedEntity: func() workflow.Event {
						return MakeEvent(tc, pid)
					},
					ExcludedEntity: func() workflow.Event {
						return MakeEvent(tc, othPID)
					},
				}
			}).Spec(s)

		s.When("multiple entries are present in the storage", func(s *testcase.Spec) {
			var exp = let.VarOf[[]workflow.Event](s, nil)

			s.Before(func(t *testcase.T) {
				for range t.Random.IntBetween(3, 7) {
					timecop.Travel(t, t.Random.DurationBetween(-time.Minute, time.Minute))
					event := MakeEvent(t, pid.Get(t))
					c.Helper().Create(t, subject, t.Context(), &event)
					testcase.Append(t, exp, event)
				}
			})

			s.Then("process related entities are returned", func(t *testcase.T) {
				got, err := iterkit.CollectE(act(t))
				assert.NoError(t, err)
				assert.NotEmpty(t, got)
				assert.ContainsExactly(t, exp.Get(t), got)
			})

			s.Then("returned entities are ordered by their event timestamp - ASC", func(t *testcase.T) {
				got, err := iterkit.CollectE(act(t))
				assert.NoError(t, err)
				assert.NotEmpty(t, got)

				slicekit.SortBy(exp.Get(t), func(a, b workflow.Event) bool {
					return a.GetTimestamp().Before(b.GetTimestamp())
				})
				assert.Equal(t, exp.Get(t), got)
			})

			s.And("entities unrelated to the current process ID also present in the repository", func(s *testcase.Spec) {
				unrelated := let.Var(s, func(t *testcase.T) []workflow.Event {
					return make([]workflow.Event, 0)
				})

				othPid := let.Var(s, func(t *testcase.T) workflow.ProcessID {
					id, err := workflow.MakeProcessID()
					assert.NoError(t, err)
					return id
				})

				s.Before(func(t *testcase.T) {
					for range t.Random.IntBetween(3, 7) {
						timecop.Travel(t, t.Random.DurationBetween(-time.Minute, time.Minute))
						event := MakeEvent(t, othPid.Get(t))
						c.Helper().Create(t, subject, c.MakeContext(t), &event)
						testcase.Append(t, unrelated, event)
					}
				})

				s.Then("process related entities are returned", func(t *testcase.T) {
					got, err := iterkit.CollectE(act(t))
					assert.NoError(t, err)
					assert.NotEmpty(t, got)
					assert.ContainsExactly(t, exp.Get(t), got)
				})

				s.Then("unrelated process events are not returned", func(t *testcase.T) {
					got, err := iterkit.CollectE(act(t))
					assert.NoError(t, err)
					assert.NotEmpty(t, got)
					assert.ContainsExactly(t, exp.Get(t), got)

					for _, event := range unrelated.Get(t) {
						assert.NotContains(t, got, event)
					}
				})
			})
		})
	})

	return s.AsSuite("EventRepository")

}

func MakeEvent(tb testing.TB, processID workflow.ProcessID) workflow.Event {
	t := testcase.ToT(&tb)
	return random.Pick[func() workflow.Event](t.Random, func() workflow.Event {
		return workflow.EventSetVar{
			EventID:   MakeEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now(),
			Key:       "foo",
			Value:     t.Random.String(),
		}
	}, func() workflow.Event {
		return workflow.EventCompleted{
			EventID:   MakeEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now(),
		}
	}, func() workflow.Event {
		return workflow.EventParticipant{
			EventID:       MakeEventID(t),
			ProcessID:     processID,
			Timestamp:     clock.Now(),
			ParticipantID: "participant-id",
			Input: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return t.Random.String()
			}),
			Output: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return t.Random.String()
			}),
		}
	})()
}

func MakeEventID(tb testing.TB) workflow.EventID {
	t := testcase.ToT(&tb)
	id, err := uuid.Parse(t.Random.UUID())
	assert.NoError(tb, err)
	return workflow.EventID(id)
}

// ProcessLocks expresses that the subject acts as the workflow Runtime's
// per-process locker factory. Each ProcessID maps to its own
// guard.NonBlockingLocker, and acquiring a lock for one ProcessID must not
// interfere with a lock for a different one.
func ProcessLocks(subject workflow.ProcessLocks,
	opts ...guardcontract.LockerFactoryOption[workflow.ProcessID]) contract.Contract {
	s := testcase.NewSpec(nil)
	testcase.RunSuite(s, guardcontract.LockerFactory[workflow.ProcessID, workflow.ProcessLock](subject, opts...))
	return s.AsSuite("ProcessLockers")
}

// ProcessExecutionQueue expresses that the subject acts as the workflow Runtime's
// process scheduler queue. It is a non-blocking pub/sub channel whose
// published Schedules must be eventually delivered to subscribers ordered by
// Schedule.StartTime ascending. Publishers must NOT block waiting for
// subscriber acknowledgement — a blocking queue would deadlock the runtime.
func ProcessExecutionQueue(subject workflow.ProcessExecutionQueue, opts ...pubsubcontract.Option[workflow.ProcessExecution]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[pubsubcontract.Config[workflow.ProcessExecution], pubsubcontract.Option[workflow.ProcessExecution]](opts)

	testcase.RunSuite(s,
		pubsubcontract.Queue[workflow.ProcessExecution](subject, subject, c),
		pubsubcontract.Ordering[workflow.ProcessExecution](subject, subject,
			func(items []workflow.ProcessExecution) {
				sort.Slice(items, func(i, j int) bool {
					return items[i].StartTime.Before(items[j].StartTime)
				})
			},
			c,
		),
	)

	return s.AsSuite("ProcessQueue")
}

// ProcessChangeBroadcast expresses that the subject acts as the workflow
// Runtime's process-queue change-notification channel. The runtime uses it
// both to publish ProcessQueueChange events (e.g. ProcessStart) and to
// subscribe to them in runListenToChanges; therefore the contract asserts
// that messages are delivered to subscribers in publish order (Queue) and
// are not durable across subscriber reconnects (Volatile).
//
// The runtime's ProcessChangeBroadcast interface combines Publisher +
// Subscriber on a single value, so this wrapper takes that combined shape.
// Real implementations are expected to provide a fan-out under the hood
// (RabbitMQ topic exchange, NATS, Kafka consumer group, or an in-memory
// FanOutExchange with a single queue bound to it). To exercise the
// multi-subscriber fan-out contract directly, use pubsubcontract.Broadcast
// against the underlying exchange primitive.
func ProcessChangeBroadcast(
	subject workflow.ProcessChangeBroadcast,
	opts ...pubsubcontract.Option[workflow.ProcessChangeEvent],
) contract.Contract {
	s := testcase.NewSpec(nil)

	// ProcessChangeEvent is a polymorphic interface. The default MakeData
	// (spechelper.MakeValue) cannot fabricate an interface value, so we
	// supply one that picks a concrete implementation at random. This
	// also exercises the channel against every concrete type the runtime
	// could publish, which is what we actually want to assert.
	makeData := func(tb testing.TB) workflow.ProcessChangeEvent {
		var pid workflow.ProcessID
		if pidtb, err := workflow.MakeProcessID(); err == nil {
			pid = pidtb
		}
		switch tbRandom(tb).IntN(3) {
		case 0:
			return workflow.ProcessStart{ProcessID: pid}
		case 1:
			return workflow.ProcessStop{ProcessID: pid}
		default:
			return workflow.ProcessSleep{ProcessID: pid}
		}
	}

	opts = append(opts, pubsubcontract.Config[workflow.ProcessChangeEvent]{
		MakeData: makeData,
	})

	// The role interface combines Publisher + Subscriber on a single
	// subject. The contract asserts what that combined shape actually
	// delivers: a pub-sub channel where subscribers see published events
	// (in order, when subscribed before publish) and where re-subscribing
	// after the previous subscription ended does not replay prior events.
	//
	// We intentionally do NOT compose pubsubcontract.Queue here because
	// Queue's "multiple subscribers divide messages (unicast)" assertion
	// does not match the combined shape: a single subject can only carry
	// one effective subscription slot. We also avoid pubsubcontract.Broadcast
	// because Broadcast requires a separate Exchange + MakeSubscriber pair,
	// which the role interface does not expose. To exercise the fan-out
	// primitive directly, drive pubsubcontract.Broadcast against the
	// underlying FanOutExchange (see adapter/memory TestWorkflowFanOutBroadcast).
	pubsubcontract.Volatile[workflow.ProcessChangeEvent](subject, subject, opts...).Spec(s)

	s.Context("publish-then-consume", func(s *testcase.Spec) {
		c := option.ToConfig[pubsubcontract.Config[workflow.ProcessChangeEvent], pubsubcontract.Option[workflow.ProcessChangeEvent]](opts)
		mctx := c.MakeContext

		s.Test("a subscriber sees events published while it was subscribed", func(t *testcase.T) {
			ctx := mctx(t)

			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			sub := subject.Subscribe(subCtx)
			t.Cleanup(func() {
				for range sub {
				}
			})

			ev := makeData(t)
			assert.NoError(t, subject.Publish(ctx, ev))

			t.Eventually(func(it *testcase.T) {
				pubsubtest.Waiter.Wait()
				var got []workflow.ProcessChangeEvent
				for msg, err := range sub {
					assert.NoError(it, err)
					got = append(got, msg.Data())
					assert.NoError(it, msg.ACK())
					break
				}
				assert.ContainsExactly(it, []workflow.ProcessChangeEvent{ev}, got)
			})
		})
	})

	return s.AsSuite("ProcessQueueChangeBroadcast")
}

func Definition(mk func(tb testing.TB, c DefinitionContext) workflow.Definition) contract.Contract {
	s := testcase.NewSpec(nil)
	c := wftest.LetC(s)

	processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		id, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return id
	})

	defContext := let.Var(s, func(t *testcase.T) DefinitionContext {
		return DefinitionContext{
			tb:        t,
			processID: processID.Get(t),
		}
	})

	def := let.Var(s, func(t *testcase.T) workflow.Definition {
		return mk(t, defContext.Get(t))
	})

	s.Describe("Execute", func(s *testcase.Spec) {
		var (
			Context = let.Var(s, func(t *testcase.T) context.Context {
				return c.Runtime.Get(t).Context(t.Context())
			})
			ProcessID = processID.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return def.Get(t).Execute(Context.Get(t), ProcessID.Get(t))
		})

		s.Then("it can be executed", func(t *testcase.T) {
			assert.NoError(t, act(t),
				assert.MessageF("expected %T to execute successfully", def.Get(t)))
		})

		s.Then("repeated execution does not generate additional events", func(t *testcase.T) {
			assert.NoError(t, act(t))
			firstPassEvents := c.ProcessEvents(t, ProcessID.Get(t))
			if len(firstPassEvents) == 0 {
				return
			}

			assert.NoError(t, act(t))
			assert.ContainsExactly(t, firstPassEvents, c.ProcessEvents(t, ProcessID.Get(t)))
		})
	})

	return s.AsSuite("Definition")
}

type DefinitionContext struct {
	tb        testing.TB
	processID workflow.ProcessID
}

// MakeDefinition can be used in case your definition requires sub definitions as its dependency.
func (dc DefinitionContext) MakeDefinition(tb testing.TB) workflow.Definition {
	return definitionStub{
		ExecuteFunc: func(ctx context.Context, id workflow.ProcessID) error {
			if !dc.processID.IsZero() {
				assert.Equal(dc.tb, id, dc.processID)
			} else {
				assert.NotEmpty(dc.tb, id)
			}
			assert.NotEmpty(dc.tb, workflow.CurrentPath(ctx))
			return nil
		},
	}
}

type definitionStub struct {
	ExecuteFunc func(context.Context, workflow.ProcessID) error
}

func (definitionStub) Error() string { return "wfcontract.definitionStub" }

func (stub definitionStub) Execute(ctx context.Context, pid workflow.ProcessID) error {
	return stub.ExecuteFunc(ctx, pid)
}

// tbRandom returns the testcase.T's Random source when tb is a testcase TB,
// falling back to a crypto-seeded random otherwise. Mirrors spechelper.MakeValue's
// fallback so contract helpers stay deterministic under TESTCASE_SEED.
func tbRandom(tb testing.TB) *random.Random {
	if t, ok := tb.(*testcase.T); ok {
		return t.Random
	}
	return random.New(random.CryptoSeed{})
}
