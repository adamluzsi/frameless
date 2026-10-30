package wfcontract

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
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
			return workflow.EventSetVar{
				EventID:   wftest.MakeEventID(t),
				ProcessID: wftest.MakeProcessID(t),
				Timestamp: t.Random.Time(),
				Name:      "foo",
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

		s.When("EventID is missing", func(s *testcase.Spec) {
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

		s.When("ProcessID is missing", func(s *testcase.Spec) {
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
					return a.GetEventID().Less(b.GetEventID())
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
			EventID:   wftest.MakeEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now().UTC(),
			Name:      "foo",
			Value:     t.Random.String(),
		}
	}, func() workflow.Event {
		return workflow.EventCompleted{
			EventID:   wftest.MakeEventID(t),
			ProcessID: processID,
			Timestamp: clock.Now().UTC(),
		}
	}, func() workflow.Event {
		return workflow.EventParticipant{
			EventID:       wftest.MakeEventID(t),
			ProcessID:     processID,
			Timestamp:     clock.Now().UTC(),
			ParticipantID: "participant-id",
			Input: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return t.Random.String()
			}),
			Output: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return t.Random.String()
			}),
		}
	}, func() workflow.Event {
		return workflow.EventError{
			EventID:       wftest.MakeEventID(t),
			ProcessID:     processID,
			Timestamp:     clock.Now().UTC(),
			Error:         t.Random.Error().Error(),
			ParticipantID: "participant-id",
		}
	})()
}

// ProcessLocks expresses that the subject acts as the workflow Runtime's
// per-process locker factory. Each ProcessID maps to its own
// guard.NonBlockingLocker, and acquiring a lock for one ProcessID must not
// interfere with a lock for a different one.
func ProcessLocks(subject workflow.ProcessLocks,
	opts ...guardcontract.LockerFactoryOption[workflow.ProcessID]) contract.Contract {
	s := testcase.NewSpec(nil)
	testcase.RunSuite(s, guardcontract.LockerFactory[workflow.ProcessID, workflow.Lock](subject, opts...))
	return s.AsSuite("ProcessLocks")
}

// Queue expresses that the subject acts as the workflow Runtime's
// process scheduler queue. It is a non-blocking pub/sub channel whose
// published Schedules must be eventually delivered to subscribers ordered by
// Schedule.StartTime ascending. Publishers must NOT block waiting for
// subscriber acknowledgement — a blocking queue would deadlock the runtime.
//
// The workflow queue is also expected to be NonTransactional: publishes
// performed under a delivery's message context (notably Spawn's child
// schedule, which is enqueued from inside the parent's execution) must be
// visible to other workers as soon as they are made, and must not be rolled
// back by NACK. The runtime guarantees repeatability through the event log
// (replay-safe execution, per-process locks, EventCompleted short-circuit),
// so it has no use for transactional delivery semantics — and would be
// broken by them, because a busy parent would otherwise hold its spawned
// children's schedule entries invisible until its own execution ACKed.
func Queue(subject workflow.Queue, opts ...pubsubcontract.Option[workflow.ExecutionRequest]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[pubsubcontract.Config[workflow.ExecutionRequest], pubsubcontract.Option[workflow.ExecutionRequest]](opts)

	var sorting = func(items []workflow.ExecutionRequest) {
		sort.Slice(items, func(i, j int) bool {
			return items[i].StartTime.Before(items[j].StartTime)
		})
	}

	testcase.RunSuite(s,
		pubsubcontract.Queue(subject, subject, c),
		pubsubcontract.Ordering(subject, subject, sorting, c),
		pubsubcontract.NonTransactionalMessageContext(subject, subject, c),
	)

	return s.AsSuite("Queue")
}

