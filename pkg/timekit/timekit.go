// Package timekit is a collection of time related helpers
//
// Deprecated: not deprecated, but experimental, API is subject to changes.
package timekit

import (
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/pp"
)

func EnableTimeEnum() func() {
	var tdWeekday = enum.Register[time.Weekday](Weekdays()...)                       // zero accepted as Sunday
	var tdMonth = enum.Register[time.Month](append([]time.Month{0}, Months()...)...) // zero accepted as well for backward compability of no month
	return func() {
		defer tdWeekday()
		defer tdMonth()
	}
}

func Weekdays() []time.Weekday {
	return []time.Weekday{
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
		time.Saturday,
		time.Sunday,
	}
}

func Months() []time.Month {
	return []time.Month{
		time.January,
		time.February,
		time.March,
		time.April,
		time.May,
		time.June,
		time.July,
		time.August,
		time.September,
		time.October,
		time.November,
		time.December,
	}
}

type Interval interface {
	UntilNext(since time.Time) time.Duration
}

type DayTime struct {
	Hour   int `range:"0..24"`
	Minute int `range:"0..60"`
}

func ToDate(ref time.Time) (year int, month time.Month, day int, tz *time.Location) {
	year, month, day = ref.Date()
	tz = ref.Location()
	return
}

func (d DayTime) ToTime(year int, month time.Month, day int, tz *time.Location) time.Time {
	return time.Date(year, month, day, d.Hour, d.Minute, 0, 0, tz)
}

// ToTimeRelTo will return the time.Time equalement of a DayTime,
// which is relative to the provided reference time's date.
func (d DayTime) ToTimeRelTo(ref time.Time) time.Time {
	return d.ToTime(ToDate(ref))
}

func (d DayTime) IsZero() bool {
	return d.Hour == 0 && d.Minute == 0
}

func (d DayTime) Compare(oth DayTime) int {
	if d.Hour < oth.Hour {
		return -1
	}
	if oth.Hour < d.Hour {
		return 1
	}
	if d.Minute < oth.Minute {
		return -1
	}
	if oth.Minute < d.Minute {
		return 1
	}
	return 0
}

type Range struct {
	From time.Time
	Till time.Time
}

func (tr Range) Validate() error {
	if tr.Till.Before(tr.From) {
		return fmt.Errorf("Range.From is before Range.Till")
	}
	return nil
}

func (tr Range) Contain(ref time.Time) bool {
	return tr.From.Compare(ref) <= 0 &&
		ref.Compare(tr.Till) <= 0
}

func (tr Range) IsZero() bool { // TODOL testme
	return tr.From.IsZero() && tr.Till.IsZero()
}

func (tr Range) Overlaps(other Range) bool { // TODO:testme
	return false
}

func (tr Range) Duration() time.Duration { // TODO:testme
	return tr.Till.Sub(tr.From)
}

func ShiftWeekday(wd time.Weekday, d int) time.Weekday {
	delta := int(wd) + d        // Compute total days to add/subtract from the weekday
	newDay := (delta%7 + 7) % 7 // Ensure result is between [0-6] even for negative values
	return time.Weekday(newDay)
}

func ShiftMonth(m time.Month, n int) time.Month {
	delta := int(m) + n                  // Compute delta from original month (1-12)
	adjusted := ((delta-1)%12 + 12) % 12 // Ensure non-negative modulo result
	return time.Month(adjusted + 1)
}

// MonthDiff calculates the number of months between ref and target.
// It assumes a cyclic nature, meaning that if target is before ref in the year,
// it wraps around after December.
func MonthDiff(ref, target time.Month) int {
	diff := int(target) - int(ref)
	if diff < 0 {
		diff += 12
	}
	return diff
}

// WeekdayDiff calculates the number of days between ref and target.
// It assumes a cyclic nature, meaning that if target is before ref in the week,
// it wraps around after Sunday.
func WeekdayDiff(ref, target time.Weekday) int {
	diff := int(target) - int(ref)
	if diff < 0 {
		diff += 7
	}
	return diff
}

