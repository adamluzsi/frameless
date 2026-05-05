package pubsubcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// TransactionalMessageContext defines a contract for testing transactional publishing behavior using a subscription message's context.
// It verifies that ACK/NACK combined with Publish within a transaction behaves correctly:
//   - ack + publish → the original message is removed, new message is queued safely
//   - nack + publish → the original message remains available (released), new message is also queued
func TransactionalMessageContext[Data any](
	publisher pubsub.Publisher[Data],
	subscriber pubsub.Subscriber[Data],
	commitManager interface {
		BeginTx(context.Context) (context.Context, error)
		CommitTx(context.Context) error
		RollbackTx(context.Context) error
	},
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

	s.Context("requeue transactional behavior", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.Test("ACK + Publish", func(t *testcase.T) {
			val1 := c.MakeData(t)
			val2 := c.MakeData(t)

			assert.NoError(t, publisher.Publish(c.MakeContext(t), val1, val2))

			for msg, err := range subscriber.Subscribe(c.MakeContext(t)) {
				assert.NoError(t, err)
				assert.NoError(t, publisher.Publish(msg.Context(), msg.Data()))
				assert.NoError(t, msg.ACK())
				break
			}

			sih := b.newSubscriptionIteratorHelper(t)
			sih.Start(t, c.MakeContext(t))
			defer sih.Stop()

			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sih.Values(), []Data{val1, val2})
			})
		})

		s.Test("NACK + Publish", func(t *testcase.T) {
			val1 := c.MakeData(t)
			val2 := c.MakeData(t)
			val3 := c.MakeData(t)

			assert.NoError(t, publisher.Publish(c.MakeContext(t), val1, val2))

			for msg, err := range subscriber.Subscribe(c.MakeContext(t)) {
				assert.NoError(t, err)
				assert.NoError(t, publisher.Publish(msg.Context(), val3)) // never gets published inf msg.Context
				assert.NoError(t, msg.NACK())
				break
			}

			sih := b.newSubscriptionIteratorHelper(t)
			sih.Start(t, c.MakeContext(t))
			defer sih.Stop()

			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sih.Values(), []Data{val1, val2},
					"it was expected that due to NACK, the publish which used the subscribe message context failed to publish due to rollback with NACK")

				assert.NotContains(t, sih.Values(), val3)
			})
		})
	})

	return s.AsSuite("Requeue")
}
