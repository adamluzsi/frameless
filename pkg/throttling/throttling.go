package throttling

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
)

type ThrottlingStrategy interface {
	Throttle(context.Context) error
}

// FixedWindow
// This strategy limits the number of requests allowed within a fixed time window.
// When a new window starts, the request count is reset, allowing up to `N` requests within that window (e.g., 10 requests per minute).
// However, this can lead to bursty behaviour, as many requests could arrive at the start of a new window.
// Once the limit is reached, further requests in that window are throttled until the next window begins.
type FixedWindow struct {
	Rate Rate

	events events
}

func (fw *FixedWindow) Throttle(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if fw.Rate.IsZero() {
		return nil
	}

	stats := fw.events.Tick(fw.Rate)
	isRateLimited := fw.Rate.N < stats.N

	// If the stats.N is greater than or equal to fw.Rate.N, it means the rate limit has been reached.
	if isRateLimited {
		// Calculate the time until the next window starts.
		nextWindowEnd := stats.FirstAt.Add(fw.Rate.Per)
		remainingTime := nextWindowEnd.Sub(stats.CurrentTime)

		// Sleep until the next window starts.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-clock.After(remainingTime):
		}
	}

	return nil
}

// SlidingWindow
// SlidingWindow improves upon FixedWindow by using a rolling or sliding time window.
// Instead of resetting at the start of each window, it tracks requests within a moving time frame (e.g., 10 requests in the last 60 seconds).
// This approach avoids bursty spikes and smooths out the request flow by dynamically counting requests within the current sliding window.
// Once the limit is exceeded, further requests are throttled until enough time has passed for requests to "fall out" of the window.
type SlidingWindow struct{}

// type TokenBucket struct {
// 	// Capacity defines the maximum number of tokens the bucket can hold. It acts as the upper limit on burstiness.
// 	// A request consumes one token from the bucket. If there are no tokens left, the request will be throttled.
// 	// This allows the system to handle bursts of requests up to the capacity, as long as tokens are available.
// 	Capacity int

// 	// Refill defines the rate at which tokens are replenished in the bucket.
// 	// Tokens are added to the bucket periodically, according to this rate (e.g., 5 tokens per second).
// 	// Even after handling a burst, the bucket will gradually refill based on this rate, allowing more requests to be handled over time.
// 	Refill Rate

// 	// Leak [optional] defines a rate at which tokens or requests might be leaked out of the bucket.
// 	// This allows for more fine-grained control, simulating the removal of tokens at a constant rate.
// 	// If enabled, this prevents token accumulation when the system is underutilized, ensuring that the bucket doesn't stay full.
// 	Leak Rate
// }

// Rate expresses a rate of N tokens per a given duration.
// This can be used both for refilling the TokenBucket and for determining the leak rate, if applicable.
type Rate struct {
	// N represents the number of tokens to add or leak per the specified duration.
	N int
	// Per defines the duration over which N tokens are added or leaked (e.g., 1 second or 1 minute).
	Per time.Duration
}

func (r Rate) String() string {
	return fmt.Sprintf("%d/%s", r.N, r.Per.String())
}

func (r Rate) IsZero() bool {
	return r.N == 0 && r.Per == 0
}

type event struct {
	Timestamp time.Time
}

type events struct {
	es []event
	m  sync.RWMutex
}

type stats struct {
	WindowStart time.Time
	CurrentTime time.Time
	FirstAt     time.Time
	N           int
}

func (st stats) String() string {
	const format = "15:04:05"
	return fmt.Sprintf("%d/%s [window start: %s | now: %s]", st.N, st.CurrentTime.Sub(st.WindowStart).String(),
		st.WindowStart.Format(format), st.CurrentTime.Format(format))
}

func (es *events) Tick(r Rate) stats {
	if r.IsZero() {
		return stats{}
	}

	es.m.Lock()
	defer es.m.Unlock()

	currentTime := clock.Now()
	windowStart := currentTime.Add(r.Per * -1)
	firstAt := currentTime

	// drop old entities
	es.es = slicekit.Filter(es.es, func(e event) bool {
		if e.Timestamp.Before(firstAt) {
			firstAt = e.Timestamp
		}
		return windowStart.Equal(e.Timestamp) || windowStart.Before(e.Timestamp)
	})

	// add current occasion
	es.es = append(es.es, event{Timestamp: currentTime})

	st := stats{
		WindowStart: windowStart,
		CurrentTime: currentTime,
		FirstAt:     firstAt,
		N:           len(es.es),
	}

	return st
}
