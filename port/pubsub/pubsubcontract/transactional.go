package pubsubcontract

import (
	"testing"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
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

	s.Context("message context is transactional with ACK/NACK", func(s *testcase.Spec) {
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

			sub := subscribeTo(t, c.MakeContext(t), subscriber)

			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sub.Values(), []Data{val1, val2})
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

			sub := subscribeTo(t, c.MakeContext(t), subscriber)

			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sub.Values(), []Data{val1, val2},
					"it was expected that due to NACK, the publish which used the subscribe message context failed to publish due to rollback with NACK")

				assert.NotContains(t, sub.Values(), val3)
			})
		})
	})

	return s.AsSuite("TransactionalMessageContext")
}

func TransactionalPublisher[Data any](
	publisher pubsub.Publisher[Data],
	subscriber pubsub.Subscriber[Data],
	cm comproto.OnePhaseCommitProtocol,
	opts ...Option[Data]) contract.Contract {

	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[Data]](opts)

	s.Context("transactional Publish", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			pubsubtest.TryCleanup(t, subscriber, c.MakeContext(t))
		})

		s.Test("Publish + CommitTx", func(t *testcase.T) {
			var (
				val1 = c.MakeData(t)
				val2 = c.MakeData(t)
			)

			tx, err := cm.BeginTx(c.MakeContext(t))
			assert.NoError(t, err)

			assert.NoError(t, publisher.Publish(tx, val1, val2))

			sub := pubsubtest.Subscribe(t, subscriber, c.MakeContext(t))

			sub.AssertEmpty(t,
				"unexpected that we received any message in our subscription",
				"since the transaction is not yet published")

			assert.NoError(t, cm.CommitTx(tx))

			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sub.Values(), []Data{val1, val2})
			})
		})

		s.Test("Publish + RollbackTx", func(t *testcase.T) {
			var (
				val1 = c.MakeData(t)
				val2 = c.MakeData(t)
			)

			tx, err := cm.BeginTx(c.MakeContext(t))
			assert.NoError(t, err)

			assert.NoError(t, publisher.Publish(tx, val1, val2))

			sub := pubsubtest.Subscribe(t, subscriber, c.MakeContext(t))

			sub.AssertEmpty(t,
				"unexpected that we received any message in our subscription",
				"since the transaction is not yet published")

			assert.NoError(t, cm.RollbackTx(tx))

			sub.AssertEmpty(t,
				"unexpected that we received any message in our subscription",
				"since the transaction is rolled back already")
		})
	})

	return s.AsSuite("TransactionalPublisher")
}
