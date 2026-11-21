package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRuntime_scheduleContext(t *testing.T) {
	s := testcase.NewSpec(t)
	e := bindEnv(s)
	pid := wftest.LetProcessID(s)
	queue := let.Var(s, func(t *testcase.T) *schedulePublishRecorder[workflow.ExecutionRequest] {
		return &schedulePublishRecorder[workflow.ExecutionRequest]{scheduleTopic: e.queue.Get(t)}
	})
	notifications := let.Var(s, func(t *testcase.T) *schedulePublishRecorder[workflow.Notification] {
		return &schedulePublishRecorder[workflow.Notification]{scheduleTopic: e.notifications.Get(t)}
	})
	subject := e.runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := e.runtime.Super(t)
		rt.RetryStrategy = nil // Use the runtime default, not the fixture's single attempt.
		rt.Queue = queue.Get(t)
		rt.Notifications = notifications.Get(t)
		return rt
	})

	s.Test("Schedule succeeds if the final publication cancels the context before returning", func(t *testcase.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		notifications.Get(t).afterPublish = cancel
		id := pid.Get(t)

		err := subject.Get(t).Schedule(ctx, id)

		assert.NoError(t, err)
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
		requests := queue.Get(t).calls
		assert.Equal(t, len(requests), 1)
		assert.Equal(t, requests[0].ProcessID, id)
		assert.Equal(t, notifications.Get(t).calls, []workflow.Notification{workflow.ProcessSchedule{ProcessID: id}})
	})

	s.When("the context is already canceled", func(s *testcase.Spec) {
		ctx := let.Var(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		})

		s.Then("Schedule returns cancellation without publishing a request or notification", func(t *testcase.T) {
			err := subject.Get(t).Schedule(ctx.Get(t), pid.Get(t))

			assert.Empty(t, queue.Get(t).calls)
			assert.Empty(t, notifications.Get(t).calls)
			assert.ErrorIs(t, err, context.Canceled)
		})

		s.Then("Bind returns cancellation without recording a definition", func(t *testcase.T) {
			rt, id := subject.Get(t), pid.Get(t)
			err := rt.Bind(ctx.Get(t), id, workflow.Sequence{})

			assert.Empty(t, mustHistory(t, rt, id))
			assert.ErrorIs(t, err, context.Canceled)
		})

		s.Then("Execute returns cancellation without calling the participant or changing history", func(t *testcase.T) {
			rt, id := subject.Get(t), pid.Get(t)
			calls := 0
			rt.Participants = workflow.Participants{
				"work": func(context.Context) error {
					calls++
					return nil
				},
			}
			assert.NoError(t, rt.Bind(t.Context(), id, workflow.Execute{Participant: "work"}))
			before := mustHistory(t, rt, id)

			err := rt.Execute(ctx.Get(t), id)

			assert.Equal(t, calls, 0)
			assert.Equal(t, mustHistory(t, rt, id), before)
			assert.ErrorIs(t, err, context.Canceled)
		})

		s.Then("Terminate returns cancellation without recording termination or notifying workers", func(t *testcase.T) {
			rt, id := subject.Get(t), pid.Get(t)
			assert.NoError(t, rt.Bind(t.Context(), id, workflow.Sequence{}))
			before := mustHistory(t, rt, id)

			err := rt.Terminate(ctx.Get(t), id)

			assert.Equal(t, mustHistory(t, rt, id), before)
			assert.Empty(t, notifications.Get(t).calls)
			assert.ErrorIs(t, err, context.Canceled)
		})
	})
}

type scheduleTopic[T any] interface {
	pubsub.Publisher[T]
	pubsub.Subscriber[T]
}

type schedulePublishRecorder[T any] struct {
	scheduleTopic[T]
	calls        []T
	afterPublish func()
}

func (r *schedulePublishRecorder[T]) Publish(ctx context.Context, msg T) error {
	r.calls = append(r.calls, msg)
	if err := r.scheduleTopic.Publish(ctx, msg); err != nil {
		return err
	}
	if r.afterPublish != nil {
		r.afterPublish()
	}
	return nil
}
