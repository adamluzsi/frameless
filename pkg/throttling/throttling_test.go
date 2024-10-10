package throttling_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/throttling"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

var rate = testcase.Var[throttling.Rate]{
	ID: "throttling.Rate",
	Init: func(t *testcase.T) throttling.Rate {
		return throttling.Rate{
			N:   t.Random.IntBetween(50, 100),
			Per: t.Random.DurationBetween(time.Minute, 5*time.Minute),
		}
	},
}

func TestFixedWindow(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *throttling.FixedWindow {
		return &throttling.FixedWindow{
			Rate: rate.Get(t),
		}
	})

	var CTX = let.Context(s)

	act := func(t *testcase.T) error {
		return subject.Get(t).Throttle(CTX.Get(t))
	}

	s.Before(func(t *testcase.T) {
		timecop.Travel(t, time.Now(), timecop.Freeze) // time is not moving during the test
	})

	s.When(".Rate left as empty value", func(s *testcase.Spec) {
		rate.LetValue(s, throttling.Rate{})

		s.Then("calling won't hangs due to inferred default values", func(t *testcase.T) {
			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})
		})
	})

	s.Then("we can spike within the rate without throathling", func(t *testcase.T) {
		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			spikeAct(t, act)
		})
	})

	s.Then("we can make calls evenly during the time windows", func(t *testcase.T) {
		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			pacedAct(t, act)
		})
	})

	s.When("we already reached the rate limit are within the rate limit", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				pacedAct(t, act)
			}, "arrangement had an issue, we probably got rate limited too early")
		})

		s.Then("rate limiting applies", func(t *testcase.T) {
			w := assert.NotWithin(t, time.Second/2, func(ctx context.Context) {
				assert.NoError(t, act(t))
			}, "expected that rate limit prevented it from returning early")

			timecop.Travel(t, rate.Get(t).Per)

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				w.Wait()
			}, "expected that after the rate limit window went away, the throttling ended")
		})
	})
}

func pacedAct(t *testcase.T, act func(t *testcase.T) error) {
	pace := rate.Get(t).Per / time.Duration(rate.Get(t).N)
	for i := 0; i < rate.Get(t).N; i++ {
		assert.NoError(t, act(t))
		timecop.Travel(t, pace, timecop.Freeze)
	}
}

func spikeAct(t *testcase.T, act func(t *testcase.T) error) {
	timecop.SetSpeed(t, 0.1)
	defer timecop.SetSpeed(t, 1)
	for i := 0; i < rate.Get(t).N; i++ {
		assert.NoError(t, act(t))
	}
}
