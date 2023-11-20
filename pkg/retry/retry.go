package retry

import (
	"context"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
	"math"
	"math/rand"
	"time"
)

type Strategy interface {
	// ShouldTry will tell if retry should be attempted after a given number of failed attempts.
	ShouldTry(ctx context.Context, failureCount int) bool
}

// ExponentialBackoff will answer if retry can be made.
// It waits as well the amount of time based on the failure count.
// The waiting time before returning is doubled for each failed attempts
// This ensures that the system gets progressively more time to recover from any issues.
type ExponentialBackoff struct {
	// MaxRetries is the amount of retry which is allowed before giving up the application.
	//
	// Default: 5
	MaxRetries int
	// BackoffDuration is the time duration which will be used to calculate the exponential backoff wait time.
	//
	// Default: 1/2 time.Second
	BackoffDuration time.Duration
}

func (rs ExponentialBackoff) ShouldTry(ctx context.Context, failureCount int) bool {
	if rs.getMaxRetries() <= failureCount {
		return false
	}
	if ctx.Err() == nil && failureCount == 0 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(rs.backoffDurationFor(failureCount)):
		return true
	}
}

func (rs ExponentialBackoff) backoffDurationFor(failureCount int) time.Duration {
	backoffMultiplier := math.Pow(2, float64(failureCount-1))
	return time.Duration(backoffMultiplier) * rs.getBackoffDuration()
}

func (rs ExponentialBackoff) getBackoffDuration() time.Duration {
	const fallback = 500 * time.Millisecond
	return zerokit.Coalesce(rs.BackoffDuration, fallback)
}

func (rs ExponentialBackoff) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.MaxRetries, defaultMaxRetries)
}

// Jitter is a random variation added to the backoff time. This helps to distribute the retry attempts evenly over time, reducing the risk of overwhelming the system and avoiding synchronization between multiple clients that might be retrying simultaneously.
type Jitter struct {
	// MaxRetries is the amount of retry that is allowed before giving up the application.
	//
	// Default: 5
	MaxRetries int
	// MaxWaitDuration is the time duration that will be used to calculate the exponential backoff wait time.
	//
	// Default: 5 * time.Second
	MaxWaitDuration time.Duration
}

func (rs Jitter) ShouldTry(ctx context.Context, failureCount int) bool {
	if rs.getMaxRetries() <= failureCount {
		return false
	}
	if ctx.Err() == nil && failureCount == 0 {
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
	return time.Duration(jitterRandom.Intn(int(rs.getMaxWaitDuration()) + 1))
}

func (rs Jitter) getMaxWaitDuration() time.Duration {
	const fallback = 5 * time.Second
	return zerokit.Coalesce(rs.MaxWaitDuration, fallback)
}

func (rs Jitter) getMaxRetries() int {
	const defaultMaxRetries = 5
	return zerokit.Coalesce(rs.MaxRetries, defaultMaxRetries)
}
