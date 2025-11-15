package timekit_test

import (
	"context"
	"math"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/timekit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/zerokit"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Test_debug(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("delta-between", func(s *testcase.Spec) {
		var (
			start = let.VarOf(s, time.Date(2004, time.June, 15, 8, 2, 30, 333738005, time.Local))
			end   = let.VarOf(s, time.Date(2025, time.April, 14, 11, 53, 49, 0, time.Local))
		)
		s.Test("positive", func(t *testcase.T) {
			var dp = timekit.Duration{}.Between(start.Get(t), end.Get(t))
			assert.True(t, dp.AddTo(start.Get(t)).Equal(end.Get(t)))
		})
		s.Test("negative", func(t *testcase.T) {
			var dn = timekit.Duration{}.Between(end.Get(t), start.Get(t))
			assert.True(t, dn.AddTo(end.Get(t)).Equal(start.Get(t)))
		})
	})

	s.Test("Add a minute of time duration", func(t *testcase.T) {
		od := timekit.Duration{}
		nd := od.AddDuration(time.Minute)
		assert.NotEqual(t, od, nd)
	})

	s.Test("AddDuration", func(t *testcase.T) {
		// timekit.go:494 time.Date(2004, time.June, 15, 8, 2, 30, 333738005, time.Local)	"start"
		// timekit.go:495 time.Date(2025, time.April, 14, 10, 53, 49, 0, time.Local)	"start + delta"
		// timekit.go:496 time.Date(2025, time.April, 14, 11, 53, 49, 0, time.Local)	"end"
		var (
			begin = t.Random.Time()
			end   = begin.AddDate(1000, 0, 0)
		)

		var (
			d timekit.Duration
			c = begin
		)
		for {
			remaining := end.Sub(c)
			t.Log(remaining)
			if remaining == 0 {
				break
			}

			c = c.Add(remaining)
			d = d.AddDuration(remaining)

			if !d.AddTo(begin).Equal(c) {
				t.LogPretty(d)
				t.LogPretty(begin, "begin")
				t.LogPretty(d.AddTo(begin), "begin+delta")
				t.LogPretty(c, "cursor")
				t.FailNow()
			}
		}

		t.LogPretty(begin, "begin")
		t.LogPretty(d.AddTo(begin), "begin+delta")
		t.LogPretty(end, "end")
		t.LogPretty(c, "cursor")

		assert.True(t, d.AddTo(begin).Equal(end), assert.MessageF("diff: %s", end.Sub(d.AddTo(begin))))

	})
}

func Test_time_contract(t *testing.T) {
	s := testcase.NewSpec(t)
	let.OneOf(s, time.Local, time.UTC)
	s.Test("day overflow", func(t *testcase.T) {
		tm := time.Date(2000, time.February, 33, 0, 0, 0, 0, time.Local)
		assert.NotEmpty(t, tm)
		assert.Equal(t, 2000, tm.Year())
		assert.Equal(t, time.March, tm.Month())
		assert.Equal(t, 04, tm.Day())
	})
	s.Test("month overflow", func(t *testcase.T) {
		tm := time.Date(2000, 13, 1, 0, 0, 0, 0, time.Local)
		assert.NotEmpty(t, tm)
		assert.Equal(t, 2001, tm.Year())
		assert.Equal(t, time.January, tm.Month())
	})
	s.Test("day+1 & .Add(-1) to get get the last moment of a day", func(t *testcase.T) {
		ref := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.Local)
		exp := time.Date(2000, time.April, 1, 23, 59, 59, 999999999, time.Local)
		got := time.Date(ref.Year(), ref.Month(), ref.Day()+1, 0, 0, 0, 0, ref.Location()).Add(-1)
		assert.Equal(t, exp, got)
	})
	s.Test("time.Time.Truncate with timekit.Day", func(t *testcase.T) {
		ref := t.Random.Time()
		year, month, day := ref.Date()
		exp := time.Date(year, month, day, 0, 0, 0, 0, ref.Location())
		got := time.Time.Truncate(ref, 24*time.Hour)
		assert.Equal(t, exp, got)
	})
	s.Test("max time.Duration", func(t *testcase.T) {
		assert.Equal(t, reflectkit.TypeOf[time.Duration]().Kind(), reflect.Int64,
			"if this fails, you need to update the `const maxTimeDuration time.Duration` value")
	})
}

func TestShiftWeekday(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		weekday = let.OneOf(s, timekit.Weekdays()...)
		n       = let.IntB(s, 0, 7)
	)
	act := let.Act(func(t *testcase.T) time.Weekday {
		return timekit.ShiftWeekday(weekday.Get(t), n.Get(t))
	})

	s.Then("the result is a valid weekday", func(t *testcase.T) {
		assert.Contains(t, timekit.Weekdays(), act(t))
	})

	s.When("positive value is added", func(s *testcase.Spec) {
		s.And("it doesn't overreach into the next week", func(s *testcase.Spec) {
			weekday.LetValue(s, time.Monday)

			n.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(1, 4)
			})

			s.Then("the weekday is correctly changed", func(t *testcase.T) {
				expected := time.Monday + time.Weekday(n.Get(t))
				assert.Equal(t, expected, act(t))
			})
		})

		s.And("it causes the weekday result to overreach into the next week", func(s *testcase.Spec) {
			weekday.LetValue(s, time.Friday)

			n.LetValue(s, 3)

			s.Then("the weekday is correctly changed", func(t *testcase.T) {
				assert.Equal(t, time.Monday, act(t))
			})
		})
	})

	s.When("negative value is added", func(s *testcase.Spec) {
		s.And("time doesn't reach back to the previous week", func(s *testcase.Spec) {
			weekday.LetValue(s, time.Friday)

			n.LetValue(s, -2)

			s.Then("the weekday is correctly changed", func(t *testcase.T) {
				assert.Equal(t, time.Wednesday, act(t))
			})
		})

		s.And("time reach back to the previous week", func(s *testcase.Spec) {
			weekday.LetValue(s, time.Monday)

			n.LetValue(s, -3)

			s.Then("the weekday is correctly changed", func(t *testcase.T) {
				assert.Equal(t, time.Friday, act(t))
			})
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 1), time.Tuesday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 2), time.Wednesday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 3), time.Thursday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 4), time.Friday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 5), time.Saturday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 6), time.Sunday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, 7), time.Monday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -1), time.Sunday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -2), time.Saturday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -3), time.Friday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -4), time.Thursday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -5), time.Wednesday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -6), time.Tuesday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -7), time.Monday)
		assert.Equal(t, timekit.ShiftWeekday(time.Monday, -8), time.Sunday)
	})
}

