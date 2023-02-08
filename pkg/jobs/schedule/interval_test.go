package schedule_test

import (
	"github.com/adamluzsi/frameless/pkg/jobs/schedule"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"testing"
	"time"
)

func TestInterval_smoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())
	duration := time.Duration(random.New(random.CryptoSeed{}).IntB(int(time.Second), int(time.Hour)))
	interval := schedule.Interval(duration)

	assert.Equal(t, 0, interval.UntilNext(now.Add(-1*duration)),
		"when the next interval compared to the previous time is right now")

	assert.Equal(t, time.Second, interval.UntilNext(now.Add(-1*duration).Add(time.Second)),
		"when the next interval is in the future",
		"then remaining time until the next occurrence is returned")

	assert.Equal(t, -1*time.Second, interval.UntilNext(now.Add(-1*duration).Add(-1*time.Second)),
		"when the next interval is in the past",
		"then time elapsed since the last occurrence is returned")
}

func TestMonthly_smoke(t *testing.T) {
	var (
		now  = time.Now().UTC()
		rnd  = random.New(random.CryptoSeed{})
		day  = rnd.IntB(1, 25)
		hour = rnd.IntB(0, 23)
		min  = rnd.IntB(0, 59)
	)
	timecop.Travel(t, now, timecop.Freeze())
	interval := schedule.Monthly{
		Day:      day,
		Hour:     hour,
		Minute:   min,
		Location: time.UTC,
	}

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(0, -1, 0)),
		"when the next occurrence in that moment")

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(0, -2, 0)),
		"when we skipped the past month's occurrence")

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(-1, 0, 0)),
		"when we skipped all the occurrence in the past year")

	expUntilNext := time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, time.UTC).
		AddDate(0, 1, 0).
		Sub(now)

	assert.Equal(t, expUntilNext, interval.UntilNext(now),
		"when the next interval is in the future",
		"then remaining time until the next occurrence is returned")
}

func TestDaily_smoke(t *testing.T) {
	var (
		now  = time.Now().UTC()
		rnd  = random.New(random.CryptoSeed{})
		hour = rnd.IntB(0, 23)
		min  = rnd.IntB(0, 59)
	)
	timecop.Travel(t, now, timecop.Freeze())
	interval := schedule.Daily{
		Hour:     hour,
		Minute:   min,
		Location: time.UTC,
	}

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(0, 0, -1)),
		"when the next occurrence in that moment")

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(0, 0, -2)),
		"when we skipped the past day's occurrence")

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(0, -1, 0)),
		"when we skipped all the occurrence in the past month")

	assert.Equal(t, 0, interval.UntilNext(now.AddDate(-1, 0, 0)),
		"when we skipped all the occurrence in the past year")

	expUntilNext := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC).
		AddDate(0, 0, 1).
		Sub(now)

	assert.Equal(t, expUntilNext, interval.UntilNext(now),
		"when the next interval is in the future",
		"then remaining time until the next occurrence is returned")
}
