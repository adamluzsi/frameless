package logging

import (
	"time"

	"go.llib.dev/testcase/clock"
)

func Duration(key string, since time.Time, unit time.Duration) Detail {
	return LazyDetail(func() Detail {
		var (
			duration = clock.Now().Sub(since)
			value    = timeDurationToField(duration, unit)
		)
		return Field(key, value)
	})
}

func timeDurationToField(d time.Duration, unit time.Duration) any {
	switch unit {
	case 0:
		return d.String()
	case time.Hour:
		return d.Hours()
	case time.Minute:
		return d.Minutes()
	case time.Second:
		return d.Seconds()
	case time.Millisecond:
		return d.Milliseconds()
	case time.Microsecond:
		return d.Microseconds()
	case time.Nanosecond:
		return d.Nanoseconds()
	default:
		return d.String()
	}
}