// DayDiff returns the difference in days between two times,
// truncating each to midnight before comparison.
func DayDiff(ref, target time.Time) int {
	ref = truncateDay(ref)
	target = truncateDay(target)
	diff := target.Sub(ref)
	return int(diff.Hours() / 24)
}

// ScheduleMVP allows to define an abstract time,
// that continously reoccurs in time based on the specifications set in its field.
type ScheduleMVP struct {
	// DayTime is the time during a day, where the scheduling begins.
	DayTime DayTime
	// Duration is length of the scheduling.
	Duration time.Duration
	// Month defines for which month the AvailabilityPolicy is meant for.
	Month *time.Month
	// Day defines for which day the AvailabilityPolicy is meant for.
	Day *int
	// Weekday defines if the policy is meant for a given day, or for any day of the week.
	// if nil, then it is interpreted as unbound.
	Weekday *time.Weekday
	// Location of the AvailabilityPolicy.
	// When no Location set, then Product specific configuration is expected.
	Location *time.Location
}

func (sch ScheduleMVP) IsZero() bool {
	return sch.DayTime.IsZero() &&
		sch.Duration == 0 &&
		sch.Month == nil &&
		sch.Day == nil &&
		sch.Weekday == nil &&
		sch.Location == nil
}

func (sch ScheduleMVP) Validate() error {
	if err := validate.Struct(sch, validate.InsideValidateFunc); err != nil {
		return err
	}
	if sch.Duration < 0 {
		return fmt.Errorf("negative Schedule.Duration is not accepted")
	}
	return nil
}

func (sch ScheduleMVP) Check(ref time.Time) bool {
	if sch.Location != nil {
		ref = ref.In(sch.Location)
	}
	if sch.Weekday != nil && ref.Weekday() != *sch.Weekday {
		return false
	}
	if sch.Month != nil && ref.Month() != *sch.Month {
		return false
	}
	if !sch.DayTime.IsZero() {
		if !sch.getRangeOnDate(ref).Contain(ref) {
			return false
		}
	}
	return true
}

func (sch ScheduleMVP) getRangeOnDate(ref time.Time) Range {
	var (
		from = sch.DayTime.ToTimeRelTo(ref)
		till = from.Add(sch.Duration)
	)
	return Range{
		From: from,
		Till: till,
	}
}

func (sch ScheduleMVP) Next(ref time.Time) (nextOccurrence Range, ok bool) {
	for {
		occurrence, ok := sch.near(ref)
		if !ok {
			return occurrence, ok
		}
		// Is ref Less than occurrence.From ?
		if compare.IsLess(ref.Compare(occurrence.From)) {
			return occurrence, true
		}
		ref = occurrence.Till.Add(1)
	}
}

func (sch ScheduleMVP) near(ref time.Time) (nearOccurrence Range, ok bool) {
	if sch.Validate() != nil {
		return Range{}, false
	}

	if sch.IsZero() {
		return sch.getRangeOnDate(nextDay(ref)), true
	}

	var candidate time.Time = ref

	if sch.Location != nil {
		candidate = candidate.In(sch.Location)
	}

	for {
		if sch.Month != nil {
			distance := MonthDiff(candidate.Month(), *sch.Month)
			if distance != 0 {
				targetMonth := candidate.Month() + time.Month(distance)
				// first day of the target Month
				candidate = time.Date(candidate.Year(), targetMonth, 1,
					0, 0, 0, 0, candidate.Location())
				continue
			}
		}
		if sch.Day != nil {
			switch {
			case candidate.Day() < *sch.Day:
				distance := *sch.Day - candidate.Day()
				candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day()+distance,
					0, 0, 0, 0, candidate.Location())
				continue
			case *sch.Day < candidate.Day(): // then we likely need next month's day
				target := candidate.AddDate(0, 1, 0)
				target = time.Date(target.Year(), target.Month()+1, *sch.Day, 0, 0, 0, 0, target.Location())
				candidate = target
				continue
			}
		}

		if sch.Weekday != nil {
			daysTill := WeekdayDiff(candidate.Weekday(), *sch.Weekday)
			if daysTill != 0 {
				candidate = truncateDay(candidate)
				candidate = candidate.AddDate(0, 0, daysTill)
				continue
			}
		}

		near := sch.getRangeOnDate(candidate)

		if candidate.Compare(near.Till) < 1 {
			return near, true
		}

		// goto next day start
		candidate = nextDay(candidate)

		continue
	}
}

