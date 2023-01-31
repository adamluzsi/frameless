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

// Buffered defines a publisher behaviour where if the subscription is canceled,
// the publisher messages can be still consumed after resubscribing.
type Buffered[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V
}

func (c Buffered[V]) Spec(s *testcase.Spec) {
	b := pubsubBase[V]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeV,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s is buffered", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.WhenIsEmpty(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.And("messages are published", func(s *testcase.Spec) {
			val1 := let.With[V](s, c.MakeV)
			val2 := let.With[V](s, c.MakeV)
			b.WhenWePublish(s, val1, val2)

			s.And("after resubscribing to the publisher", func(s *testcase.Spec) {
				sub := b.GivenWeHaveSubscription(s)

				s.Then("messages are received", func(t *testcase.T) {
					expected := []V{val1.Get(t), val2.Get(t)}
					t.Eventually(func(it assert.It) {
						it.Must.ContainExactly(expected, sub.Get(t).Values())
					})
				})
			})
		})
	})
}

func (c Buffered[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Buffered[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// Volatile defines a publisher behaviour where if the subscription is canceled, published messages won't be delivered.
// In certain scenarios, you may want to send a volatile message with no assurances over a publisher,
// when timely delivery is more important than losing messages.
type Volatile[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V
}

func (c Volatile[V]) Spec(s *testcase.Spec) {
	b := pubsubBase[V]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeV,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s is volatile", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.WhenIsEmpty(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := let.With[V](s, c.MakeV)
			val2 := let.With[V](s, c.MakeV)
			b.WhenWePublish(s, val1, val2)

			s.And("after resubscribing to the publisher", func(s *testcase.Spec) {
				sub := b.GivenWeHaveSubscription(s)

				s.Then("messages published previously won't arrive", func(t *testcase.T) {
					pubsubtest.Waiter.Wait()

					t.Eventually(func(it assert.It) {
						it.Must.Empty(sub.Get(t).Values())
					})
				})
			})
		})
	})
}

func (c Volatile[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Volatile[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
