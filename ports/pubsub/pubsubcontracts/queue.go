package pubsubcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/ports/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// Queue defines a publisher behaviour where each message is only delivered to a single subscriber,
// and not to all registered subscribers.
// If a message is ack-ed, the message will be permanently removed from the Queue.
type Queue[Data any] func(testing.TB) QueueSubject[Data]

type QueueSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c Queue[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) QueueSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := subject.Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})

	b.Spec(s)

	s.Context("queue", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.When("a subscription is made", func(s *testcase.Spec) {
			sub := b.GivenWeHaveSubscription(s)

			s.And("messages are published", func(s *testcase.Spec) {
				val1 := testcase.Let(s, func(t *testcase.T) Data {
					return subject.Get(t).MakeData()
				})
				val2 := testcase.Let(s, func(t *testcase.T) Data {
					return subject.Get(t).MakeData()
				})
				b.WhenWePublish(s, val1, val2)

				s.Then("subscription receives the messages", func(t *testcase.T) {
					expected := []Data{val1.Get(t), val2.Get(t)}
					t.Eventually(func(it assert.It) {
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
				for i := 0; i < 42; i++ {
					values = append(values, testcase.Let(s, func(t *testcase.T) Data {
						return subject.Get(t).MakeData()
					}))
				}

				b.WhenWePublish(s, values...)

				s.Then("message is unicast between the subscribers", func(t *testcase.T) {
					// TODO: continue

					var expected []Data
					for _, v := range values {
						expected = append(expected, v.Get(t))
					}
					pubsubtest.Waiter.Wait()

					t.Eventually(func(it assert.It) {
						it.Must.NotEmpty(sub1.Get(t).Values())
						it.Must.NotEmpty(sub2.Get(t).Values())

						var actual []Data
						actual = append(actual, sub1.Get(t).Values()...)
						actual = append(actual, sub2.Get(t).Values()...)
						it.Must.ContainExactly(expected, actual)
					})
				})
			})
		})
	})
}

func (c Queue[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Queue[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