func nextDay(t time.Time) time.Time {
	t = truncateDay(t)
	t = t.AddDate(0, 0, 1)
	return t
}

func truncateDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

type FromDistanceCalculator interface {
	DistanceFrom(ref time.Time) Delta
}

// Delta allows to describe distances between two time point,
// that normally would not be possible with time.Duration.
type Delta struct {
	Year  int
	Month int

	Day        int
	Hour       int
	Minute     int
	Second     int
	Nanosecond int
}

func (d Delta) Compare(o Delta) int {
	if cmp := compare.Numbers(d.Year, o.Year); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Month, o.Month); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Day, o.Day); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Hour, o.Hour); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Minute, o.Minute); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Second, o.Second); !compare.IsEqual(cmp) {
		return cmp
	}
	if cmp := compare.Numbers(d.Nanosecond, o.Nanosecond); !compare.IsEqual(cmp) {
		return cmp
	}
	return 0
}

func (d Delta) Add(duration time.Duration) Delta {
	d.Nanosecond += int(duration)
	return d.Normalise()
}

func (Delta) ByDuration(duration time.Duration) Delta {
	var (
		dist Delta
	)

	{ // day
		const Day = 24 * time.Hour
		for i := 0; 0 <= duration-time.Duration(i)*Day; i++ {
			dist.Day = i
		}
		duration -= time.Duration(dist.Day) * Day
	}

	const (
		nanoPerSecond = int64(time.Second)
		nanoPerMinute = 60 * nanoPerSecond
		nanoPerHour   = 60 * nanoPerMinute
	)

	remaining := int64(duration)

	if remaining == 0 {
		return dist
	}

	dist.Hour = int(remaining / nanoPerHour)
	remaining %= nanoPerHour

	dist.Minute = int(remaining / nanoPerMinute)
	remaining %= nanoPerMinute

	dist.Second = int(remaining / nanoPerSecond)
	remaining %= nanoPerSecond

	dist.Nanosecond = int(remaining)

	return dist
}

func (Delta) Between(a, b time.Time) Delta {
	var (
		d          Delta
		isNegative = a.After(b)
	)
	if isNegative {
		b, a = a, b
	}

	{ // years
		for c, i := a, 1; c.Year() < b.Year(); i++ {
			c = a.AddDate(i, 0, 0)
			if !compare.IsLessOrEqual(c.Compare(b)) {
				break
			}
			d.Year = i
		}
		a = a.AddDate(d.Year, 0, 0)
	}
	pp.PP(a, b, d.Year)

	{ // months

		for c, i := a, 1; 0 < MonthDiff(c.Month(), b.Month()); i++ {
			c = a.AddDate(0, i, 0)
			if !compare.IsLessOrEqual(c.Compare(b)) {
				break
			}
			d.Month = i
		}
		a = a.AddDate(0, d.Month, 0)
	}
	pp.PP(a, b, d.Year, d.Month)

	rem := Delta{}.ByDuration(b.Sub(a))
	d.Day = rem.Day
	d.Hour = rem.Hour
	d.Minute = rem.Minute
	d.Second = rem.Second
	d.Nanosecond = rem.Nanosecond

	if isNegative {
		return d.invert()
	}

	return d
}

