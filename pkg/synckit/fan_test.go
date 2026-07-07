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
	defer fanOut.Cancel()

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

			var sampling = t.Random.IntBetween(2, 5)
			for range sampling {
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

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, g.Len(), sampling,
					"expected that all test goroutine is scheduled and active already")
				assert.Equal(t, f.Len(), sampling,
					"expected that all subscriber should be there, ready for receiving a publication")
			})

			assert.NoError(t, f.Publish(t.Context(), data))

			t.Eventually(func(t *testcase.T) {
				var got = atomic.LoadInt64(&n)
				assert.True(t, 42 < got,
					assert.MessageF("expected that due to inf loop with constant NACK -s, n (%d) keeps incremented", got))
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
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Close()
		})

		s.Then("it will cancel waiting publish", func(t *testcase.T) {
			f := subject.Get(t)

			var gotErr error
			w := assert.NotWithin(t, timeout, func(context.Context) {
				gotErr = f.Publish(t.Context(), t.Random.UUID())
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					assert.NoError(t, act(t))
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.ErrorIs(t, gotErr, context.Canceled)
		})

		s.Then("it will cancel subscription when nothing requires processing", func(t *testcase.T) {
			f := subject.Get(t)

			var count int
			w := assert.NotWithin(t, timeout, func(context.Context) {
				for range f.Subscribe(t.Context()) {
					count++
				}
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					assert.NoError(t, act(t))
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.Equal(t, count, 0)
		})

		s.Then("it will NOT cancel subscription when value is still needs to be processed", func(t *testcase.T) {
			f := subject.Get(t)

			expected := random.Slice(t.Random.IntBetween(100, 300), t.Random.UUID)
			var actually []string
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
			assert.NoError(t, act(t))
			assert.NoError(t, g.Wait())
		})

		s.Then("it will NOT cancel the context of an in-flight message that is still being processed", func(t *testcase.T) {
			f := subject.Get(t)

			var (
				processing        = make(chan struct{}) // signals that a message delivery began
				release           = make(chan struct{}) // gate that keeps the message in-flight
				ctxErrDuringClose error
				ctxErrAfterClose  error
			)

			var g synckit.Group
			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range f.Subscribe(ctx) {
					if err != nil {
						return err
					}
					// a message is now in-flight; keep processing it while Close is called.
					close(processing)
					ctxErrDuringClose = msg.Context().Err()
					<-release
					ctxErrAfterClose = msg.Context().Err()
					assert.NoError(t, msg.ACK())
					return nil
				}
				return nil
			})

			// hand a message off to the subscriber; Publish blocks until it is taken.
			assert.NoError(t, f.Publish(t.Context(), t.Random.UUID()))

			// wait until the subscriber has taken the message and started processing it.
			<-processing

			// Close while the message is in-flight must not cancel its context.
			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})

			// let the in-flight processing complete after Close returned.
			close(release)
			assert.NoError(t, g.Wait())

			assert.NoError(t, ctxErrDuringClose,
				"the in-flight message context must not be cancelled while Close is called")
			assert.NoError(t, ctxErrAfterClose,
				"the in-flight message context must remain valid after Close returned")
		})
	})

	// #Cancel behaves like a stronger #Close: besides signalling that no more
	// values are expected (unblocking pending Publish calls and ending idle
	// subscriptions), it also cancels the context of any message that is still
	// in-flight, so consumers are asked to abort their current processing.
	s.Describe("#Cancel", func(s *testcase.Spec) {
		act := let.Act0(func(t *testcase.T) {
			subject.Get(t).Cancel()
		})

		s.Then("it will cancel waiting publish", func(t *testcase.T) {
			f := subject.Get(t)

			var gotErr error
			w := assert.NotWithin(t, timeout, func(context.Context) {
				gotErr = f.Publish(t.Context(), t.Random.UUID())
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					act(t)
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.ErrorIs(t, gotErr, context.Canceled)
		})

		s.Then("it will cancel subscription when nothing requires processing", func(t *testcase.T) {
			f := subject.Get(t)

			var count int
			w := assert.NotWithin(t, timeout, func(context.Context) {
				for range f.Subscribe(t.Context()) {
					count++
				}
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					act(t)
				})
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			})

			assert.Equal(t, count, 0)
		})

		s.Then("it will cancel the context of an in-flight message that is still being processed", func(t *testcase.T) {
			f := subject.Get(t)

			var (
				processing = make(chan struct{}) // signals that a message delivery began
				ctxErr     error
			)

			var g synckit.Group
			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range f.Subscribe(ctx) {
					if err != nil {
						return err
					}
					// a message is now in-flight; keep processing it until Cancel is called.
					close(processing)
					// Cancel must cancel the in-flight message's context, unblocking this wait.
					assert.Within(t, timeout, func(context.Context) {
						<-msg.Context().Done()
					})
					ctxErr = msg.Context().Err()
					return nil
				}
				return nil
			})

			// hand a message off to the subscriber; Publish blocks until it is taken.
			assert.NoError(t, f.Publish(t.Context(), t.Random.UUID()))

			// wait until the subscriber has taken the message and started processing it.
			<-processing

			// Cancel while the message is in-flight must cancel its context.
			assert.Within(t, timeout, func(ctx context.Context) {
				act(t)
			})

			assert.NoError(t, g.Wait())
			assert.ErrorIs(t, ctxErr, context.Canceled,
				"the in-flight message context must be cancelled by Cancel")
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

// BenchmarkFan measures the synchronous hand-off throughput of Fan across its
// common topologies. In every case a set of subscribers is drained in the
// background and the timed section only publishes, so the numbers reflect the
// cost of a publish -> deliver -> ACK round trip.
func BenchmarkFan(b *testing.B) {
	// consume drains a subscription, acknowledging every message, until ctx ends.
	consume := func(f *synckit.Fan[int]) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			for msg, err := range f.Subscribe(ctx) {
				if err != nil {
					return err
				}
				_ = msg.Data()
				_ = msg.ACK()
			}
			return nil
		}
	}

	b.Run("unicast (1 publisher, 1 subscriber)", func(b *testing.B) {
		var f synckit.Fan[int]
		var g synckit.Group
		g.Go(context.Background(), consume(&f))

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := f.Publish(context.Background(), i); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()
		g.Cancel()
		_ = g.Wait()
	})

	b.Run("fan-out (1 publisher, NumCPU subscribers)", func(b *testing.B) {
		var f synckit.Fan[int]
		var g synckit.Group
		for range runtime.NumCPU() {
			g.Go(context.Background(), consume(&f))
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := f.Publish(context.Background(), i); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()
		g.Cancel()
		_ = g.Wait()
	})

	b.Run("parallel (GOMAXPROCS publishers, GOMAXPROCS subscribers)", func(b *testing.B) {
		var f synckit.Fan[int]
		var g synckit.Group
		for range runtime.GOMAXPROCS(0) {
			g.Go(context.Background(), consume(&f))
		}

		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if err := f.Publish(context.Background(), 42); err != nil {
					b.Error(err)
					return
				}
			}
		})

		b.StopTimer()
		g.Cancel()
		_ = g.Wait()
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
