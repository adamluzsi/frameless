package chankit_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/chankit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/option"

	// "go.llib.dev/frameless/pkg/synckit"
	// "go.llib.dev/frameless/port/option"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/random"

	// "go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	// "go.llib.dev/testcase/random"
)

const waitTimeout = time.Second / 8

func TestBatch(tt *testing.T) {
	const defaultBatchSize = 64

	s := testcase.NewSpec(tt)

	var (
		src = let.Var(s, func(t *testcase.T) chan string {
			ch := make(chan string)
			t.Defer(func() {
				select {
				case _, ok := <-ch:
					if ok {
						close(ch)
					}
				default:
					close(ch)
				}
			})
			return ch
		})
		opts = let.VarOf[[]chankit.BatchOption](s, nil)
	)

	act := func(t *testcase.T) chan []string {
		return chankit.Batch(src.Get(t), opts.Get(t)...)
	}

	s.Then("output channel is non-nil", func(t *testcase.T) {
		assert.NotNil(t, act(t))
	})

	s.Then("the output channel closes when the input channel is closed", func(t *testcase.T) {
		out := act(t)
		close(src.Get(t))

		assert.Within(t, waitTimeout, func(ctx context.Context) {
			select {
			case _, ok := <-out:
				assert.False(t, ok, "expected that the output channel got closed too")
			case <-ctx.Done():
				t.Error("expected that the output channel sent a close signal by now")
			}
		})
	})

	var ThenOnValuesItWillIterate = func(s *testcase.Spec, in testcase.Var[chan string], subs ...func(s *testcase.Spec, values testcase.Var[[]string])) {
		s.Context("given values are sent through the source channel", func(s *testcase.Spec) {
			values := let.Var(s, func(t *testcase.T) []string {
				count := t.Random.IntBetween(50, 100)
				return random.Slice(count, t.Random.UUID)
			}).EagerLoading(s)

			s.Before(func(t *testcase.T) {
				ch := in.Get(t)
				vs := values.Get(t)
				go func() {
					defer func() { _ = recover() }()
					defer close(ch)
				produce:
					for _, v := range vs {
						select {
						case <-t.Done():
							break produce
						case ch <- v:
						}
					}
				}()
			})

			s.Then("the batches contain all elements", func(t *testcase.T) {
				var got []string
				for vs := range act(t) {
					assert.Must(t).NotEmpty(vs)
					got = append(got, vs...)
				}
				assert.Must(t).Equal(values.Get(t), got, "expected both to contain all elements and also that the order is not affected")
			})

			s.Then("output channel is closed when input is done", func(t *testcase.T) {
				ch := act(t)

				assert.Within(t, waitTimeout, func(ctx context.Context) {
					// range exit only when channel is closed
					for range ch {
					}
					// Channel must be closed — this shouldn't block
					_, ok := <-ch
					assert.False(t, ok, "output channel should be closed")
				})
			})

			for _, sub := range subs {
				sub(s, values)
			}
		})
	}

	ThenOnValuesItWillIterate(s, src)

	var WhenBatchSizeDefined = func(s *testcase.Spec, in testcase.Var[chan string]) {
		var batchSizeCasesWith = func(s *testcase.Spec, expectedSize testcase.Var[int]) {
			ThenOnValuesItWillIterate(s, in, func(s *testcase.Spec, values testcase.Var[[]string]) {
				s.Context("given the number of value sent on the channel exceeds the expected batch size", func(s *testcase.Spec) {
					values.Let(s, func(t *testcase.T) []string {
						batchCount := t.Random.IntBetween(3, 7)
						total := expectedSize.Get(t) * batchCount
						return random.Slice(total, t.Random.UUID)
					})

					s.Then("it will batch values with the default size", func(t *testcase.T) {
						vs := chankit.Collect(act(t))

						for _, batch := range vs {
							assert.Assert(t, len(batch) <= expectedSize.Get(t))
						}

						assert.OneOf(t, vs, func(tb testing.TB, batch []string) {
							assert.Equal(tb, len(batch), expectedSize.Get(t))
						}, "at least one should be with the expected batch size since the input values count exceed the batch size")
					})
				})
			})
		}

		s.When("size is not configured", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c := option.ToConfig(opts.Get(t))
				assert.Empty(t, c.Size)
			})

			batchSizeCasesWith(s, let.VarOf(s, defaultBatchSize))
		})

		s.When("size is configured", func(s *testcase.Spec) {
			batchSize := let.IntB(s, 5, 10)

			opts.Let(s, func(t *testcase.T) []chankit.BatchOption {
				t.Logf("given the batch size option is configured (%d)", batchSize.Get(t))
				return []chankit.BatchOption{chankit.BatchConfig{Size: batchSize.Get(t)}}
			})

			batchSizeCasesWith(s, batchSize)

			s.Context("but it is zero or below, then it will fallback to default batch size", func(s *testcase.Spec) {
				batchSize.Let(s, func(t *testcase.T) int {
					return t.Random.IntBetween(-100, 0)
				})

				batchSizeCasesWith(s, let.VarOf(s, defaultBatchSize))
			})
		})
	}

	WhenBatchSizeDefined(s, src)

	s.When("wait limit is set", func(s *testcase.Spec) {
		timeout := let.DurationBetween(s, time.Minute, time.Hour)

		opts.Let(s, func(t *testcase.T) []chankit.BatchOption {
			o := opts.Super(t)
			o = append(o, chankit.BatchConfig{WaitLimit: timeout.Get(t)})
			return o
		})

		ThenOnValuesItWillIterate(s, src)
		WhenBatchSizeDefined(s, src)

		s.And("it is less or equal to zero", func(s *testcase.Spec) {
			timeout.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(t.Random.IntB(-1*int(time.Minute), 0))
			})

			ThenOnValuesItWillIterate(s, src)
			WhenBatchSizeDefined(s, src)

			s.Then("timeout configuration is ignored", func(t *testcase.T) {
				in := src.Get(t)
				out := act(t)

				exp := t.Random.UUID()
				go func() {
					select {
					case in <- exp:
					case <-t.Done():
						return
					}
				}()

				w := assert.NotWithin(t, waitTimeout, func(ctx context.Context) {
					vs, ok := <-out
					assert.True(t, ok)
					assert.NotEmpty(t, vs)
					assert.Contains(t, vs, exp)
				})

				close(src.Get(t))

				assert.Within(t, waitTimeout, func(ctx context.Context) {
					w.Wait()
				})
			})
		})

		s.When("the source channel is slower than the batch wait time", func(s *testcase.Spec) {
			in := let.Var(s, func(t *testcase.T) chan string {
				return make(chan string)
			})

			src.Let(s, func(t *testcase.T) chan string {
				to := timeout.Get(t)
				in := in.Get(t)
				out := make(chan string)
				go func() {
					defer close(out)
					for {
						select {
						case v, ok := <-in:
							if !ok {
								return
							}
							select {
							case <-clock.After(to):
								out <- v
							case <-t.Done():
								return
							}
						case <-t.Done():
							return
						}
					}
				}()
				return out
			})

			s.Then("on no events, we don't get empty batches", func(t *testcase.T) {
				timecop.SetSpeed(t, timecop.BlazingFast)

				ch := act(t)

				var total synckit.Slice[string]
				w := assert.NotWithin(t, waitTimeout, func(ctx context.Context) {
					vs, ok := <-ch
					if !ok {
						return
					}
					assert.NotEmpty(t, vs)
					total.Append(vs...)
				})

				t.Log("then we reach the timeout")
				timecop.Travel(t, timeout.Get(t)+time.Second)

				w = assert.NotWithin(t, waitTimeout, func(ctx context.Context) {
					w.Wait()
				}, "we still didn't got anything since nothing was sent")

				assert.Equal(t, 0, total.Len())
			})

			s.Then("batch timeout flushes partial batches", func(t *testcase.T) {
				ch := act(t)

				var total synckit.Slice[string]
				w := assert.NotWithin(t, waitTimeout, func(ctx context.Context) {
					vs, ok := <-ch
					if ok {
						assert.NotEmpty(t, vs)
						total.Append(vs...)
					}
				})

				// something below the default batch size
				expValues := random.Slice(5, t.Random.UUID)

				t.Log("given values sent through the src channel")
				assert.Within(t, waitTimeout, func(ctx context.Context) {
				produce:
					for _, v := range expValues {
						select {
						case src.Get(t) <- v:
						case <-ctx.Done():
							break produce
						case <-t.Done():
							break produce
						}
					}
				}, "expected that batcher took out the values as soon it could")

				w = assert.NotWithin(t, waitTimeout, func(ctx context.Context) {
					w.Wait()
				}, "expected that we still didn't got any values back")
				assert.Equal(t, 0, total.Len())

				t.Log("and we reach the configured timeout deadline")
				timecop.Travel(t, timeout.Get(t)+time.Second)

				t.Log("then we expect that the output channel pushes out a partial batch")
				assert.Within(t, waitTimeout, func(ctx context.Context) {
					w.Wait()
				})

				assert.Equal(t, len(expValues), total.Len())
				assert.Equal(t, expValues, total.Slice())
			})
		})
	})
}
