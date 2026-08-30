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
		var method = func(t *testcase.T, ctx context.Context, data string) error {
			return subject.Get(t).Publish(ctx, data)
		}
		var (
			ctx, cancel = let.ContextWithCancel(s)
			data        = let.String(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return method(t, ctx.Get(t), data.Get(t))
		})

		s.When("there are messages to publish", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []string {
				var vs []string
				t.Random.Repeat(3, 7, func() {
					vs = append(vs, t.Random.UUID())
				})
				return vs
			})
			s.Before(func(t *testcase.T) {
				// Fan.Publish is a synchronous handoff, so seeding the queue must not
				// block the setup: publish the values in the background where they
				// wait until a subscriber consumes them (draining on ctx cancel).
				fan := subject.Get(t)
				publishCtx := ctx.Get(t)
				for _, data := range values.Get(t) {
					go func(data string) {
						_ = fan.Publish(publishCtx, data)
					}(data)
				}
			})

			var allValues = func(t *testcase.T) []string {
				var vs []string
				vs = append(vs, values.Get(t)...)
				vs = append(vs, data.Get(t))
				return vs
			}

			s.And("a subscriber is consuming the queue", func(s *testcase.Spec) {
				res := subscribe(s).EagerLoading(s)

				s.Then("the subscriber receives the published messages", func(t *testcase.T) {
					assert.NoError(t, act(t))

					expected := allValues(t)
					res.Get(t).Eventually(t, func(tb testing.TB, got []string) {
						assert.ContainsExactly(tb, expected, got)
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

					expected := allValues(t)
					t.Eventually(func(t *testcase.T) {
						var got []string
						for _, sub := range subs {
							got = append(got, sub.Get(t).Values()...)
						}
						assert.ContainsExactly(t, expected, got)
					})
				})
			})

			s.And("no subscriber is consuming the queue", func(s *testcase.Spec) {
				s.Then("it blocks until a subscriber becomes available to take the message", func(t *testcase.T) {
					var gotErr error

					w := assert.NotWithin(t, timeout, func(ctx context.Context) {
						gotErr = act(t)
					}, "expected Publish to block while there is no subscriber")

					// once a subscriber consumes the queue, the handoff completes and publish returns.
					async := pubsubtest.Subscribe[string](t, subject.Get(t), t.Context())

					assert.Within(t, deadline, func(ctx context.Context) {
						w.Wait()
					}, "expected Publish to unblock once a subscriber is available")

					assert.NoError(t, gotErr)

					async.Eventually(t, func(tb testing.TB, values []string) {
						assert.Contains(tb, values, data.Get(t))
					})
				})
			})
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
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

				var expData int = t.Random.Int()

				assert.NoError(t, f.Publish(t.Context(), expData))
				time.Sleep(timeout)
				assert.NoError(t, j.Wait())
				t.Log("nack should yield no error")
				assert.NoError(t, nackErr)

				t.Log("and upon resubscribing, it should be received again")
				assert.Within(t, deadline, func(ctx context.Context) {
					for msg, err := range f.Subscribe(ctx) {
						assert.NoError(t, err)
						assert.Equal(t, msg.Data(), expData)
						assert.NoError(t, msg.ACK())
						break
					}
				})

				t.Log("but only once, and after ack, it should be considered done")
				assert.NotWithin(t, timeout, func(ctx context.Context) {
					for msg, err := range f.Subscribe(ctx) { // should hang until context cancel
						assert.NoError(t, err)
						assert.NotNil(t, msg)
						break
					}
				})
			})

			s.Context(">1 consumers", func(s *testcase.Spec) {
				value := let.Var(s, func(t *testcase.T) string {
					return t.Random.UUID()
				})

				// ackedValues records the messages that were ultimately acknowledged.
				ackedValues := let.Var(s, func(t *testcase.T) *synckit.Slice[string] {
					return &synckit.Slice[string]{}
				})

				// Two consumers subscribe to the queue. The first delivery is NACK-ed
				// (requeued); every later delivery is acknowledged and recorded. A second
				// consumer is required so the requeued message has a peer to be handed to,
				// instead of deadlocking the NACK-ing consumer.
				s.Before(func(t *testcase.T) {
					var fan = subject.Get(t)

					var g synckit.Group
					t.Cleanup(g.Cancel)

					var nackFirst atomic.Bool

					n := t.Random.Repeat(2, 7, func() {
						g.Go(t.Context(), func(ctx context.Context) error {
							for msg, err := range fan.Subscribe(ctx) {
								if err != nil {
									t.Error(err)
									return err
								}
								if nackFirst.CompareAndSwap(false, true) {
									if err := msg.NACK(); err != nil {
										t.Error(err)
										return err
									}
								} else {
									ackedValues.Get(t).Append(msg.Data())
									if err := msg.ACK(); err != nil {
										t.Error(err)
										return err
									}
								}
							}
							return nil
						})
					})

					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, g.Len(), n)
					})
					t.Eventually(func(t *testcase.T) {
						assert.Equal(t, fan.Len(), n)
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
						ackedValues.Get(t)
						assert.Contains(t, ackedValues.Get(t).ToSlice(), value.Get(t))
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

			for _, data := range expected {
				assert.NoError(t, f.Publish(t.Context(), data))
			}
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

// --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- --- ---

func ExampleBroadcast() {
	var (
		ctx context.Context = context.Background()
		bc  synckit.Broadcast[string]
	)

	bc.Subscribe(ctx)      // every subscriber receives its own copy
	bc.Publish(ctx, "foo") // delivered identically to all current subscribers
}

// TestBroadcast documents the behaviour of synckit.Broadcast.
//
// Broadcast is a broadcasting unit: every value handed to Publish is delivered,
// identical, to every currently subscribed consumer (fan-out). In every other
// respect it behaves like synckit.Fan - a synchronous hand-off with back
// pressure to the active subscribers, context aware Publish/Subscribe, and
// Close/Cancel life-cycle control.
//
// Unlike a durable queue, Broadcast is volatile: it delivers only to the
// consumers subscribed at the moment of publishing. A consumer that subscribes
// later does not receive earlier messages.
func TestBroadcast(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *synckit.Broadcast[string] {
		return &synckit.Broadcast[string]{}
	})

	// subscribe registers a background consumer bound to the subject that
	// acknowledges and records every message it receives.
	subscribe := func(s *testcase.Spec) testcase.Var[*pubsubtest.AsyncResults[string]] {
		return let.Var(s, func(t *testcase.T) *pubsubtest.AsyncResults[string] {
			return pubsubtest.Subscribe[string](t, subject.Get(t), context.Background())
		})
	}

	// waitForSubscribers blocks until the subject reports exactly n active
	// subscriptions, so a publication is never raced against subscriber
	// registration (a broadcast only reaches the consumers subscribed at
	// publish time).
	waitForSubscribers := func(t *testcase.T, n int) {
		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, subject.Get(t).Len(), n)
		})
	}

	s.Describe("#Publish", func(s *testcase.Spec) {
		var (
			ctx, cancel = let.ContextWithCancel(s)
			data        = let.String(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Publish(ctx.Get(t), data.Get(t))
		})

		// By default a single subscriber is consuming the broadcast, so the happy
		// path can assert delivery without any additional arrangement.
		res := subscribe(s)
		s.Before(func(t *testcase.T) {
			res.Get(t) // start the default subscriber
			waitForSubscribers(t, 1)
		})

		s.Then("the subscribed consumer receives the published message", func(t *testcase.T) {
			assert.NoError(t, act(t))

			res.Get(t).Eventually(t, func(tb testing.TB, got []string) {
				assert.Contains(tb, got, data.Get(t))
			})
		})

		s.When("multiple subscribers are consuming", func(s *testcase.Spec) {
			const extraCount = 2
			var extra []testcase.Var[*pubsubtest.AsyncResults[string]]
			for i := 0; i < extraCount; i++ {
				extra = append(extra, subscribe(s))
			}

			// all returns every subscriber bound to the subject (default + extra).
			all := func(t *testcase.T) []*pubsubtest.AsyncResults[string] {
				rs := []*pubsubtest.AsyncResults[string]{res.Get(t)}
				for _, v := range extra {
					rs = append(rs, v.Get(t))
				}
				return rs
			}

			s.Before(func(t *testcase.T) {
				for _, v := range extra {
					v.Get(t) // start the additional subscribers
				}
				waitForSubscribers(t, 1+extraCount)
			})

			s.Then("every subscriber receives an identical copy of the message (broadcast, not unicast)", func(t *testcase.T) {
				assert.NoError(t, act(t))

				for i, r := range all(t) {
					r.Eventually(t, func(tb testing.TB, got []string) {
						assert.Contains(tb, got, data.Get(t),
							assert.MessageF("expected subscriber #%d to also receive the broadcast", i+1))
					})
				}
			})

			s.And("several distinct messages are published", func(s *testcase.Spec) {
				values := let.Var(s, func(t *testcase.T) []string {
					var vs []string
					t.Random.Repeat(3, 7, func() {
						vs = append(vs, t.Random.UUID())
					})
					return vs
				})

				s.Then("every subscriber receives all of them", func(t *testcase.T) {
					for _, v := range values.Get(t) {
						assert.Within(t, deadline, func(ctx context.Context) {
							assert.NoError(t, subject.Get(t).Publish(ctx, v))
						})
					}

					for i, r := range all(t) {
						r.Eventually(t, func(tb testing.TB, got []string) {
							assert.ContainsExactly(tb, values.Get(t), got,
								assert.MessageF("expected subscriber #%d to receive every broadcast message", i+1))
						})
					}
				})
			})
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				cancel.Get(t)()
			})

			s.Then("it returns the context's error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), ctx.Get(t).Err())
			})
		})

		// The no-subscriber scenario uses a fresh broadcast so the default
		// subscriber above is not involved.
		s.Test("with no subscriber consuming, publishing is a no-op and a later subscriber does not receive the message", func(t *testcase.T) {
			var bc synckit.Broadcast[string]
			value := t.Random.UUID()

			// a broadcast delivers only to currently-subscribed consumers, so with
			// none present the publish returns immediately without blocking.
			assert.Within(t, timeout, func(context.Context) {
				assert.NoError(t, bc.Publish(t.Context(), value))
			})

			// a subscriber that joins afterwards must not receive the earlier message.
			async := pubsubtest.Subscribe[string](t, &bc, t.Context())
			async.AssertEmpty(t)
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

		s.Test("a subscriber only receives messages published while it is subscribed", func(t *testcase.T) {
			early := pubsubtest.Subscribe[string](t, subject.Get(t), t.Context())
			waitForSubscribers(t, 1)

			before := t.Random.UUID()
			assert.Within(t, deadline, func(ctx context.Context) {
				assert.NoError(t, subject.Get(t).Publish(ctx, before))
			})
			early.Eventually(t, func(tb testing.TB, got []string) {
				assert.Contains(tb, got, before)
			})

			// a second subscriber joins only after `before` was already broadcast.
			late := pubsubtest.Subscribe[string](t, subject.Get(t), t.Context())
			waitForSubscribers(t, 2)

			after := t.Random.UUID()
			assert.Within(t, deadline, func(ctx context.Context) {
				assert.NoError(t, subject.Get(t).Publish(ctx, after))
			})

			early.Eventually(t, func(tb testing.TB, got []string) {
				assert.ContainsExactly(tb, []string{before, after}, got)
			})
			late.Eventually(t, func(tb testing.TB, got []string) {
				assert.Contains(tb, got, after)
			})
			assert.NotContains(t, late.Values(), before,
				"the late subscriber must not receive a message published before it subscribed")
		})

		s.Test("a delivered message that is not ACK-ed by the end of the for-round is redelivered, not lost", func(t *testcase.T) {
			var bc synckit.Broadcast[int]
			var g synckit.Group
			t.Cleanup(g.Cancel)

			var data = t.Random.Int()
			var n int64

			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range bc.Subscribe(ctx) {
					if err != nil {
						return err
					}
					assert.Equal(t, msg.Data(), data)
					atomic.AddInt64(&n, 1)
					continue // move on without ACK-ing
				}
				return nil
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bc.Len(), 1,
					"expected the subscriber to be ready to receive a publication")
			})

			assert.NoError(t, bc.Publish(t.Context(), data))

			// Not ACK-ing a delivered message must not consume it: the broadcast
			// re-queues it for the same subscriber, so it is delivered again.
			t.Eventually(func(t *testcase.T) {
				var got = atomic.LoadInt64(&n)
				assert.True(t, 1 < got,
					assert.MessageF("expected the un-ACK-ed message to be redelivered to the same subscriber, n=%d", got))
			})

			g.Cancel()
		})

		s.When("a delivered message gets NACK-ed by the consumer", func(s *testcase.Spec) {
			s.Test("it is redelivered to the same subscriber and, after ACK, is considered done", func(t *testcase.T) {
				var bc synckit.Broadcast[int]
				var nackErr error
				j := synckit.Go(t.Context(), func(ctx context.Context) error {
					for msg, err := range bc.Subscribe(ctx) {
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

				var expData = t.Random.Int()

				// wait for the subscriber to register before publishing, otherwise the
				// volatile broadcast would drop the message.
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, bc.Len(), 1)
				})

				assert.NoError(t, bc.Publish(t.Context(), expData))
				time.Sleep(timeout)
				assert.NoError(t, j.Wait())
				t.Log("nack should yield no error")
				assert.NoError(t, nackErr)

				t.Log("and upon resubscribing, it should be received again")
				assert.Within(t, deadline, func(ctx context.Context) {
					for msg, err := range bc.Subscribe(ctx) {
						assert.NoError(t, err)
						assert.Equal(t, msg.Data(), expData)
						assert.NoError(t, msg.ACK())
						break
					}
				})

				t.Log("but only once, and after ack, it should be considered done")
				assert.NotWithin(t, timeout, func(ctx context.Context) {
					for msg, err := range bc.Subscribe(ctx) { // should hang until context cancel
						assert.NoError(t, err)
						assert.NotNil(t, msg)
						break
					}
				})
			})
		})
	})

	s.Describe("#Close", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Close()
		})

		s.Then("it unblocks a publish that is waiting on a busy subscriber", func(t *testcase.T) {
			bc := subject.Get(t)

			var (
				processing = make(chan struct{}) // signals the first delivery began
				release    = make(chan struct{}) // keeps the subscriber busy, off the receive path
			)

			var g synckit.Group
			t.Cleanup(g.Cancel)
			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range bc.Subscribe(ctx) {
					if err != nil {
						return err
					}
					close(processing)
					<-release // stay busy so a subsequent publish blocks on the hand-off
					_ = msg.ACK()
					return nil
				}
				return nil
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bc.Len(), 1)
			})

			// the first publish is taken by the subscriber, which then becomes busy.
			assert.Within(t, deadline, func(ctx context.Context) {
				assert.NoError(t, bc.Publish(ctx, t.Random.UUID()))
			})
			<-processing

			// the second publish now blocks on the busy subscriber's hand-off.
			var gotErr error
			w := assert.NotWithin(t, timeout, func(context.Context) {
				gotErr = bc.Publish(t.Context(), t.Random.UUID())
			})

			// Close signals shutdown, so the blocked publish returns with an error.
			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NotPanic(t, func() {
					assert.NoError(t, act(t))
				})
			})

			assert.Within(t, deadline, func(ctx context.Context) {
				w.Wait()
			})
			assert.ErrorIs(t, gotErr, context.Canceled)

			close(release)
		})

		s.Then("it ends a subscription when nothing requires processing", func(t *testcase.T) {
			bc := subject.Get(t)

			var count int
			w := assert.NotWithin(t, timeout, func(context.Context) {
				for range bc.Subscribe(t.Context()) {
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

		s.Then("it will NOT end subscriptions while values still need to be processed", func(t *testcase.T) {
			bc := subject.Get(t)

			expected := random.Slice(t.Random.IntBetween(100, 300), t.Random.UUID)
			var actually []string
			var m sync.Mutex

			var g synckit.Group
			n := t.Random.IntBetween(3, 7)

			for range n {
				g.Go(t.Context(), func(ctx context.Context) error {
					for msg, err := range bc.Subscribe(t.Context()) {
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

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bc.Len(), n)
			})

			for _, data := range expected {
				assert.NoError(t, bc.Publish(t.Context(), data))
			}
			assert.NoError(t, act(t))
			assert.NoError(t, g.Wait())
		})

		s.Then("it will NOT cancel the context of an in-flight message that is still being processed", func(t *testcase.T) {
			bc := subject.Get(t)

			var (
				processing        = make(chan struct{}) // signals that a message delivery began
				release           = make(chan struct{}) // gate that keeps the message in-flight
				ctxErrDuringClose error
				ctxErrAfterClose  error
			)

			var g synckit.Group
			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range bc.Subscribe(ctx) {
					if err != nil {
						return err
					}
					close(processing)
					ctxErrDuringClose = msg.Context().Err()
					<-release
					ctxErrAfterClose = msg.Context().Err()
					assert.NoError(t, msg.ACK())
					return nil
				}
				return nil
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bc.Len(), 1)
			})

			assert.NoError(t, bc.Publish(t.Context(), t.Random.UUID()))
			<-processing

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})

			close(release)
			assert.NoError(t, g.Wait())

			assert.NoError(t, ctxErrDuringClose,
				"the in-flight message context must not be cancelled while Close is called")
			assert.NoError(t, ctxErrAfterClose,
				"the in-flight message context must remain valid after Close returned")
		})
	})

	// #Cancel behaves like a stronger #Close: besides signalling that no more
	// values are expected (ending idle subscriptions), it also cancels the
	// context of any message that is still in-flight, so consumers are asked to
	// abort their current processing.
	s.Describe("#Cancel", func(s *testcase.Spec) {
		act := let.Act0(func(t *testcase.T) {
			subject.Get(t).Cancel()
		})

		s.Then("it ends a subscription when nothing requires processing", func(t *testcase.T) {
			bc := subject.Get(t)

			var count int
			w := assert.NotWithin(t, timeout, func(context.Context) {
				for range bc.Subscribe(t.Context()) {
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
			bc := subject.Get(t)

			var (
				processing = make(chan struct{}) // signals that a message delivery began
				ctxErr     error
			)

			var g synckit.Group
			g.Go(t.Context(), func(ctx context.Context) error {
				for msg, err := range bc.Subscribe(ctx) {
					if err != nil {
						return err
					}
					close(processing)
					assert.Within(t, timeout, func(context.Context) {
						<-msg.Context().Done()
					})
					ctxErr = msg.Context().Err()
					return nil
				}
				return nil
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bc.Len(), 1)
			})

			assert.NoError(t, bc.Publish(t.Context(), t.Random.UUID()))
			<-processing

			assert.Within(t, timeout, func(ctx context.Context) {
				act(t)
			})

			assert.NoError(t, g.Wait())
			assert.ErrorIs(t, ctxErr, context.Canceled,
				"the in-flight message context must be cancelled by Cancel")
		})
	})

	s.Describe("#Len", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) int {
			return subject.Get(t).Len()
		})

		s.Then("it reports zero when there is no subscriber", func(t *testcase.T) {
			assert.Equal(t, act(t), 0)
		})

		s.When("subscribers are consuming", func(s *testcase.Spec) {
			count := let.Var(s, func(t *testcase.T) int {
				return t.Random.IntBetween(2, 5)
			})

			s.Before(func(t *testcase.T) {
				for i := 0; i < count.Get(t); i++ {
					pubsubtest.Subscribe[string](t, subject.Get(t), t.Context())
				}
			})

			s.Then("it reports the number of active subscribers", func(t *testcase.T) {
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, act(t), count.Get(t))
				})
			})
		})
	})

	s.Context("race", func(s *testcase.Spec) {
		s.Test("concurrent publish, subscribe and close", func(t *testcase.T) {
			var bc synckit.Broadcast[int]

			testcase.Race(func() {
				for bc.Len() != 2 {
					runtime.Gosched()
				}
				_ = bc.Publish(t.Context(), 42)
			}, func() {
				for msg, err := range bc.Subscribe(t.Context()) {
					assert.NoError(t, err)
					assert.NoError(t, msg.ACK())
				}
			}, func() {
				var o sync.Once
				for msg, err := range bc.Subscribe(t.Context()) {
					assert.NoError(t, err)
					var ok bool
					o.Do(func() {
						msg.NACK()
						ok = true
					})
					if ok {
						continue
					}
					assert.NoError(t, msg.ACK())
				}
			}, func() {
				<-time.After(timeout)
				_ = bc.Close()
			}, func() {
				<-time.After(timeout)
				bc.Cancel()
			})
		})
	})

	s.Test("nack w cancel should not result in any issue", func(t *testcase.T) {
		var b synckit.Broadcast[int]
		job := t.Go(func(ctx context.Context) error {
			for msg, err := range b.Subscribe(ctx) {
				assert.NoError(t, err)
				assert.Within(t, timeout, func(ctx context.Context) {
					assert.NoError(t, msg.NACK()) // nack so the message is requeued
				})
				return nil
			}
			return nil
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, b.Len(), 1)
		})

		assert.NoError(t, b.Publish(t.Context(), 42)) // send first message in the broadcast

		job.Wait()

		assert.Within(t, timeout, func(ctx context.Context) {
			b.Cancel() // cancel
		}, "cancel should not block")

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, b.Len(), 0)
		})
	})
}