func TestShiftMonth(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		month = let.OneOf(s, timekit.Months()...)
		n     = let.IntB(s, -12, 12)
	)
	act := let.Act(func(t *testcase.T) time.Month {
		return timekit.ShiftMonth(month.Get(t), n.Get(t))
	})

	s.Then("the result is a valid month", func(t *testcase.T) {
		assert.Contains(t, timekit.Months(), act(t))
	})

	s.When("positive value is added", func(s *testcase.Spec) {
		s.And("it doesn't overreach into the next year's months", func(s *testcase.Spec) {
			month.LetValue(s, time.January)
			n.Let(s, func(t *testcase.T) int { return t.Random.IntBetween(1, 3) })

			s.Then("the month is correctly changed", func(t *testcase.T) {
				expected := time.Month(int(month.Get(t)) + n.Get(t))
				assert.Equal(t, expected, act(t))
			})
		})

		s.And("it causes the month to overreach into next year's months", func(s *testcase.Spec) {
			month.LetValue(s, time.October)
			n.LetValue(s, 3)

			s.Then("the result is correctly wrapped", func(t *testcase.T) {
				assert.Equal(t, time.January, act(t))
			})
		})
	})

	s.When("negative value is added", func(s *testcase.Spec) {
		s.And("it doesn't reach back to the previous year's months", func(s *testcase.Spec) {
			month.LetValue(s, time.February)
			n.LetValue(s, -1)

			s.Then("the result is correctly changed", func(t *testcase.T) {
				assert.Equal(t, time.January, act(t))
			})
		})

		s.And("it reaches back to the previous year's months", func(s *testcase.Spec) {
			month.LetValue(s, time.January)
			n.LetValue(s, -3)

			s.Then("the result is correctly wrapped", func(t *testcase.T) {
				assert.Equal(t, time.October, act(t))
			})
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.ShiftMonth(time.January, 0), time.January)
		assert.Equal(t, timekit.ShiftMonth(time.January, 1), time.February)
		assert.Equal(t, timekit.ShiftMonth(time.January, 2), time.March)
		assert.Equal(t, timekit.ShiftMonth(time.January, 3), time.April)
		assert.Equal(t, timekit.ShiftMonth(time.January, 4), time.May)
		assert.Equal(t, timekit.ShiftMonth(time.January, 5), time.June)
		assert.Equal(t, timekit.ShiftMonth(time.January, 6), time.July)
		assert.Equal(t, timekit.ShiftMonth(time.January, 7), time.August)
		assert.Equal(t, timekit.ShiftMonth(time.January, 8), time.September)
		assert.Equal(t, timekit.ShiftMonth(time.January, 9), time.October)
		assert.Equal(t, timekit.ShiftMonth(time.January, 10), time.November)
		assert.Equal(t, timekit.ShiftMonth(time.January, 11), time.December)
		assert.Equal(t, timekit.ShiftMonth(time.January, 12), time.January)

		assert.Equal(t, timekit.ShiftMonth(time.December, 1), time.January)
		assert.Equal(t, timekit.ShiftMonth(time.October, 3), time.January)

		assert.Equal(t, timekit.ShiftMonth(time.January, -1), time.December)
		assert.Equal(t, timekit.ShiftMonth(time.February, -2), time.December)
		assert.Equal(t, timekit.ShiftMonth(time.March, -4), time.November)
		assert.Equal(t, timekit.ShiftMonth(time.April, -12), time.April)
	})
}

func ExampleMonthDiff() {
	_ = timekit.MonthDiff(time.January, time.January)   // 0
	_ = timekit.MonthDiff(time.January, time.February)  // 1
	_ = timekit.MonthDiff(time.January, time.March)     // 2
	_ = timekit.MonthDiff(time.January, time.April)     // 3
	_ = timekit.MonthDiff(time.January, time.May)       // 4
	_ = timekit.MonthDiff(time.January, time.June)      // 5
	_ = timekit.MonthDiff(time.January, time.July)      // 6
	_ = timekit.MonthDiff(time.January, time.August)    // 7
	_ = timekit.MonthDiff(time.January, time.September) // 8
	_ = timekit.MonthDiff(time.January, time.October)   // 9
	_ = timekit.MonthDiff(time.January, time.November)  // 10
	_ = timekit.MonthDiff(time.January, time.December)  // 11
	_ = timekit.MonthDiff(time.December, time.January)  // 1
	_ = timekit.MonthDiff(time.October, time.January)   // 3
}

func TestMonthDiff(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.MonthDiff(time.January, time.January), 0)
		assert.Equal(t, timekit.MonthDiff(time.January, time.February), 1)
		assert.Equal(t, timekit.MonthDiff(time.January, time.March), 2)
		assert.Equal(t, timekit.MonthDiff(time.January, time.April), 3)
		assert.Equal(t, timekit.MonthDiff(time.January, time.May), 4)
		assert.Equal(t, timekit.MonthDiff(time.January, time.June), 5)
		assert.Equal(t, timekit.MonthDiff(time.January, time.July), 6)
		assert.Equal(t, timekit.MonthDiff(time.January, time.August), 7)
		assert.Equal(t, timekit.MonthDiff(time.January, time.September), 8)
		assert.Equal(t, timekit.MonthDiff(time.January, time.October), 9)
		assert.Equal(t, timekit.MonthDiff(time.January, time.November), 10)
		assert.Equal(t, timekit.MonthDiff(time.January, time.December), 11)
		assert.Equal(t, timekit.MonthDiff(time.December, time.January), 1)
		assert.Equal(t, timekit.MonthDiff(time.October, time.January), 3)
	})
}

