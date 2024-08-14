package pubsubcontracts

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// FanOut defines an exchange behaviour where messages are published to all the associated pubsub.Queue.
func FanOut[Data any](
	// Exchange is the publisher that suppose to publish to all queue made with MakeQueue.
	Exchange pubsub.Publisher[Data],
	// MakeQueue creates a queue and binds it to the Exchange to receive events.
	// Queues made with MakeQueue suppose to be cleaned up after the test.
	// For the cleanup purpose, use the testing.TB received as part of FanOut.
	MakeQueue func(testing.TB) pubsub.Subscriber[Data],
	opts ...Option[Data]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[Data]](opts)

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		return baseSubject[Data]{
			Publisher:   Exchange,
			Subscriber:  MakeQueue(tb),
			MakeContext: c.MakeContext,
			MakeData:    c.MakeData,
		}
	})
	b.Spec(s)

	s.Context("exchange strategy is fan-out", func(s *testcase.Spec) {
		val1 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})
		val2 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})
		val3 := testcase.Let(s, func(t *testcase.T) Data {
			return c.MakeData(t)
		})

		s.Test("with a single queue, a consumer will receives all the messages", func(t *testcase.T) {
			q1 := MakeQueue(t)

			t.Must.NoError(Exchange.Publish(c.MakeContext(),
				val1.Get(t), val2.Get(t), val3.Get(t),
			))

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			res1 := pubsubtest.Subscribe(t, q1, c.MakeContext())

			res1.Eventually(t, func(tb testing.TB, got []Data) {
				assert.Must(tb).ContainExactly(expected, got)
			})
		})

		s.Test("with multiple queues on the exchange, all consumer will receives every messages", func(t *testcase.T) {
			var results []*pubsubtest.AsyncResults[Data]

			t.Random.Repeat(2, 7, func() {
				results = append(results,
					pubsubtest.Subscribe(t, MakeQueue(t), c.MakeContext()))
			})

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			t.Must.NoError(Exchange.Publish(c.MakeContext(), expected...))

			for i, res := range results {
				res.Eventually(t, func(tb testing.TB, got []Data) {
					assert.Must(tb).ContainExactly(expected, got,
						assert.Message(fmt.Sprintf("expected that the %d. subscription also received all events", i+1)))
				})
			}
		})
	})

	return s.AsSuite("FanOut")
}
