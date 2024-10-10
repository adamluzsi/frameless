package throttling_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/throttling"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestFixedWindow(t *testing.T) {
	s := testcase.NewSpec(t)

	var rate = testcase.Let(s, func(t *testcase.T) throttling.Rate {
		return throttling.Rate{
			N: t.Random.IntBetween(50, 100),
			Per: ,
		}
	})

	subject := testcase.Let(s, func(t *testcase.T) *throttling.FixedWindow {
		return &throttling.FixedWindow{
			Rate: rate.Get(t),
		}
	})

	var CTX = let.Context(s)
	act := func(t *testcase.T) error {
		return subject.Get(t).Throttle(CTX.Get(t))
	}

	s.When(".Rate left as empty value", func(s *testcase.Spec) {
		rate.LetValue(s, throttling.Rate{})

		s.Then("calling won't hangs due to inferred default values", func(t *testcase.T) {
			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				act(t)
			})
		})
	})

}

func wbuffer(d time.Duration, m float64) time.Duration {
	return time.Duration(float64(d) * m)
}
