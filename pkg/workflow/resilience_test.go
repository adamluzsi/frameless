package workflow_test

import (
	"context"
	"iter"
	"math/rand"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Test_resilience(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := c.Runtime.Super(t)
		t.Log("given the retry strategy is quick to retry again")
		rt.RetryStrategy = resilience.Jitter{
			Delay:    time.Nanosecond,
			Attempts: 9999,
		}
		return rt
	})

	_, pid := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			return ctx.Err()
		}
	})

	// greet can only greet with the name it is given.
	_, greet := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context, string) (string, error) {
		return func(ctx context.Context, name string) (string, error) {
			return "hello " + name, ctx.Err()
		}
	})

	// itIsResilient is what must hold, whatever faults a dependency of the Runtime has.
	itIsResilient := func(s *testcase.Spec) {
		s.Test("scheduled tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.Execute{Participant: pid.Get(t)},
				workflow.Execute{Participant: pid.Get(t)},
				workflow.Execute{Participant: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))
				assert.Eventually(t, 5*time.Second, func(tb testing.TB) {
					done, err := workflow.IsCompleted(tb.Context(), c.EventRepository.Get(t), processID)
					assert.NoError(tb, err)
					assert.True(tb, done)
				})
			})
		})

		s.Test("executed tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.Execute{Participant: pid.Get(t)},
				workflow.Execute{Participant: pid.Get(t)},
				workflow.Execute{Participant: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Execute(t.Context(), processID))

				isCompleted, err := workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), processID)
				assert.NoError(t, err)
				assert.True(t, isCompleted)
			})
		})

		s.Test("variables passed to a spawned child and collected from it are not lost", func(t *testcase.T) {
			t.Random.Repeat(3, 7, func() {
				name := t.Random.UUID()
				// greeting := make(chan string)
				// go func(name string) { greeting <- greet(name) }(name)
				// <-greeting
				var def = workflow.Sequence{
					workflow.SetVar{Name: "name", Value: name},
					workflow.Spawn{
						Name: "greeter",
						Definition: workflow.Execute{
							Participant: greet.Get(t),
							Input:       []workflow.VarName{"name"},
							Output:      []workflow.VarName{"greeting"},
						},
						Vars: workflow.VarMapping{"name": "name"},
					},
					workflow.Join{Spawn: "greeter", Collect: workflow.VarMapping{"greeting": "greeting"}},
				}
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))
				assert.Eventually(t, 5*time.Second, func(tb testing.TB) {
					done, err := workflow.IsCompleted(tb.Context(), c.EventRepository.Get(t), processID)
					assert.NoError(tb, err)
					assert.True(tb, done, "the Join waits for the child, which can't greet without the name it is given")
				})

				vars := workflow.Vars{ProcessID: processID, EventsRepository: c.EventRepository.Get(t)}
				greeting, err := vars.Get(t.Context(), "greeting")
				assert.NoError(t, err)
				assert.Equal[any](t, "hello "+name, greeting)
			})
		})
	}

	// faults is where the flaky dependencies take their faults from, while the Runtime's goroutines use them at once.
	// It is not t.Random, as the test keeps using that one too,
	// and random.Random#Read, so random.Random#UUID, is not guarded against concurrent use.
	faults := let.Var(s, func(t *testcase.T) *random.Random {
		return random.New(rand.NewSource(int64(t.Random.Int())))
	})

	s.Context("flaky event repository", func(s *testcase.Spec) {
		statementLevel := let.VarOf(s, false)
		c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			rt := c.Runtime.Super(t)
			rt.Events = &FlakyEventRepository{
				R:              c.EventRepository.Get(t),
				Random:         faults.Get(t),
				StatementLevel: statementLevel.Get(t),
			}
			return rt
		})

		itIsResilient(s)

		s.When("a fault fails only the operation it hits, and leaves its transaction open", func(s *testcase.Spec) {
			statementLevel.LetValue(s, true)

			itIsResilient(s)
		})
	})

	s.Context("flaky queue", func(s *testcase.Spec) {
		c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			rt := c.Runtime.Super(t)
			rt.Queue = &FlakyQueue{
				Q:   c.Queue.Get(t),
				RND: faults.Get(t),
			}
			return rt
		})

		itIsResilient(s)
	})
}

// TestRuntime_Execute_runtimeSignalIsNotRetried pins that Runtime#Execute does
// not spend its retry budget on a workflow.RuntimeSignal.
//
// The retry loop is there for faults. A RuntimeSignal is not one — it is the
// runtime's own control flow. workflow.Suspend in particular means "come back
// later", and coming back later is the scheduler's job: Runtime#runSignalHandler
// re-queues a suspended process WITHOUT incrementing its FailureCount, precisely
// because a suspension is not a failure.
//
// Retrying it inside a single Execute contradicts that, and it is not free.
// Raising a signal is deliberately never recorded in the event history (see
// TestRuntime_Execute_participantRuntimeSignal), so there is nothing to replay:
// every retry walks back to the waiting step and asks it again, immediately,
// which is the opposite of what the step just asked for.
func TestRuntime_Execute_runtimeSignalIsNotRetried(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := c.Runtime.Super(t)
		t.Log("given the runtime has a retry budget to spend")
		rt.RetryStrategy = resilience.Jitter{
			Delay:    time.Nanosecond,
			Attempts: 5,
		}
		return rt
	})

	var participantCalls = let.VarOf(s, 0)

	_, participantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			participantCalls.Set(t, participantCalls.Get(t)+1)
			return workflow.Suspend{}
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Execute{Participant: participantID.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("the signal is handed back to the caller", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Suspend{})
		})

		s.Then("the suspending participant is asked exactly once", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Suspend{})

			assert.Equal(t, 1, participantCalls.Get(t),
				assert.MessageF("a suspension is not a fault to retry; "+
					"resuming a suspended process belongs to the scheduler"))
		})
	})
}

