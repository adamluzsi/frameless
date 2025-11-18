package tasker

import (
	"encoding"
	"strings"
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
	return lastRanAt.Add(time.Duration(i)).Sub(clock.Now())
}

type Monthly struct {
	Day, Hour, Minute int
	Location          *time.Location
}

func (i Monthly) UntilNext(lastRanAt time.Time) time.Duration {
	loc := getLocation(i.Location)
	now := clock.Now().In(loc)
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
	now := clock.Now().In(loc)
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

var _ encoding.TextUnmarshaler = (*Daily)(nil)

func (i *Daily) UnmarshalText(text []byte) error {
	var (
		raw = string(text)
		t   time.Time
		err error
		loc *time.Location
	)
	if strings.ContainsAny(raw, "+Z") {
		const layoutWTZ = "15:04Z07:00"
		t, err = time.Parse(layoutWTZ, string(text))
		loc = t.Location()
	} else {
		const layout = "15:04"
		t, err = time.Parse(layout, string(text))
		loc = time.Local
	}
	if err != nil {
		return err
	}
	i.Hour = t.Hour()
	i.Minute = t.Minute()
	i.Location = loc
	return nil
}

var _ encoding.TextMarshaler = (*Daily)(nil)

func (i Daily) MarshalText() (text []byte, err error) {
	if i.Location != nil {
		const layoutWTZ = "15:04Z07:00"
		d := time.Date(0, 0, 0, i.Hour, i.Minute, 0, 0, i.Location)
		return []byte(d.Format(layoutWTZ)), nil
	}
	const layoutLocal = "15:04"
	d := time.Date(0, 0, 0, i.Hour, i.Minute, 0, 0, time.Local)
	return []byte(d.Format(layoutLocal)), nil
}
