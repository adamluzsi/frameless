package pubsubcontracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
)

// Buffered defines a publisher behaviour where if the subscription is canceled,
// the publisher messages can be still consumed after resubscribing.
type Buffered[Data any] struct {
	MakeSubject func(testing.TB) PubSub[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
}

func (c Buffered[Data]) Spec(s *testcase.Spec) {
	b := base[Data]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeData:    c.MakeData,
	}
	b.Spec(s)

	s.Context("buffered", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.And("messages are published", func(s *testcase.Spec) {
			val1 := let.With[Data](s, c.MakeData)
			val2 := let.With[Data](s, c.MakeData)
			b.WhenWePublish(s, val1, val2)

			s.And("after resubscribing to the publisher", func(s *testcase.Spec) {
				sub := b.GivenWeHaveSubscription(s)

				s.Then("messages are received", func(t *testcase.T) {
					expected := []Data{val1.Get(t), val2.Get(t)}
					t.Eventually(func(it assert.It) {
						it.Must.ContainExactly(expected, sub.Get(t).Values())
					})
				})
			})
		})
	})
}

func (c Buffered[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Buffered[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// Volatile defines a publisher behaviour where if the subscription is canceled, published messages won't be delivered.
// In certain scenarios, you may want to send a volatile message with no assurances over a publisher,
// when timely delivery is more important than losing messages.
type Volatile[Data any] struct {
	MakeSubject func(testing.TB) PubSub[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
}

func (c Volatile[Data]) Spec(s *testcase.Spec) {
	b := base[Data]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeData:    c.MakeData,
	}
	b.Spec(s)

	s.Context("volatile", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := let.With[Data](s, c.MakeData)
			val2 := let.With[Data](s, c.MakeData)
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

func (c Volatile[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Volatile[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
