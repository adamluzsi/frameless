package tasker

import (
	"time"

	"go.llib.dev/testcase/clock"
)

// Interval a scheduling interval component that can identify
type Interval interface {
	UntilNext(lastRanAt time.Time) time.Duration
}

// Every returns an Interval which scheduling frequency is the received time duration.
func Every(d time.Duration) Interval { return timeDuration(d) }

type timeDuration time.Duration

func (i timeDuration) UntilNext(lastRanAt time.Time) time.Duration {
	return lastRanAt.Add(time.Duration(i)).Sub(clock.TimeNow())
}

type Monthly struct {
	Day, Hour, Minute int
	Location          *time.Location
}

func (i Monthly) UntilNext(lastRanAt time.Time) time.Duration {
	loc := getLocation(i.Location)
	now := clock.TimeNow().In(loc)
	if lastRanAt.IsZero() {
		lastRanAt = now
	}
	lastRanAt = lastRanAt.In(loc)

	if lastRanAt.Year() < now.Year() {
		return 0
	}
	if lastRanAt.Month() < now.Month() {
		return 0
	}

	occurrenceAt := time.Date(now.Year(), now.Month(), i.Day,
		i.Hour, i.Minute, 0, 0, loc).
		AddDate(0, 1, 0)

	return occurrenceAt.Sub(lastRanAt)
}

type Daily struct {
	Hour, Minute int
	Location     *time.Location
}

func (i Daily) UntilNext(lastRanAt time.Time) time.Duration {
	loc := getLocation(i.Location)
	now := clock.TimeNow().In(loc)
	if lastRanAt.IsZero() {
		lastRanAt = now
	}
	lastRanAt = lastRanAt.In(loc)

	if lastRanAt.Year() < now.Year() {
		return 0
	}
	if lastRanAt.Month() < now.Month() {
		return 0
	}
	if lastRanAt.Day() < now.Day() {
		return 0
	}

	occurrenceAt := time.Date(now.Year(), now.Month(), now.Day(),
		i.Hour, i.Minute, 0, 0, loc).
		AddDate(0, 0, 1)

	return occurrenceAt.Sub(lastRanAt)
}

func getLocation(loc *time.Location) *time.Location {
	if loc == nil {
		return time.Local
	}
	return loc
}
