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
			var (
				ctx  = c.MakeContext(t)
				val1 = c.MakeData(t)
				val2 = c.MakeData(t)
			)

			assert.NoError(t, publisher.Publish(ctx, val1))
			assert.NoError(t, publisher.Publish(ctx, val2))

			t.Log("given when we receive a message as part of a subscription")
			t.Log("and we republish that message")
			t.Log("then it gets republished as expected after an Message#ACK")
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

			assert.NoError(t, publisher.Publish(c.MakeContext(t), val1))
			assert.NoError(t, publisher.Publish(c.MakeContext(t), val2))

			t.Log("given when we receive a message as part of a subscription")
			t.Log("and we republish that message")
			t.Log("but then some error occurs that we handle by NACK-ing the message")
			t.Log("we expect that the message should be repeatable")
			t.Log("by having the in-flight publishes rolled back")
			for msg, err := range subscriber.Subscribe(c.MakeContext(t)) {
				assert.NoError(t, err)
				assert.NoError(t, publisher.Publish(msg.Context(), val3)) // never gets published inf msg.Context
				assert.NoError(t, msg.NACK())
				break
			}

			sub := subscribeTo(t, c.MakeContext(t), subscriber)

			var failMessage = []assert.Message{
				"it was expected that due to NACK,",
				"the publish which used the subscribe message context failed to publish",
				"due to rollback with NACK",
				"hence we don't have value-3 in the pubsub",
			}
			t.Eventually(func(t *testcase.T) {
				assert.ContainsExactly(t, sub.Values(), []Data{val1, val2}, failMessage...)
				assert.NotContains(t, sub.Values(), val3, failMessage...)
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

			assert.NoError(t, publisher.Publish(tx, val1))
			assert.NoError(t, publisher.Publish(tx, val2))

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

			assert.NoError(t, publisher.Publish(tx, val1))
			assert.NoError(t, publisher.Publish(tx, val2))

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
