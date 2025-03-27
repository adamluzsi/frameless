package timekit

import (
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/validate"
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

type DayTime struct {
	Hour   int `range:"0..24"`
	Minute int `range:"0..60"`
}

func (d DayTime) ToDate(year int, month time.Month, day int, tz *time.Location) time.Time {
	return time.Date(year, month, day, d.Hour, d.Minute, 0, 0, tz)
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

// Scheduling allows to define an abstract time,
// that continously reoccurs in time based on the specifications set in its field.
type Scheduling struct {
	// From is the beginning of the scheduling's time interval.
	From DayTime
	// Till is the end of the scheduling's time interval.
	Till DayTime
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

func (p Scheduling) Validate() error {
	if err := validate.Struct(p, validate.InsideValidateFunc); err != nil {
		return err
	}
	if !p.From.IsZero() && !p.Till.IsZero() && p.From.Compare(p.Till) == 1 {
		return fmt.Errorf(".From must be before .Till")
	}
	return nil
}

func (p Scheduling) Check(t time.Time) bool {
	if p.Location != nil {
		t = t.In(p.Location)
	}
	if p.Weekday != nil && t.Weekday() != *p.Weekday {
		return false
	}
	if p.Month != nil && t.Month() != *p.Month {
		return false
	}
	var hour = t.Hour()
	if !(p.From.Hour <= hour && hour <= p.Till.Hour) {
		return false
	}
	var minute = t.Minute()
	if p.From.Hour == hour && !(p.From.Minute <= minute) {
		return false
	}
	if p.Till.Hour == hour && !(minute <= p.Till.Minute) {
		return false
	}
	return true // TODO: real implementation
}

func (ap Scheduling) Next(ref time.Time) (nextOccurrence Range, ok bool) {
	var cur = ref
	for {
		occurrence, ok := ap.Near(cur)
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

func (ap Scheduling) Near(ref time.Time) (nearOccurrence Range, ok bool) {
	if ap.Validate() != nil {
		return Range{}, false
	}

	var candidate time.Time = ref

	if ap.Location != nil {
		candidate = candidate.In(ap.Location)
	}

	candidate = candidate.Truncate(time.Hour)
	location := candidate.Location()

	for {
		if ap.Month != nil {
			distance := DiffMonth(candidate.Month(), *ap.Month)
			if distance != 0 {
				// first day of the target Month
				candidate = candidate.AddDate(0, distance, 0)
				candidate = time.Date(candidate.Year(), candidate.Month(), 1, 0, 0, 0, 0, candidate.Location())
				continue
			}
		}
		if ap.Day != nil {
			switch {
			case candidate.Day() < *ap.Day:
				distance := *ap.Day - candidate.Day()
				candidate = candidate.AddDate(0, 0, distance)
				candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, candidate.Location())
				continue
			case *ap.Day < candidate.Day(): // then we likely need next month's day
				target := candidate.AddDate(0, 1, 0)
				target = time.Date(target.Year(), target.Month()+1, *ap.Day, 0, 0, 0, 0, target.Location())
				candidate = target
				continue
			}
		}

		if ap.Weekday != nil {
			daysTill := DiffWeekday(candidate.Weekday(), *ap.Weekday)
			if daysTill != 0 {
				candidate = candidate.AddDate(0, 0, daysTill)
				candidate = candidate.Truncate(24 * time.Hour)
				continue
			}
		}

		year, month, day := candidate.Date()

		dayStart := time.Date(year, month, day,
			ap.From.Hour, ap.From.Minute, 0, 0, location)

		var dayEnd time.Time
		if !ap.Till.IsZero() {
			dayEnd = time.Date(year, month, day,
				ap.Till.Hour, ap.Till.Minute, 0, 0, location)
		} else {
			dayEnd = toDayEnd(dayStart)
		}

		if !ap.Till.IsZero() && dayEnd.Before(ref) {
			candidate = candidate.Add(Day)
			continue
		}

		return Range{From: dayStart, Till: dayEnd}, true
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