func (d Delta) invert() Delta {
	return Delta{
		Year:       -1 * d.Year,
		Month:      -1 * d.Month,
		Day:        -1 * d.Day,
		Hour:       -1 * d.Hour,
		Minute:     -1 * d.Minute,
		Second:     -1 * d.Second,
		Nanosecond: -1 * d.Nanosecond,
	}
}

func (d Delta) AddTo(t time.Time) time.Time {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	return time.Date(
		year+d.Year, month+time.Month(d.Month), day+d.Day,
		hour+d.Hour, min+d.Minute, sec+d.Second,
		t.Nanosecond()+d.Nanosecond, t.Location(),
	)
}

// func (d Delta) AddTo(t time.Time) time.Time {
// 	t = t.AddDate(d.Year, d.Month, d.Day)
// 	t = t.Add(d.clockDuration())
// 	return t
// }

// func (d Delta) clockDuration() time.Duration {
// 	var duration time.Duration
// 	duration += time.Hour * time.Duration(d.Hour)
// 	duration += time.Minute * time.Duration(d.Minute)
// 	duration += time.Second * time.Duration(d.Second)
// 	duration += time.Nanosecond * time.Duration(d.Nanosecond)
// 	return duration
// }

const maxNormalisedNanosecond = 999_999_999

func (d Delta) Normalise() Delta {
	if d.isNormalised() {
		return d
	}

	const (
		maxMonth  = 11
		maxHour   = 23
		maxMinute = 59
		maxSecond = 59
	)

	if maxNormalisedNanosecond < d.Nanosecond || d.Nanosecond < -maxNormalisedNanosecond {
		const secondNanos = int(time.Second)
		v := d.Nanosecond / secondNanos
		d.Second += v
		d.Nanosecond = d.Nanosecond - v*secondNanos
	}

	if maxSecond < d.Second || d.Second < -maxSecond {
		const minuteSeconds = 60
		v := d.Second / minuteSeconds
		d.Minute += v
		d.Second = d.Second - v*minuteSeconds
	}

	if maxMinute < d.Minute || d.Minute < -maxMinute {
		const hourMinutes = 60
		v := d.Minute / hourMinutes
		d.Hour += v
		d.Minute = d.Minute - v*hourMinutes
	}

	if maxHour < d.Hour || d.Hour < -maxHour {
		const dayHours = 24
		v := d.Hour / dayHours
		d.Day += v
		d.Hour = d.Hour - v*dayHours
	}

	return d
}

func (d Delta) IsZero() bool {
	return d.Day == 0 &&
		d.Hour == 0 &&
		d.Minute == 0 &&
		d.Second == 0 &&
		d.Nanosecond == 0
}

func (d Delta) IsPositive() bool {
	return 0 <= d.Day &&
		0 <= d.Hour &&
		0 <= d.Minute &&
		0 <= d.Second &&
		0 <= d.Nanosecond
}

func (d Delta) isNormalised() bool {
	// not correct fully, because -3 month + 3 day is considered also normalised by this, and might not be what people would expect
	// also, a Day cannot be normalised as not all month has the same amount of days.
	return d.Hour <= 23 &&
		-1*23 <= d.Hour &&
		d.Minute <= 59 &&
		-1*59 <= d.Minute &&
		d.Second <= 59 &&
		-1*59 <= d.Second &&
		d.Nanosecond <= maxNormalisedNanosecond &&
		-1*maxNormalisedNanosecond <= d.Nanosecond
}

type Schedule interface {
	// Near will return the nearest time to the reference time, for the given Schedule.
	// Near included the reference time as valid result.
	Near(ref time.Time) (time.Time, bool)
}

func FindNext(ref time.Time, schedules ...Schedule) (time.Time, bool) {
	return time.Time{}, false
}

var _ Schedule = Monthly(0)

type Monthly time.Month

// Near will return the nearest time that Has the given Month.
func (m Monthly) Near(ref time.Time) (_ time.Time, _ bool) {
	panic("not implemented") // TODO: Implement
}
