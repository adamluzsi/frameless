// Package timekit is a collection of time related helpers
//
// Deprecated: not deprecated, but experimental, API is subject to changes.
package timekit

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/internal/mathkit"
	"go.llib.dev/frameless/pkg/validate"
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
	DistanceFrom(ref time.Time) Duration
}

// Duration allows to describe distances between two time point,
// that normally would not be possible with time.Duration.
type Duration struct {
	i *big.Int
	d time.Duration
}

func (Duration) fromBigInt(i *big.Int) Duration {
	if i == nil {
		i = big.NewInt(0)
	}
	return Duration{i: i}
}

func (d Duration) toBigInt() *big.Int {
	if d.i != nil {
		return d.i
	}
	return big.NewInt(d.d.Nanoseconds())
}

const ErrParseDuration errorkit.Error = "ErrParseDuration"

func (Duration) Parse(raw string) (Duration, error) {
	i, ok := big.NewInt(0).SetString(raw, 10)
	if !ok {
		return Duration{}, ErrParseDuration.F("unable to parse Duration: %s", raw)
	}

	return Duration{}.fromBigInt(i), nil
}

func (d Duration) String() string {
	return d.toBigInt().String()
}

func (d Duration) Compare(o Duration) int {
	return d.toBigInt().Cmp(o.toBigInt())
}

func (d Duration) Add(o Duration) Duration {
	a := d.toBigInt()
	b := o.toBigInt()
	return d.fromBigInt(big.NewInt(0).Add(a, b))
}

func (d Duration) AddDuration(duration time.Duration) Duration {
	return d.Add(d.fromBigInt(big.NewInt(int64(duration))))
}

func (Duration) ByDuration(duration time.Duration) Duration {
	return Duration{}.AddDuration(duration)
}

func (Duration) Between(from, till time.Time) Duration {
	var delta Duration

	cursor := from
	for {
		remaining := till.Sub(cursor)
		if remaining == 0 {
			break
		}

		cursor = cursor.Add(remaining)
		delta = delta.AddDuration(remaining)
	}

	return delta
}

const maxTimeDuration = math.MaxInt64

var maxTimeDurationBigInt = big.NewInt(maxTimeDuration)

func (d Duration) AddTo(t time.Time) time.Time {
	dur := d.toBigInt()
	cmp := dur.Cmp(maxTimeDurationBigInt)

	if compare.IsLessOrEqual(cmp) {
		return t.Add(time.Duration(dur.Int64()))
	}

	big.NewInt(0).Sub(dur, maxTimeDurationBigInt)

	return time.Time{}
}

// func (d Delta) AddTo(t time.Time) time.Time {
// 	return t.
// 		AddDate(d.Year, d.Month, d.Day).
// 		Add(time.Hour * time.Duration(d.Hour)).
// 		Add(time.Minute * time.Duration(d.Minute)).
// 		Add(time.Second * time.Duration(d.Second)).
// 		Add(time.Nanosecond * time.Duration(d.Nanosecond))
// }

// func (d Duration) AddTo(t time.Time) time.Time {
// 	t = add(t, time.Hour, d.Hour)
// 	t = add(t, time.Minute, d.Minute)
// 	t = add(t, time.Second, d.Second)
// 	t = add(t, time.Nanosecond, d.Nanosecond)
// 	return t
// }

func add[int mathkit.Int](t time.Time, unit time.Duration, m int) time.Time {
	var n = int64(m)
	if n == 0 {
		return t
	}

	var maxMultiplier = mathkit.MaxIntMultiplier(int64(unit)) // unit is positive here

	if n < 0 {
		n *= -1 // n is ensured to be positive after this
		unit *= -1
	}

	// MVP version
	// The use of for loop helps avoiding int overflow related issues with multiplication.
	if maxMultiplier < n {
		t = t.Add(unit * time.Duration(maxMultiplier))
		t = t.Add(unit * time.Duration(int64(n)-maxMultiplier))
		return t
	}

	return t.Add(unit * time.Duration(n))
}

const (
	ubMonth  = 12
	ubHour   = 24
	ubMinute = 60
	ubSecond = 60

	ubNanosecond = 1_000_000_000
)

func (d Duration) Normalise() Duration {
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

	return d
}

func (d Duration) IsZero() bool {
	return d.Day == 0 &&
		d.Hour == 0 &&
		d.Minute == 0 &&
		d.Second == 0 &&
		d.Nanosecond == 0
}

func (d Duration) IsPositive() bool {
	return 0 <= d.Day &&
		0 <= d.Hour &&
		0 <= d.Minute &&
		0 <= d.Second &&
		0 <= d.Nanosecond
}

func (d Duration) isNormalised() bool {
	// not correct fully, because -3 month + 3 day is considered also normalised by this, and might not be what people would expect
	// also, a Day cannot be normalised as not all month has the same amount of days.
	return d.Minute <= 59 &&
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
