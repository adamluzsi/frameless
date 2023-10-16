package pubsubcontracts

import (
	"context"
	"go.llib.dev/frameless/ports/pubsub/pubsubtest"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// Ordering is a contract that describes how the ordering should happen with a given
type Ordering[Data any] func(testing.TB) OrderingSubject[Data]

type OrderingSubject[Data any] struct {
	PubSub PubSub[Data]
	// Sort function arranges a list of data
	// in the order that it's expected to be received from the PubSub
	// when the data is published into the PubSub.
	Sort func([]Data)

	MakeContext func() context.Context
	MakeData    func() Data
}

func (c Ordering[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) OrderingSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := c(tb)
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("ordering", func(s *testcase.Spec) {
		b.TryCleanup(s)

		s.Test("", func(t *testcase.T) {
			var (
				ctx  = subject.Get(t).MakeContext()
				val1 = subject.Get(t).MakeData()
				val2 = subject.Get(t).MakeData()
				val3 = subject.Get(t).MakeData()
			)

			ps := subject.Get(t).PubSub
			sub := ps.Subscribe(ctx)
			defer sub.Close()

			t.Must.NoError(ps.Publish(ctx, val1, val2, val3))
			pubsubtest.Waiter.Wait()

			expected := []Data{val1, val2, val3}
			subject.Get(t).Sort(expected)

			var got []Data
			for i, m := 0, len(expected); i < m; i++ {
				t.Must.Within(pubsubtest.Waiter.Timeout, func(context.Context) {
					t.Must.True(sub.Next())
				})

				msg := sub.Value()
				got = append(got, msg.Data())
				t.Must.NoError(msg.ACK())
			}

			t.Must.Equal(expected, got)
		})
	})
}

func (c Ordering[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Ordering[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// FIFO
//
// It stands for First-In-First-Out approach.
// In this, the new element is inserted below the existing element, So that the oldest element can be at the top and taken out first.
// Therefore, the first element to be entered in this approach, gets out First.
// In computing, FIFO approach is used as an operating system algorithm, which gives every process CPU time in the order they arrive.
// The data structure that implements FIFO is Queue.
type FIFO[Data any] func(testing.TB) FIFOSubject[Data]

type FIFOSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c FIFO[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) FIFOSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := c(tb)
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("ordering is FIFO", func(s *testcase.Spec) {
		b.TryCleanup(s)

		subscription := b.GivenWeHaveSubscription(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
			val2 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
			val3 := testcase.Let(s, func(t *testcase.T) Data {
				return subject.Get(t).MakeData()
			})
			b.WhenWePublish(s, val1, val2, val3)

			s.Then("messages are received in their publishing order", func(t *testcase.T) {
				t.Eventually(func(it assert.It) {
					it.Must.Equal([]Data{val1.Get(t), val2.Get(t), val3.Get(t)}, subscription.Get(t).Values())
				})
			})
		})
	})
}

func (c FIFO[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c FIFO[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// LIFO
//
// It stands for Last-In-First-Out approach in programming.
// In this, the new element is inserted above the existing element, So that the newest element can be at the top and taken out first.
// Therefore, the first element to be entered in this approach, gets out Last.
// In computing, LIFO approach is used as a queuing theory that refers to the way items are stored in types of data structures.
// The data structure that implements LIFO is Stack.
type LIFO[Data any] func(testing.TB) LIFOSubject[Data]

type LIFOSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c LIFO[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) LIFOSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := subject.Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("ordering is LIFO", func(s *testcase.Spec) {
		b.TryCleanup(s)

		val1 := testcase.Let(s, func(t *testcase.T) Data {
			return subject.Get(t).MakeData()
		})
		val2 := testcase.Let(s, func(t *testcase.T) Data {
			return subject.Get(t).MakeData()
		})
		val3 := testcase.Let(s, func(t *testcase.T) Data {
			return subject.Get(t).MakeData()
		})

		s.Then("messages are received in their publishing order", func(t *testcase.T) {
			ps := subject.Get(t).PubSub
			sub := ps.Subscribe(subject.Get(t).MakeContext())
			defer sub.Close()

			t.Must.NoError(ps.Publish(subject.Get(t).MakeContext(), val1.Get(t), val2.Get(t), val3.Get(t)))
			expected := []Data{val3.Get(t), val2.Get(t), val1.Get(t)}

			var got []Data
			for i, m := 0, len(expected); i < m; i++ {
				t.Must.Within(pubsubtest.Waiter.Timeout, func(context.Context) {
					t.Must.True(sub.Next())
				})
				msg := sub.Value()
				got = append(got, msg.Data())
				t.Must.NoError(msg.ACK())
			}

			t.Must.Equal(expected, got)
		})
	})
}

func (c LIFO[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c LIFO[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
