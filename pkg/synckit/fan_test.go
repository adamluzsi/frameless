package synckit_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleFan_out() {
	var ctx = context.Background()

	var g synckit.Group
	defer g.Wait()
	defer g.Cancel()

	var fanOut synckit.Fan[string]
	defer fanOut.Close()

	for range runtime.NumCPU() {
		g.Go(ctx, func(ctx context.Context) error {
			for msg, err := range fanOut.Subscribe(ctx) {
				if err != nil {
					return err
				}
				_ = msg.Data()
				_ = msg.ACK()
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

func TestFan(t *testing.T) {
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

		s.Test("a delivered message is not ACK ed upon finishing the current for round, it gets NACK-ed automatically", func(t *testcase.T) {
			var f synckit.Fan[int]
			var g synckit.Group

			var data = t.Random.Int()

			var n int64
			for range 2 {
				g.Go(t.Context(), func(ctx context.Context) error {
					for msg, err := range f.Subscribe(ctx) {
						if err != nil {
							return err
						}
						assert.Equal(t, msg.Data(), data)
						atomic.AddInt64(&n, 1)
						continue // without ACK
					}
					return nil
				})
			}

			assert.NoError(t, f.Publish(t.Context(), data))

			t.Eventually(func(t *testcase.T) {
				assert.True(t, atomic.LoadInt64(&n) > 42,
					"expected that due to inf loop with constant NACK -s, n keeps incremented")
			})

			g.Cancel()
		})

		s.When("a delivered message gets NACK-ed by a consumer", func(s *testcase.Spec) {
			s.Test("single consumer", func(t *testcase.T) {
				var f synckit.Fan[int]
				var nackErr error
				j := synckit.Go(t.Context(), func(ctx context.Context) error {
					for msg, err := range f.Subscribe(ctx) {
						if err != nil {
							return err
						}
						assert.Within(t, timeout, func(ctx context.Context) {
							nackErr = msg.NACK()
						})
						break
					}
					return nil
				})

				assert.NoError(t, f.Publish(t.Context(), 42))
				time.Sleep(timeout)
				// nack should be attempted, but since there is only a single subscriber,
				// nack is not possible
				// thus it yields an error
				assert.NoError(t, j.Wait())
				assert.Error(t, nackErr)
			})

			s.Context(">1 consumers", func(s *testcase.Spec) {
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
	})

	s.Describe("#Close", func(s *testcase.Spec) {

		s.Then("it will cancel waiting publish", func(t *testcase.T) {
			var f synckit.Fan[int]

			var gotErr error
			w := assert.NotWithin(t, timeout, func(context.Context) {
				gotErr = f.Publish(t.Context(), t.Random.Int())
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					assert.NoError(t, f.Close())
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.ErrorIs(t, gotErr, context.Canceled)
		})

		s.Then("it will cancel subscription when nothing requires processing", func(t *testcase.T) {
			var f synckit.Fan[int]

			var count int
			w := assert.NotWithin(t, timeout, func(context.Context) {
				for range f.Subscribe(t.Context()) {
					count++
				}
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					assert.NoError(t, f.Close())
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.Equal(t, count, 0)
		})

		s.Then("it will NOT cancel subscription when value is still needs to be processed", func(t *testcase.T) {
			var f synckit.Fan[int]

			expected := random.Slice(t.Random.IntBetween(100, 300), t.Random.Int)
			var actually []int
			var m sync.Mutex

			var g synckit.Group
			n := t.Random.IntBetween(3, 7)

			for range n {
				g.Go(t.Context(), func(ctx context.Context) error {
					for msg, err := range f.Subscribe(t.Context()) {
						assert.NoError(t, err)
						assert.NotNil(t, msg)

						m.Lock()
						actually = append(actually, msg.Data())
						m.Unlock()
						msg.ACK()
					}
					return nil
				})
			}

			assert.NoError(t, f.Publish(t.Context(), expected...))
			assert.NoError(t, f.Close())
			assert.NoError(t, g.Wait())
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

	s.Context("race", func(s *testcase.Spec) {
		s.Test("pub/sub/close", func(t *testcase.T) {
			var f synckit.Fan[int]

			testcase.Race(func() {
				f.Publish(t.Context(), 42)
			}, func() {
				for msg, err := range f.Subscribe(t.Context()) {
					assert.NoError(t, err)
					msg.NACK()
				}
			}, func() {
				for msg, err := range f.Subscribe(t.Context()) {
					assert.NoError(t, err)
					msg.ACK()
				}
			}, func() {
				assert.NoError(t, f.Close())
			})
		})

		s.Test("close after publish", func(t *testcase.T) {
			var f synckit.Fan[int]

			var done = make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)

			testcase.Race(func() {
				defer close(done)
				assert.NoError(t, f.Publish(t.Context(), 42))
			}, func() {
				defer wg.Done()
				for msg, err := range f.Subscribe(t.Context()) {
					assert.NoError(t, err)
					msg.NACK()
				}
			}, func() {
				defer wg.Done()
				for msg, err := range f.Subscribe(t.Context()) {
					assert.NoError(t, err)
					msg.ACK()
				}
			}, func() {
				<-done
				assert.NoError(t, f.Close())
				assert.Within(t, timeout, func(ctx context.Context) {
					wg.Wait()
				})
			})
		})
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
