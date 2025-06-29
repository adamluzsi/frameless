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

// Queue defines a publisher behaviour where each message is only delivered to a single subscriber,
// and not to all registered subscribers.
// If a message is ack-ed, the message will be permanently removed from the Queue.
func Queue[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
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

	s.Context("queue", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.When("a subscription is made", func(s *testcase.Spec) {
			sub := b.GivenWeHaveSubscription(s)

			s.And("messages are published", func(s *testcase.Spec) {
				val1 := testcase.Let(s, func(t *testcase.T) Data {
					return c.MakeData(t)
				})
				val2 := testcase.Let(s, func(t *testcase.T) Data {
					return c.MakeData(t)
				})
				b.WhenWePublish(s, val1, val2)

				s.Then("subscription receives the messages", func(t *testcase.T) {
					expected := []Data{val1.Get(t), val2.Get(t)}
					t.Eventually(func(it *testcase.T) {
						it.Must.ContainExactly(expected, sub.Get(t).Values())
					})
				})
			})
		})

		s.When("multiple subscriptions are made", func(s *testcase.Spec) {
			sub1 := b.GivenWeHaveSubscription(s)
			sub2 := b.GivenWeHaveSubscription(s)

			s.And("messages are published", func(s *testcase.Spec) {
				var values []testcase.Var[Data]
				for i := 0; i < 8; i++ {
					values = append(values, testcase.Let(s, func(t *testcase.T) Data {
						return c.MakeData(t)
					}))
				}

				b.WhenWePublish(s, values...)

				s.Then("messages are unicast between the subscribers", func(t *testcase.T) {
					// TODO: continue

					var expected []Data
					for _, v := range values {
						expected = append(expected, v.Get(t))
					}

					t.Eventually(func(t *testcase.T) {
						pubsubtest.Waiter.Wait()
						sub1vs := sub1.Get(t).Values()
						assert.NotNil(t, sub1vs)
						sub2vs := sub2.Get(t).Values()
						assert.NotNil(t, sub2vs)
						var actual []Data
						actual = append(actual, sub1vs...)
						actual = append(actual, sub2vs...)
						assert.ContainExactly(t, expected, actual)
					})
				})
			})
		})
	})

	return s.AsSuite("Queue")
}
