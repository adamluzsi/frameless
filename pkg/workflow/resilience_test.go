package workflow_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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

	_, pid := wftest.LetParticipant(s, c, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			return ctx.Err()
		}
	})

	s.Context("flaky event repository", func(s *testcase.Spec) {
		c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			rt := c.Runtime.Super(t)
			rt.Events = &FlakyEventRepository{
				R:      c.EventRepository.Get(t),
				Random: t.Random,
			}
			return rt
		})

		s.Test("scheduled tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))

				var complete = workflow.Complete{ProcessID: processID}
				assert.Eventually(t, 5*time.Second, func(tb testing.TB) {
					done, err := complete.IsCompleted(tb.Context(), c.EventRepository.Get(t))
					assert.NoError(tb, err)
					assert.True(tb, done)
				})
			})
		})

		s.Test("executed tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Execute(t.Context(), processID))

				isCompleted, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), c.EventRepository.Get(t))
				assert.NoError(t, err)
				assert.True(t, isCompleted)
			})
		})
	})

	s.Context("flaky queue", func(s *testcase.Spec) {
		c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			rt := c.Runtime.Super(t)
			rt.ProcessExecutionQueue = &FlakyProcessExecutionQueue{
				Q:   c.ProcessExecutionQueue.Get(t),
				RND: t.Random,
			}
			return rt
		})

		s.Test("scheduled tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Schedule(t.Context(), processID))

				var complete = workflow.Complete{ProcessID: processID}
				assert.Eventually(t, 5*time.Second, func(tb testing.TB) {
					done, err := complete.IsCompleted(tb.Context(), c.EventRepository.Get(t))
					assert.NoError(tb, err)
					assert.True(tb, done)
				})
			})
		})

		s.Test("executed tasks are resilient", func(t *testcase.T) {
			var def = workflow.Sequence{
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
				workflow.ExecuteParticipant{ID: pid.Get(t)},
			}
			t.Random.Repeat(3, 7, func() {
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, def))
				assert.NoError(t, c.Runtime.Get(t).Execute(t.Context(), processID))

				isCompleted, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), c.EventRepository.Get(t))
				assert.NoError(t, err)
				assert.True(t, isCompleted)
			})
		})
	})
}

func flake(rnd *random.Random) bool {
	if rnd == nil {
		return false
	}
	return rnd.IntBetween(0, 100) <= 10
}

type FlakyEventRepository struct {
	R *memory.WorkflowEventRepository

	*random.Random
}

var _ workflow.EventRepository = (*FlakyEventRepository)(nil)

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
	return r.R.BeginTx(r.withFlake(ctx))
}

func (r *FlakyEventRepository) CommitTx(ctx context.Context) error {
	return r.R.CommitTx(r.withFlake(ctx))
}

func (r *FlakyEventRepository) RollbackTx(ctx context.Context) error {
	return r.R.RollbackTx(r.withFlake(ctx))
}

func (r *FlakyEventRepository) Create(ctx context.Context, ptr *workflow.Event) error {
	return r.R.Create(r.withFlake(ctx), ptr)
}

func (r *FlakyEventRepository) Update(ctx context.Context, ptr *workflow.Event) error {
	return r.R.Update(r.withFlake(ctx), ptr)
}

func (r *FlakyEventRepository) FindByID(ctx context.Context, id workflow.EventID) (workflow.Event, bool, error) {
	return r.R.FindByID(r.withFlake(ctx), id)
}

func (r *FlakyEventRepository) DeleteByID(ctx context.Context, id workflow.EventID) error {
	return r.R.DeleteByID(r.withFlake(ctx), id)
}

func (r *FlakyEventRepository) FindAll(ctx context.Context) iter.Seq2[workflow.Event, error] {
	return r.R.FindAll(r.withFlake(ctx))
}

func (r *FlakyEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	return r.R.FindByProcessID(r.withFlake(ctx), pid)
}

type FlakyProcessExecutionQueue struct {
	Q *memory.WorkflowProcessExecutionQueue

	RND *random.Random
}

var _ workflow.ProcessExecutionQueue = (*FlakyProcessExecutionQueue)(nil)

func (q *FlakyProcessExecutionQueue) Publish(ctx context.Context, d workflow.ProcessExecution) error {
	return q.Q.Publish(q.withFlake(ctx), d)
}

func (q *FlakyProcessExecutionQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ProcessExecution] {
	return func(yield func(pubsub.Message[workflow.ProcessExecution], error) bool) {
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
	Q *FlakyProcessExecutionQueue
	pubsub.Message[workflow.ProcessExecution]
}

func (msg *FlakyMessage) ACK() error {
	if flake(msg.Q.RND) {
		msg.Message.NACK()
		return msg.Q.RND.Error()
	}
	return msg.Message.ACK()
}

func (q *FlakyProcessExecutionQueue) withFlake(ctx context.Context) context.Context {
	if !flake(q.RND) {
		return ctx
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}
