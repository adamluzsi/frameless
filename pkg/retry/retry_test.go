package retry_test

import (
	"context"
	"math"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/retry"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

var _ retry.Strategy[int] = &retry.ExponentialBackoff{}

func ExampleExponentialBackoff() {
	ctx := context.Background()
	rs := retry.ExponentialBackoff{}

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
		maxRetries      = testcase.LetValue[int](s, 0)
		backoffDuration = testcase.LetValue(s, time.Nanosecond)
		timeout         = testcase.LetValue[time.Duration](s, 0)
	)
	subject := testcase.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
		return &retry.ExponentialBackoff{
			MaxRetries: maxRetries.Get(t),
			WaitTime:   backoffDuration.Get(t),
			Timeout:    timeout.Get(t),
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
		subject.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
			v := subject.Super(t)
			v.MaxRetries = 7
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return subject.Get(t).MaxRetries
		})

		s.Then("we can't attempt to retry", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we didn't reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
			v := subject.Super(t)
			v.MaxRetries = 5
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, subject.Get(t).MaxRetries-1)
		})

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})
	})

	s.When("the was no failed attempt before", func(s *testcase.Spec) {
		failureCount.LetValue(s, 0)

		subject.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
			v := subject.Super(t)
			v.WaitTime = time.Hour
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
		subject.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
			v := subject.Super(t)
			v.WaitTime = time.Millisecond
			v.MaxRetries = 10
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
			assert.Eventually(t, 10, func(it assert.It) {
				duration := measure(func() { act(t) })
				it.Must.True(duration <= tc.WaitTime+buffer,
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

		subject.Let(s, func(t *testcase.T) *retry.ExponentialBackoff {
			v := subject.Super(t)
			v.WaitTime = time.Hour
			v.MaxRetries = 10
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
			maxRetries.LetValue(s, 0) // zero value
			backoffDuration.LetValue(s, time.Millisecond)

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

			backoffDuration.Let(s, func(t *testcase.T) time.Duration {
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

			backoffDuration.Let(s, func(t *testcase.T) time.Duration {
				t.Log("given backoff duration took exactly as long as what timeout")
				return timeout.Get(t)
			})

			s.Then("we expect that we are over the timeout duration, and we are asked to not attempt further retries", func(t *testcase.T) {
				t.Must.False(act(t))
			})
		})
	})
}

var _ retry.Strategy[int] = &retry.Jitter{}

func ExampleJitter() {
	ctx := context.Background()
	rs := retry.Jitter{}

	for i := 0; rs.ShouldTry(ctx, i); i++ {
		// do an action
		// return on success
	}
	// return failure
}

func TestJitter_ShouldTry(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *retry.Jitter {
		return &retry.Jitter{MaxWaitTime: time.Nanosecond}
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

		subject.Let(s, func(t *testcase.T) *retry.Jitter {
			v := subject.Super(t)
			v.MaxWaitTime = time.Hour
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
		subject.Let(s, func(t *testcase.T) *retry.Jitter {
			v := subject.Super(t)
			v.MaxRetries = 7
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return subject.Get(t).MaxRetries
		})

		s.Then("we can't attempt to retry", func(t *testcase.T) {
			t.Must.False(act(t))
		})
	})

	s.When("we didn't reached the allowed maximum number attempts", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *retry.Jitter {
			v := subject.Super(t)
			v.MaxRetries = 5
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, subject.Get(t).MaxRetries-1)
		})

		s.Then("we can attempt to retry", func(t *testcase.T) {
			t.Must.True(act(t))
		})
	})

	s.Context("on each retry attempt when failure already happened", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *retry.Jitter {
			v := subject.Super(t)
			v.MaxWaitTime = 10 * time.Millisecond
			v.MaxRetries = 10
			return v
		})

		failureCount.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(1, 8)
		})

		s.Test("we wait a bit, but less than the maximum wait time", func(t *testcase.T) {
			failureCount.Set(t, 1)
			var buffer = time.Duration(float64(subject.Get(t).MaxWaitTime) * 0.30)
			assert.Eventually(t, 10, func(it assert.It) {
				duration := measure(func() { act(t) })
				it.Must.True(duration <= subject.Get(t).MaxWaitTime+buffer)
			})
		})
	})

	s.Context("integration with testcase/clock package", func(s *testcase.Spec) {
		const multiplier = 200000
		s.Before(func(t *testcase.T) {
			timecop.SetSpeed(t, multiplier)
		})

		subject.Let(s, func(t *testcase.T) *retry.Jitter {
			v := subject.Super(t)
			v.MaxWaitTime = time.Hour
			v.MaxRetries = 10
			return v
		})

		failureCount.LetValue(s, 5)

		s.Then("it will finish quickly", func(t *testcase.T) {
			const buffer = 500 * time.Millisecond

			var duration time.Duration
			t.Must.Within(subject.Get(t).MaxWaitTime, func(ctx context.Context) {
				duration = measure(func() { act(t) })
			})

			t.Must.True(duration <= (subject.Get(t).MaxWaitTime/multiplier)+buffer)
		})
	})
}

func ExampleWaiter_ShouldTry() {
	var (
		ctx = context.Background()
		rs  = retry.Waiter{Timeout: time.Minute}
		now = time.Now()
	)

	for rs.ShouldTry(ctx, now) {
		// do an action
		// return on success
	}
	// return failure
}

func TestWaiter_ShouldTry(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		timeout = testcase.Let[time.Duration](s, func(t *testcase.T) time.Duration {
			return time.Hour
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) retry.Waiter {
		return retry.Waiter{Timeout: timeout.Get(t)}
	})

	var (
		Context   = let.Context(s)
		startedAt = testcase.Let[retry.StartedAt](s, func(t *testcase.T) retry.StartedAt {
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
		startedAt.Let(s, func(t *testcase.T) retry.StartedAt {
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
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func measure(blk func()) time.Duration {
	startTime := time.Now()
	blk()
	endTime := time.Now()
	return endTime.Sub(startTime)
}
