package retry

import (
	"context"
	"math"
	"math/rand"
	"time"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
)

type Strategy[U StrategyUnit] interface {
	// ShouldTry will tell if retry should be attempted after a given number of failed attempts.
	ShouldTry(ctx context.Context, u U) bool
}

type (
	StrategyUnit interface{ FailureCount | StartedAt }
	FailureCount = int
	StartedAt    = time.Time
)

// ExponentialBackoff will answer if retry can be made.
// It waits as well the amount of time based on the failure count.
// The waiting time before returning is doubled for each failed attempts
// This ensures that the system gets progressively more time to recover from any issues.
type ExponentialBackoff struct {
	// WaitTime is the time duration used to calculate exponential backoff wait times.
	// Initially, it serves as the starting wait duration, and then it evolves.
	//
	// Default: 1/2 Second
	WaitTime time.Duration
	// Timeout is the time within the Strategy is attempting further retries.
	// If the total waited time is greater than the Timeout, ExponentialBackoff will stop further attempts.
	// When Timeout is given, but MaxRetries is not, ExponentialBackoff will continue to retry until
	//
	// Default: ignored
	Timeout time.Duration
	// MaxRetries is the amount of retry which is allowed before giving up the application.
	//
	// Default: 5 if Timeout is not set.
	MaxRetries int
}

func (rs ExponentialBackoff) ShouldTry(ctx context.Context, failureCount FailureCount) bool {
	if rs.isDeadlineReached(ctx, failureCount) {
		return false
	}
	waitTime, ok := rs.waitTime(ctx, failureCount)
	if !ok {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(waitTime):
		return true
	}
}

func (rs ExponentialBackoff) waitTime(ctx context.Context, count FailureCount) (duration time.Duration, ok bool) {
	var (
		maxRetries = rs.getMaxRetries()
		waitTime   = rs.getWaitTime()
	)
	if maxRetries <= count && rs.Timeout == 0 {
		return 0, false
	}
	if ctx.Err() == nil && count == 0 {
		return 0, true
	}
	if ctx.Err() != nil {
		return 0, false
	}
	return rs.calcWaitTime(waitTime, count), true
}

func (rs ExponentialBackoff) calcWaitTime(waitTime time.Duration, count FailureCount) time.Duration {
	backoffMultiplier := math.Pow(2, float64(count))
	return time.Duration(backoffMultiplier) * waitTime
}

func (rs ExponentialBackoff) getWaitTime() time.Duration {
	const fallback = 500 * time.Millisecond
	return zerokit.Coalesce(rs.WaitTime, fallback)
}

func (rs ExponentialBackoff) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.MaxRetries, defaultMaxRetries)
}

func (rs ExponentialBackoff) isDeadlineReached(ctx context.Context, failureCount FailureCount) bool {
	if rs.Timeout == 0 {
		return false
	}
	var totalWaitedTime time.Duration
	for i := 0; i <= failureCount; i++ { // exclude current failure count
		waitTime, ok := rs.waitTime(ctx, i)
		if !ok {
			return false
		}
		totalWaitedTime += waitTime
	}
	return rs.Timeout <= totalWaitedTime
}

// Jitter is a random variation added to the backoff time. This helps to distribute the retry attempts evenly over time, reducing the risk of overwhelming the system and avoiding synchronization between multiple clients that might be retrying simultaneously.
type Jitter struct {
	// MaxRetries is the amount of retry that is allowed before giving up the application.
	//
	// Default: 5
	MaxRetries int
	// MaxWaitTime is the duration the Jitter will maximum wait between two retries.
	//
	// Default: 5 Second
	MaxWaitTime time.Duration
}

func (rs Jitter) ShouldTry(ctx context.Context, count FailureCount) bool {
	if rs.getMaxRetries() <= count {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	if count == 0 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(rs.waitTime()):
		return true
	}
}

var jitterRandom = rand.New(rand.NewSource(time.Now().Unix()))

func (rs Jitter) waitTime() time.Duration {
	return time.Duration(jitterRandom.Intn(int(rs.getMaxWaitTime()) + 1))
}

func (rs Jitter) getMaxWaitTime() time.Duration {
	const fallback = 5 * time.Second
	return zerokit.Coalesce(rs.MaxWaitTime, fallback)
}

func (rs Jitter) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.MaxRetries, defaultMaxRetries)
}

type Waiter struct {
	// Timeout refers to the maximum duration we can wait
	// before a retry attempt is deemed unreasonable.
	//
	// Default: 30 seconds
	Timeout time.Duration
}

func (rs Waiter) ShouldTry(ctx context.Context, startedAt StartedAt) bool {
	now := clock.Now()
	deadline := startedAt.Add(rs.timeout())
	return now.Before(deadline) && ctx.Err() == nil
}

func (rs Waiter) timeout() time.Duration {
	const defaultTimeout = 30 * time.Second
	return zerokit.Coalesce(rs.Timeout, defaultTimeout)
}

type FixedDelay struct {
	// WaitTime is the time duration used to calculate exponential backoff wait times.
	// Initially, it serves as the starting wait duration, and then it evolves.
	//
	// Default: 1/2 Second
	WaitTime time.Duration
	// Timeout is the time within the Strategy is attempting further retries.
	// If the total waited time is greater than the Timeout, ExponentialBackoff will stop further attempts.
	// When Timeout is given, but MaxRetries is not, ExponentialBackoff will continue to retry until
	//
	// Default: ignored
	Timeout time.Duration
	// MaxRetries is the amount of retry which is allowed before giving up the application.
	//
	// Default: 5 if Timeout is not set.
	MaxRetries int
}

func (rs FixedDelay) ShouldTry(ctx context.Context, failureCount FailureCount) bool {
	if rs.isDeadlineReached(ctx, failureCount) {
		return false
	}
	waitTime, ok := rs.waitTime(ctx, failureCount)
	if !ok {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(waitTime):
		return true
	}
}

func (rs FixedDelay) waitTime(ctx context.Context, count FailureCount) (duration time.Duration, ok bool) {
	var (
		maxRetries = rs.getMaxRetries()
		waitTime   = rs.getWaitTime()
	)
	if maxRetries <= count && rs.Timeout == 0 {
		return 0, false
	}
	if ctx.Err() == nil && count == 0 {
		return 0, true
	}
	if ctx.Err() != nil {
		return 0, false
	}
	return waitTime, true
}

func (rs FixedDelay) getWaitTime() time.Duration {
	const fallback = 500 * time.Millisecond
	return zerokit.Coalesce(rs.WaitTime, fallback)
}

func (rs FixedDelay) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.MaxRetries, defaultMaxRetries)
}

func (rs FixedDelay) isDeadlineReached(ctx context.Context, failureCount FailureCount) bool {
	if rs.Timeout == 0 {
		return false
	}
	var totalWaitedTime time.Duration
	for i := 0; i <= failureCount; i++ { // exclude current failure count
		waitTime, ok := rs.waitTime(ctx, i)
		if !ok {
			return false
		}
		totalWaitedTime += waitTime
	}
	return rs.Timeout <= totalWaitedTime
}