func ExampleWeekdayDiff() {
	_ = timekit.WeekdayDiff(time.Monday, time.Monday)    // 0
	_ = timekit.WeekdayDiff(time.Monday, time.Tuesday)   // 1
	_ = timekit.WeekdayDiff(time.Monday, time.Wednesday) // 2
	_ = timekit.WeekdayDiff(time.Monday, time.Thursday)  // 3
	_ = timekit.WeekdayDiff(time.Monday, time.Friday)    // 4
	_ = timekit.WeekdayDiff(time.Monday, time.Saturday)  // 5
	_ = timekit.WeekdayDiff(time.Monday, time.Sunday)    // 6
}

func TestWeekdayDiff(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Monday), 0)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Tuesday), 1)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Wednesday), 2)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Thursday), 3)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Friday), 4)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Saturday), 5)
		assert.Equal(t, timekit.WeekdayDiff(time.Monday, time.Sunday), 6)
	})
}

func TestDayDiff(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ref    = let.Var[time.Time](s, nil)
		target = let.Var[time.Time](s, nil)
	)
	act := let.Act(func(t *testcase.T) int {
		return timekit.DayDiff(ref.Get(t), target.Get(t))
	})

	s.When("ref time is before target time", func(s *testcase.Spec) {
		days := let.IntB(s, 1, 7)

		ref.Let(s, let.Time(s).Get)

		target.Let(s, func(t *testcase.T) time.Time {
			return ref.Get(t).AddDate(0, 0, days.Get(t))
		})

		s.Then("the days are returned", func(t *testcase.T) {
			assert.Equal(t, days.Get(t), act(t))
		})

		s.Context("but if they are on the same day", func(s *testcase.Spec) {
			target.Let(s, func(t *testcase.T) time.Time {
				year, month, day := ref.Get(t).Date()
				dayEnd := time.Date(year, month, day, 23, 59, 59, 0, ref.Get(t).Location())
				return ref.Get(t).Add(t.Random.DurationBetween(time.Second, dayEnd.Sub(ref.Get(t))))
			})

			s.Then("the resulting day fiff will be zero", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})
	})

	s.When("ref time is after target time", func(s *testcase.Spec) {
		days := let.IntB(s, 1, 7)

		target.Let(s, let.Time(s).Get)

		ref.Let(s, func(t *testcase.T) time.Time {
			return target.Get(t).AddDate(0, 0, days.Get(t))
		})

		s.Then("the days are returned", func(t *testcase.T) {
			assert.Equal(t, -1*days.Get(t), act(t))
		})

		s.Context("but if they are on the same day", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				year, month, day := target.Get(t).Date()
				dayEnd := time.Date(year, month, day, 23, 59, 59, 0, target.Get(t).Location())
				return target.Get(t).Add(t.Random.DurationBetween(time.Second, dayEnd.Sub(target.Get(t))))
			})

			s.Then("the resulting day fiff will be zero", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})
	})

	s.When("the compared times are equal", func(s *testcase.Spec) {
		ref.Let(s, let.Time(s).Get)
		target.Let(s, ref.Get)

		s.Then("days diff will be zero", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})
	})
}

func TestSchedule(t *testing.T) {
	ScheduleSpec{}.Test(t)
}

type ScheduleSpec struct{}

func (suite ScheduleSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Describe("#Next", suite.specNext)
	s.Describe("#Near", suite.specNear)
	s.Context("implements", func(s *testcase.Spec) {
		// s.Context("timekit.Interval", suite.implementsInterval)
	})
}