func flake(rnd *random.Random) bool {
	if rnd == nil {
		return false
	}
	return rnd.IntBetween(0, 100) <= 10
}

// FlakyEventRepository is an event log behind an unreliable connection:
// any of its operations may fail with a fault.
type FlakyEventRepository struct {
	R *memory.WorkflowEventRepository

	*random.Random

	// StatementLevel makes a fault fail only the operation it hits,
	// while the transaction of the operation stays open, and it can still be committed,
	// like after a failed statement in MySQL.
	// Otherwise, a fault aborts the transaction as a whole, like in PostgreSQL.
	//
	// Either way, a failed commit commits nothing.
	StatementLevel bool
}

var _ workflow.EventRepository = (*FlakyEventRepository)(nil)

// fault makes the operation of ctx fail at random, in the way StatementLevel tells.
func (r *FlakyEventRepository) fault(ctx context.Context) (context.Context, error) {
	if !r.StatementLevel {
		return r.withFlake(ctx), nil
	}
	if flake(r.Random) {
		return ctx, r.Random.Error()
	}
	return ctx, nil
}

// withFlake makes ctx fail the operation it is used for at random,
// while it aborts the transaction of ctx too, if there is one.
func (r *FlakyEventRepository) withFlake(ctx context.Context) context.Context {
	if !flake(r.Random) {
		return ctx
	}

	if _, ok := r.R.LookupTx(ctx); ok {
		_ = r.R.RollbackTx(ctx)
		return ctx
	}

	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}

func (r *FlakyEventRepository) BeginTx(ctx context.Context) (context.Context, error) {
	ctx, err := r.fault(ctx)
	if err != nil {
		return ctx, err
	}
	return r.R.BeginTx(ctx)
}

func (r *FlakyEventRepository) CommitTx(ctx context.Context) error {
	return r.R.CommitTx(r.withFlake(ctx))
}

func (r *FlakyEventRepository) RollbackTx(ctx context.Context) error {
	return r.R.RollbackTx(r.withFlake(ctx))
}

func (r *FlakyEventRepository) Create(ctx context.Context, ptr *workflow.Event) error {
	ctx, err := r.fault(ctx)
	if err != nil {
		return err
	}
	return r.R.Create(ctx, ptr)
}

func (r *FlakyEventRepository) Update(ctx context.Context, ptr *workflow.Event) error {
	ctx, err := r.fault(ctx)
	if err != nil {
		return err
	}
	return r.R.Update(ctx, ptr)
}

func (r *FlakyEventRepository) FindByID(ctx context.Context, id workflow.EventID) (workflow.Event, bool, error) {
	ctx, err := r.fault(ctx)
	if err != nil {
		return nil, false, err
	}
	return r.R.FindByID(ctx, id)
}

func (r *FlakyEventRepository) DeleteByID(ctx context.Context, id workflow.EventID) error {
	ctx, err := r.fault(ctx)
	if err != nil {
		return err
	}
	return r.R.DeleteByID(ctx, id)
}

func (r *FlakyEventRepository) FindAll(ctx context.Context) iter.Seq2[workflow.Event, error] {
	ctx, err := r.fault(ctx)
	if err != nil {
		return iterkit.Error[workflow.Event](err)
	}
	return r.R.FindAll(ctx)
}

func (r *FlakyEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	ctx, err := r.fault(ctx)
	if err != nil {
		return iterkit.Error[workflow.Event](err)
	}
	return r.R.FindByProcessID(ctx, pid)
}

type FlakyQueue struct {
	Q *memory.WorkflowQueue

	RND *random.Random
}

var _ workflow.Queue = (*FlakyQueue)(nil)

func (q *FlakyQueue) Publish(ctx context.Context, d workflow.ExecutionRequest) error {
	return q.Q.Publish(q.withFlake(ctx), d)
}

func (q *FlakyQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ExecutionRequest] {
	return func(yield func(pubsub.Message[workflow.ExecutionRequest], error) bool) {
		if flake(q.RND) {
			yield(nil, q.RND.Error())
			return
		}
		for msg, err := range q.Q.Subscribe(ctx) {
			if err != nil {
				if !yield(msg, err) {
					return
				}
				continue
			}
			if !yield(&FlakyMessage{Q: q, Message: msg}, nil) {
				return
			}
		}
	}
}

type FlakyMessage struct {
	Q *FlakyQueue
	pubsub.Message[workflow.ExecutionRequest]
}

func (msg *FlakyMessage) ACK() error {
	if flake(msg.Q.RND) {
		msg.Message.NACK()
		return msg.Q.RND.Error()
	}
	return msg.Message.ACK()
}

func (q *FlakyQueue) withFlake(ctx context.Context) context.Context {
	if !flake(q.RND) {
		return ctx
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}
