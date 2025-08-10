package pubsubcontract

import (
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// Buffered defines a publisher behaviour where if the subscription is canceled,
// the publisher messages can be still consumed after resubscribing.
func Buffered[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
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

	b.Spec(s)

	s.Context("buffered", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.And("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			b.WhenWePublish(s, val1, val2)

			s.And("after resubscribing to the publisher", func(s *testcase.Spec) {
				sub := b.GivenWeHaveSubscription(s)

				s.Then("messages are received", func(t *testcase.T) {
					expected := []Data{val1.Get(t), val2.Get(t)}
					t.Eventually(func(it *testcase.T) {
						assert.ContainsExactly(it, expected, sub.Get(t).Values())
					})
				})
			})
		})
	})

	return s.AsSuite("Buffered")
}

// Volatile defines a publisher behaviour where if the subscription is canceled, published messages won't be delivered.
// In certain scenarios, you may want to send a volatile message with no assurances over a publisher,
// when timely delivery is more important than losing messages.
func Volatile[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
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
	b.Spec(s)

	s.Context("volatile", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			b.WhenWePublish(s, val1, val2)

			s.And("after resubscribing to the publisher", func(s *testcase.Spec) {
				sub := b.GivenWeHaveSubscription(s)

				s.Then("messages published previously won't arrive", func(t *testcase.T) {
					pubsubtest.Waiter.Wait()

					t.Eventually(func(it *testcase.T) {
						it.Must.Empty(sub.Get(t).Values())
					})
				})
			})
		})
	})

	return s.AsSuite("Volatile")
}
