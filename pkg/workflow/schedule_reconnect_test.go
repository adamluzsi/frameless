package workflow_test

import (
	"context"
	"io"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
)

func TestRuntime_Run_notificationReconnect(t *testing.T) {
	s := testcase.NewSpec(t)
	s.HasSideEffect() // the future-entry case moves the clock
	e := bindEnv(s)

	s.Test("a running participant observes cancellation after reconnect", func(t *testcase.T) {
		rt := e.runtime.Get(t)
		entered := make(chan context.Context, 1)
		cancelled := make(chan error, 1)
		rt.Participants = workflow.Participants{"wait": func(ctx context.Context) error {
			entered <- ctx
			<-ctx.Done()
			cancelled <- ctx.Err()
			return ctx.Err()
		}}
		_, reconnect := startReconnectWorker(t, &rt)
		pid := mustProcessID(t)
		assert.NoError(t, rt.Spawn(t.Context(), pid, workflow.Execute{Participant: "wait"}))
		participant := awaitScheduleValue(t, entered)
		awaitScheduleListener(t, rt)

		reconnect()
		assert.NoError(t, participant.Err(), "losing the notification stream must not cancel the participant")
		assert.NoError(t, rt.Notifications.Publish(t.Context(), workflow.ProcessCancel{ProcessID: pid}))
		assert.ErrorIs(t, awaitScheduleValue(t, cancelled), context.Canceled)
	})

	s.Test("a future entry waits without redelivery and executes when due after reconnect", func(t *testcase.T) {
		timecop.Travel(t, clock.Now(), timecop.Freeze)
		rt := e.runtime.Get(t)
		var release synckit.Phaser
		defer release.Finish()
		rt.Participants = workflow.Participants{"warm-up": func(context.Context) error { release.Wait(); return nil }}
		q, reconnect := startReconnectWorker(t, &rt)

		// Prove this worker is already listening on the first connection, then leave it idle.
		warmUp := mustProcessID(t)
		assert.NoError(t, rt.Spawn(t.Context(), warmUp, workflow.Execute{Participant: "warm-up"}))
		initial := awaitScheduleValue(t, q.deliveries)
		awaitScheduleListener(t, rt)
		release.Finish()
		assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: initial, acked: true})
		reconnect()

		pid := mustProcessID(t)
		assert.NoError(t, rt.Bind(t.Context(), pid, workflow.Sequence{}))
		now := clock.Now()
		req := workflow.ExecutionRequest{ProcessID: pid, CreatedAt: now, StartTime: now.Add(time.Hour)}
		// Queue a harmless probe *before* the entry. Only the future guard can consume it,
		// after arming its timer. Nothing is published to wake the worker after delivery.
		probe := &scheduleListenerProbe{seen: make(chan struct{})}
		assert.NoError(t, rt.Notifications.Publish(t.Context(), probe))
		assert.NoError(t, q.Publish(t.Context(), req))
		assert.Equal(t, awaitScheduleValue(t, q.deliveries), req)
		t.Eventually(func(t *testcase.T) {
			assert.True(t, synckit.IsDone(probe.seen) || len(q.settlements) != 0,
				"worker neither entered its deadline wait nor settled the delivery")
		})
		assert.Empty(t, q.settlements, "the original delivery must remain unsettled until due")
		assert.Empty(t, q.deliveries, "there must be no redelivery while waiting")
		e.mustProcessCompletionIs(t, pid, false)

		timecop.Travel(t, req.StartTime, timecop.Freeze)
		assert.Equal(t, awaitScheduleValue(t, q.settlements), scheduleSettlement{request: req, acked: true})
		e.mustProcessCompletionIs(t, pid, true)
		assert.Empty(t, q.deliveries)
		assert.Equal(t, q.Requests(), []workflow.ExecutionRequest{initial, req}, "no replacement should be needed")
	})
}

func startReconnectWorker(t *testcase.T, rt *workflow.Runtime) (*scheduleObservedQueue, func()) {
	t.Helper()
	q := &scheduleObservedQueue{
		availabilityQueue: availabilityQueue{Queue: rt.Queue},
		deliveries:        make(chan workflow.ExecutionRequest, 8),
		settlements:       make(chan scheduleSettlement, 8),
	}
	upstream := reconnectNotifications{NotificationBroadcast: rt.Notifications, connected: make(chan context.CancelFunc, 1)}
	rt.Queue, rt.Notifications = q, upstream
	rt.NumQueueSubscriber = 1
	rt.RetryStrategy = scheduleRetryAttempts(2)
	rt.WaitTime = time.Hour
	t.Go(rt.Run)
	disconnect := awaitScheduleValue(t, upstream.connected)
	return q, func() {
		t.Helper()
		disconnect()
		awaitScheduleValue(t, upstream.connected)
	}
}

type reconnectNotifications struct {
	workflow.NotificationBroadcast
	connected chan context.CancelFunc
}

func (n reconnectNotifications) Subscribe(ctx context.Context) pubsub.Subscription[workflow.Notification] {
	return func(yield func(pubsub.Message[workflow.Notification], error) bool) {
		conn, disconnect := context.WithCancel(ctx)
		defer disconnect()
		// The memory broadcast binds a fresh queue before Subscribe returns, so a
		// publication immediately after this handshake cannot miss the connection.
		sub := n.NotificationBroadcast.Subscribe(conn)
		select {
		case n.connected <- disconnect:
		case <-ctx.Done():
			return
		}
		for msg, err := range sub {
			if conn.Err() != nil {
				break
			}
			if !yield(msg, err) {
				return
			}
		}
		if ctx.Err() == nil && conn.Err() != nil {
			yield(nil, io.ErrUnexpectedEOF)
		}
	}
}
