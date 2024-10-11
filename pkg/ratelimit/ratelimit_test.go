package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/ratelimit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

var rate = testcase.Var[ratelimit.Rate]{
	ID: "rate",
	Init: func(t *testcase.T) ratelimit.Rate {
		return ratelimit.Rate{
			N:   t.Random.IntBetween(5, 10),
			Per: t.Random.DurationBetween(time.Minute, 5*time.Minute),
		}
	},
}

const timeout = time.Second / 2

var _ ratelimit.Throttling = &ratelimit.SlidingWindow{}

func ExampleSlidingWindow() {
	_ = ratelimit.SlidingWindow{Rate: ratelimit.Rate{N: 100, Per: time.Minute}}
}
func TestSlidingWindow(t *testing.T) {
	s := testcase.NewSpec(t)

	rate.Bind(s)

	subject := testcase.Let(s, func(t *testcase.T) *ratelimit.SlidingWindow {
		return &ratelimit.SlidingWindow{
			Rate: rate.Get(t),
		}
	})

	var Context, ContextCancel = let.ContextWithCancel(s)

	act := func(t *testcase.T) error {
		return subject.Get(t).Throttle(Context.Get(t))
	}

	initialTime := testcase.Let(s, func(t *testcase.T) time.Time {
		return time.Now()
	})

	s.Before(func(t *testcase.T) {
		timecop.Travel(t, initialTime.Get(t), timecop.DeepFreeze) // time is not moving during the test
	})

	s.When("the context is cancelled", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			ContextCancel.Get(t)()
		})

		s.Then("throttling returns with context's error", func(t *testcase.T) {
			assert.ErrorIs(t, Context.Get(t).Err(), act(t))
		})
	})

	s.When(".Rate left as empty value", func(s *testcase.Spec) {
		rate.LetValue(s, ratelimit.Rate{})

		s.Then("calling won't hangs due to inferred default values", func(t *testcase.T) {
			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})
		})
	})

	s.Then("we can spike within the rate without throathling", func(t *testcase.T) {
		assert.Within(t, timeout, func(ctx context.Context) {
			spikeAct(t, act)
		})
	})

	s.Then("we can make calls evenly during the time window", func(t *testcase.T) {
		assert.Within(t, timeout, func(ctx context.Context) {
			pacedAct(t, act)
		})
	})

	s.When("we already reached the rate limit are within the rate limit", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.Within(t, timeout, func(ctx context.Context) {
				pacedAct(t, act)
			}, "arrangement had an issue, we probably got rate limited too early")
		})

		s.Then("rate limiting applies", func(t *testcase.T) {
			w := assert.NotWithin(t, timeout, func(ctx context.Context) {
				assert.NoError(t, act(t))
			}, "expected rate limiting from throttling")

			timecop.Travel(t, rate.Get(t).Per)

			assert.Within(t, timeout, func(ctx context.Context) {
				w.Wait()
			}, "expected that after the rate limit window went away, the throttling ended")
		})

		s.And("we wait till the sliding window has capacity again", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				timecop.Travel(t, rate.Get(t).Per)
			})

			s.Then("we can again make calls in a new window", func(t *testcase.T) {
				assert.Within(t, timeout, func(ctx context.Context) {
					pacedAct(t, act)
				})
			})
		})

		s.And("the context is cancelled", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ContextCancel.Get(t)()
			})

			s.Then("throttling returns with context's error", func(t *testcase.T) {
				assert.ErrorIs(t, Context.Get(t).Err(), act(t))
			}, testcase.Flaky(3))
		})
	})

	s.Test("race", func(t *testcase.T) {
		testcase.Race(
			func() { act(t) },
			func() { act(t) },
			func() { act(t) },
		)
	})
}

func pacedAct(t *testcase.T, act func(t *testcase.T) error) {
	t.Helper()
	pace := rate.Get(t).Per / time.Duration(rate.Get(t).N)
	for i := 0; i < rate.Get(t).N; i++ {
		assert.NoError(t, act(t))
		timecop.Travel(t, pace)
	}
}

func spikeAct(t *testcase.T, act func(t *testcase.T) error) {
	t.Helper()
	timecop.SetSpeed(t, 0.1)
	defer timecop.SetSpeed(t, 1)
	for i := 0; i < rate.Get(t).N; i++ {
		assert.NoError(t, act(t))
	}
}
