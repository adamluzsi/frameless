package pubsubcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase/let"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

// FIFO
//
// It stands for First-In-First-Out approach.
// In this, the new element is inserted below the existing element, So that the oldest element can be at the top and taken out first.
// Therefore, the first element to be entered in this approach, gets out First.
// In computing, FIFO approach is used as an operating system algorithm, which gives every process CPU time in the order they arrive.
// The data structure that implements FIFO is Queue.
type FIFO[Data any] struct {
	MakeSubject func(testing.TB) PubSub[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
}

func (c FIFO[Data]) Spec(s *testcase.Spec) {
	b := pubsubBase[Data]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeData,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s ordering is FIFO", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.TryCleanup(s)

		subscription := b.GivenWeHaveSubscription(s)

		s.When("messages are published", func(s *testcase.Spec) {
			val1 := let.With[Data](s, c.MakeData)
			val2 := let.With[Data](s, c.MakeData)
			val3 := let.With[Data](s, c.MakeData)
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
type LIFO[Data any] struct {
	MakeSubject func(testing.TB) PubSub[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
}

func (c LIFO[Data]) Spec(s *testcase.Spec) {
	b := pubsubBase[Data]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeData,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s ordering is LIFO", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.TryCleanup(s)

		val1 := let.With[Data](s, c.MakeData)
		val2 := let.With[Data](s, c.MakeData)
		val3 := let.With[Data](s, c.MakeData)

		s.Then("messages are received in their publishing order", func(t *testcase.T) {
			ps := c.MakeSubject(t)
			sub := ps.Subscribe(c.MakeContext(t))
			defer sub.Close()

			t.Must.NoError(ps.Publish(c.MakeContext(t), val1.Get(t), val2.Get(t), val3.Get(t)))
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
