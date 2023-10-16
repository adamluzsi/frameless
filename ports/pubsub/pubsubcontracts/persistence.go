package pubsubcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/ports/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// Buffered defines a publisher behaviour where if the subscription is canceled,
// the publisher messages can be still consumed after resubscribing.
type Buffered[Data any] func(testing.TB) BufferedSubject[Data]

type BufferedSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c Buffered[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) BufferedSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := subject.Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})

	b.Spec(s)

	s.Context("buffered", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.And("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
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
type Volatile[Data any] func(testing.TB) VolatileSubject[Data]

type VolatileSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c Volatile[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) VolatileSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := subject.Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("volatile", func(s *testcase.Spec) {
		b.TryCleanup(s)
		b.GivenWeHadSubscriptionBefore(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
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
