package synckit_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleFan_out() {
	var g synckit.Group
	defer g.Wait()
	defer g.Cancel()

	var (
		ctx = context.Background()

		fanOut synckit.Fan[string]
		fanIn  synckit.Fan[string]
	)

	for range runtime.NumCPU() {
		g.Go(ctx, func(ctx context.Context) error {
			for msg, err := range fanOut.Subscribe(ctx) {
				if err != nil {
					return err
				}
				var data = strings.ToUpper(msg.Data())
				if err := fanIn.Publish(ctx, data); err != nil {
					msg.NACK()
					return err
				}
				msg.ACK()
			}
			return nil
		})
	}

	var values = []string{"foo", "bar", "baz"}

	for _, v := range values {
		if err := fanOut.Publish(ctx, v); err != nil {
			return // err
		}
	}
}

func ExampleFan_in() {
	var (
		ctx   = context.Background()
		fanIn synckit.Fan[int]
	)

	var g synckit.Group
	defer g.Wait()
	for range runtime.NumCPU() {
		g.Go(ctx, func(ctx context.Context) error {
			for n := range 42 {
				if err := fanIn.Publish(ctx, n); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// Fan IN
	vs, err := iterkit.CollectE(fanIn.Subscribe(ctx))
	_, _ = vs, err
}

func ExampleFan() {
	var (
		ctx    = context.Background()
		fanOut synckit.Fan[string]
	)

	fanOut.Subscribe(ctx)      // receive
	fanOut.Publish(ctx, "foo") // publish
}

func TestFanOut(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *synckit.Fan[string] {
		return &synckit.Fan[string]{}
	})

	// subscribe registers a background consumer bound to the subject that
	// acknowledges and records every message it receives.
	subscribe := func(s *testcase.Spec) testcase.Var[*pubsubtest.AsyncResults[string]] {
		return let.Var(s, func(t *testcase.T) *pubsubtest.AsyncResults[string] {
			return pubsubtest.Subscribe[string](t, subject.Get(t), context.Background())
		})
	}

	s.Describe("#Publish", func(s *testcase.Spec) {
		var (
			ctx, cancel = let.ContextWithCancel(s)
			values      = let.VarOf[[]string](s, nil)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Publish(ctx.Get(t), values.Get(t)...)
		})

		s.When("there are messages to publish", func(s *testcase.Spec) {
			values.Let(s, func(t *testcase.T) []string {
				var vs []string
				t.Random.Repeat(3, 7, func() {
					vs = append(vs, t.Random.UUID())
				})
				return vs
			})

			s.And("a subscriber is consuming the queue", func(s *testcase.Spec) {
				res := subscribe(s).EagerLoading(s)

				s.Then("it publishes without an error", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})

				s.Then("the subscriber receives the published messages", func(t *testcase.T) {
					assert.NoError(t, act(t))

					res.Get(t).Eventually(t, func(tb testing.TB, got []string) {
						assert.ContainsExactly(tb, values.Get(t), got)
					})
				})
			})

			s.And("multiple subscribers are consuming the queue", func(s *testcase.Spec) {
				var subs []testcase.Var[*pubsubtest.AsyncResults[string]]
				for i := 0; i < 3; i++ {
					subs = append(subs, subscribe(s).EagerLoading(s))
				}

				s.Then("each message is handled by exactly one subscriber (unicast, not broadcast)", func(t *testcase.T) {
					assert.NoError(t, act(t))

					t.Eventually(func(t *testcase.T) {
						var got []string
						for _, sub := range subs {
							got = append(got, sub.Get(t).Values()...)
						}
						assert.ContainsExactly(t, values.Get(t), got)
					})
				})
			})

			s.And("no subscriber is consuming the queue", func(s *testcase.Spec) {
				s.Then("it blocks until a subscriber becomes available to take the message", func(t *testcase.T) {
					done := make(chan error, 1)
					go func() { done <- act(t) }()

					// without a consumer the handoff cannot happen, so publish stays blocked.
					pubsubtest.Waiter.Wait()
					assert.False(t, isFinished(done), "expected Publish to block while there is no subscriber")

					// once a subscriber consumes the queue, the handoff completes and publish returns.
					pubsubtest.Subscribe[string](t, subject.Get(t), context.Background())

					t.Eventually(func(t *testcase.T) {
						assert.True(t, isFinished(done), "expected Publish to unblock once a subscriber is available")
					})
				})
			})
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
			values.Let(s, func(t *testcase.T) []string {
				return []string{t.Random.UUID()}
			})

			s.Before(func(t *testcase.T) {
				cancel.Get(t)()
			})

			s.Then("it returns the context's error", func(t *testcase.T) {
				assert.ErrorIs(t, ctx.Get(t).Err(), act(t))
			})
		})
	})

	s.Describe("#Subscribe", func(s *testcase.Spec) {
		var (
			ctx, cancel = let.ContextWithCancel(s)
		)
		act := let.Act(func(t *testcase.T) pubsub.Subscription[string] {
			return subject.Get(t).Subscribe(ctx.Get(t))
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
			s.Then("the subscription ends", func(t *testcase.T) {
				sub := act(t)

				done := make(chan struct{})
				go func() {
					defer close(done)
					for range sub {
					}
				}()

				cancel.Get(t)()

				t.Eventually(func(t *testcase.T) {
					assert.True(t, isFinished(done), "expected the subscription to stop after the context is cancelled")
				})
			})
		})

		s.When("a delivered message gets NACK-ed by a consumer", func(s *testcase.Spec) {
			value := let.Var(s, func(t *testcase.T) string {
				return t.Random.UUID()
			})
			// handled records the messages that were ultimately acknowledged.
			handled := let.Var(s, func(t *testcase.T) *results {
				return &results{}
			})

			// Two consumers subscribe to the queue. The first delivery is NACK-ed
			// (requeued); every later delivery is acknowledged and recorded. A second
			// consumer is required so the requeued message has a peer to be handed to,
			// instead of deadlocking the NACK-ing consumer.
			s.Before(func(t *testcase.T) {
				var (
					wg         sync.WaitGroup
					nackedOnce atomic.Bool
				)
				rec := handled.Get(t)
				consume := func(sub pubsub.Subscription[string]) {
					defer wg.Done()
					for msg, err := range sub {
						assert.NoError(t, err)
						if nackedOnce.CompareAndSwap(false, true) {
							assert.NoError(t, msg.NACK())
							continue
						}
						rec.add(msg.Data())
						assert.NoError(t, msg.ACK())
					}
				}

				wg.Add(2)
				go consume(act(t))
				go consume(act(t))
				t.Defer(func() {
					cancel.Get(t)()
					wg.Wait()
				})
			})

			// a message is queued for delivery. The queue hands messages off
			// synchronously, so publishing runs in the background until a consumer takes it.
			s.Before(func(t *testcase.T) {
				fo, v := subject.Get(t), value.Get(t)
				pubCtx, cancelPub := context.WithCancel(context.Background())
				t.Defer(cancelPub)
				go func() { _ = fo.Publish(pubCtx, v) }()
			})

			s.Then("the message is redelivered until it is acknowledged", func(t *testcase.T) {
				t.Eventually(func(t *testcase.T) {
					assert.Contains(t, handled.Get(t).get(), value.Get(t))
				})
			})
		})
	})

	s.Test("implements pubsub Queue contract", func(t *testcase.T) {
		conf := pubsubcontract.Config[string]{
			MakeData: func(tb testing.TB) string {
				return testcase.ToT(&tb).Random.UUID()
			},
		}
		var sub = testcase.NewSpec(t)
		defer sub.Finish()
		pubsubcontract.Queue[string](subject.Get(t), subject.Get(t), conf).Spec(sub)
	})
}

// results is a concurrency-safe recorder of handled message data.
type results struct {
	mutex sync.Mutex
	data  []string
}

func (r *results) add(v string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.data = append(r.data, v)
}

func (r *results) get() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]string{}, r.data...)
}

func isFinished[T any](ch <-chan T) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
