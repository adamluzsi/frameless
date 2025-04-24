// Package timekit is a collection of time related helpers
//
// Deprecated: not deprecated, but experimental, API is subject to changes.
package timekit

import (
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/internal/mathkit"
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
	Year       int
	Month      int
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

func (d Delta) add(o Delta) Delta {
	d = d.Normalise()
	o = o.Normalise()
	if 0 < o.Nanosecond {
		d.Nanosecond = mathkit.MustSum(d.Nanosecond, o.Nanosecond)
		d = d.Normalise()
	}
	if 0 < o.Second {
		d.Second = mathkit.MustSum(d.Second, o.Second)
		d = d.Normalise()
	}
	if 0 < o.Minute {
		d.Minute = mathkit.MustSum(d.Minute, o.Minute)
		d = d.Normalise()
	}
	if 0 < o.Hour {
		d.Hour = mathkit.MustSum(d.Hour, o.Hour)
		d = d.Normalise()
	}
	if 0 < o.Day {
		d.Day = mathkit.MustSum(d.Day, o.Day)
	}
	if 0 < o.Month {
		d.Month = mathkit.MustSum(d.Month, o.Month)
		d = d.Normalise()
	}
	if 0 < o.Year {
		// since we don't have a higher level of structure, thus year can't be protected from int overflow
		if mathkit.CanSumOverflow(d.Year, o.Year) {
			panic(mathkit.ErrOverflow.F("timekit.Delta Year doesn't have a higher level time unit, and thus can't avoid int overflow"))
		}
		d.Year += o.Year
	}
	return d
}

func (d Delta) AddDuration(duration time.Duration) Delta {
	return d.add(Delta{Nanosecond: int(duration)})
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

func (Delta) Between(from, till time.Time) Delta {
	var (
		delta      Delta
		start, end = from, till
		isNegative = from.After(till)
	)
	if isNegative {
		start, end = end, start
	}
	var cursor = start

	for { // years
		candidate := cursor.AddDate(1, 0, 0)
		if candidate.After(end) {
			break
		}
		cursor = candidate
		delta.Year++
	}

	for { // months
		candidate := cursor.AddDate(0, 1, 0)
		if candidate.After(end) {
			break
		}
		cursor = candidate
		delta.Month++
	}

	for { // months
		candidate := cursor.AddDate(0, 0, 1)
		if candidate.After(end) {
			break
		}
		cursor = candidate
		delta.Day++
	}

	if !delta.AddTo(start).Equal(cursor) {
		panic("!!!")
	}

	for { // remaining
		remaining := end.Sub(cursor)
		if remaining <= 0 {
			pp.PP()
			pp.PP(delta.AddTo(start), "start+delta")
			pp.PP(end, "end")
			break
		}
		cursor = cursor.Add(remaining)
		pp.PP(remaining)
		delta = delta.AddDuration(remaining)

		if !delta.AddTo(start).Equal(cursor) {
			panic("!!!")
		}
	}

	if !delta.AddTo(start).Equal(end) {
		panic("???")
	}

	// 	for cursor.Before(end) {
	// 		dur := end.Sub(cursor)
	// 		cursor = cursor.Add(dur)
	// 		delta = delta.AddDuration(dur)
	// 	}

	pp.PP("---")
	pp.PP(start, "start")
	pp.PP(delta.AddTo(start), "start + delta")
	pp.PP(end, "end")
	pp.PP("===")
	pp.PP(start, "start")
	pp.PP(delta.invert().AddTo(end), "end + invert delta")
	pp.PP(end, "end")
	pp.PP("~~~")

	if isNegative {
		return delta.invert()
	}

	return delta
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
	return t.
		AddDate(d.Year, d.Month, d.Day).
		Add(time.Hour * time.Duration(d.Hour)).
		Add(time.Minute * time.Duration(d.Minute)).
		Add(time.Second * time.Duration(d.Second)).
		Add(time.Nanosecond * time.Duration(d.Nanosecond))
}

const (
	ubMonth  = 12
	ubHour   = 24
	ubMinute = 60
	ubSecond = 60

	ubNanosecond = 1_000_000_000
)

func (d Delta) Normalise() Delta {
	if d.isNormalised() {
		return d
	}

	if ubNanosecond <= d.Nanosecond || d.Nanosecond <= -ubNanosecond {
		const secondNanos = int(time.Second)
		n := d.Nanosecond / secondNanos
		d.Second += n
		d.Nanosecond = d.Nanosecond - n*secondNanos
	}

	if ubSecond <= d.Second || d.Second <= -ubSecond {
		const minuteSeconds = 60
		n := d.Second / minuteSeconds
		d.Minute += n
		d.Second = d.Second - n*minuteSeconds
	}

	if ubMinute <= d.Minute || d.Minute <= -ubMinute {
		const hourMinutes = 60
		n := d.Minute / hourMinutes
		d.Hour += n
		d.Minute = d.Minute - n*hourMinutes
	}

	if ubHour <= d.Hour || d.Hour <= -ubHour {
		const dayHours = 24
		n := d.Hour / dayHours
		d.Day += n
		d.Hour = d.Hour - n*dayHours
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
		d.Nanosecond <= ubNanosecond &&
		-1*ubNanosecond < d.Nanosecond
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
