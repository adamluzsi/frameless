package pubsubcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// NonTransactionalMessageContext expresses that the subject's subscription message context is NOT coupled to ACK/NACK.
// Publishes performed under msg.Context() are visible to every other subscriber immediately,
// and are not rolled back if the message is later NACK-ed.
func NonTransactionalMessageContext[Data any](
	publisher pubsub.Publisher[Data],
	subscriber pubsub.Subscriber[Data],
	opts ...Option[Data]) contract.Contract {

	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[Data]](opts)

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		return baseSubject[Data]{
			Publisher:   publisher,
			Subscriber:  subscriber,
			MakeContext: c.MakeContext,
			MakeData:    c.MakeData,
		}
	})

	s.Context("message context is NOT transactional with ACK/NACK", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.Test("Publish under msg.Context() is observable to other subscribers even before the ACK", func(t *testcase.T) {
			t.Log("given an initial message is send through the publisher")
			var initiallyPublishedData = c.MakeData(t)
			t.Go(func(ctx context.Context) error {
				ctx, cancel := contextkit.Merge(c.MakeContext(t), ctx)
				defer cancel()
				return publisher.Publish(ctx, initiallyPublishedData)
			})

			t.Log("and a subscription receives a message")
			for msg, err := range subscriber.Subscribe(c.MakeContext(t)) {
				assert.NoError(t, err)
				assert.Equal(t, initiallyPublishedData, msg.Data())

				t.Log("then it can publish a new message under msg.Context()")
				var fromMessageContextData = c.MakeData(t)
				assert.NoError(t, publisher.Publish(msg.Context(), fromMessageContextData))

				assert.Within(t, timeout, func(ctx context.Context) {
					ctx, cancel := contextkit.Merge(c.MakeContext(t), ctx)
					defer cancel()
					for msg, err := range subscriber.Subscribe(ctx) {
						assert.NoError(t, err)
						assert.Equal(t, msg.Data(), fromMessageContextData)
						break
					}
				})
				break
			}
		})

		s.Test("Publish under msg.Context() survives NACK", func(t *testcase.T) {
			val1 := c.MakeData(t)
			val2 := c.MakeData(t)

			assert.NoError(t, publisher.Publish(c.MakeContext(t), val1))

			t.Log("given a subscription receives a message")
			for msg, err := range subscriber.Subscribe(c.MakeContext(t)) {
				assert.NoError(t, err)
				t.Log("and the handler publishes a new message under msg.Context()")
				assert.NoError(t, publisher.Publish(msg.Context(), val2))
				t.Log("and then NACK the delivery")
				assert.NoError(t, msg.NACK())
				break
			}

			t.Log("then the new message is still observable to other subscribers")
			sub := subscribeTo(t, c.MakeContext(t), subscriber)
			t.Eventually(func(t *testcase.T) {
				assert.Contains(t, sub.Values(), val2)
			})
		})
	})

	return s.AsSuite("NonTransactionalMessageContext")
}
