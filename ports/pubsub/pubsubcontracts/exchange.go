package pubsubcontracts

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/ports/pubsub"
	"go.llib.dev/frameless/ports/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"testing"
)

// FanOut defines an exchange behaviour where messages are published to all the associated pubsub.Queue.
type FanOut[Data any] func(testing.TB) FanOutSubject[Data]

type FanOutSubject[Data any] struct {
	// Exchange is the publisher that suppose to publish to all queue made with MakeQueue.
	Exchange pubsub.Publisher[Data]
	// MakeQueue creates a queue and binds it to the Exchange to receive events.
	// Queues made with MakeQueue suppose to be cleaned up after the test.
	// For the cleanup purpose, use the testing.TB received as part of FanOut.
	MakeQueue func() pubsub.Subscriber[Data]

	MakeContext func() context.Context
	MakeData    func() Data
}

func (c FanOut[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c FanOut[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

func (c FanOut[Data]) subject() testcase.Var[FanOutSubject[Data]] {
	return testcase.Var[FanOutSubject[Data]]{
		ID:   "FanOutSubject[Data]",
		Init: func(t *testcase.T) FanOutSubject[Data] { return c(t) },
	}
}

func (c FanOut[Data]) Spec(s *testcase.Spec) {
	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := c.subject().Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub: PubSub[Data]{
				Publisher:  sub.Exchange,
				Subscriber: sub.MakeQueue(),
			},
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("exchange strategy is fan-out", func(s *testcase.Spec) {
		val1 := testcase.Let(s, func(t *testcase.T) Data {
			return c.subject().Get(t).MakeData()
		})
		val2 := testcase.Let(s, func(t *testcase.T) Data {
			return c.subject().Get(t).MakeData()
		})
		val3 := testcase.Let(s, func(t *testcase.T) Data {
			return c.subject().Get(t).MakeData()
		})

		s.Test("with a single queue, a consumer will receives all the messages", func(t *testcase.T) {
			q1 := c.subject().Get(t).MakeQueue()

			t.Must.NoError(c.subject().Get(t).Exchange.Publish(c.subject().Get(t).MakeContext(),
				val1.Get(t), val2.Get(t), val3.Get(t),
			))

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			res1 := pubsubtest.Subscribe(t, q1, c.subject().Get(t).MakeContext())

			res1.Eventually(t, func(tb testing.TB, got []Data) {
				assert.Must(tb).ContainExactly(expected, got)
			})
		})

		s.Test("with multiple queues on the exchange, all consumer will receives every messages", func(t *testcase.T) {
			var results []*pubsubtest.AsyncResults[Data]

			t.Random.Repeat(2, 7, func() {
				results = append(results,
					pubsubtest.Subscribe(t, c.subject().Get(t).MakeQueue(), c.subject().Get(t).MakeContext()))
			})

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			t.Must.NoError(c.subject().Get(t).Exchange.Publish(c.subject().Get(t).MakeContext(), expected...))

			for i, res := range results {
				res.Eventually(t, func(tb testing.TB, got []Data) {
					assert.Must(tb).ContainExactly(expected, got,
						assert.Message(fmt.Sprintf("expected that the %d. subscription also received all events", i+1)))
				})
			}
		})
	})
}