func (spec ScheduleSpec) specNext(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return t.Random.Time()
	})

	var schedule = let.Var(s, func(t *testcase.T) timekit.Schedule {
		return timekit.Schedule{}
	})

	s.Before(func(t *testcase.T) {
		t.OnFail(func() {
			t.LogPretty("ref", ref.Get(t))
			t.LogPretty("schedule", schedule.Get(t))
		})
	})

	act := let.Act2(func(t *testcase.T) (timekit.Range, bool) {
		t.Helper()
		var (
			next timekit.Range
			ok   bool
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			next, ok = schedule.Get(t).Next(ref.Get(t))
		})
		t.OnFail(func() {
			t.LogPretty(next, ok)
		})
		return next, ok
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, validate.Value(t.Context(), schedule.Get(t)), "sanity check")
	})

	s.Test("A zero Schedule should still be valid and yield a next occurence related to a reference time", func(t *testcase.T) {
		got, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, got)
		assert.True(t, ref.Get(t).Before(got.From))
	})

	s.Test("A zero Schedule means 0-7/0-24", func(t *testcase.T) {
		got, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, got)

		t.Log("so the next occurence will be the next day related to the reference time")
		loc := zerokit.Coalesce(schedule.Get(t).Location, ref.Get(t).Location())

		expFrom := time.Date(
			ref.Get(t).Year(), ref.Get(t).Month(), ref.Get(t).Day()+1,
			schedule.Get(t).DayTime.Hour, schedule.Get(t).DayTime.Minute, 0, 0, loc)

		assert.Equal(t, expFrom, got.From)
	})

	s.When(".From is provided", func(s *testcase.Spec) {
		from := let.VarOf(s, timekit.DayTime{Hour: 12, Minute: 00})

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.DayTime = from.Get(t)
			return sch
		})

		s.And("the ref time takes place prior to from", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				rf := ref.Super(t)
				year, month, day := rf.Date()
				_, min, sec := rf.Clock()
				return time.Date(year, month, day, from.Get(t).Hour-1, min, sec, rf.Nanosecond(), rf.Location())
			})

			s.Then("then occurence starts from the defined .From time", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)

				assert.Equal(t, got.From.Hour(), from.Get(t).Hour)
				assert.Equal(t, got.From.Minute(), from.Get(t).Minute)
			})

			s.Then("then occurence is after the ref time", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.True(t, ref.Get(t).Before(got.From))
			})
		})
	})

	s.When("schedule defines the month", func(s *testcase.Spec) {
		month := let.OneOf(s, timekit.Months()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Months = []time.Month{month.Get(t)}
			t.OnFail(func() {
				t.LogPretty(sch)
			})
			return sch
		})

		s.Then("month will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(1, 3, func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Month(), month.Get(t))

				if t.Random.Bool() {
					ref.Set(t, got.Till.AddDate(0, 0, 1))
					return
				}
				ref.Set(t, got.Till.Add(1))
			})
		})
	})

	s.When("schedule defines the weekday", func(s *testcase.Spec) {
		weekday := let.OneOf(s, timekit.Weekdays()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Weekdays = []time.Weekday{weekday.Get(t)}
			return sch
		})

		s.Test("weekday will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)
			assert.NotEmpty(t, got)
			assert.Equal(t, got.From.Weekday(), weekday.Get(t))
		})
	})

	s.When("schedule defines the day", func(s *testcase.Spec) {
		day := let.IntB(s, 1, 25)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Days = []int{day.Get(t)}
			return sch
		})

		s.Test("day will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(1, 3, func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Day(), day.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the location", func(s *testcase.Spec) {
		location := let.Var(s, func(t *testcase.T) *time.Location {
			timeGMT, err := time.LoadLocation("GMT")
			assert.NoError(t, err)
			return random.Pick(t.Random, time.Local, time.UTC, timeGMT)
		})

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Location = location.Get(t)
			return sch
		})

		s.Test("location is used as part of the next occurence calculation", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Location(), location.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})

		// TODO add more coverage for edge cases with location.
	})

	s.When("current time is within the availability policy", func(s *testcase.Spec) {
		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 15, 30, 45),
			}
			return timekit.Schedule{
				DayTime:  from,
				Duration: random.Pick[time.Duration](t.Random, 0, 15, 30, 45) * time.Minute,
				Location: time.Local,
			}
		})

		ref.Let(s, func(t *testcase.T) time.Time {
			super := ref.Super(t)
			sch := schedule.Get(t)
			loc := sch.Location
			hour := t.Random.IntBetween(sch.DayTime.Hour,
				sch.DayTime.Hour+int(sch.Duration/time.Hour))
			return time.Date(super.Year(), super.Month(), super.Day(),
				hour, sch.DayTime.Minute, t.Random.IntBetween(0, 59),
				0, loc)
		})

		s.Then("the next occurence is returned", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)

			from := schedule.Get(t).DayTime.
				ToTimeRelTo(ref.Get(t)).
				AddDate(0, 0, 1) // next day

			assert.Equal(t, timekit.Range{
				From: from,
				Till: from.Add(schedule.Get(t).Duration),
			}, got, "exp | got")
		})

		s.Then("the the next occurence doesn't include the current ref time", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)
			assert.False(t, got.Contain(ref.Get(t)))
			assert.True(t, got.From.After(ref.Get(t)))
		})
	})

	s.When("current time is before the next occurence", func(s *testcase.Spec) {
		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 30),
			}
			weekday := timekit.ShiftWeekday(ref.Get(t).Weekday(), 1)
			return timekit.Schedule{
				Weekdays: []time.Weekday{weekday},
				DayTime:  from,
				Duration: random.Pick[time.Duration](t.Random, 0, 15, 30) * time.Minute,
				Location: ref.Get(t).Location(),
			}
		})

		s.Then("next occurence is given back", func(t *testcase.T) {
			loc := schedule.Get(t).Location
			from := schedule.Get(t).DayTime.ToTimeRelTo(ref.Get(t).AddDate(0, 0, 1).In(loc))
			till := from.Add(schedule.Get(t).Duration)
			exp := timekit.Range{From: from, Till: till}

			got, ok := act(t)
			assert.True(t, ok)
			assert.Equal(t, exp, got)
		})
	})
}

