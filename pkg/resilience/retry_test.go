package resilience_test

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

var _ resilience.RetryPolicy[int] = &resilience.ExponentialBackoff{}

func ExampleExponentialBackoff() {
	ctx := context.Background()
	rs := resilience.ExponentialBackoff{}

	for i := 0; rs.ShouldTry(ctx, i); i++ {
		// do an action
		// return on success
	}
	// return failure
}

func TestExponentialBackoff_ShouldTry(t *testing.T) {
	s := testcase.NewSpec(t)

	const defaultMaxRetries = 5

	var (
		attempts = testcase.LetValue[int](s, 0)
		delay    = testcase.LetValue(s, time.Nanosecond)
		timeout  = testcase.LetValue[time.Duration](s, 0)
	)
	subject := testcase.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
		return &resilience.ExponentialBackoff{
			Attempts: attempts.Get(t),
			Delay:    delay.Get(t),
			Timeout:  timeout.Get(t),
		}
	})

	var (
		Context      = let.Context(s)
		failureCount = testcase.LetValue[int](s, 0)
	)
	act := func(t *testcase.T) bool {
		return subject.Get(t).ShouldTry(Context.Get(t), failureCount.Get(t))
	}

	s.Before(func(t *testcase.T) { timecop.SetSpeed(t, math.MaxFloat64) })

	s.Then("we can attempt to retry", func(t *testcase.T) {
		t.Must.True(act(t))
	})

	s.When(".Attempts field is configured", func(s *testcase.Spec) {
		attempts.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})

		s.Then("we are allowed to make the configured number of attempts before we need to give up", func(t *testcase.T) {
			var n int
			for fc := 0; subject.Get(t).ShouldTry(Context.Get(t), fc); fc++ {
				n++
			}

			assert.Equal(t, n, attempts.Get(t))
		})
	})

	s.When("context is cancelled", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(Context.Super(t))
			cancel()
			return ctx
		})

		s.Then("it will report that retry shouldn't be attempted", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
			v := subject.Super(t)
			v.Attempts = 7
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return subject.Get(t).Attempts
		})

		s.Then("we can't attempt to retry", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we didn't reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
			v := subject.Super(t)
			v.Attempts = 5
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, subject.Get(t).Attempts-1)
		})

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})
	})

	s.When("the was no failed attempt before", func(s *testcase.Spec) {
		failureCount.LetValue(s, 0)

		subject.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
			v := subject.Super(t)
			v.Delay = time.Hour
			return v
		})

		s.Then("it will instantly return", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)
				act(t)
			})
		})
	})

	s.Context("depending on the number of failed failed attempts we wait longer based on the exponential backoff times", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
			v := subject.Super(t)
			v.Delay = time.Millisecond
			v.Attempts = 10
			return v
		})

		type FaultAttemptCase struct {
			FailureCount int
			WaitTime     time.Duration
		}
		FaultUseCases := map[string]FaultAttemptCase{
			"when one failure is made so far": {
				FailureCount: 1,
				WaitTime:     time.Millisecond,
			},
			"when two failure is made so far": {
				FailureCount: 2,
				WaitTime:     2 * time.Millisecond,
			},
			"when three failure is made so far": {
				FailureCount: 3,
				WaitTime:     4 * time.Millisecond,
			},
			"when four failure is made so far": {
				FailureCount: 4,
				WaitTime:     8 * time.Millisecond,
			},
			"when five failure is made so far": {
				FailureCount: 5,
				WaitTime:     16 * time.Millisecond,
			},
		}
		testcase.TableTest(s, FaultUseCases, func(t *testcase.T, tc FaultAttemptCase) {
			failureCount.Set(t, tc.FailureCount)
			var buffer = time.Duration(float64(tc.WaitTime) * 0.30)
			assert.Eventually(t, 10, func(it testing.TB) {
				duration := measure(func() { act(t) })
				assert.True(it, duration <= tc.WaitTime+buffer,
					"expected duration", assert.Message(tc.WaitTime.String()),
					"got duration:", assert.Message(duration.String()),
					"buffer", assert.Message(buffer.String()))
			})
		})
	})

	s.Context("integration with testcase/clock package", func(s *testcase.Spec) {
		const multiplier = 200000
		s.Before(func(t *testcase.T) {
			timecop.SetSpeed(t, multiplier)
		})

		subject.Let(s, func(t *testcase.T) *resilience.ExponentialBackoff {
			v := subject.Super(t)
			v.Delay = time.Hour
			v.Attempts = 10
			return v
		})

		failureCount.LetValue(s, 6)

		s.Then("it will finish quickly", func(t *testcase.T) {
			var expected = 25 * time.Hour / multiplier // 450ms
			const buffer = 500 * time.Millisecond
			var duration time.Duration
			t.Must.Within(expected+buffer, func(ctx context.Context) {
				duration = measure(func() {
					act(t)
				})
			}, "expected duration:", assert.Message(expected.String()))
			t.Must.True(duration <= expected+buffer)
		}, testcase.Flaky(3))
	})

	s.Describe(".Timeout", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntBetween(int(time.Minute), int(time.Hour)))
		})

		s.When(".MaxRetries is absent, but .Timeout is supplied", func(s *testcase.Spec) {
			attempts.LetValue(s, 0) // zero value
			delay.LetValue(s, time.Millisecond)

			s.Then("retry will be attempted until timeout is reached", func(t *testcase.T) {
				var total int
				for i := 0; subject.Get(t).ShouldTry(Context.Get(t), i); i++ {
					total++
				}

				assert.True(t, defaultMaxRetries < total,
					"expected that the total retry attempt is greater than the fallback default max retries count",
					assert.MessageF("measured retry attempt count: %d", total))
			})
		})

		s.When("we are within the defined timeout duration", func(s *testcase.Spec) {
			failureCount.LetValue(s, 1)

			delay.Let(s, func(t *testcase.T) time.Duration {
				return timeout.Get(t) / 4
			})

			s.Then("we are allowed to proceed with the retry", func(t *testcase.T) {
				t.Must.True(act(t))
			})
		})

		s.When("we ran out of time compared to the timeout duration", func(s *testcase.Spec) {
			failureCount.Let(s, func(t *testcase.T) int {
				t.Log("given we failed once already")
				t.Log("then we susected that we have waited already one backoff duration worth of time")
				return 1
			})

			delay.Let(s, func(t *testcase.T) time.Duration {
				t.Log("given backoff duration took exactly as long as what timeout")
				return timeout.Get(t)
			})

			s.Then("we expect that we are over the timeout duration, and we are asked to not attempt further retries", func(t *testcase.T) {
				t.Must.False(act(t))
			})
		})
	})
}