// NotificationBroadcast expresses that the subject acts as the workflow
// Runtime's process-queue change-notification channel. The runtime uses it
// both to publish ProcessQueueChange events (e.g. ProcessStart) and to
// subscribe to them in runListenToChanges; therefore the contract asserts
// that messages are delivered to subscribers in publish order (Queue) and
// are not durable across subscriber reconnects (Volatile).
//
// The runtime's NotificationBroadcast interface combines Publisher +
// Subscriber on a single value, so this wrapper takes that combined shape.
// Real implementations are expected to provide a fan-out under the hood
// (RabbitMQ topic exchange, NATS, Kafka consumer group, or an in-memory
// FanOutExchange with a single queue bound to it). To exercise the
// multi-subscriber fan-out contract directly, use pubsubcontract.Broadcast
// against the underlying exchange primitive.
func NotificationBroadcast(
	subject workflow.NotificationBroadcast,
	opts ...pubsubcontract.Option[workflow.Notification],
) contract.Contract {
	s := testcase.NewSpec(nil)

	// Notification is a polymorphic interface. The default MakeData
	// (spechelper.MakeValue) cannot fabricate an interface value, so we
	// supply one that picks a concrete implementation at random. This
	// also exercises the channel against every concrete type the runtime
	// could publish, which is what we actually want to assert.
	makeData := func(tb testing.TB) workflow.Notification {
		pid, err := workflow.MakeProcessID()
		assert.NoError(tb, err)
		return random.Pick[workflow.Notification](tbRandom(tb),
			workflow.ProcessSchedule{ProcessID: pid},
			workflow.ProcessCancel{ProcessID: pid},
		)
	}

	opts = append(opts, pubsubcontract.Config[workflow.Notification]{
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
	pubsubcontract.Volatile[workflow.Notification](subject, subject, opts...).Spec(s)

	s.Context("publish-then-consume", func(s *testcase.Spec) {
		c := option.ToConfig[pubsubcontract.Config[workflow.Notification], pubsubcontract.Option[workflow.Notification]](opts)
		mctx := c.MakeContext

		s.Test("a subscriber sees events published while it was subscribed", func(t *testcase.T) {
			ctx := mctx(t)

			subCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)

			sub := subject.Subscribe(subCtx)

			ev := makeData(t)
			assert.NoError(t, subject.Publish(ctx, ev))

			t.Eventually(func(it *testcase.T) {
				pubsubtest.Waiter.Wait()
				var got []workflow.Notification
				for msg, err := range sub {
					assert.NoError(it, err)
					got = append(got, msg.Data())
					assert.NoError(it, msg.ACK())
					break
				}
				assert.ContainsExactly(it, []workflow.Notification{ev}, got)
			})
		})
	})

	return s.AsSuite("NotificationBroadcast")
}

// Definition is the contract of a workflow.Definition implementation.
//
// It asserts that the definition, and the events it records, round-trip through
// the codec of DefinitionConfig, which is a wfjson codec by default,
// as the runtime persists both with its codec.
func Definition(mk func(tb testing.TB, c DefinitionContext) workflow.Definition, opts ...DefinitionOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := wftest.LetC(s)
	o := option.ToConfig(opts)

	processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		id, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return id
	})

	subDefs := let.Var(s, func(t *testcase.T) *subDefinitions {
		return &subDefinitions{}
	})

	defContext := let.Var(s, func(t *testcase.T) DefinitionContext {
		return DefinitionContext{subDefinitions: subDefs.Get(t)}
	})

	def := let.Var(s, func(t *testcase.T) workflow.Definition {
		return mk(t, defContext.Get(t))
	})

	s.Test("it round-trips through the codec", func(t *testcase.T) {
		t.OnFail(func() { t.Log(codecHint) })
		assertRoundTripDefinition(t, o.Codec, def.Get(t))
	})

	s.Describe("Execute", func(s *testcase.Spec) {
		var (
			Context = let.Var(s, func(t *testcase.T) context.Context {
				ctx := c.Runtime.Get(t).Context(t.Context())
				return workflow.ContextWithParticipants(ctx, subDefs.Get(t))
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

		s.Then("the events it records round-trip through the codec", func(t *testcase.T) {
			assert.NoError(t, act(t))
			t.OnFail(func() { t.Log(codecHint) })
			for _, event := range c.ProcessEvents(t, ProcessID.Get(t)) {
				assertRoundTripEvent(t, o.Codec, event)
			}
		})

		s.Then("its sub definitions are executed within the process of the definition", func(t *testcase.T) {
			assert.NoError(t, act(t))

			events := c.ProcessEvents(t, ProcessID.Get(t))
			for _, exec := range subDefs.Get(t).Executions() {
				recorded := slices.ContainsFunc(events, func(event workflow.Event) bool {
					call, ok := event.(workflow.EventParticipant)
					return ok && call.ParticipantID == exec.ID && call.Path.Equal(exec.Path)
				})
				assert.True(t, recorded, assert.MessageF(
					"the sub definition at %q was executed outside of the process of the definition; "+
						"execute it with the ProcessID that %T was executed with",
					formatPath(exec.DefinitionPath()), def.Get(t)))
			}
		})

		// The contract executes the definition at the root path,
		// so an empty path is the path of the definition itself.
		s.Then("its sub definitions are executed below the path of the definition", func(t *testcase.T) {
			assert.NoError(t, act(t))

			for _, exec := range subDefs.Get(t).Executions() {
				assert.NotEmpty(t, exec.DefinitionPath(), assert.MessageF(
					"a sub definition of %T was executed at the path of the definition itself; "+
						"give it a path segment of its own with workflow.WithName", def.Get(t)))
			}
		})

		// The Path is the identity of a step in the event history. Two
		// executions at the same Path look like a single step to the runtime,
		// so the replay guards of the second one find the records of the first.
		//
		// Only an equal Path is a collision: a sub definition may run within the
		// Path of another, as For runs its Do within the Path of its Post.
		s.Then("its sub definitions are executed at distinct paths", func(t *testcase.T) {
			assert.NoError(t, act(t))

			execs := subDefs.Get(t).Executions()
			for i, exec := range execs {
				path := exec.DefinitionPath()
				for _, prev := range execs[:i] {
					assert.False(t, path.Equal(prev.DefinitionPath()), assert.MessageF(
						"two sub definition executions of %T share the path %q; "+
							"give each sub definition a distinct path segment with workflow.WithName",
						def.Get(t), formatPath(path)))
				}
			}
		})
	})

	return s.AsSuite("Definition")
}

const codecHint = "The runtime persists the definition and the events it records with its workflow.Codec. " +
	"Register your types on the codec, and pass it to the contract with wfcontract.DefinitionConfig{Codec: codec}."

type DefinitionContext struct {
	subDefinitions *subDefinitions
}

type DefinitionOption option.Option[DefinitionConfig]

type DefinitionConfig struct {
	// Codec is the codec that the definition and its events must round-trip through.
	// It defaults to wfjson.NewCodec().
	Codec workflow.Codec
}

func (c *DefinitionConfig) Init() {
	c.Codec = wfjson.NewCodec()
}

func (c DefinitionConfig) Configure(t *DefinitionConfig) {
	t.Codec = cmp.Or(c.Codec, t.Codec)
}

// MakeDefinition can be used in case your definition requires sub definitions as its dependency.
//
// The sub definition is a workflow.Execute of a participant that only the contract knows about,
// so it round-trips through any codec that supports the built-in definitions.
// The contract asserts that the sub definition is executed within the process of the definition,
// below the path of the definition, and that no two executions of sub definitions share a path.
func (dc DefinitionContext) MakeDefinition(tb testing.TB) workflow.Definition {
	return workflow.Execute{Participant: dc.subDefinitions.Make()}
}

// subDefinitions makes the sub definitions of DefinitionContext#MakeDefinition,
// and serves as the ParticipantRepository of their participants,
// which record where they were executed.
type subDefinitions struct {
	mutex      sync.Mutex
	ids        []workflow.ParticipantID
	executions []subDefinitionExecution
}

var _ workflow.ParticipantRepository = (*subDefinitions)(nil)

type subDefinitionExecution struct {
	ID workflow.ParticipantID
	// Path is the path of the participant call,
	// which is the path of the sub definition followed by the participant ID.
	Path workflow.Path
}

// DefinitionPath is the path that the sub definition was executed at.
func (e subDefinitionExecution) DefinitionPath() workflow.Path {
	return e.Path[:len(e.Path)-1]
}

func (sd *subDefinitions) Make() workflow.ParticipantID {
	const prefix = "wfcontract::sub-definition"
	if sd == nil { // a DefinitionContext made outside of the contract
		return prefix
	}
	sd.mutex.Lock()
	defer sd.mutex.Unlock()
	id := workflow.ParticipantID(fmt.Sprintf("%s-%d", prefix, len(sd.ids)+1))
	sd.ids = append(sd.ids, id)
	return id
}

func (sd *subDefinitions) FindByID(_ context.Context, id workflow.ParticipantID) (workflow.Participant, bool, error) {
	sd.mutex.Lock()
	defer sd.mutex.Unlock()
	if !slices.Contains(sd.ids, id) {
		return workflow.Participant{}, false, nil
	}
	return workflow.Participant{ID: id, Func: func(ctx context.Context) error {
		sd.record(subDefinitionExecution{ID: id, Path: workflow.CurrentPath(ctx)})
		return nil
	}}, true, nil
}

func (sd *subDefinitions) record(exec subDefinitionExecution) {
	sd.mutex.Lock()
	defer sd.mutex.Unlock()
	sd.executions = append(sd.executions, exec)
}

func (sd *subDefinitions) Executions() []subDefinitionExecution {
	sd.mutex.Lock()
	defer sd.mutex.Unlock()
	return slices.Clone(sd.executions)
}

func formatPath(path workflow.Path) string {
	var names = make([]string, 0, len(path))
	for _, scope := range path {
		names = append(names, scope.Name)
	}
	return strings.Join(names, "/")
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