func (spec ScheduleSpec) specNear(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return t.Random.Time()
	})

	var schedule = let.Var(s, func(t *testcase.T) timekit.Schedule {
		return timekit.Schedule{}
	})

	s.Before(func(t *testcase.T) {
		t.OnFail(func() {
			t.LogPretty("ref", ref.Get(t))
			t.LogPretty("schedule", schedule.Get(t))
		})
	})

	act := let.Act2(func(t *testcase.T) (timekit.Range, bool) {
		t.Helper()
		var (
			near timekit.Range
			ok   bool
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			near, ok = schedule.Get(t).Near(ref.Get(t))
		})
		t.OnFail(func() {
			t.LogPretty(near, ok)
		})
		return near, ok
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, validate.Value(t.Context(), schedule.Get(t)), "sanity check")
	})

	s.Test("A zero Schedule should still be valid and yield a near occurence related to a reference time", func(t *testcase.T) {
		got, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, got)
		assert.True(t, ref.Get(t).Before(got.From))
	})

	s.Test("A zero Schedule means 0-7/0-24", func(t *testcase.T) {
		got, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, got)

		t.Log("so the near occurence will be the near day related to the reference time")
		loc := zerokit.Coalesce(schedule.Get(t).Location, ref.Get(t).Location())

		expFrom := time.Date(
			ref.Get(t).Year(), ref.Get(t).Month(), ref.Get(t).Day()+1,
			schedule.Get(t).DayTime.Hour, schedule.Get(t).DayTime.Minute, 0, 0, loc)

		assert.Equal(t, expFrom, got.From)
	})

	s.When(".From is provided", func(s *testcase.Spec) {
		from := let.VarOf(s, timekit.DayTime{Hour: 12, Minute: 00})

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.DayTime = from.Get(t)
			return sch
		})

		s.And("the ref time takes place prior to from", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				rf := ref.Super(t)
				year, month, day := rf.Date()
				_, min, sec := rf.Clock()
				return time.Date(year, month, day, from.Get(t).Hour-1, min, sec, rf.Nanosecond(), rf.Location())
			})

			s.Then("then occurence starts from the defined .From time", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)

				assert.Equal(t, got.From.Hour(), from.Get(t).Hour)
				assert.Equal(t, got.From.Minute(), from.Get(t).Minute)
			})

			s.Then("then occurence is after the ref time", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.True(t, ref.Get(t).Before(got.From))
			})
		})
	})

	s.When("schedule defines the month", func(s *testcase.Spec) {
		month := let.OneOf(s, timekit.Months()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Months = []time.Month{month.Get(t)}
			t.OnFail(func() {
				t.LogPretty(sch)
			})
			return sch
		})

		s.Then("month will be used as a constraing to retrieve the near occurence", func(t *testcase.T) {
			t.Random.Repeat(1, 3, func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Month(), month.Get(t))

				if t.Random.Bool() {
					ref.Set(t, got.Till.AddDate(0, 0, 1))
					return
				}
				ref.Set(t, got.Till.Add(1))
			})
		})
	})

	s.When("schedule defines the weekday", func(s *testcase.Spec) {
		weekday := let.OneOf(s, timekit.Weekdays()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Weekdays = []time.Weekday{weekday.Get(t)}
			return sch
		})

		s.Test("weekday will be used as a constraing to retrieve the near occurence", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)
			assert.NotEmpty(t, got)
			assert.Equal(t, got.From.Weekday(), weekday.Get(t))
		})
	})

	s.When("schedule defines the day", func(s *testcase.Spec) {
		day := let.IntB(s, 1, 25)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Days = []int{day.Get(t)}
			return sch
		})

		s.Test("day will be used as a constraing to retrieve the near occurence", func(t *testcase.T) {
			t.Random.Repeat(1, 3, func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Day(), day.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the location", func(s *testcase.Spec) {
		location := let.Var(s, func(t *testcase.T) *time.Location {
			timeGMT, err := time.LoadLocation("GMT")
			assert.NoError(t, err)
			return random.Pick(t.Random, time.Local, time.UTC, timeGMT)
		})

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Location = location.Get(t)
			return sch
		})

		s.Test("location is used as part of the near occurence calculation", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Location(), location.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})

		// TODO add more coverage for edge cases with location.
	})

	s.When("current time is within the availability policy", func(s *testcase.Spec) {
		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 15, 30, 45),
			}
			return timekit.Schedule{
				DayTime:  from,
				Duration: random.Pick[time.Duration](t.Random, 15, 30, 45) * time.Minute,
				Location: time.Local,
			}
		})

		ref.Let(s, func(t *testcase.T) time.Time {
			super := ref.Super(t)
			sch := schedule.Get(t)
			loc := sch.Location
			return time.Date(super.Year(), super.Month(), super.Day(),
				sch.DayTime.Hour,
				sch.DayTime.Minute,
				t.Random.IntBetween(0, int(sch.Duration/time.Second)),
				0, loc)
		})

		s.Then("the current range is returned", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)

			exp := timekit.Range{}
			exp.From = schedule.Get(t).DayTime.ToTimeRelTo(ref.Get(t))
			exp.Till = exp.From.Add(schedule.Get(t).Duration)
			assert.Equal(t, exp, got, "exp | got")
			assert.True(t, got.Contain(ref.Get(t)))
		})

		s.Then("the the near occurence does include the current ref time", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)
			assert.True(t, got.Contain(ref.Get(t)))
			assert.False(t, got.From.After(ref.Get(t)))
		})
	})

	s.When("current time is before the near occurence", func(s *testcase.Spec) {
		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 30),
			}
			weekday := timekit.ShiftWeekday(ref.Get(t).Weekday(), 1)
			return timekit.Schedule{
				Weekdays: []time.Weekday{weekday},
				DayTime:  from,
				Duration: random.Pick[time.Duration](t.Random, 0, 15, 30) * time.Minute,
				Location: ref.Get(t).Location(),
			}
		})

		s.Then("near occurence is given back", func(t *testcase.T) {
			loc := schedule.Get(t).Location
			from := schedule.Get(t).DayTime.ToTimeRelTo(ref.Get(t).AddDate(0, 0, 1).In(loc))
			till := from.Add(schedule.Get(t).Duration)
			exp := timekit.Range{From: from, Till: till}

			got, ok := act(t)
			assert.True(t, ok)
			assert.Equal(t, exp, got)
		})
	})
}

func TestRange(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		From = let.Time(s)
		Till = let.Var(s, func(t *testcase.T) time.Time {
			return From.Get(t).Add(t.Random.DurationBetween(0, 48*time.Hour))
		})
	)
	subject := let.Var(s, func(t *testcase.T) timekit.Range {
		return timekit.Range{
			From: From.Get(t),
			Till: Till.Get(t),
		}
	})

	s.Describe("Validate", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Validate(ctx.Get(t))
		})

		s.Test("for a valid range, error is not returned", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When(".Till is before .From", func(s *testcase.Spec) {
			Till.Let(s, func(t *testcase.T) time.Time {
				return From.Get(t).Add(t.Random.DurationBetween(-time.Hour, -time.Second))
			})

			s.Then("error returned", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})

	s.Describe("#Contain", func(s *testcase.Spec) {
		var (
			ref = let.Var[time.Time](s, nil)
		)
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).Contain(ref.Get(t))
		})

		s.When("ref time within time range", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				return t.Random.TimeBetween(subject.Get(t).From, subject.Get(t).Till)
			})

			s.Then("it will comtain it", func(t *testcase.T) {
				assert.True(t, act(t))
			})

			s.Context("as reference time is equal to the range's from time", func(s *testcase.Spec) {
				ref.Let(s, func(t *testcase.T) time.Time {
					return subject.Get(t).From
				})

				s.Then("range contains the ref time", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})

			s.Context("as reference time is equal to the range's till time", func(s *testcase.Spec) {
				ref.Let(s, func(t *testcase.T) time.Time {
					return subject.Get(t).Till
				})

				s.Then("range contains the ref time", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})
		})

		s.When("ref time is before to the range", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				return t.Random.TimeBetween(subject.Get(t).From, subject.Get(t).Till)
			})

			s.Then("it will comtain it", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})
}

func ExampleDuration() {
	var d timekit.Duration
	d = d.Between(time.Now(), time.Now().AddDate(1000, 0, 0)) // 1000 years worth of duration relative to time now.
	_ = d.String()
}

