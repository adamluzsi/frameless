package workflow_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRuntime_participantAvailability(t *testing.T) {
	s := testcase.NewSpec(t)

	e := bindEnv(s)
	var (
		process     = wftest.LetProcessID(s)
		firstID     = wftest.LetParticipantID(s)
		secondID    = wftest.LetParticipantID(s)
		value       = let.String(s)
		firstCalls  = let.Var(s, func(t *testcase.T) *atomic.Int64 { return new(atomic.Int64) })
		secondCalls = let.Var(s, func(t *testcase.T) *atomic.Int64 { return new(atomic.Int64) })
		attempts    = let.Var(s, func(t *testcase.T) *atomic.Int64 { return new(atomic.Int64) })
		queue       = let.Var(s, func(t *testcase.T) *availabilityQueue {
			return &availabilityQueue{Queue: e.queue.Get(t)}
		})
	)
	subject := e.runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := e.runtime.Super(t)
		calls, executions := firstCalls.Get(t), attempts.Get(t)
		rt.Participants = workflow.Participants{
			firstID.Get(t): func(ctx context.Context, in string) (string, error) {
				calls.Add(1)
				return in + "a", nil
			},
		}
		rt.ContextSetup = workflow.ContextSetup{func(ctx context.Context) context.Context {
			executions.Add(1)
			return ctx
		}}
		rt.Queue = queue.Get(t)
		rt.WaitTime = time.Millisecond
		rt.NumQueueSubscriber = 1
		rt.RetryStrategy = resilience.Jitter{Delay: time.Nanosecond, Attempts: 3}
		return rt
	})
	peer := let.Var(s, func(t *testcase.T) workflow.Runtime {
		rt := subject.Get(t)
		calls := secondCalls.Get(t)
		rt.ContextSetup = nil
		rt.Participants = workflow.Participants{
			secondID.Get(t): func(ctx context.Context, in string) (string, error) {
				calls.Add(1)
				return in + "b", nil
			},
		}
		return rt
	})
	definition := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "value", Value: value.Get(t)},
			workflow.Execute{ParticipantID: firstID.Get(t), Input: []workflow.VarName{"value"}, Output: []workflow.VarName{"value"}},
			workflow.Execute{ParticipantID: secondID.Get(t), Input: []workflow.VarName{"value"}, Output: []workflow.VarName{"value"}},
			workflow.Execute{ParticipantID: firstID.Get(t), Input: []workflow.VarName{"value"}, Output: []workflow.VarName{"value"}},
		}
	})
	s.Before(func(t *testcase.T) {
		assert.Must(t).NoError(subject.Get(t).Bind(t.Context(), process.Get(t), definition.Get(t)))
	})

	isCompleted := func(t *testcase.T) bool {
		completed, err := workflow.IsCompleted(t.Context(), e.events.Get(t), process.Get(t))
		assert.NoError(t, err)
		return completed
	}
	assertNoFailures := func(t *testcase.T) {
		for _, event := range mustHistory(t, subject.Get(t), process.Get(t)) {
			_, failed := event.(workflow.EventError)
			assert.False(t, failed, "participant availability is not an execution failure")
			_, terminated := event.(workflow.EventTerminated)
			assert.False(t, terminated)
		}
	}
	assertCompleted := func(t *testcase.T) {
		assert.True(t, isCompleted(t))
		assert.Equal(t, firstCalls.Get(t).Load(), int64(2))
		assert.Equal(t, secondCalls.Get(t).Load(), int64(1))
		assert.Equal[any](t, getVar(t, subject.Get(t), process.Get(t), "value"), value.Get(t)+"aba")
		assertNoFailures(t)
	}

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(t.Context(), process.Get(t))
		}

		s.Test("returns the plain missing-participant error without retrying or recording a failure", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, workflow.ErrParticipantNotFound{ID: secondID.Get(t)})
			assert.False(t, errors.Is(err, workflow.Suspend{}))
			_, signal := err.(workflow.RuntimeSignal)
			assert.False(t, signal)
			assert.False(t, workflow.ErrIsFatal(err))
			assert.Equal(t, attempts.Get(t).Load(), int64(1))
			assert.Equal(t, firstCalls.Get(t).Load(), int64(1))
			assert.False(t, isCompleted(t))
			assertNoFailures(t)
			for _, event := range mustHistory(t, subject.Get(t), process.Get(t)) {
				if event, ok := event.(workflow.EventParticipant); ok {
					assert.Equal(t, event.ParticipantID, firstID.Get(t))
				}
			}
		})

		s.Test("resumes on another specialised node and returns to the first without repeating completed steps", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{ID: secondID.Get(t)})
			assert.ErrorIs(t, peer.Get(t).Execute(t.Context(), process.Get(t)), workflow.ErrParticipantNotFound{ID: firstID.Get(t)})
			assert.False(t, isCompleted(t))
			assert.NoError(t, act(t))
			assertCompleted(t)
			assert.NoError(t, peer.Get(t).Execute(t.Context(), process.Get(t)))
			assertCompleted(t)
		})

		s.Test("an older node with neither participant preserves progress while handing the process back", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{ID: secondID.Get(t)})
			before := mustHistory(t, subject.Get(t), process.Get(t))
			older := subject.Get(t)
			older.Participants = nil
			t.Random.Repeat(2, 5, func() {
				err := older.Execute(t.Context(), process.Get(t))
				assert.ErrorIs(t, err, workflow.ErrParticipantNotFound{ID: secondID.Get(t)})
			})
			assert.Equal(t, mustHistory(t, subject.Get(t), process.Get(t)), before)
			assert.ErrorIs(t, peer.Get(t).Execute(t.Context(), process.Get(t)), workflow.ErrParticipantNotFound{ID: firstID.Get(t)})
			assert.NoError(t, act(t))
			assertCompleted(t)
		})

		s.When("the node has no participant repository", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := subject.Super(t)
				rt.Participants = nil
				return rt
			})

			s.Then("returns the first missing participant without failing fatally", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, workflow.ErrParticipantNotFound{ID: firstID.Get(t)})
				assert.False(t, workflow.ErrIsFatal(err))
				assert.False(t, isCompleted(t))
				assertNoFailures(t)
			})
		})
	})

	s.Describe("#Schedule", func(s *testcase.Spec) {
		failureCount := let.Var(s, func(t *testcase.T) int { return t.Random.IntBetween(1, 10) })
		act := func(t *testcase.T) error {
			return subject.Get(t).Schedule(t.Context(), process.Get(t), func(req *workflow.ExecutionRequest) {
				req.FailureCount = failureCount.Get(t)
			})
		}

		s.Test("requeues until a capable peer joins, then shuffles between both nodes to completion", func(t *testcase.T) {
			first, second := subject.Get(t), peer.Get(t)
			t.Go(first.Run)
			assert.NoError(t, act(t))
			t.Eventually(func(t *testcase.T) {
				assert.True(t, len(queue.Get(t).Requests()) >= 3, "keep rescheduling while no capable peer is available")
			})
			assert.False(t, isCompleted(t))
			assert.Equal(t, firstCalls.Get(t).Load(), int64(1))
			assertNoFailures(t)

			t.Go(second.Run)
			t.Eventually(func(t *testcase.T) { assert.True(t, isCompleted(t)) })
			assertCompleted(t)
			requests := queue.Get(t).Requests()
			for i, req := range requests {
				assert.Equal(t, req.ProcessID, process.Get(t))
				assert.Equal(t, req.FailureCount, failureCount.Get(t)+i)
				assert.Equal(t, req.CreatedAt, requests[0].CreatedAt)
			}
			assert.True(t, requests[1].StartTime.After(requests[0].StartTime))
		})
	})
}

// availabilityQueue observes real queue publications without changing delivery.
type availabilityQueue struct {
	workflow.Queue
	mu       sync.Mutex
	requests []workflow.ExecutionRequest
}

func (q *availabilityQueue) Publish(ctx context.Context, req workflow.ExecutionRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.Queue.Publish(ctx, req); err != nil {
		return err
	}
	q.requests = append(q.requests, req)
	return nil
}

func (q *availabilityQueue) Requests() []workflow.ExecutionRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]workflow.ExecutionRequest(nil), q.requests...)
}
