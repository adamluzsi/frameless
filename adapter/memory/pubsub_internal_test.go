package memory

import (
	"iter"
	"testing"

	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestQueue_snapshot(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *Queue[int] {
		return &Queue[int]{}
	})
	message := let.Var[pubsub.Message[int]](s, nil)
	stopSubscription := let.Var[func()](s, nil)
	candidate := let.Var[*queueMessage[int]](s, nil)

	s.Before(func(t *testcase.T) {
		q := subject.Get(t)
		assert.Must(t).NoError(q.Publish(t.Context(), t.Random.Int()))
		next, stop := iter.Pull2(q.Subscribe(t.Context()))
		t.Cleanup(stop)
		stopSubscription.Set(t, stop)
		msg, err, ok := next()
		assert.Must(t).NoError(err)
		assert.Must(t).True(ok)
		message.Set(t, msg)

		// Pause a competing consumer after it snapshots the queue, before claiming.
		for msg := range q.rIter() {
			candidate.Set(t, msg)
			break
		}
		assert.Must(t).NoError(message.Get(t).ACK())
	})

	act := func(t *testcase.T) bool {
		return candidate.Get(t).take(2)
	}

	s.Then("an acknowledged message cannot be claimed from an older snapshot", func(t *testcase.T) {
		assert.False(t, act(t))
	})

	s.When("the acknowledging subscription closes", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			stopSubscription.Get(t)()
		})

		s.Then("implicit NACK does not make the acknowledged message available again", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})
}
