// Package timekit is a collection of time related helpers
//
// Deprecated: not deprecated, but experimental, API is subject to changes.
package timekit

import (
	"cmp"
	"context"
	"encoding"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/mathkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/clock"
)

const (
	ISO8601      = "2006-01-02T15:04:05Z07:00"           // with seconds only (0 digits)
	ISO8601Milli = "2006-01-02T15:04:05.000Z07:00"       // with milliseconds (3 digits)
	ISO8601Micro = "2006-01-02T15:04:05.000000Z07:00"    // with microseconds (6 digits)
	ISO8601Nano  = "2006-01-02T15:04:05.000000000Z07:00" // with nanoseconds  (9 digits)
)

const RFC5424 = ISO8601Milli

var _ = time.RFC3339

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
	Hour     int `range:"0..24"`
	Minute   int `range:"0..60"`
	Location *time.Location
}

func (dt DayTime) loc() *time.Location {
	return cmp.Or(dt.Location, time.Local)
}

func (dt DayTime) ToTime(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, dt.Hour, dt.Minute, 0, 0, dt.loc())
}

func (dt DayTime) IsZero() bool {
	return dt.Hour == 0 && dt.Minute == 0
}

func (dt DayTime) Compare(oth DayTime) int {
	if dt.Hour < oth.Hour {
		return -1
	}
	if oth.Hour < dt.Hour {
		return 1
	}
	if dt.Minute < oth.Minute {
		return -1
	}
	if oth.Minute < dt.Minute {
		return 1
	}
	return 0
}

func (dt DayTime) UntilNext(since time.Time) time.Duration {
	loc := dt.loc()
	now := clock.Now().In(loc)
	if since.IsZero() {
		since = now
	}
	since = since.In(loc)

	if since.Year() < now.Year() {
		return 0
	}
	if since.Month() < now.Month() {
		return 0
	}
	if since.Day() < now.Day() {
		return 0
	}

	occurrenceAt := time.Date(now.Year(), now.Month(), now.Day(),
		dt.Hour, dt.Minute, 0, 0, loc).
		AddDate(0, 0, 1)

	return occurrenceAt.Sub(since)
}

var _ encoding.TextUnmarshaler = (*DayTime)(nil)

func (dt *DayTime) UnmarshalText(text []byte) error {
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
	dt.Hour = t.Hour()
	dt.Minute = t.Minute()
	dt.Location = loc
	return nil
}

var _ encoding.TextMarshaler = (*DayTime)(nil)

func (dt DayTime) MarshalText() (text []byte, err error) {
	if dt.Location != nil {
		const layoutWTZ = "15:04Z07:00"
		d := time.Date(0, 0, 0, dt.Hour, dt.Minute, 0, 0, dt.Location)
		return []byte(d.Format(layoutWTZ)), nil
	}
	const layoutLocal = "15:04"
	d := time.Date(0, 0, 0, dt.Hour, dt.Minute, 0, 0, time.Local)
	return []byte(d.Format(layoutLocal)), nil
}

type Range struct {
	From time.Time
	Till time.Time
}

