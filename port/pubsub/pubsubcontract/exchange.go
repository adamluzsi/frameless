package pubsubcontract

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

// Broadcast ensures that an identical data is sent to every subscriber.
func Broadcast[Data any](
	// Exchange is the publisher that suppose to publish to all queue made with MakeQueue.
	Exchange pubsub.Publisher[Data],
	// MakeSubscriber creates a queue and binds it to the Exchange to receive events.
	// Subs made with MakeSubscriber suppose to be cleaned up after the test.
	// For the cleanup purpose, use the testing.TB received as part of FanOut.
	MakeSubscriber func(testing.TB) pubsub.Subscriber[Data],
	opts ...Option[Data]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[Data]](opts)

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		return baseSubject[Data]{
			Publisher:   Exchange,
			Subscriber:  MakeSubscriber(tb),
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

		s.Test("with a single sub, a consumer will receives all the messages", func(t *testcase.T) {
			q1 := MakeSubscriber(t)

			assert.Must(t).NoError(Exchange.Publish(c.MakeContext(t), val1.Get(t)))
			assert.Must(t).NoError(Exchange.Publish(c.MakeContext(t), val2.Get(t)))
			assert.Must(t).NoError(Exchange.Publish(c.MakeContext(t), val3.Get(t)))

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			res1 := pubsubtest.Subscribe(t, q1, c.MakeContext(t))

			res1.Eventually(t, func(tb testing.TB, got []Data) {
				assert.Must(tb).ContainsExactly(expected, got)
			})
		})

		s.Test("with multiple subs on the exchange, subscribers will receive identical messages", func(t *testcase.T) {
			var results []*pubsubtest.AsyncResults[Data]

			t.Random.Repeat(2, 7, func() {
				results = append(results,
					pubsubtest.Subscribe(t, MakeSubscriber(t), c.MakeContext(t)))
			})

			expected := []Data{val1.Get(t), val2.Get(t), val3.Get(t)}
			for _, val := range expected {
				assert.NoError(t, Exchange.Publish(c.MakeContext(t), val))
			}

			for i, res := range results {
				res.Eventually(t, func(tb testing.TB, got []Data) {
					assert.Must(tb).ContainsExactly(expected, got,
						assert.Message(fmt.Sprintf("expected that the %d. subscription also received all events", i+1)))
				})
			}
		})
	})

	return s.AsSuite("Broadcast")
}
