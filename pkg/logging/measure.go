package logging

import (
	"context"
	"time"

	"go.llib.dev/testcase/clock"
)

func Measure() func() Detail {
	var start = clock.Now()
	return func() Detail {
		return measurement{
			Start:  start,
			Finish: clock.Now(),
		}
	}
}

// type optionMeasure int
//
// const (
// 	MeasureMillisecond optionMeasure = iota
// 	MeasureNanoseconds
// )

type measurement struct {
	Start  time.Time
	Finish time.Time
}

func (m measurement) addTo(ctx context.Context, l *Logger, r entry) {
	finish := clock.Now()
	duration := finish.Sub(m.Start)
	field{Key: "duration", Value: duration.String()}.addTo(ctx, l, r)
	field{Key: "duration_ms", Value: duration.Milliseconds()}.addTo(ctx, l, r)
	field{Key: "duration_ms", Value: duration.Nanoseconds()}.addTo(ctx, l, r)
}
