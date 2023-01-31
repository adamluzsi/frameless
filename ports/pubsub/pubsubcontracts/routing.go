package pubsubcontracts

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
)

// Queue defines a publisher behaviour where each message is only delivered to a single subscriber,
// and not to all registered subscribers.
// If a message is ack-ed, the message will be permanently removed from the Queue.
type Queue[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V
}

func (c Queue[V]) Spec(s *testcase.Spec) {
	b := pubsubBase[V]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeV,
	}

	b.Spec(s)

	s.Context(fmt.Sprintf("%s is a queue", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.WhenIsEmpty(s)

		s.When("a subscription is made", func(s *testcase.Spec) {
			sub := b.GivenWeHaveSubscription(s)

			s.And("messages are published", func(s *testcase.Spec) {
				val1 := let.With[V](s, c.MakeV)
				val2 := let.With[V](s, c.MakeV)
				b.WhenWePublish(s, val1, val2)

				s.Then("subscription receives the messages", func(t *testcase.T) {
					expected := []V{val1.Get(t), val2.Get(t)}
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
				var values []testcase.Var[V]
				for i := 0; i < 42; i++ {
					values = append(values, let.With[V](s, c.MakeV))
				}

				b.WhenWePublish(s, values...)

				s.Then("message is unicast between the subscribers", func(t *testcase.T) {
					// TODO: continue

					var expected []V
					for _, v := range values {
						expected = append(expected, v.Get(t))
					}
					pubsubtest.Waiter.Wait()

					t.Eventually(func(it assert.It) {
						it.Must.NotEmpty(sub1.Get(t).Values())
						it.Must.NotEmpty(sub2.Get(t).Values())

						var actual []V
						actual = append(actual, sub1.Get(t).Values()...)
						actual = append(actual, sub2.Get(t).Values()...)
						it.Must.ContainExactly(expected, actual)
					})
				})
			})
		})
	})
}

func (c Queue[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Queue[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// Broadcast defines a publisher behaviour where each message is published to all registered subscription members.
type Broadcast[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V
}

func (c Broadcast[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Broadcast[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

func (c Broadcast[V]) Spec(s *testcase.Spec) {
	b := pubsubBase[V]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeV,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s is fan-out", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.WhenIsEmpty(s)

		s.When("a subscription is made", func(s *testcase.Spec) {
			sub := b.GivenWeHaveSubscription(s)

			s.And("messages are published", func(s *testcase.Spec) {
				val1 := let.With[V](s, c.MakeV)
				val2 := let.With[V](s, c.MakeV)
				b.WhenWePublish(s, val1, val2)

				s.Then("subscription receives the messages", func(t *testcase.T) {
					expected := []V{val1.Get(t), val2.Get(t)}

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
				var values []testcase.Var[V]
				for i := 0; i < 42; i++ {
					values = append(values, let.With[V](s, c.MakeV))
				}

				b.WhenWePublish(s, values...)

				s.Then("message is multicast to all subscribers", func(t *testcase.T) {
					var expected []V
					for _, v := range values {
						expected = append(expected, v.Get(t))
					}

					pubsubtest.Waiter.Wait()

					t.Eventually(func(it assert.It) {
						it.Must.ContainExactly(expected, sub1.Get(t).Values())
						it.Must.ContainExactly(expected, sub2.Get(t).Values())
					})
				})
			})
		})
	})
}
