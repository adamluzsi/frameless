package internal

import "time"

type Interval interface {
	UntilNext(lastRanAt time.Time) time.Duration
}
