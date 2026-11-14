package fileio

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

var (
	errTransient = errors.New("transient")
	errPermanent = errors.New("permanent")
)

func TestRetryWhile(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		budget = let.VarOf(s, 50*time.Millisecond)
		// strategy paces the retries quickly, so the budget runs out after a few attempts.
		strategy = let.Var(s, func(t *testcase.T) resilience.RetryStrategy {
			return resilience.FixedDelay{Delay: 5 * time.Millisecond, Timeout: budget.Get(t)}
		})
		// failures is the number of attempts which fail, before the operation succeeds.
		failures = let.Var(s, func(t *testcase.T) int { return t.Random.IntBetween(1, 3) })
		failWith = let.Var(s, func(t *testcase.T) error { return errTransient })
		attempts = let.VarOf(s, 0)
	)
	act := func(t *testcase.T) error {
		isTransient := func(err error) bool { return errors.Is(err, errTransient) }
		return retryWhile(isTransient, strategy.Get(t), func() error {
			attempts.Set(t, attempts.Get(t)+1)
			if attempts.Get(t) <= failures.Get(t) {
				return fmt.Errorf("attempt %d: %w", attempts.Get(t), failWith.Get(t))
			}
			return nil
		})
	}

	s.Then("a transient failure is retried until the operation succeeds", func(t *testcase.T) {
		assert.NoError(t, act(t))
		assert.Equal(t, failures.Get(t)+1, attempts.Get(t))
	})

	s.When("the operation keeps failing with a transient error", func(s *testcase.Spec) {
		failures.LetValue(s, math.MaxInt)

		s.Then("the error of the last attempt is returned, once the budget runs out", func(t *testcase.T) {
			start := time.Now()
			err := act(t)
			elapsed := time.Since(start)

			assert.ErrorIs(t, errTransient, err)
			assert.True(t, 1 < attempts.Get(t))
			assert.Equal(t, fmt.Sprintf("attempt %d: %v", attempts.Get(t), errTransient), err.Error())
			assert.True(t, elapsed < 2*budget.Get(t), assert.Message(elapsed.String()))
		})
	})

	s.When("the strategy doesn't allow a retry", func(s *testcase.Spec) {
		strategy.Let(s, func(t *testcase.T) resilience.RetryStrategy {
			return resilience.FixedDelay{Attempts: 1}
		})

		s.Then("the operation runs once, and its transient error is returned", func(t *testcase.T) {
			assert.ErrorIs(t, errTransient, act(t))
			assert.Equal(t, 1, attempts.Get(t))
		})
	})

	s.When("the operation fails with an error which is not transient", func(s *testcase.Spec) {
		failWith.Let(s, func(t *testcase.T) error { return errPermanent })

		s.Then("the error is returned without a retry", func(t *testcase.T) {
			assert.ErrorIs(t, errPermanent, act(t))
			assert.Equal(t, 1, attempts.Get(t))
		})
	})
}

func TestRetryStrategy(t *testing.T) {
	s := testcase.NewSpec(t)

	// act retries an operation which keeps failing with a transient error, and returns when each attempt started.
	act := func(t *testcase.T) []time.Time {
		var attempts []time.Time
		isTransient := func(err error) bool { return errors.Is(err, errTransient) }
		_ = retryWhile(isTransient, retryStrategy, func() error {
			attempts = append(attempts, time.Now())
			return errTransient
		})
		return attempts
	}

	s.Then("it retries within milliseconds, then backs off, and gives up after about a second", func(t *testcase.T) {
		attempts := act(t)
		assert.Must(t).True(3 <= len(attempts), assert.Message(fmt.Sprint(len(attempts))))

		var (
			firstWait = attempts[1].Sub(attempts[0])
			lastWait  = attempts[len(attempts)-1].Sub(attempts[len(attempts)-2])
			total     = attempts[len(attempts)-1].Sub(attempts[0])
		)
		t.OnFail(func() {
			t.Log("attempts:", len(attempts), "first wait:", firstWait, "last wait:", lastWait, "total:", total)
		})

		// a file held open by a virus scanner or a search indexer is usually released within milliseconds
		assert.True(t, firstWait < 50*time.Millisecond)
		// a steady pace would keep competing for the file with the process which holds it
		assert.True(t, 10*firstWait < lastWait)
		assert.True(t, len(attempts) <= 16)
		assert.True(t, time.Second/2 <= total && total < 3*time.Second)
	})
}
