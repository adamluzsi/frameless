package pubsubcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
	"testing"
)

// FanOut defines an exchange behaviour where messages are published to all the associated pubsub.Queue.
type FanOut[Data any] struct {
	MakeSubject func(testing.TB) FanOutSubject[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
}

type FanOutSubject[Data any] struct {
	// Exchange is the publisher that suppose to publish to all queue made with MakeQueue.
	Exchange pubsub.Publisher[Data]
	// MakeQueue creates a queue and binds it to the Exchange to receive events.
	// Queues made with MakeQueue suppose to be cleaned up after the test.
	// For the cleanup purpose, use the testing.TB received as part of FanOut.MakeSubject.
	MakeQueue func() pubsub.Subscriber[Data]
}

func (c FanOut[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c FanOut[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

func (c FanOut[Data]) Spec(s *testcase.Spec) {
	b := pubsubBase[Data]{
		MakeSubject: func(tb testing.TB) PubSub[Data] {
			exchange := c.MakeSubject(tb)
			return PubSub[Data]{
				Publisher:  exchange.Exchange,
				Subscriber: exchange.MakeQueue(),
			}
		},
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeData,
	}
	b.Spec(s)

	s.Context("exchange strategy is fan-out", func(s *testcase.Spec) {
		exchange := let.With[FanOutSubject[Data]](s, c.MakeSubject)
		val1 := let.With[Data](s, c.MakeData)
		val2 := let.With[Data](s, c.MakeData)
		val3 := let.With[Data](s, c.MakeData)

		s.Test("with a single queue, a consumer will receives all the messages", func(t *testcase.T) {
			q1 := exchange.Get(t).MakeQueue()

			t.Must.NoError(exchange.Get(t).Exchange.Publish(c.MakeContext(t),
				val1.Get(t), val2.Get(t), val3.Get(t),
			))

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			res1 := pubsubtest.Subscribe(t, q1, c.MakeContext(t))

			res1.Eventually(t, func(tb testing.TB, got []Data) {
				assert.Must(tb).ContainExactly(expected, got)
			})
		})

		s.Test("with multiple queues on the exchange, all consumer will receives every messages", func(t *testcase.T) {
			var results []*pubsubtest.AsyncResults[Data]

			t.Random.Repeat(2, 7, func() {
				results = append(results,
					pubsubtest.Subscribe(t, exchange.Get(t).MakeQueue(), c.MakeContext(t)))
			})

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			t.Must.NoError(exchange.Get(t).Exchange.Publish(c.MakeContext(t), expected...))

			for i, res := range results {
				res.Eventually(t, func(tb testing.TB, got []Data) {
					assert.Must(tb).ContainExactly(expected, got,
						fmt.Sprintf("expected that the %d. subscription also received all events", i+1))
				})
			}
		})
	})
}