func TestDuration(t *testing.T) {
	s := testcase.NewSpec(t)

	duration := let.Var(s, func(t *testcase.T) timekit.Duration {
		return timekit.Duration{}
	})

	var assertDurationByTimeDuration = func(t *testcase.T, tkd timekit.Duration, td time.Duration) {
		t.Helper()
		var diff = td
		for dur := range tkd.Iter() {
			diff -= dur
		}
		assert.Empty(t, diff, "expected that difference is zero")
	}

	var assertDurationByDates = func(t *testcase.T, duration timekit.Duration, from, till time.Time) {
		t.Helper()
		var cursor = from
		for dur := range duration.Iter() {
			cursor = cursor.Add(dur)
		}
		assert.Equal(t, cursor, till)
	}

	s.Describe("#ByDuration", func(s *testcase.Spec) {
		var (
			timeDuration = let.DurationBetween(s, 0, 365*24*time.Hour)
		)
		act := let.Act(func(t *testcase.T) timekit.Duration {
			return duration.Get(t).ByDuration(timeDuration.Get(t))
		})

		s.Then("non-empty distance returned", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.Then("result equals to the time duration used for construction", func(t *testcase.T) {
			assertDurationByTimeDuration(t, act(t), timeDuration.Get(t))
		})

		s.When("duration is zero", func(s *testcase.Spec) {
			timeDuration.LetValue(s, 0)

			s.Then("zero distance returned", func(t *testcase.T) {
				got := act(t)
				assert.Empty(t, got)
				assert.True(t, got.IsZero())
			})
		})

		s.When("negative duration is given", func(s *testcase.Spec) {
			timeDuration.Let(s, func(t *testcase.T) time.Duration {
				return timeDuration.Super(t) * -1
			})

			s.Then("result equals to the time duration used for construction", func(t *testcase.T) {
				assertDurationByTimeDuration(t, act(t), timeDuration.Get(t))
			})

			s.Then("negative duration is returned", func(t *testcase.T) {
				assert.True(t, act(t).Compare(timekit.Duration{}) < 0)
			})
		})

		s.When("max duration (int64) is given", func(s *testcase.Spec) {
			timeDuration.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(math.MaxInt64)
			})

			s.Then("result equals to the time duration used for construction", func(t *testcase.T) {
				assertDurationByTimeDuration(t, act(t), timeDuration.Get(t))
			})
		})
	})

	s.Describe("#Parse", func(s *testcase.Spec) {
		var (
			raw = let.Var[string](s, nil)
		)
		act := let.Act2(func(t *testcase.T) (timekit.Duration, error) {
			return duration.Get(t).Parse(raw.Get(t))
		})

		s.When("raw is in a correct format", func(s *testcase.Spec) {
			val := let.Var(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}.Between(time.Now(), time.Now().AddDate(t.Random.IntBetween(1, 1000), 0, 0))
			})

			raw.Let(s, func(t *testcase.T) string {
				return val.Get(t).String()
			})

			s.Then("it parses successfully", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, val.Get(t))
			})
		})

		s.When("value is invalid", func(s *testcase.Spec) {
			raw.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(5, random.CharsetAlpha())
			})

			s.Then("it returns back an error", func(t *testcase.T) {
				_, err := act(t)

				assert.ErrorIs(t, err, timekit.ErrParseDuration)
			})
		})
	})

	s.Describe("#Between", func(s *testcase.Spec) {
		var (
			A = let.Time(s)
			B = let.Time(s)
		)
		act := let.Act(func(t *testcase.T) timekit.Duration {
			a, b := A.Get(t), B.Get(t)
			d := duration.Get(t).Between(a, b)
			t.OnFail(func() {
				t.LogPretty(a, "A")
				t.LogPretty(b, "B")
				t.LogPretty(d, "Delta")
			})
			return d
		})

		// In this case, where we need stability, we'll use fix times to simplify things.
		// Using flexible dates could get complicated
		// because different years have varying numbers of days,
		// which might make the test harder to manage than needed.
		fixA := let.VarOf(s, time.Date(2025, time.April, 14, 11, 53, 49, 0, time.Local))

		s.Then("non-empty distance returned", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.Then("original delta is not affected", func(t *testcase.T) {
			assert.Empty(t, duration.Get(t))
		})

		var ThenDeltaPlusFirstEqualsSecond = func(s *testcase.Spec) {
			s.Then("adding the result delta to first ref time argumentref will yield the second ref time as a result", func(t *testcase.T) {
				delta := act(t)
				aPlusDelta := delta.AddTo(A.Get(t))

				t.OnFail(func() {
					t.LogPretty(aPlusDelta, "A + delta")
				})

				assert.Equal(t, aPlusDelta, B.Get(t))
			})
		}

		ThenDeltaPlusFirstEqualsSecond(s)

		s.When("the delta between the two time is zero", func(s *testcase.Spec) {
			B.Let(s, A.Get)

			s.Then("zero distance returned", func(t *testcase.T) {
				got := act(t)
				assert.Empty(t, got)
				assert.True(t, got.IsZero())
			})

			ThenDeltaPlusFirstEqualsSecond(s)
		})

		s.When("the delta between A and B is negative", func(s *testcase.Spec) {
			A.Let(s, fixA.Get)

			B.Let(s, func(t *testcase.T) time.Time {
				return A.Get(t).Add(t.Random.DurationBetween(-1, -math.MaxInt64))
			})

			ThenDeltaPlusFirstEqualsSecond(s)

			s.Context("by years", func(s *testcase.Spec) {
				var years = let.IntB(s, -500, -1000)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(years.Get(t), 0, 0)
				})

				ThenDeltaPlusFirstEqualsSecond(s)

			})

			s.Context("by months", func(s *testcase.Spec) {
				var months = let.IntB(s, -1, -11)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(0, months.Get(t), 0)
				})

				ThenDeltaPlusFirstEqualsSecond(s)
			})

			s.Context("by days", func(s *testcase.Spec) {
				var days = let.IntB(s, -1, -25)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(0, 0, days.Get(t))
				})

				ThenDeltaPlusFirstEqualsSecond(s)
			})

			s.Context("by something smaller than a day", func(s *testcase.Spec) {
				var rem = let.DurationBetween(s, 0, -1*23*time.Hour+59*time.Minute+59*time.Second)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).Add(rem.Get(t))
				})

				ThenDeltaPlusFirstEqualsSecond(s)

				s.Then("negative remainder is returned", func(t *testcase.T) {
					assertDurationByDates(t, act(t), A.Get(t), B.Get(t))
				})
			})
		})

		s.When("max duration (int64) is given", func(s *testcase.Spec) {
			A.Let(s, fixA.Get)

			B.Let(s, func(t *testcase.T) time.Time {
				return A.Get(t).Add(math.MaxInt64)
			})

			s.Then("result distance's ToDuration is equal to the max int64 duration", func(t *testcase.T) {
				assertDurationByTimeDuration(t, act(t), math.MaxInt64)
			})

			ThenDeltaPlusFirstEqualsSecond(s)
		})

		s.When("delta is between the two is bigger than what time.Duration can express", func(s *testcase.Spec) {
			var (
				years  = let.IntB(s, 500, 1000)
				months = let.IntB(s, 1, 11)
				days   = let.IntB(s, 1, 25)
				rem    = let.DurationBetween(s, 0, 23*time.Hour+59*time.Minute+59*time.Second)
			)

			B.Let(s, func(t *testcase.T) time.Time {
				return A.Get(t).
					AddDate(years.Get(t), months.Get(t), days.Get(t)).
					Add(rem.Get(t))
			})

			ThenDeltaPlusFirstEqualsSecond(s)
		})
	})

	s.Describe("#Compare", func(s *testcase.Spec) {
		var (
			oth = let.Var(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}.AddDuration(t.Random.DurationBetween(
					time.Nanosecond,
					math.MaxInt64,
				))
			})
		)
		act := let.Act(func(t *testcase.T) int {
			return duration.Get(t).Compare(oth.Get(t))
		})

		s.When("both is duration is zero value", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}
			})
			oth.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}
			})

			s.Then("values are equal", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})

		s.When("both is duration is equal", func(s *testcase.Spec) {
			duration.Let(s, oth.Get)

			s.Then("values are equal", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})

		s.When("duration is greater than other duration", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return oth.Get(t).AddDuration(t.Random.DurationB(time.Second, 1000*time.Hour))
			})

			s.Then("results indicates that duration is more compared to the other value", func(t *testcase.T) {
				assert.True(t, 0 < act(t))
			})
		})

		s.When("duration is lesser than other duration", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return oth.Get(t).AddDuration(t.Random.DurationB(time.Second, 1000*time.Hour) * -1)
			})

			s.Then("results indicates that duration is less compared to the other value", func(t *testcase.T) {
				assert.True(t, act(t) < 0)
			})
		})
	})

	s.Describe("#AddDuration", func(s *testcase.Spec) {
		var timeDuration = let.DurationBetween(s, time.Nanosecond, 48*time.Hour)

		act := let.Act(func(t *testcase.T) timekit.Duration {
			return duration.Get(t).AddDuration(timeDuration.Get(t))
		})

		s.Then("a non-empty delta returned", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.Then("returned delta is bigger than initial", func(t *testcase.T) {
			got := act(t)

			assert.Equal(t, duration.Get(t).Compare(got), -1)
		})

		s.Then("the duration difference between the two delta exactly as much was added to the delta", func(t *testcase.T) {
			assertDurationByTimeDuration(t, act(t), timeDuration.Get(t))
		})

		s.When("the duration is negative", func(s *testcase.Spec) {
			timeDuration.Let(s, func(t *testcase.T) time.Duration {
				d := timeDuration.Super(t)
				return -1 * d
			})

			s.Then("the returned delta is smaller than the initial", func(t *testcase.T) {
				got := act(t)

				assert.Equal(t, duration.Get(t).Compare(got), 1)
			})
		})

		s.Test("smoke", func(t *testcase.T) {
			var (
				start = t.Random.Time()
				end   = t.Random.TimeBetween(start,
					start.AddDate(
						t.Random.IntBetween(1000, 2000),
						t.Random.IntBetween(1, 100),
						t.Random.IntBetween(1, 100),
					))
				cursor = start
				delta  = timekit.Duration{}
			)
			for {
				remaining := end.Sub(cursor)
				if remaining <= 0 {
					break
				}
				cursor = cursor.Add(remaining)
				delta = delta.AddDuration(remaining)

				assert.True(t, delta.AddTo(start).Equal(cursor),
					assert.MessageF("diff: %s", cursor.Sub(delta.AddTo(start))))
			}
			assert.True(t, delta.AddTo(start).Equal(end),
				assert.MessageF("diff between Start+Delta and End: %s", end.Sub(delta.AddTo(start))))
		})

		s.Test("1000 year length", func(t *testcase.T) {
			var (
				begin = time.Date(2025, time.May, 10, 16, 16, 47, 0, time.UTC)
				end   = begin.AddDate(1000, 0, 0)
			)

			var (
				d = timekit.Duration{}
				c = begin
			)
			for {
				remaining := end.Sub(c)
				if remaining == 0 {
					break
				}

				c = c.Add(remaining)
				d = d.AddDuration(remaining)
			}

			t.LogPretty(begin, "begin")
			t.LogPretty(d.AddTo(begin), "begin+delta")
			t.LogPretty(end, "end")

			assert.True(t, d.AddTo(begin).Equal(end), assert.MessageF("diff: %s", end.Sub(d.AddTo(begin))))

		})
	})

	s.Describe("#AddTo", func(s *testcase.Spec) {
		var ref = let.Time(s)
		act := let.Act(func(t *testcase.T) time.Time {
			return duration.Get(t).AddTo(ref.Get(t))
		})

		s.When("duration is zero", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}
			})

			s.Then("same time returned", func(t *testcase.T) {
				assert.Equal(t, ref.Get(t), act(t))
			})
		})

		s.When("duration is negative", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}.AddDuration(t.Random.DurationBetween(time.Second, 1000*time.Hour) * -1)
			})

			s.Then("the returned time is earlier than the input", func(t *testcase.T) {
				assert.True(t, 0 < ref.Get(t).Compare(act(t)))
			})
		})

		s.When("duration is positive", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{}.AddDuration(t.Random.DurationBetween(time.Second, 1000*time.Hour))
			})

			s.Then("the returned time is later than the input", func(t *testcase.T) {
				assert.True(t, ref.Get(t).Compare(act(t)) < 0)
			})
		})
	})
}

