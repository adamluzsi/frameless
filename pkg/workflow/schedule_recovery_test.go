package workflow_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

func TestRuntime_Run_scheduleRecovery(t *testing.T) {
	s := testcase.NewSpec(t)
	s.HasSideEffect() // scheduled retries and future starts use a controlled clock
	e := bindEnv(s)
	pid := wftest.LetProcessID(s)
	queue := let.Var(s, func(t *testcase.T) *scheduleObservedQueue {
		return &scheduleObservedQueue{
			availabilityQueue: availabilityQueue{Queue: e.queue.Get(t)},
			deliveries:        make(chan workflow.ExecutionRequest, 8),
			settlements:       make(chan scheduleSettlement, 8),
		}
	})
	schedule := func(t *testcase.T, work func(context.Context) error, opts ...func(*workflow.ExecutionRequest)) workflow.Runtime {
		t.Helper()
		rt := e.runtime.Get(t)
		rt.Queue = queue.Get(t)
		rt.NumQueueSubscriber = 1
		rt.Participants = workflow.Participants{"work": work}
		assert.NoError(t, rt.Bind(t.Context(), pid.Get(t), workflow.Execute{Participant: "work"}))
		assert.NoError(t, rt.Schedule(t.Context(), pid.Get(t), opts...))
		return rt
	}
	assertRedelivery := func(t *testcase.T, rt workflow.Runtime) {
		t.Helper()
		q := queue.Get(t)
		original := awaitScheduleValue(t, q.deliveries)
		assert.Equal(t, original.ProcessID, pid.Get(t))
		// The first worker has exited. An implicit NACK on subscription close
		// is fine too; what must not have happened is an ACK of unfinished work.
		for len(q.settlements) > 0 {
			assert.Should(t).False((<-q.settlements).acked, "the first worker ACKed the unfinished request")
		}
		e.mustProcessCompletionIs(t, original.ProcessID, false)

		t.Go(rt.Run)
		assert.Equal(t, awaitScheduleValue(t, q.deliveries), original)
		assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: original, acked: true})
		e.mustProcessCompletionIs(t, original.ProcessID, true)
		assert.Equal(t, q.Requests(), []workflow.ExecutionRequest{original}, "recovery must not depend on a replacement publication")
	}

	for _, tc := range []struct {
		name       string
		publishErr error
	}{
		{name: "an expired Terminate leaves the interrupted request recoverable"},
		{name: "a failed interrupted retry publication NACKs the original and propagates the error", publishErr: errors.New("retry publication failed")},
	} {
		s.Test(tc.name, func(t *testcase.T) {
			timecop.Travel(t, clock.Now(), timecop.Freeze)
			var calls atomic.Int32
			var unwind synckit.Phaser
			defer unwind.Finish()
			rt := schedule(t, func(ctx context.Context) error {
				if calls.Add(1) == 1 {
					<-ctx.Done()
					unwind.Wait() // retain the process lock even after observing cancellation
					return ctx.Err()
				}
				return nil
			}, func(req *workflow.ExecutionRequest) { req.FailureCount = 3 })
			rt.WaitTime = time.Nanosecond // shorter than Terminate's lock-poll interval
			q := queue.Get(t)
			publisher := &gatedRetryQueue{Queue: q, attempts: make(chan workflow.ExecutionRequest, 1), err: tc.publishErr}
			defer publisher.release.Finish()
			first := rt
			first.Queue = publisher
			wait := startSingleDeliveryWorker(t, first, tc.publishErr)
			original := awaitScheduleValue(t, q.deliveries)
			t.Eventually(func(t *testcase.T) { assert.Equal(t, calls.Load(), int32(1)) })
			awaitScheduleListener(t, rt)

			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			assert.ErrorIs(t, rt.Terminate(ctx, pid.Get(t)), context.DeadlineExceeded)
			t.Eventually(func(t *testcase.T) { assert.Equal(t, unwind.Len(), 1) })
			assert.NoneOf(t, mustHistory(t, rt, pid.Get(t)), func(t testing.TB, ev workflow.Event) {
				assert.Equal(t, ev.EventType(), (workflow.EventTerminated{}).EventType())
			}, "a cancellation notification is not a durable termination")
			assert.Empty(t, q.settlements, "keep the original delivery until Execute unwinds")
			assert.Empty(t, publisher.attempts, "do not requeue while the participant holds the process lock")

			// Separate the retry's scheduling instant from the original CreatedAt.
			timecop.Travel(t, time.Second, timecop.Freeze)
			releasedAt := clock.Now()
			unwind.Finish()
			retry := awaitScheduleValue(t, publisher.attempts)
			assert.False(t, retry.StartTime.Before(releasedAt.Add(20*time.Millisecond)), "leave time for at least two termination lock polls after unwind")
			wantRetry := original
			wantRetry.StartTime = retry.StartTime
			assert.Equal(t, retry, wantRetry, "interruption preserves process ID, creation time and failure count")
			assert.Empty(t, q.settlements, "the original must remain unsettled while the replacement publication is pending")
			publisher.release.Finish()
			wait()

			published := tc.publishErr == nil
			assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: original, acked: published})
			assert.Empty(t, q.settlements, "settle the original only once")
			e.mustProcessCompletionIs(t, original.ProcessID, false)
			next := original
			wantPublications := []workflow.ExecutionRequest{original}
			if published {
				next = retry
				wantPublications = append(wantPublications, retry)
			}
			assert.Equal(t, q.Requests(), wantPublications)

			wait = startSingleDeliveryWorker(t, rt, nil)
			assert.Equal(t, awaitScheduleValue(t, q.deliveries), next)
			if published {
				awaitScheduleListener(t, rt)
				assert.Equal(t, calls.Load(), int32(1), "the replacement must not reclaim the lock before its start time")
				timecop.Travel(t, retry.StartTime, timecop.Freeze)
			}
			wait()
			assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: next, acked: true})
			e.mustProcessCompletionIs(t, original.ProcessID, true)
			assert.Equal(t, calls.Load(), int32(2))
			assert.Equal(t, q.Requests(), wantPublications, "the fresh worker completes without another Schedule")
		})
	}

	s.Test("a scheduled run survives contention with a synchronous execution that suspends", func(t *testcase.T) {
		timecop.Travel(t, clock.Now(), timecop.Freeze)
		var calls atomic.Int32
		var hold synckit.Phaser
		rt := schedule(t, func(context.Context) error {
			if calls.Add(1) == 1 {
				hold.Wait()
				return workflow.Suspend{}
			}
			return nil
		}, func(req *workflow.ExecutionRequest) { req.FailureCount = 2 })
		rt.WaitTime = time.Hour
		id := pid.Get(t)
		execution := synckit.Go(t.Context(), func(ctx context.Context) error { return rt.Execute(ctx, id) })
		defer func() { hold.Finish(); execution.Wait() }()
		t.Eventually(func(t *testcase.T) { assert.Equal(t, hold.Len(), 1) })

		t.Go(rt.Run)
		q := queue.Get(t)
		initial := awaitScheduleValue(t, q.deliveries)
		assert.True(t, awaitScheduleValue(t, q.settlements).acked)
		retry := initial
		retry.StartTime = clock.Now().Add(rt.WaitTime)
		assert.Equal(t, q.Requests(), []workflow.ExecutionRequest{initial, retry}, "ACK only after a delayed retry is queued, without counting contention as failure")
		assert.Equal(t, awaitScheduleValue(t, q.deliveries), retry)
		awaitScheduleListener(t, rt)

		hold.Finish()
		assert.ErrorIs(t, execution.Wait(), workflow.Suspend{})
		assert.Equal(t, calls.Load(), int32(1))
		timecop.Travel(t, retry.StartTime, timecop.Freeze)
		assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: retry, acked: true})
		e.mustProcessCompletionIs(t, id, true)
		assert.Equal(t, calls.Load(), int32(2))
	})

	s.Test("a participant panic propagates and its request is redelivered", func(t *testcase.T) {
		sentinel := errors.New("participant panic")
		var calls atomic.Int32
		rt := schedule(t, func(context.Context) error {
			if calls.Add(1) == 1 {
				panic(sentinel)
			}
			return nil
		})

		wait := startSingleDeliveryWorker(t, rt, nil)
		got := assert.Panic(t, wait)
		assert.True(t, got == sentinel, assert.MessageF("expected the original panic, recovered %#v", got))
		assertRedelivery(t, rt)
		assert.Equal(t, calls.Load(), int32(2))
	})

	s.Test("a cancellation before StartTime leaves the future request recoverable", func(t *testcase.T) {
		timecop.Travel(t, clock.Now(), timecop.Freeze)
		start := clock.Now().Add(time.Hour)
		var calls atomic.Int32
		rt := schedule(t, func(context.Context) error {
			calls.Add(1)
			return nil
		}, func(req *workflow.ExecutionRequest) { req.StartTime = start })
		wait := startSingleDeliveryWorker(t, rt, nil)
		awaitScheduleListener(t, rt)

		assert.NoError(t, rt.Notifications.Publish(t.Context(), workflow.ProcessCancel{ProcessID: pid.Get(t)}))
		wait()
		assert.Equal(t, calls.Load(), int32(0), "a cancellation must not start the participant early")
		timecop.Travel(t, start, timecop.Freeze)
		assertRedelivery(t, rt)
		assert.Equal(t, calls.Load(), int32(1))
	})
}