var _ resilience.RetryPolicy[int] = &resilience.Jitter{}

func ExampleJitter() {
	ctx := context.Background()
	rs := resilience.Jitter{}

	for i := 0; rs.ShouldTry(ctx, i); i++ {
		// do an action
		// return on success
	}
	// return failure
}

func TestJitter_ShouldTry(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *resilience.Jitter {
		return &resilience.Jitter{Delay: time.Nanosecond}
	})

	var (
		Context      = let.Context(s)
		failureCount = testcase.LetValue[int](s, 0)
	)
	act := func(t *testcase.T) bool {
		return subject.Get(t).ShouldTry(Context.Get(t), failureCount.Get(t))
	}

	s.Then("we can attempt to retry", func(t *testcase.T) {
		t.Must.True(act(t))
	})

	s.When(".Attempts field is configured", func(s *testcase.Spec) {
		attempts := testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})

		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			super := subject.Super(t)
			super.Attempts = attempts.Get(t)
			return super
		})

		s.Then("we are allowed to make the configured number of attempts before we need to give up", func(t *testcase.T) {
			var n int
			for fc := 0; subject.Get(t).ShouldTry(Context.Get(t), fc); fc++ {
				n++
			}

			assert.Equal(t, n, attempts.Get(t))
		})
	})

	s.When("context is cancelled", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(Context.Super(t))
			cancel()
			return ctx
		})

		s.Then("it will report that retry shouldn't be attempted", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("the was no failed attempt before", func(s *testcase.Spec) {
		failureCount.LetValue(s, 0)

		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			v := subject.Super(t)
			v.Delay = time.Hour
			return v
		})

		s.Then("it will instantly return", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)
				act(t)
			})
		})
	})

	s.When("we reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			v := subject.Super(t)
			v.Attempts = 7
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return subject.Get(t).Attempts
		})

		s.Then("we can't attempt to retry", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we didn't reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			v := subject.Super(t)
			v.Attempts = 5
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, subject.Get(t).Attempts-1)
		})

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})
	})

	s.Context("on each retry attempt when failure already happened", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			v := subject.Super(t)
			v.Delay = 10 * time.Millisecond
			v.Attempts = 10
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(1, 8)
		})

		s.Test("we wait a bit, but less than the maximum wait time", func(t *testcase.T) {
			failureCount.Set(t, 1)
			var buffer = time.Duration(float64(subject.Get(t).Delay) * 0.30)
			assert.Eventually(t, 10, func(it testing.TB) {
				duration := measure(func() { act(t) })
				assert.True(it, duration <= subject.Get(t).Delay+buffer)
			})
		})
	})

	s.Context("integrate testcase/clock", func(s *testcase.Spec) {
		const multiplier = 200000

		s.Before(func(t *testcase.T) {
			timecop.SetSpeed(t, multiplier)
		})

		delay := let.DurationBetween(s, 30*time.Second, time.Minute)

		subject.Let(s, func(t *testcase.T) *resilience.Jitter {
			v := subject.Super(t)
			v.Delay = delay.Get(t)
			v.Attempts = 10
			return v
		})

		failureCount.LetValue(s, 5)

		s.Then("it will finish quickly", func(t *testcase.T) {
			t.Must.Within(time.Hour, func(ctx context.Context) {
				act(t)
			})
		})
	})
}

