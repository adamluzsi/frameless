package pubsubcontract

import (
	"context"
	"iter"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

// Ordering is a contract that describes how the ordering should happen with a given
func Ordering[Data any](
	publisher pubsub.Publisher[Data],
	subscriber pubsub.Subscriber[Data],
	// Sort function arranges a list of data
	// in the order that it's expected to be received from the PubSub
	// when the data is published into the PubSub.
	Sort func([]Data),
	opts ...Option[Data],
) contract.Contract {

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

	s.Context("ordering", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.Test("", func(t *testcase.T) {
			var (
				ctx  = c.MakeContext(t)
				val1 = c.MakeData(t)
				val2 = c.MakeData(t)
				val3 = c.MakeData(t)
			)

			sub := subscriber.Subscribe(ctx)

			assert.Must(t).NoError(publisher.Publish(ctx, val1, val2, val3))
			pubsubtest.Waiter.Wait()

			expected := []Data{val1, val2, val3}
			Sort(expected)

			next, stop := iter.Pull2(iter.Seq2[pubsub.Message[Data], error](sub))
			defer stop()

			var got []Data
			for i, m := 0, len(expected); i < m; i++ {
				var (
					msg pubsub.Message[Data]
					err error
					ok  bool
				)
				assert.Must(t).Within(pubsubtest.Waiter.Timeout, func(context.Context) {
					msg, err, ok = next()
					assert.True(t, ok)
				})

				assert.NoError(t, err)
				got = append(got, msg.Data())
				assert.NoError(t, msg.ACK())
			}

			assert.Must(t).Equal(expected, got)
		})
	})

	return s.AsSuite("Ordering")
}

// FIFO
//
// It stands for First-In-First-Out approach.
// In this, the new element is inserted below the existing element, So that the oldest element can be at the top and taken out first.
// Therefore, the first element to be entered in this approach, gets out First.
// In computing, FIFO approach is used as an operating system algorithm, which gives every process CPU time in the order they arrive.
// The data structure that implements FIFO is Queue.
func FIFO[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
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

	s.Context("ordering is FIFO", func(s *testcase.Spec) {
		b.TryCleanup(s)

		subscription := b.GivenWeHaveSubscription(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			val3 := testcase.Let(s, func(t *testcase.T) Data {
				return c.MakeData(t)
			})
			b.WhenWePublish(s, val1, val2, val3)

			s.Then("messages are received in their publishing order", func(t *testcase.T) {
				t.Eventually(func(it *testcase.T) {
					assert.Equal(it, []Data{val1.Get(t), val2.Get(t), val3.Get(t)}, subscription.Get(t).Values())
				})
			})
		})
	})
	return s.AsSuite("FIFO")
}

// LIFO
//
// It stands for Last-In-First-Out approach in programming.
// In this, the new element is inserted above the existing element, So that the newest element can be at the top and taken out first.
// Therefore, the first element to be entered in this approach, gets out Last.
// In computing, LIFO approach is used as a queuing theory that refers to the way items are stored in types of data structures.
// The data structure that implements LIFO is Stack.
func LIFO[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
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

	s.Context("ordering is LIFO", func(s *testcase.Spec) {
		b.TryCleanup(s)

		val1 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})
		val2 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})
		val3 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})

		s.Then("messages are received in their publishing order", func(t *testcase.T) {
			sub := subscriber.Subscribe(c.MakeContext(t))

			assert.Must(t).NoError(publisher.Publish(c.MakeContext(t), val1.Get(t), val2.Get(t), val3.Get(t)))
			expected := []Data{val3.Get(t), val2.Get(t), val1.Get(t)}

			next, stop := iter.Pull2(iter.Seq2[pubsub.Message[Data], error](sub))
			defer stop()

			var got []Data
			for i, m := 0, len(expected); i < m; i++ {
				var (
					msg pubsub.Message[Data]
					err error
					ok  bool
				)
				assert.Must(t).Within(pubsubtest.Waiter.Timeout, func(context.Context) {
					msg, err, ok = next()
					assert.True(t, ok)
				})

				assert.NoError(t, err)
				got = append(got, msg.Data())
				assert.NoError(t, msg.ACK())
			}

			assert.Must(t).Equal(expected, got)
		})
	})

	return s.AsSuite("LIFO")
}