func Test_iso8601(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("seconds", func(t *testcase.T) {
		// Use case: Simple systems, audit logs, or APIs that don't require sub-second accuracy
		tm := t.Random.Time() // ← Your global t.Random.Time()
		expected := tm.Truncate(time.Second)

		formatted := expected.Format(timekit.ISO8601)
		parsed, err := time.Parse(timekit.ISO8601, formatted)
		assert.NoError(t, err, "should parse ISO 8601 seconds format without error")
		assert.Equal(t, expected, parsed, "parsed time should match original after roundtrip at seconds precision")
	})

	s.Test("milliseconds", func(t *testcase.T) {
		// Use case: Most common in web applications (JavaScript, frontend, REST), balances precision and readability
		tm := t.Random.Time() // ← Your global t.Random.Time()
		expected := tm.Truncate(time.Millisecond)

		formatted := expected.Format(timekit.ISO8601Milli)
		parsed, err := time.Parse(timekit.ISO8601Milli, formatted)
		assert.NoError(t, err, "should parse ISO 8601 millisecond format without error")
		assert.Equal(t, expected, parsed, "parsed time should match original after roundtrip at millisecond precision")
	})

	s.Test("microseconds", func(t *testcase.T) {
		// Use case: Systems like PostgreSQL, Kafka, or internal event stores needing finer granularity than ms
		tm := t.Random.Time() // ← Your global t.Random.Time()
		expected := tm.Truncate(time.Microsecond)

		formatted := expected.Format(timekit.ISO8601Micro)
		parsed, err := time.Parse(timekit.ISO8601Micro, formatted)
		assert.NoError(t, err, "should parse ISO 8601 microsecond format without error")
		assert.Equal(t, expected, parsed, "parsed time should match original after roundtrip at microsecond precision")
	})

	s.Test("nanoseconds", func(t *testcase.T) {
		// Use case: Go’s native time precision; used in internal systems, benchmarks, financial tick data
		tm := t.Random.Time()                    // ← Your global t.Random.Time()
		expected := tm.Truncate(time.Nanosecond) // Go time is nanosecond-precise by default

		formatted := expected.Format(timekit.ISO8601Nano)
		parsed, err := time.Parse(timekit.ISO8601Nano, formatted)
		assert.NoError(t, err, "should parse ISO 8601 nanosecond format without error")
		assert.Equal(t, expected, parsed, "parsed time should match original after roundtrip at nanosecond precision")
	})
}