func ExampleWaiter_ShouldTry() {
	var (
		ctx = context.Background()
		rs  = resilience.Waiter{Timeout: time.Minute}
		now = time.Now()
	)

	for rs.ShouldTry(ctx, now) {
		// do an action
		// return on success
	}
	// return failure
}

func TestWaiter(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		timeout      = let.DurationBetween(s, time.Minute, time.Hour)
		waitDuration = let.VarOf[time.Duration](s, 0)

		subject = let.Var(s, func(t *testcase.T) resilience.Waiter {
			return resilience.Waiter{
				Timeout:      timeout.Get(t),
				WaitDuration: waitDuration.Get(t),
			}
		})
	)

	s.Test("implements testcase/assert.RetryStrategy", func(t *testcase.T) {
		var rs assert.Loop = subject.Get(t) // implements assert retry strategy
		var r = assert.Retry{Strategy: rs}

		var once int32
		r.Assert(t, func(t testing.TB) {
			if atomic.CompareAndSwapInt32(&once, 0, 1) {
				t.FailNow()
			}
			t.Log("pass")
		})
	})

	s.Describe("ShouldTry", func(s *testcase.Spec) {
		var (
			Context   = let.Context(s)
			startedAt = let.Var(s, func(t *testcase.T) resilience.StartedAt {
				return clock.Now()
			}).EagerLoading(s)
		)
		act := func(t *testcase.T) bool {
			return subject.Get(t).ShouldTry(Context.Get(t), startedAt.Get(t))
		}

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})

		s.When("context is cancelled", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(Context.Super(t))
				cancel()
				return ctx
			})

			s.Then("it will report that retry shouldn't be attempted", func(t *testcase.T) {
				t.Must.False(act(t))
			})
		})

		s.When("the last failure occured within the deadline", func(s *testcase.Spec) {
			startedAt.Let(s, func(t *testcase.T) resilience.StartedAt {
				return time.Now()
			})
			timeout.Let(s, func(t *testcase.T) time.Duration {
				return time.Hour
			})

			s.Then("it will instantly return", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					Context.Set(t, ctx)
					act(t)
				})
			})
		})

		s.When("we are over the deadline", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				timecop.Travel(t, time.Hour+time.Second)
			})

			s.Then("we are told to not avoid a new retry attempt", func(t *testcase.T) {
				t.Must.False(act(t))
			})
		})
	})

	s.Describe("WaitWhile", func(s *testcase.Spec) {
		var condition = let.Var[func() (cont bool)](s, nil)
		act := func(t *testcase.T) {
			subject.Get(t).While(condition.Get(t))
		}

		s.When("condition keeps remaining true", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) func() (cont bool) {
				return func() (cont bool) {
					return true // while true
				}
			})

			s.Then("the wait time will fully reached", func(t *testcase.T) {
				const wtime = time.Second / 4
				assert.True(t, wtime < timeout.Get(t))

				w := assert.NotWithin(t, wtime, func(ctx context.Context) {
					act(t)
				}, "expected that the timeout is not reached due to the `while true {}` condition")

				timecop.Travel(t, timeout.Get(t))

				assert.Within(t, wtime, func(ctx context.Context) {
					w.Wait()
				}, "expected that deadline reached already by this point")
			})
		})

		s.When("condition terminates the while loop", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) func() (cont bool) {
				return func() (cont bool) {
					return false // while false
				}
			})

			s.Then("the timeout is not reached", func(t *testcase.T) {
				const wtime = time.Second / 4
				assert.True(t, wtime < timeout.Get(t))

				assert.Within(t, wtime, func(ctx context.Context) {
					act(t)
				})
			})
		})

		s.When("while condition is true for once", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) func() (cont bool) {
				var once int32
				return func() (cont bool) {
					if atomic.CompareAndSwapInt32(&once, 0, 1) {
						return true // while true once
					}
					return false
				}
			})

			s.And("wait duration is not set", func(s *testcase.Spec) {
				waitDuration.LetValue(s, 0)

				s.Then("default wait duration is waited", func(t *testcase.T) {
					const defaultWaitDuration = time.Millisecond

					w := assert.NotWithin(t, defaultWaitDuration-(defaultWaitDuration/4), func(ctx context.Context) {
						act(t)
					})

					timecop.Travel(t, defaultWaitDuration)

					assert.Within(t, defaultWaitDuration+(defaultWaitDuration/10), func(ctx context.Context) {
						w.Wait()
					})
				})
			})

			s.And("wait duration is set", func(s *testcase.Spec) {
				waitDuration.Let(s, let.DurationBetween(s, time.Minute, time.Hour).Get)

				s.Then("default wait duration is waited", func(t *testcase.T) {
					var wtime = 25 * time.Millisecond
					assert.True(t, wtime < waitDuration.Get(t))

					w := assert.NotWithin(t, wtime-(wtime/4), func(ctx context.Context) {
						act(t)
					})

					timecop.Travel(t, waitDuration.Get(t))

					assert.Within(t, wtime, func(ctx context.Context) {
						w.Wait()
					})
				})
			})

		})

	})

	// s.Describe("#WaitWhile", func(s *testcase.Spec) {
	// 	var condition = let.Var[func() bool](s, nil)
	// 	act := let.Act0(func(t *testcase.T) {
	// 		subject.Get(t).WaitWhile(condition.Get(t))
	// 	})

	// 	s.When("condition is forever true", func(s *testcase.Spec) {
	// 		condition.Let(s, func(t *testcase.T) func() (cont bool) {
	// 			return func() (cont bool) { return true }
	// 		})
	// 	})

	// 	s.When("condition is always false", func(s *testcase.Spec) {
	// 		condition.Let(s, func(t *testcase.T) func() (cont bool) {
	// 			return func() (cont bool) { return true }
	// 		})
	// 	})

	// })
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func measure(blk func()) time.Duration {
	startTime := time.Now()
	blk()
	endTime := time.Now()
	return endTime.Sub(startTime)
}

