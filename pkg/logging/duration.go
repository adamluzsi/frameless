package logging

import (
	"context"
	"time"

	"go.llib.dev/testcase/clock"
)

func Duration() Detail {
	return duration{Start: clock.Now()}
}

type duration struct {
	Start time.Time
}

func (d duration) addTo(ctx context.Context, l *Logger, r entry) {
	finish := clock.Now()
	duration := finish.Sub(d.Start)
	field{Key: "duration", Value: duration.String()}.addTo(ctx, l, r)
	field{Key: "duration_ms", Value: duration.Milliseconds()}.addTo(ctx, l, r)
}