func (tr Range) Validate(context.Context) error {
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

// Schedule allows to define an abstract time,
// that continously reoccurs in time based on the specifications set in its field.
type Schedule struct {
	// DayTime is the time during a day, where the scheduling begins.
	DayTime DayTime
	// Duration is length of the scheduling.
	Duration time.Duration
	// Month defines for which month the AvailabilityPolicy is meant for.
	Months []time.Month
	// Day defines for which day the AvailabilityPolicy is meant for.
	Days []int
	// Weekday defines if the policy is meant for a given day, or for any day of the week.
	// if nil, then it is interpreted as unbound.
	Weekdays []time.Weekday
	// Location of the AvailabilityPolicy.
	// When no Location set, then Product specific configuration is expected.
	Location *time.Location
}

func (sch Schedule) IsZero() bool {
	return sch.DayTime.IsZero() &&
		sch.Duration == 0 &&
		len(sch.Months) == 0 &&
		len(sch.Days) == 0 &&
		len(sch.Weekdays) == 0 &&
		sch.Location == nil
}

func (sch Schedule) Validate(ctx context.Context) error {
	if err := validate.Struct(ctx, sch); err != nil {
		return err
	}
	if sch.Duration < 0 {
		return fmt.Errorf("negative Schedule.Duration is not accepted")
	}
	return nil
}

func (sch Schedule) Check(ref time.Time) bool {
	if sch.Location != nil {
		ref = ref.In(sch.Location)
	}
	if 0 < len(sch.Months) && !slices.Contains(sch.Months, ref.Month()) {
		return false
	}
	if 0 < len(sch.Weekdays) && !slices.Contains(sch.Weekdays, ref.Weekday()) {
		return false
	}
	if 0 < len(sch.Days) && !slices.Contains(sch.Days, ref.Day()) {
		return false
	}
	if !sch.DayTime.IsZero() && !sch.getRangeOnDate(ref).Contain(ref) {
		return false
	}
	return true
}

func (sch Schedule) getRangeOnDate(ref time.Time) Range {
	var dt = sch.DayTime
	dt.Location = ref.Location()
	var (
		from = dt.ToTime(ref.Date())
		till = from.Add(sch.Duration)
	)
	return Range{
		From: from,
		Till: till,
	}
}

func (sch Schedule) Next(ref time.Time) (nextOccurrence Range, ok bool) {
	for {
		occurrence, ok := sch.Near(ref)
		if !ok {
			return occurrence, ok
		}
		// Is ref Less than occurrence.From ?
		if ref.Compare(occurrence.From) < 0 {
			return occurrence, true
		}
		ref = occurrence.Till.Add(1)
	}
}

func (sch Schedule) Near(ref time.Time) (nearOccurrence Range, ok bool) {
	var ctx = context.Background()
	if sch.Validate(ctx) != nil {
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
		if 0 < len(sch.Months) {
			m := candidate.Month()
			distance, ok := closest(sch.Months, func(o time.Month) int {
				return MonthDiff(m, o)
			})
			if ok && distance != 0 {
				targetMonth := candidate.Month() + time.Month(distance)
				// first day of the target Month
				candidate = time.Date(candidate.Year(), targetMonth, 1,
					0, 0, 0, 0, candidate.Location())
				continue
			}
		}
		if 0 < len(sch.Days) {
			var (
				cDay     = candidate.Day()
				nextTime time.Time
			)
			for _, eDay := range sch.Days {
				var next time.Time
				switch {
				case cDay == eDay:
					next = candidate
				case cDay < eDay:
					distance := eDay - cDay
					next = time.Date(candidate.Year(), candidate.Month(), cDay+distance,
						0, 0, 0, 0, candidate.Location())
				case eDay < cDay: // then we likely need next month's day
					target := candidate.AddDate(0, 1, 0)
					next = time.Date(target.Year(), target.Month()+1, eDay, 0, 0, 0, 0, target.Location())
				default:
					continue
				}
				if nextTime.IsZero() {
					nextTime = next
				}
				if next.Before(nextTime) {
					nextTime = next
				}
			}
			if !nextTime.IsZero() && !nextTime.Equal(candidate) {
				candidate = nextTime
				continue
			}
		}

		if 0 < len(sch.Weekdays) {
			wd := candidate.Weekday()
			daysTill, ok := closest(sch.Weekdays, func(o time.Weekday) int {
				return WeekdayDiff(wd, o)
			})
			if ok && daysTill != 0 {
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

func closest[O mathkit.Int, T any](vs []T, m func(T) O) (O, bool) {
	var distance O
	for _, d := range slicekit.Map(vs, m) {
		if distance == 0 {
			distance = d
		}
		if d < distance {
			distance = d
		}
	}
	return distance, len(vs) != 0
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

// Duration allows to describe distances between two time point,
// that normally would not be possible with time.Duration.
type Duration struct {
	i mathkit.BigInt[time.Duration]
}

const ErrParseDuration errorkit.Error = "ErrParseDuration"

func (Duration) Parse(raw string) (Duration, error) {
	v, err := mathkit.BigInt[time.Duration]{}.Parse(raw)
	if err != nil {
		return Duration{}, ErrParseDuration.F("unable to parse Duration: %s\n%w", raw, err)
	}
	return Duration{i: v}, nil
}

func (d Duration) String() string {
	return d.i.String()
}

func (d Duration) Compare(o Duration) int {
	return d.i.Compare(o.i)
}

func (d Duration) Add(o Duration) Duration {
	d.i = d.i.Add(o.i)
	return d
}

func (d Duration) AddDuration(duration time.Duration) Duration {
	d.i = d.i.Add(mathkit.BigInt[time.Duration]{}.Of(duration))
	return d
}

func (Duration) ByDuration(duration time.Duration) Duration {
	return Duration{}.AddDuration(duration)
}

func (Duration) Between(from, till time.Time) Duration {
	var d Duration

	var cursor = from
	for {
		remaining := till.Sub(cursor)
		if remaining == 0 {
			break
		}

		cursor = cursor.Add(remaining)
		d = d.AddDuration(remaining)
	}

	return d
}

func (d Duration) Iter() iter.Seq[time.Duration] {
	return d.i.Iter()
}

func (d Duration) AddTo(t time.Time) time.Time {
	for v := range d.i.Iter() {
		t = t.Add(v)
	}
	return t
}

func (d Duration) IsZero() bool {
	return d.i.IsZero()
}

type schedule interface {
	// Near will return the nearest time to the reference time, for the given Schedule.
	// Near included the reference time as valid result.
	Near(ref time.Time) (time.Time, bool)
}

var _ schedule = Monthly(0)

type Monthly time.Month

// Near will return the nearest time that Has the given Month.
func (m Monthly) Near(ref time.Time) (_ time.Time, _ bool) {
	panic("not implemented") // TODO: Implement
}
