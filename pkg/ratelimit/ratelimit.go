package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/clock"
)

type Throttling interface {
	Throttle(context.Context) error
}

type SlidingWindow struct {
	Rate Rate

	m  sync.Mutex
	eh eventHistory
}

func (rl *SlidingWindow) Throttle(ctx context.Context) error {
check:
	if err := ctx.Err(); err != nil {
		return err
	}
	if rl.Rate.IsZero() {
		return nil
	}

	rl.m.Lock()
	e := rl.eh.RLE(rl.Rate)

	if isRateLimited := rl.Rate.N <= e.N; isRateLimited {
		rl.m.Unlock() // since we are rate limited, we let others have their luck for now.

		// we need to wait the amount of time that is between where the window starts,
		// and till where the first registered event is.
		// So as soon the first event is out
		// we might able to go through.
		waitTime := e.FirstAt.Sub(e.WindowStart)

		// Sleep until the next window starts.
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-clock.After(waitTime):
			goto check
		}
	} else {
		rl.eh.Tick(rl.Rate)
		rl.m.Unlock()
	}

	return nil
}

type Rate struct {
	// N represents the number of tokens to add or leak per the specified duration.
	N int
	// Per defines the duration over which N tokens are added or leaked (e.g., 1 second or 1 minute).
	Per time.Duration
}

func (r Rate) Pace() time.Duration {
	return r.Per / time.Duration(r.N)
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

type eventHistory struct {
	es []event
}

type rle struct {
	WindowStart time.Time
	CurrentTime time.Time
	FirstAt     time.Time

	N int
}

func (st rle) String() string {
	return fmt.Sprintf("%d/%s", st.N, st.CurrentTime.Sub(st.WindowStart).String())
}

func (es *eventHistory) RLE(r Rate) rle {
	currentTime := clock.Now()

	// the start of the window is always compared to the current time
	//
	// window start |-----------| now
	windowStart := currentTime.Add(r.Per * -1)

	// in case we have an event in the current window,
	// and a rate limit is already reached,
	// we need to know where was the first event
	// to know the time diff we need to wait
	var firstAt time.Time

	// drop old entities
	es.es = slicekit.Filter(es.es, func(e event) bool {
		if firstAt.IsZero() {
			firstAt = e.Timestamp
		}
		if e.Timestamp.Before(firstAt) {
			firstAt = e.Timestamp
		}
		return windowStart.Equal(e.Timestamp) || windowStart.Before(e.Timestamp)
	})

	if firstAt.IsZero() {
		firstAt = windowStart
	}

	return rle{
		WindowStart: windowStart,
		CurrentTime: currentTime,
		FirstAt:     firstAt,
		N:           len(es.es),
	}
}

func (es *eventHistory) Tick(r Rate) {
	if r.IsZero() {
		return
	}

	es.es = append(es.es, event{Timestamp: clock.Now()})
}