var _ resilience.RetryPolicy[int] = &resilience.FixedDelay{}

func ExampleFixedDelay() {
	ctx := context.Background()
	rs := resilience.FixedDelay{
		Delay:   10 * time.Second,
		Timeout: 5 * time.Minute,
	}

	for i := 0; rs.ShouldTry(ctx, i); i++ {
		// do an action
		// return/break on success
	}
	// return failure
}

func TestFixedDelay_ShouldTry(t *testing.T) {
	s := testcase.NewSpec(t)
	const buffer = 500 * time.Millisecond

	var (
		delay    = testcase.LetValue[time.Duration](s, time.Second/2)
		attempts = testcase.LetValue[int](s, 5)
		timeout  = testcase.LetValue[time.Duration](s, 0)
	)
	subject := testcase.Let(s, func(t *testcase.T) *resilience.FixedDelay {
		return &resilience.FixedDelay{
			Delay:    delay.Get(t),
			Attempts: attempts.Get(t),
			Timeout:  timeout.Get(t),
		}
	})

	var (
		Context      = let.Context(s)
		failureCount = testcase.LetValue[int](s, 0)
	)
	act := func(t *testcase.T) bool {
		return subject.Get(t).ShouldTry(Context.Get(t), failureCount.Get(t))
	}

	s.Then("we can attempt to retry", func(t *testcase.T) {
		t.Must.True(act(t))
	})

	s.Then("the wait time is around to what is configured in .WaitTime", func(t *testcase.T) {
		failureCount.Set(t, t.Random.IntBetween(1, subject.Get(t).Attempts-1))
		waitTime := subject.Get(t).Delay

		assert.Within(t, waitTime+buffer, func(ctx context.Context) {
			act(t)
		})

		assert.NotWithin(t, waitTime-buffer, func(ctx context.Context) {
			act(t)
		}).Wait()
	})

	s.When(".Attempts field is configured", func(s *testcase.Spec) {
		attempts.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})

		s.Then("we are allowed to make the configured number of attempts before we need to give up", func(t *testcase.T) {
			var n int
			for fc := 0; subject.Get(t).ShouldTry(Context.Get(t), fc); fc++ {
				n++
			}

			assert.Equal(t, n, attempts.Get(t))
		})
	})

	s.When("context is cancelled", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(Context.Super(t))
			cancel()
			return ctx
		})

		s.Then("it will report that retry shouldn't be attempted", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we reached the allowed maximum number attempts", func(s *testcase.Spec) {
		attempts.LetValue(s, 7)

		failureCount.Let(s, func(t *testcase.T) int {
			return subject.Get(t).Attempts
		})

		s.Then("we can't attempt to retry", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we didn't reached the allowed maximum number attempts", func(s *testcase.Spec) {
		attempts.LetValue(s, 5)

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, subject.Get(t).Attempts-1)
		})

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})
	})

	s.When("the was no failed attempt before", func(s *testcase.Spec) {
		failureCount.LetValue(s, 0)

		subject.Let(s, func(t *testcase.T) *resilience.FixedDelay {
			v := subject.Super(t)
			v.Delay = time.Hour
			return v
		})

		s.Then("it will instantly return", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)
				act(t)
			})
		})
	})

	s.When("we wait a constant amount of time between each retry attempt", func(s *testcase.Spec) {
		delay.LetValue(s, 10*time.Millisecond)

		s.Test("we wait the same amount of time between each retry attempt", func(t *testcase.T) {
			for i := 0; i < 5; i++ {
				failureCount.Set(t, i)

				assert.Eventually(t, 10, func(it testing.TB) {
					duration := measure(func() { act(t) })

					assert.True(it, duration <= subject.Get(t).Delay+buffer)
				})
			}
		})
	})

	s.Context("integrate testcase/clock", func(s *testcase.Spec) {
		const multiplier = 200000

		s.Before(func(t *testcase.T) {
			timecop.SetSpeed(t, multiplier)
		})

		delay := let.DurationBetween(s, 30*time.Second, time.Minute)

		subject.Let(s, func(t *testcase.T) *resilience.FixedDelay {
			v := subject.Super(t)
			v.Delay = delay.Get(t)
			return v
		})

		failureCount.LetValue(s, 5)

		s.Then("it will finish quickly", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				act(t)
			})
		})
	})

	s.When(".Timeout is supplied", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntBetween(int(delay.Get(t)*2), int(time.Hour)))
		})

		s.Before(func(t *testcase.T) {
			timecop.SetSpeed(t, 100000)
		})

		s.And(".MaxRetries is absent", func(s *testcase.Spec) {
			attempts.LetValue(s, 0)

			s.And("the already waited time in total is less than the value of .Timeout", func(s *testcase.Spec) {
				failureCount.Let(s, func(t *testcase.T) int {
					max := int(timeout.Get(t) / delay.Get(t))
					return t.Random.IntBetween(0, max-1)
				})

				s.Then("retry will be attempted until timeout is reached", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})

			s.And("the already waited time in total is more than the value of .Timeout", func(s *testcase.Spec) {
				failureCount.Let(s, func(t *testcase.T) int {
					max := int(timeout.Get(t) / delay.Get(t))
					return t.Random.IntBetween(max+1, max*2)
				})

				s.Then("it is advised to abandone the retry attempt", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})

		s.Context("smoke", func(s *testcase.Spec) {

			s.And("we are within the defined timeout duration", func(s *testcase.Spec) {
				failureCount.LetValue(s, 1)

				subject.Let(s, func(t *testcase.T) *resilience.FixedDelay {
					v := subject.Super(t)
					v.Delay = timeout.Get(t) / 4
					return v
				})

				s.Then("we are allowed to proceed with the retry", func(t *testcase.T) {
					t.Must.True(act(t))
				})
			})

			s.And("we ran out of time compared to the timeout duration", func(s *testcase.Spec) {
				failureCount.LetValue(s, 2)

				subject.Let(s, func(t *testcase.T) *resilience.FixedDelay {
					v := subject.Super(t)
					v.Delay = timeout.Get(t)
					return v
				})

				s.Then("we expect that we are over the timeout duration, and we are asked to not attempt further retries", func(t *testcase.T) {
					t.Must.False(act(t))
				})
			})
		})
	})
}

func ExampleRetries() {
	var (
		ctx = context.Background()
		rp  = resilience.ExponentialBackoff{}
	)

	for range resilience.Retries(ctx, rp) {
		// on success, break out from retries
		break
	}
}

func ExampleRetries_withFailureCountBasedRetryPolicy() {
	var (
		ctx = context.Background()
		rp  = resilience.ExponentialBackoff{}
	)

	for range resilience.Retries(ctx, rp) {
		// on success, break out from retries
		break
	}
}

func ExampleRetries_withTimeDelayBasedRetryPolicy() {
	var (
		ctx = context.Background()
		rp  = resilience.Waiter{Timeout: time.Minute}
	)

	for range resilience.Retries(ctx, rp) {
		// on success, break out from retries
		break
	}
}

func ExampleRetries_withFailureCountRangeArgument() {
	var (
		ctx = context.Background()
		rp  = resilience.ExponentialBackoff{}
	)

	for failureCount := range resilience.Retries(ctx, rp) {
		_ = failureCount // starts from zero
		// on success, break out from retries
		break
	}
}

func TestRetries(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("failure count based retries", func(s *testcase.Spec) {
		s.Test("on success", func(t *testcase.T) {
			rs := resilience.FixedDelay{Delay: time.Nanosecond, Attempts: 3}

			var done bool

			assert.NotPanic(t, func() {
				for range resilience.Retries(t.Context(), rs) {
					done = true
					break
				}

			})

			assert.True(t, done)
		})

		s.Test("on failure", func(t *testcase.T) {
			var Attempts = t.Random.IntBetween(3, 7)
			var exp []int
			for i := range Attempts {
				exp = append(exp, i)
			}

			rs := resilience.FixedDelay{Delay: time.Nanosecond, Attempts: Attempts}

			var got []int
			for v := range resilience.Retries(t.Context(), rs) {
				got = append(got, v)
			}

			assert.Equal(t, exp, got)
		})
	})

	s.Context("start time based retries", func(s *testcase.Spec) {
		s.Test("on success", func(t *testcase.T) {
			rs := resilience.Waiter{
				Timeout:      time.Hour,
				WaitDuration: time.Nanosecond,
			}

			var done bool
			assert.NotPanic(t, func() {
				for range resilience.Retries(t.Context(), rs) {
					done = true
					break
				}

			})
			assert.True(t, done)
		})

		s.Test("on failure", func(t *testcase.T) {
			startAt := time.Date(2000, time.January, 1, 12, 30, 45, 0, time.UTC)
			timecop.Travel(t, startAt)

			rs := resilience.Waiter{
				Timeout:      time.Hour,
				WaitDuration: 100 * time.Millisecond,
			}

			w := assert.NotWithin(t, time.Second/2, func(ctx context.Context) {
				var expFailureCount resilience.FailureCount
				for v := range resilience.Retries(context.Background(), rs) {
					assert.Equal(t, v, expFailureCount)
					expFailureCount++
				}
			})

			timecop.Travel(t, time.Hour+time.Second)

			assert.Within(t, time.Second, func(ctx context.Context) {
				w.Wait()
			})
		})
	})

}
