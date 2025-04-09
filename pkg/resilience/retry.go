package resilience

import (
	"context"
	"math"
	"math/rand"
	"runtime"
	"time"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
)

type RetryPolicy[U FailureCount | StartedAt] interface {
	// ShouldTry will tell if retry should be attempted after a given number of failed attempts.
	ShouldTry(ctx context.Context, u U) bool
}

type (
	FailureCount = int
	StartedAt    = time.Time
)

var _ RetryPolicy[FailureCount] = ExponentialBackoff{}

// ExponentialBackoff is a RetryPolicy implementation.
//
// ExponentialBackoff will answer if retry can be made.
// It waits as well the amount of time based on the failure count.
// The waiting time before returning is doubled for each failed attempts
// This ensures that the system gets progressively more time to recover from any issues.
type ExponentialBackoff struct {
	// Delay is the time duration being waited.
	// Initially, it serves as the starting wait duration,
	// and then it increases based on the exponential backoff formula calculation.
	//
	// Default: 1/2 Second
	Delay time.Duration
	// Timeout is the time within the RetryPolicy is attempting further retries.
	// If the total waited time is greater than the Timeout, ExponentialBackoff will stop further attempts.
	// When Timeout is given, but MaxRetries is not, ExponentialBackoff will continue to retry until the calculated deadline is reached.
	//
	// Default: ignored
	Timeout time.Duration
	// Attempts is the amount of retry which is allowed before giving up the application.
	//
	// Default: 5 if Timeout is not set.
	Attempts int
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
	return zerokit.Coalesce(rs.Delay, fallback)
}

func (rs ExponentialBackoff) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.Attempts, defaultMaxRetries)
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

var _ RetryPolicy[FailureCount] = Jitter{}

// Jitter is a RetryPolicy implementation.
//
// Jitter is a random variation added to the backoff time.
// This helps to distribute the retry attempts evenly over time,
// reducing the risk of overwhelming the system and avoiding synchronization
// between multiple clients that might be retrying simultaneously.
type Jitter struct {
	// Delay is the maximum time duration that the Jitter is willing to wait between attempts.
	// There is no guarantee that it will wait the full duration.
	//
	// Default: 5 Second
	Delay time.Duration
	// Attempts is the amount of retry that is allowed before giving up the application.
	//
	// Default: 5
	Attempts int
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
	return zerokit.Coalesce(rs.Delay, fallback)
}

func (rs Jitter) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.Attempts, defaultMaxRetries)
}

var _ RetryPolicy[StartedAt] = Waiter{}

// Waiter is a RetryPolicy implementation.
//
// Waiter will check if a retry attempt should be made
// compared to when an operation was initially started.
type Waiter struct {
	// Timeout refers to the maximum duration we can wait
	// before a retry attempt is deemed unreasonable.
	//
	// Default: 30 seconds
	Timeout time.Duration
	// WaitDuration is the time how lone Waiter.Wait should wait between attempting a new retry during Waiter.While.
	//
	// Default: 1ms
	WaitDuration time.Duration
}

func (rs Waiter) getTimeout() time.Duration {
	const defaultTimeout = 30 * time.Second
	return zerokit.Coalesce(rs.Timeout, defaultTimeout)
}

func (rs Waiter) getWaitDuration() time.Duration {
	const defaultWaitDuration = time.Millisecond
	return zerokit.Coalesce(rs.WaitDuration, defaultWaitDuration)
}
func (rs Waiter) ShouldTry(ctx context.Context, startedAt StartedAt) bool {
	now := clock.Now()
	deadline := startedAt.Add(rs.getTimeout())
	return now.Before(deadline) && ctx.Err() == nil
}

// While implements the retry strategy looping part.
// Depending on the outcome of the condition,
// the RetryStrategy can decide whether further iterations can be done or not
func (rs Waiter) While(do func() (Continue bool)) {
	finishTime := clock.Now().Add(rs.getTimeout())
	for do() && clock.Now().Before(finishTime) {
		rs.wait()
	}
}

// Wait will attempt to wait a bit and leave breathing space for other goroutines to steal processing time.
// It will also attempt to schedule other goroutines.
func (rs Waiter) wait() {
	finishTime := clock.Now().Add(rs.getWaitDuration())
	for clock.Now().Before(finishTime) {
		wait(rs.getWaitDuration())
	}
}

func wait(maxWait time.Duration) {
	var (
		goroutNum = runtime.NumGoroutine()
		startedAt = clock.Now()
		WaitUnit  = maxWait / time.Duration(goroutNum)
	)
	if WaitUnit == 0 {
		WaitUnit = time.Nanosecond
	}
	for i := 0; i < goroutNum; i++ { // since goroutines don't have guarantee when they will be scheduled
		runtime.Gosched() // we explicitly mark that we are okay with other goroutines to be scheduled
		elapsed := clock.Now().Sub(startedAt)
		if maxWait <= elapsed { // if max wait time is reached
			return
		}
		if elapsed < maxWait { // if we withint the max wait time,
			var (
				ttw  = WaitUnit
				diff = maxWait - elapsed
			)
			if diff < ttw {
				ttw = diff
			}
			clock.Sleep(ttw) // then we could just yield CPU too with sleep
		}
	}
}

var _ RetryPolicy[FailureCount] = FixedDelay{}

// FixedDelay is a RetryPolicy implementation.
//
// FixedDelay will make retries with fixed delays between them.
// It is a lineral waiting time based retry policy.
type FixedDelay struct {
	// Delay is the time duration waited between attempts.
	//
	// Default: 1/2 Second
	Delay time.Duration
	// Timeout is the time within the RetryPolicy is attempting further retries.
	// If the total waited time is greater than the Timeout, ExponentialBackoff will stop further attempts.
	// When Timeout is given, but MaxRetries is not, ExponentialBackoff will continue to retry until a calculated deadline is reached.
	//
	// Default: ignored
	Timeout time.Duration
	// Attempts is the amount of retry attempt which is allowed before giving up the application.
	//
	// Default: 5 if Timeout is not set.
	Attempts int
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
	return zerokit.Coalesce(rs.Delay, fallback)
}

func (rs FixedDelay) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.Attempts, defaultMaxRetries)
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
