// Package timekit is a collection of time related helpers
//
// Deprecated: not deprecated, but experimental, API is subject to changes.
package timekit

import (
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/pp"
)

const (
	Nanosecond  = time.Nanosecond
	Microsecond = time.Microsecond
	Millisecond = time.Millisecond
	Second      = time.Second
	Minute      = time.Minute
	Hour        = time.Hour
	Day         = 24 * Hour
	Week        = 7 * Day
)

var _ = enum.Register[time.Weekday](Weekdays()...) // zero accepted as Sunday

var _ = enum.Register[time.Month](append([]time.Month{0}, Months()...)...) // zero accepted as well for backward compability of no month

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
	if ref.Before(tr.From) {
		return false
	}
	if tr.Till.Before(ref) {
		return false
	}
	return true
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

// DiffMonth calculates the number of months between ref and target.
// It assumes a cyclic nature, meaning that if target is before ref in the year,
// it wraps around after December.
func DiffMonth(ref, target time.Month) int {
	diff := int(target) - int(ref)
	if diff < 0 {
		diff += 12
	}
	return diff
}

// DiffWeekday calculates the number of days between ref and target.
// It assumes a cyclic nature, meaning that if target is before ref in the week,
// it wraps around after Sunday.
func DiffWeekday(ref, target time.Weekday) int {
	diff := int(target) - int(ref)
	if diff < 0 {
		diff += 7
	}
	return diff
}

func DiffDay(ref, target time.Time) int {
	ref = ref.Truncate(Day)
	target = target.Truncate(Day)
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

func (sch Schedule) Validate() error {
	if err := validate.Struct(sch, validate.InsideValidateFunc); err != nil {
		return err
	}
	if sch.Duration < 0 {
		return fmt.Errorf("negative Schedule.Duration is not accepted")
	}
	return nil
}

func (sch Schedule) UntilNext(ref time.Time) time.Duration {
	return 0
}

func (sch Schedule) Check(ref time.Time) bool {
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

func (sch Schedule) getRangeOnDate(ref time.Time) Range {
	var (
		from = sch.DayTime.ToTimeRelTo(ref)
		till = from.Add(sch.Duration)
	)
	return Range{
		From: from,
		Till: till,
	}
}

func (sch Schedule) NextRange(ref time.Time) (nextOccurrence Range, ok bool) {
	var cur = ref
	for {
		occurrence, ok := sch.near(cur)
		if !ok {
			return occurrence, ok
		}
		if occurrence.Contain(ref) {
			cur = occurrence.Till.Add(1)
			continue
		}
		return occurrence, true
	}
}

func (sch Schedule) near(ref time.Time) (nearOccurrence Range, ok bool) {
	if sch.Validate() != nil {
		return Range{}, false
	}

	var candidate time.Time = ref

	if sch.Location != nil {
		candidate = candidate.In(sch.Location)
	}

	candidate = candidate.Truncate(time.Hour)

	for {
		if sch.Month != nil {
			distance := DiffMonth(candidate.Month(), *sch.Month)
			if distance != 0 {
				// first day of the target Month
				candidate = candidate.AddDate(0, distance, 0)
				candidate = time.Date(candidate.Year(), candidate.Month(), 1, 0, 0, 0, 0, candidate.Location())
				pp.PP()
				continue
			}
		}
		if sch.Day != nil {
			switch {
			case candidate.Day() < *sch.Day:
				distance := *sch.Day - candidate.Day()
				candidate = candidate.AddDate(0, 0, distance)
				candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, candidate.Location())
				pp.PP()
				continue
			case *sch.Day < candidate.Day(): // then we likely need next month's day
				target := candidate.AddDate(0, 1, 0)
				target = time.Date(target.Year(), target.Month()+1, *sch.Day, 0, 0, 0, 0, target.Location())
				candidate = target
				pp.PP()
				continue
			}
		}

		if sch.Weekday != nil {
			daysTill := DiffWeekday(candidate.Weekday(), *sch.Weekday)
			if daysTill != 0 {
				candidate = time.Time.Truncate(candidate, Day)
				candidate = candidate.AddDate(0, 0, daysTill)
				pp.PP()
				continue
			}
		}

		near := sch.getRangeOnDate(candidate)

		if candidate.Compare(near.Till) < 1 {
			return near, true
		}

		// goto next day start
		candidate = candidate.
			Truncate(Day).
			Add(Day)

		continue
	}
}

// type ModifyBy struct {
// 	Year  *int
// 	Month *time.Month
// 	Day   *int

// 	Hour   *int
// 	Minute *int
// 	Second *int

// 	Nanosecond *int

// 	Location *time.Location
// }

// func Modify(t time.Time, by ModifyBy) time.Time {
// 	if by.Year == nil {
// 		by.Year = pointer.Of(t.Year())
// 	}
// 	if by.Month == nil {
// 		by.Month = pointer.Of(t.Month())
// 	}
// 	if by.Day == nil {
// 		by.Day = pointer.Of(t.Day())
// 	}
// 	if by.Hour == nil {
// 		by.Hour = pointer.Of(t.Hour())
// 	}
// 	if by.Minute == nil {
// 		by.Minute = pointer.Of(t.Minute())
// 	}
// 	if by.Second == nil {
// 		by.Second = pointer.Of(t.Second())
// 	}
// 	if by.Nanosecond == nil {
// 		by.Nanosecond = pointer.Of(t.Nanosecond())
// 	}
// 	if by.Location == nil {
// 		by.Location = t.Location()
// 	}
// 	return time.Date(*by.Year, *by.Month, *by.Day,
// 		*by.Hour, *by.Minute, *by.Second,
// 		*by.Nanosecond, by.Location)
// }

func toDayEnd(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day+1, 0, 0, 0, 0, t.Location()).Add(-1)
}