func Test_rfc5424(t *testing.T) {
	s := testcase.NewSpec(t)

	// Validate that RFC5424 is defined as ISO8601Milli — sanity check
	s.Test("RFC5424 is defined as ISO8601Milli", func(t *testcase.T) {
		assert.Equal(t, timekit.ISO8601Milli, timekit.RFC5424,
			"RFC5424 must be exactly ISO8601Milli to comply with RFC 5424's mandatory millisecond precision")
	})

	s.Test("RFC5424 requires exactly 3 fractional digits (milliseconds)", func(t *testcase.T) {
		// Use case: RFC 5424 mandates exactly 3 digits after decimal — no more, no less
		tm := t.Random.Time()
		expected := tm.Truncate(time.Millisecond)

		formatted := expected.Format(timekit.RFC5424) // == ISO8601Milli

		// Parse the formatted string back to ensure it's valid
		parsed, err := time.Parse(timekit.RFC5424, formatted)
		assert.NoError(t, err, "RFC5424 format must be parseable")

		// Assert parsed time matches original
		assert.Equal(t, expected, parsed, "parsed RFC5424 time must match original truncated time")

		// Now manually validate the format string: it MUST have exactly 3 fractional digits
		// Example: "2024-06-15T13:45:30.123Z" — exactly 3 digits after '.'
		pattern := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[Z+\-]`
		matched, err := regexp.MatchString(pattern, formatted)
		assert.NoError(t, err, "regex compilation failed")
		assert.True(t, matched, "RFC5424 formatted string must have exactly 3 fractional digits and valid timezone suffix")

		// Also ensure there are NOT more than 3
		assert.NotContains(t, formatted, ".0000", "RFC5424 must NOT contain 4+ fractional digits")
	})

	s.Test("RFC5424 requires mandatory timezone (Z or ±HH:MM)", func(t *testcase.T) {
		// Use case: RFC 5424 requires explicit time zone — no local time or omitted offset
		tm := t.Random.Time()
		expected := tm.Truncate(time.Millisecond)

		formatted := expected.Format(timekit.RFC5424)

		// Must end with Z or +HH:MM or -HH:MM
		assert.True(t, strings.HasSuffix(formatted, "Z") ||
			strings.HasPrefix(formatted[len(formatted)-6:], "+") || strings.HasPrefix(formatted[len(formatted)-6:], "-"),
			assert.MessageF("RFC5424 timestamp must end in 'Z' or '+HH:MM' or '-HH:MM', got %s", formatted))

		// Ensure the timezone offset is in correct format: ±HH:MM (length 6 if not Z)
		if !strings.HasSuffix(formatted, "Z") {
			assert.Equal(t, len(formatted), 29, "RFC5424 with offset must be exactly 29 chars (e.g., '2006-01-02T15:04:05.000+07:00')")
			assert.Equal(t, ":", string(formatted[len(formatted)-3]), "offset must use colon separator (HH:MM)")
		}
	})

	s.Test("RFC5424 does NOT allow omission of milliseconds", func(t *testcase.T) {
		// Use case: RFC 5424 explicitly requires milliseconds — omitting them is non-compliant
		tm := t.Random.Time()
		expected := tm.Truncate(time.Millisecond)

		formatted := expected.Format(timekit.RFC5424)
		// Format without milliseconds would be: "2006-01-02T15:04:05Z" — which is NOT RFC5424
		assert.Contains(t, formatted, ".", "RFC5424 must include fractional seconds (dot and milliseconds)")
	})

	s.Test("RFC5424 does NOT allow 6-digit microseconds", func(t *testcase.T) {
		// Use case: Some systems output 6 digits — this is INVALID under RFC5424
		tm := t.Random.Time()
		formattedMicros := tm.Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000Z07:00")

		// Try to parse it as RFC5424 — should FAIL
		_, err := time.Parse(timekit.RFC5424, formattedMicros)
		assert.Error(t, err, "RFC5424 parser must reject 6-digit microsecond precision")

		// Also verify the format contains 6 digits
		assert.Contains(t, formattedMicros, ".", "test setup failed: no decimal point")
		count := strings.Count(formattedMicros, ".")
		assert.Equal(t, 1, count, "test setup failed: too many decimal points")

		// Find digits after dot
		parts := strings.Split(formattedMicros, ".")
		assert.Equal(t, len(parts), 2)
		frac := parts[1]
		idx := strings.Index(frac, "Z")
		if idx == -1 {
			idx = strings.Index(frac, "+")
			if idx == -1 {
				idx = strings.Index(frac, "-")
			}
		}
		assert.Equal(t, 6, idx, "microsecond format must have exactly 6 fractional digits — this is invalid for RFC5424")
	})

}