// Give the first worker exactly one real delivery. Stop Run's other listener
// only after the handler unwinds, so shutdown cannot cause the first settlement.
func startSingleDeliveryWorker(t *testcase.T, rt workflow.Runtime, wantErr error) (wait func()) {
	t.Helper()
	ctx, stop := context.WithCancel(t.Context())
	rt.Queue = singleDeliveryQueue{Queue: rt.Queue, stop: stop}
	var worker synckit.Group
	worker.Go(ctx, rt.Run)
	t.Cleanup(func() { stop(); awaitScheduleValue(t, worker.Done()) })
	return func() {
		t.Helper()
		awaitScheduleValue(t, worker.Done())
		err := worker.Wait() // also propagates a panic from Run
		if wantErr != nil {
			assert.ErrorIs(t, err, wantErr)
		} else if err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}
	}
}

// Installed after Schedule on the first worker only, so the gate controls the
// interrupted retry, not the original publication or the fresh worker's delivery.
type gatedRetryQueue struct {
	workflow.Queue
	attempts chan workflow.ExecutionRequest
	release  synckit.Phaser
	err      error
}

func (q *gatedRetryQueue) Publish(ctx context.Context, req workflow.ExecutionRequest) error {
	select {
	case q.attempts <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	q.release.Wait()
	if q.err != nil {
		return q.err
	}
	return q.Queue.Publish(ctx, req)
}

type singleDeliveryQueue struct {
	workflow.Queue
	stop context.CancelFunc
}

func (q singleDeliveryQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ExecutionRequest] {
	return func(yield func(pubsub.Message[workflow.ExecutionRequest], error) bool) {
		defer q.stop()
		for msg, err := range q.Queue.Subscribe(ctx) {
			yield(msg, err)
			return
		}
	}
}
