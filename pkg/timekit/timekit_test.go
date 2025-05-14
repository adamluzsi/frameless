package timekit_test

import (
	"context"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/timekit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/zerokit"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
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

	s.Test("normalise", func(t *testcase.T) {
		norms := []timekit.Duration{
			{Minute: 1},
		}

		for _, d := range norms {
			assert.Equal(t, d, d.Normalise())
		}
	})

	s.Test("Add a minute of time duration", func(t *testcase.T) {
		od := timekit.Duration{}
		nd := od.AddDuration(time.Minute)
		assert.NotEqual(t, od, nd)
		assert.Equal(t, nd.Minute, 1)
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

	s.Test("normalise", func(t *testcase.T) {
		var d timekit.Duration
		d = d.AddDuration(math.MaxInt64)

		var get = func() *big.Int {
			got := big.NewInt(0)

			got = got.Add(got, big.NewInt(0).Mul(big.NewInt(int64(d.Hour)), big.NewInt(int64(time.Hour))))
			got = got.Add(got, big.NewInt(0).Mul(big.NewInt(int64(d.Minute)), big.NewInt(int64(time.Minute))))
			got = got.Add(got, big.NewInt(0).Mul(big.NewInt(int64(d.Second)), big.NewInt(int64(time.Second))))
			got = got.Add(got, big.NewInt(0).Mul(big.NewInt(int64(d.Nanosecond)), big.NewInt(int64(time.Second))))
			return got
		}

		exp := big.NewInt(0)
		exp = exp.Add(exp, big.NewInt(math.MaxInt64))

		assert.Equal(t, d.Hour, 2562047)

		assert.Equal(t, math.MaxInt64,
			time.Duration(d.Hour)*time.Hour+
				time.Duration(d.Minute)*time.Minute+
				time.Duration(d.Second)*time.Second+
				time.Duration(d.Nanosecond))

		d = d.AddDuration(math.MaxInt64)

		exp = exp.Add(exp, big.NewInt(math.MaxInt64))

		assert.Equal(t, exp, got)
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
		assert.Contain(t, timekit.Weekdays(), act(t))
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
		assert.Contain(t, timekit.Months(), act(t))
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
	s.Context("implements", func(s *testcase.Spec) {
		// s.Context("timekit.Interval", suite.implementsInterval)
	})
}

// func (spec ScheduleSpec) implementsInterval(s *testcase.Spec) {
// 	var ref = let.Var(s, func(t *testcase.T) time.Time {
// 		return t.Random.Time()
// 	})

// 	schedule := let.Var(s, func(t *testcase.T) timekit.Schedule {
// 		return timekit.Schedule{}
// 	})

// 	act := let.Act(func(t *testcase.T) time.Duration {
// 		return schedule.Get(t).UntilNext(ref.Get(t))
// 	})

// 	s.When("schedule is unspecified", func(s *testcase.Spec) {
// 		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
// 			return timekit.Schedule{}
// 		})

// 		s.Then("it will yield back zero", func(t *testcase.T) {
// 			assert.Equal(t, 0, act(t))
// 		})
// 	})

// 	s.When("schedule defines day-time", func(s *testcase.Spec) {
// 		dayTime := let.Var(s, func(t *testcase.T) timekit.DayTime {
// 			return timekit.DayTime{
// 				Hour: t.Random.IntBetween(9, 12),
// 			}
// 		})

// 		duration := let.DurationBetween(s, time.Minute, time.Hour)

// 		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
// 			sch := schedule.Super(t)
// 			sch.DayTime = dayTime.Get(t)
// 			sch.Duration = duration.Get(t)
// 			return sch
// 		})

// 		s.And("ref time is at the day-time", func(s *testcase.Spec) {
// 			ref.Let(s, func(t *testcase.T) time.Time {
// 				return dayTime.Get(t).ToTimeRelTo(ref.Super(t))
// 			})

// 			s.Then("until is time is zero", func(t *testcase.T) {
// 				assert.Equal(t, 0, act(t))
// 			})
// 		})

// 		s.And("ref time is after the day-time (exl) but within the schedule's duration", func(s *testcase.Spec) {
// 			ref.Let(s, func(t *testcase.T) time.Time {
// 				from := dayTime.Get(t).ToTimeRelTo(ref.Super(t))
// 				return t.Random.TimeBetween(from.Add(1), from.Add(schedule.Get(t).Duration))
// 			})

// 			s.Then("time till next occurence received", func(t *testcase.T) {
// 				nextDayOccurence := dayTime.Get(t).ToTimeRelTo(ref.Get(t)).AddDate(0, 0, 1)

// 				assert.Equal(t, act(t), nextDayOccurence.Sub(ref.Get(t)))
// 			})
// 		})

// 		s.And("ref time is before the day-time", func(s *testcase.Spec) {
// 			diff := let.DurationBetween(s, time.Minute, time.Hour*3)

// 			ref.Let(s, func(t *testcase.T) time.Time {
// 				return dayTime.Get(t).ToTimeRelTo(ref.Super(t)).Add(diff.Get(t) * -1)
// 			})

// 			s.Then("time until the nearest occurence is retruned", func(t *testcase.T) {
// 				assert.Equal(t, act(t), diff.Get(t))
// 			})
// 		})

// 		s.And("ref time is after the end of the schedule's duration", func(s *testcase.Spec) {
// 			ref.Let(s, func(t *testcase.T) time.Time {
// 				from := dayTime.Get(t).ToTimeRelTo(ref.Super(t))
// 				return t.Random.TimeBetween(from.Add(1), from.Add(schedule.Get(t).Duration))
// 			})

// 			diff := let.DurationBetween(s, time.Minute, time.Hour*3)

// 			ref.Let(s, func(t *testcase.T) time.Time {
// 				return dayTime.Get(t).ToTimeRelTo(ref.Super(t)).Add(diff.Get(t))
// 			})

// 			s.Then("until is then point to the next day", func(t *testcase.T) {
// 				nextDayOccurence := dayTime.Get(t).ToTimeRelTo(ref.Get(t)).AddDate(0, 0, 1)

// 				assert.Equal(t, act(t), nextDayOccurence.Sub(ref.Get(t)))
// 			})
// 		})
// 	})
// }

func (spec ScheduleSpec) specNext(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return t.Random.Time()
	})

	var schedule = let.Var(s, func(t *testcase.T) timekit.ScheduleMVP {
		return timekit.ScheduleMVP{}
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
		assert.NoError(t, validate.Value(schedule.Get(t)), "sanity check")
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

		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
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

		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
			sch := schedule.Super(t)
			sch.Month = pointer.Of(month.Get(t))
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

		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
			sch := schedule.Super(t)
			sch.Weekday = pointer.Of(weekday.Get(t))
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

		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
			sch := schedule.Super(t)
			sch.Day = pointer.Of(day.Get(t))
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

		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
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
		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 15, 30, 45),
			}
			return timekit.ScheduleMVP{
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
		schedule.Let(s, func(t *testcase.T) timekit.ScheduleMVP {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 30),
			}
			weekday := timekit.ShiftWeekday(ref.Get(t).Weekday(), 1)
			return timekit.ScheduleMVP{
				Weekday:  &weekday,
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
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Validate()
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

func TestDelta(t *testing.T) {
	s := testcase.NewSpec(t)

	delta := let.Var(s, func(t *testcase.T) timekit.Duration {
		return timekit.Duration{}
	})

	var (
		maxNanosecond = int(math.Pow(10, 9)) - 1
		maxSecond     = 59
		maxMinute     = 59
		maxHour       = 23
		maxMonth      = 11
	)

	var ToDuration = func(d timekit.Duration, r time.Time) time.Duration {
		var total time.Time = r.
			AddDate(d.Year, d.Month, d.Day).
			Add(time.Hour * time.Duration(d.Hour)).
			Add(time.Minute * time.Duration(d.Minute)).
			Add(time.Second * time.Duration(d.Second)).
			Add(time.Nanosecond * time.Duration(d.Nanosecond))
		return total.Sub(r)
	}

	s.Describe("#ByDuration", func(s *testcase.Spec) {
		var (
			duration = let.DurationBetween(s, 0, 365*24*time.Hour)
		)
		act := let.Act(func(t *testcase.T) timekit.Duration {
			return delta.Get(t).ByDuration(duration.Get(t))
		})

		s.Then("non-empty distance returned", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.Then("result distance's ToDuration can return the duration used for construction", func(t *testcase.T) {
			assert.Equal(t, duration.Get(t), ToDuration(act(t), time.Time{}))
		})

		s.Then("result distance has normalised time intervals in its fields", func(t *testcase.T) {
			got := act(t)
			assert.True(t, 0 <= got.Day, "Day")
			assert.True(t, got.Hour <= maxHour, "Hour")
			assert.True(t, got.Minute <= maxMinute, "Minute")
			assert.True(t, got.Second <= maxSecond, "Second")
			assert.True(t, got.Nanosecond <= maxNanosecond, "Nanosecond")
		})

		s.When("duration is zero", func(s *testcase.Spec) {
			duration.LetValue(s, 0)

			s.Then("zero distance returned", func(t *testcase.T) {
				got := act(t)
				assert.Empty(t, got)
				assert.True(t, got.IsZero())
			})
		})

		s.When("negative duration is given", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) time.Duration {
				return duration.Super(t) * -1
			})

			s.Then("negative distance returned", func(t *testcase.T) {
				got := act(t)
				assert.False(t, got.IsPositive())
			})

			s.Then("ToDistance returns the original negative value", func(t *testcase.T) {
				assert.Equal(t, ToDuration(act(t), time.Time{}), duration.Get(t))
			})
		})

		s.When("max duration (int64) is given", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(math.MaxInt64)
			})

			s.Then("result distance's ToDuration is equal to the max int64 duration", func(t *testcase.T) {
				assert.Equal(t, ToDuration(act(t), time.Time{}), duration.Get(t))
			})

			s.Then("proper distance is calculated", func(t *testcase.T) {
				assert.Equal(t, act(t), timekit.Duration{
					Hour:       2562047,
					Minute:     47,
					Second:     16,
					Nanosecond: 854775807,
				})
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
			d := delta.Get(t).Between(a, b)
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
			assert.Empty(t, delta.Get(t))
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

		ThenResultDeltaIsNormalised(s, act)

		s.When("the delta between the two time is zero", func(s *testcase.Spec) {
			B.Let(s, A.Get)

			s.Then("zero distance returned", func(t *testcase.T) {
				got := act(t)
				assert.Empty(t, got)
				assert.True(t, got.IsZero())
			})

			ThenDeltaPlusFirstEqualsSecond(s)
			ThenResultDeltaIsNormalised(s, act)
		})

		s.When("the delta between A and B is negative", func(s *testcase.Spec) {
			A.Let(s, fixA.Get)

			B.Let(s, func(t *testcase.T) time.Time {
				return A.Get(t).Add(t.Random.DurationBetween(-1, -math.MaxInt64))
			})

			ThenDeltaPlusFirstEqualsSecond(s)

			ThenResultDeltaIsNormalised(s, act)

			s.Context("by years", func(s *testcase.Spec) {
				var years = let.IntB(s, -500, -1000)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(years.Get(t), 0, 0)
				})

				ThenDeltaPlusFirstEqualsSecond(s)

				ThenResultDeltaIsNormalised(s, act)

				s.Then("negative years are returned", func(t *testcase.T) {
					assert.Equal(t, years.Get(t), act(t).Year)
				})
			})

			s.Context("by months", func(s *testcase.Spec) {
				var months = let.IntB(s, -1, -11)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(0, months.Get(t), 0)
				})

				ThenDeltaPlusFirstEqualsSecond(s)

				ThenResultDeltaIsNormalised(s, act)

				s.Then("negative months are returned", func(t *testcase.T) {
					assert.Equal(t, months.Get(t), act(t).Month)
				})
			})

			s.Context("by days", func(s *testcase.Spec) {
				var days = let.IntB(s, -1, -25)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).AddDate(0, 0, days.Get(t))
				})

				ThenDeltaPlusFirstEqualsSecond(s)

				ThenResultDeltaIsNormalised(s, act)

				s.Then("negative days are returned", func(t *testcase.T) {
					assert.Equal(t, days.Get(t), act(t).Day)
				})
			})

			s.Context("by something smaller than a day", func(s *testcase.Spec) {
				var rem = let.DurationBetween(s, 0, -1*23*time.Hour+59*time.Minute+59*time.Second)

				B.Let(s, func(t *testcase.T) time.Time {
					return A.Get(t).Add(rem.Get(t))
				})

				ThenDeltaPlusFirstEqualsSecond(s)

				ThenResultDeltaIsNormalised(s, act)

				s.Then("negative remainder is returned", func(t *testcase.T) {
					assert.Equal(t, ToDuration(act(t), A.Get(t)), rem.Get(t))
				})
			})
		})

		s.When("max duration (int64) is given", func(s *testcase.Spec) {
			A.Let(s, fixA.Get)

			B.Let(s, func(t *testcase.T) time.Time {
				return A.Get(t).Add(math.MaxInt64)
			})

			s.Then("result distance's ToDuration is equal to the max int64 duration", func(t *testcase.T) {
				assert.Equal(t, ToDuration(act(t), A.Get(t)), math.MaxInt64)
			})

			s.Then("proper distance is calculated", func(t *testcase.T) {
				assert.Equal(t, act(t), timekit.Duration{
					Year:       292,
					Month:      3,
					Day:        10,
					Hour:       23,
					Minute:     47,
					Second:     16,
					Nanosecond: 854775807,
				})
			})

			ThenDeltaPlusFirstEqualsSecond(s)

			ThenResultDeltaIsNormalised(s, act)
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

			s.Then("delta is calculated correctly", func(t *testcase.T) {
				d := act(t)
				assert.Equal(t, d.Year, years.Get(t))
				assert.Equal(t, d.Month, months.Get(t))
				assert.Equal(t, d.Day, days.Get(t))

				d.Year = 0
				d.Month = 0
				d.Day = 0
				assert.Equal(t, rem.Get(t), ToDuration(d, time.Time{}))
			})

			ThenDeltaPlusFirstEqualsSecond(s)

			ThenResultDeltaIsNormalised(s, act)
		})

		s.Test("years differ, but by months count it is not a whole year difference", func(t *testcase.T) {
			a := time.Date(2000, time.December, 1, 0, 0, 0, 0, time.Local)
			b := time.Date(2001, time.January, 3, 0, 0, 0, 0, time.Local)

			d := timekit.Duration{}.Between(a, b)
			assert.Equal(t, 0, d.Year)
			assert.Equal(t, 1, d.Month)
			assert.Equal(t, 2, d.Day)
		})
	})

	s.Describe("#Normalise", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) timekit.Duration {
			return delta.Get(t).Normalise()
		})

		s.When("nanosec has so much time, it overflows to all", func(s *testcase.Spec) {})

		s.When("positive distance is already normalised", func(s *testcase.Spec) {
			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Year:       t.Random.IntBetween(0, 999),
					Month:      t.Random.IntBetween(0, maxMonth),
					Day:        t.Random.IntBetween(0, 20),
					Hour:       t.Random.IntBetween(0, maxHour),
					Minute:     t.Random.IntBetween(0, maxMinute),
					Second:     t.Random.IntBetween(0, maxSecond),
					Nanosecond: t.Random.IntBetween(0, maxNanosecond),
				}
			})

			s.Then("same distance returned", func(t *testcase.T) {
				assert.Equal(t, act(t), delta.Get(t))
			})
		})

		s.When("negative distance is already normalised", func(s *testcase.Spec) {
			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Year:       t.Random.IntBetween(-999, 0),
					Month:      t.Random.IntBetween(-maxMonth, 0),
					Day:        t.Random.IntBetween(-20, 0),
					Hour:       t.Random.IntBetween(-maxHour, 0),
					Minute:     t.Random.IntBetween(-maxMinute, 0),
					Second:     t.Random.IntBetween(-maxSecond, 0),
					Nanosecond: t.Random.IntBetween(-maxNanosecond, 0),
				}
			})

			s.Then("same distance returned", func(t *testcase.T) {
				assert.Equal(t, act(t), delta.Get(t))
			})
		})

		s.When("nanosec is not normalised", func(s *testcase.Spec) {
			nanosecs := let.IntB(s, 1, maxNanosecond)
			seconds := let.IntB(s, 1, maxSecond)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Nanosecond: seconds.Get(t)*maxNanosecond + nanosecs.Get(t),
				}
			})

			s.Then("it gets normalised", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, got.Second, seconds.Get(t))
				assert.Equal(t, got.Nanosecond, nanosecs.Get(t))
			})

			s.And("it is a negative", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					d := delta.Super(t)
					d.Nanosecond = d.Nanosecond * -1
					return d
				})

				s.Then("it gets normalised", func(t *testcase.T) {
					got := act(t)
					assert.Equal(t, got.Second, -seconds.Get(t))
					assert.Equal(t, got.Nanosecond, -nanosecs.Get(t))
				})
			})
		})

		s.When("sec is not normalised", func(s *testcase.Spec) {
			minutes := let.IntB(s, 1, maxMinute)
			seconds := let.IntB(s, 0, maxSecond)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Second: minutes.Get(t)*60 + seconds.Get(t),
				}
			})

			s.Then("it gets normalised", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, got.Minute, minutes.Get(t))
				assert.Equal(t, got.Second, seconds.Get(t))
			})

			s.And("it is a negative", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					d := delta.Super(t)
					d.Second = d.Second * -1
					return d
				})

				s.Then("it gets normalised", func(t *testcase.T) {
					got := act(t)
					assert.Equal(t, got.Minute, -minutes.Get(t))
					assert.Equal(t, got.Second, -seconds.Get(t))
				})
			})
		})

		s.When("minute is not normalised", func(s *testcase.Spec) {
			hours := let.IntB(s, 1, maxHour)
			minutes := let.IntB(s, 0, maxMinute)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Minute: hours.Get(t)*60 + minutes.Get(t),
				}
			})

			s.Then("it gets normalised", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, got.Hour, hours.Get(t))
				assert.Equal(t, got.Minute, minutes.Get(t))
			})

			s.And("it is a negative", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					d := delta.Super(t)
					d.Minute = d.Minute * -1
					return d
				})

				s.Then("it gets normalised", func(t *testcase.T) {
					got := act(t)
					assert.Equal(t, got.Hour, -hours.Get(t))
					assert.Equal(t, got.Minute, -minutes.Get(t))
				})
			})
		})

		s.When("hours is not normalised", func(s *testcase.Spec) {
			days := let.IntB(s, 1, 10)
			hours := let.IntB(s, 0, maxHour)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Hour: days.Get(t)*24 + hours.Get(t),
				}
			})

			s.Then("it gets normalised", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, got.Day, days.Get(t))
				assert.Equal(t, got.Hour, hours.Get(t))
			})

			s.And("it is a negative", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					d := delta.Super(t)
					d.Hour = d.Hour * -1
					return d
				})

				s.Then("it gets normalised", func(t *testcase.T) {
					got := act(t)
					assert.Equal(t, got.Day, -days.Get(t))
					assert.Equal(t, got.Hour, -hours.Get(t))
				})
			})
		})

		s.When("hours is not normalised", func(s *testcase.Spec) {
			days := let.IntB(s, 1, 10)
			hours := let.IntB(s, 0, maxHour)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Hour: days.Get(t)*24 + hours.Get(t),
				}
			})

			s.Then("it gets normalised", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, got.Day, days.Get(t))
				assert.Equal(t, got.Hour, hours.Get(t))
			})

			s.And("it is a negative", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					d := delta.Super(t)
					d.Hour = d.Hour * -1
					return d
				})

				s.Then("it gets normalised", func(t *testcase.T) {
					got := act(t)
					assert.Equal(t, got.Day, -days.Get(t))
					assert.Equal(t, got.Hour, -hours.Get(t))
				})
			})
		})
	})

	s.Describe("#Compare", func(s *testcase.Spec) {
		var (
			oth = let.VarOf(s, timekit.Duration{})
		)
		act := let.Act(func(t *testcase.T) int {
			return delta.Get(t).Compare(oth.Get(t))
		})

		s.When("both is delta is zero value", func(s *testcase.Spec) {
			delta.LetValue(s, timekit.Duration{})
			oth.LetValue(s, timekit.Duration{})

			s.Then("values are equal", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})

		var WhenX = func(s *testcase.Spec, name string, accr func(*timekit.Duration) *int) {
			s.When("receiver's "+name+" is less", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					var d timekit.Duration
					*accr(&d) = t.Random.IntBetween(1, 7) * -1
					return d
				})

				s.Then("receiver is less", func(t *testcase.T) {
					assert.Equal(t, -1, act(t))
				})
			})

			s.When("argument's "+name+" is less", func(s *testcase.Spec) {
				oth.Let(s, func(t *testcase.T) timekit.Duration {
					var d timekit.Duration
					*accr(&d) = t.Random.IntBetween(1, 7) * -1
					return d
				})

				s.Then("argument is less", func(t *testcase.T) {
					assert.Equal(t, 1, act(t))
				})
			})

			s.When("receiver's "+name+" is greater", func(s *testcase.Spec) {
				delta.Let(s, func(t *testcase.T) timekit.Duration {
					var d timekit.Duration
					*accr(&d) = t.Random.IntBetween(1, 7)
					return d
				})

				s.Then("receiver is greater", func(t *testcase.T) {
					assert.Equal(t, 1, act(t))
				})
			})

			s.When("argument's "+name+" is greater", func(s *testcase.Spec) {
				oth.Let(s, func(t *testcase.T) timekit.Duration {
					var d timekit.Duration
					*accr(&d) = t.Random.IntBetween(1, 7)
					return d
				})

				s.Then("argument is greater", func(t *testcase.T) {
					assert.Equal(t, -1, act(t))
				})
			})
		}

		WhenX(s, "Year", func(d *timekit.Duration) *int {
			return &d.Year
		})

		WhenX(s, "Day", func(d *timekit.Duration) *int {
			return &d.Year
		})

		WhenX(s, "Hour", func(d *timekit.Duration) *int {
			return &d.Hour
		})

		WhenX(s, "Minute", func(d *timekit.Duration) *int {
			return &d.Minute
		})

		WhenX(s, "Second", func(d *timekit.Duration) *int {
			return &d.Second
		})

		WhenX(s, "Nanosecond", func(d *timekit.Duration) *int {
			return &d.Nanosecond
		})
	})

	s.Describe("#AddDuration", func(s *testcase.Spec) {
		var duration = let.DurationBetween(s, time.Nanosecond, 48*time.Hour)

		act := let.Act(func(t *testcase.T) timekit.Duration {
			return delta.Get(t).AddDuration(duration.Get(t))
		})

		s.Then("a non-empty delta returned", func(t *testcase.T) {
			assert.NotEmpty(t, act(t))
		})

		s.Then("returned delta is bigger than initial", func(t *testcase.T) {
			got := act(t)

			assert.Equal(t, delta.Get(t).Compare(got), -1)
		})

		s.Then("the duration difference between the two delta exactly as much was added to the delta", func(t *testcase.T) {
			d1 := ToDuration(delta.Get(t), time.Time{})
			d2 := ToDuration(act(t), time.Time{})
			assert.Equal(t, d2-d1, duration.Get(t))
		})

		s.Then("result is in a normalised form already", func(t *testcase.T) {
			got := act(t)
			type D timekit.Duration // to avoid any Equal implementation to hijack the assertEqual
			assert.Equal(t, D(got), D(got.Normalise()))
		})

		s.When("the duration is negative", func(s *testcase.Spec) {
			duration.Let(s, func(t *testcase.T) time.Duration {
				d := duration.Super(t)
				return -1 * d
			})

			s.Then("the returned delta is smaller than the initial", func(t *testcase.T) {
				got := act(t)

				assert.Equal(t, delta.Get(t).Compare(got), 1)
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
			pp.PP(d)

			assert.True(t, d.AddTo(begin).Equal(end), assert.MessageF("diff: %s", end.Sub(d.AddTo(begin))))

		})
	})

	s.Describe("#AddTo", func(s *testcase.Spec) {
		var ref = let.Time(s)
		act := let.Act(func(t *testcase.T) time.Time {
			return delta.Get(t).AddTo(ref.Get(t))
		})

		s.When("delta is zero", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Empty(t, delta.Get(t))
			})

			s.Then("same time returned", func(t *testcase.T) {
				assert.Equal(t, ref.Get(t), act(t))
			})
		})

		s.When("it has years", func(s *testcase.Spec) {
			years := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Year: years.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).AddDate(years.Get(t), 0, 0))
			})
		})

		s.When("it has months", func(s *testcase.Spec) {
			months := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Month: months.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).AddDate(0, months.Get(t), 0))
			})
		})

		s.When("it has days", func(s *testcase.Spec) {
			days := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Day: days.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).AddDate(0, 0, days.Get(t)))
			})
		})

		s.When("it has hours", func(s *testcase.Spec) {
			hours := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Hour: hours.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).Add(time.Duration(hours.Get(t))*time.Hour))
			})
		})

		s.When("it has minutes", func(s *testcase.Spec) {
			minutes := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Minute: minutes.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).Add(time.Duration(minutes.Get(t))*time.Minute))
			})
		})

		s.When("it has seconds", func(s *testcase.Spec) {
			seconds := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Second: seconds.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).Add(time.Duration(seconds.Get(t))*time.Second))
			})
		})

		s.When("it has nanoseconds", func(s *testcase.Spec) {
			nanoseconds := let.IntB(s, -3, 7)

			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{Nanosecond: nanoseconds.Get(t)}
			})

			s.Then("delta added to the ref time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).Add(time.Duration(nanoseconds.Get(t))*time.Nanosecond))
			})
		})

		s.When("it has all kinds of time attributes", func(s *testcase.Spec) {
			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Year:       t.Random.IntBetween(3, 7),
					Month:      t.Random.IntBetween(3, 7),
					Day:        t.Random.IntBetween(3, 7),
					Hour:       t.Random.IntBetween(3, 7),
					Minute:     t.Random.IntBetween(3, 7),
					Second:     t.Random.IntBetween(3, 7),
					Nanosecond: t.Random.IntBetween(3, 7),
				}
			})

			s.Then("the delta is added to the reference time", func(t *testcase.T) {
				assert.Equal(t, act(t), ref.Get(t).Add(ToDuration(delta.Get(t), ref.Get(t))))
			})
		})
	})

	s.Describe("#Normalise", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) timekit.Duration {
			return delta.Get(t).Normalise()
		})

		var ThenValueSumIsTheSame = func(s *testcase.Spec) {
			s.Then("the sum of delta remains the same", func(t *testcase.T) {
				var (
					ref  = t.Random.Time()
					got  = act(t)
					end1 = delta.Get(t).AddTo(ref)
					end2 = got.AddTo(ref)
				)
				assert.True(t, end1.Equal(end2), "expected that delta and normalised delta is the same time delta length")
			})
		}

		ThenValueSumIsTheSame(s)

		s.Test("smoke", func(t *testcase.T) {
			assert.Equal(t, timekit.Duration{Second: 1}, timekit.Duration{Nanosecond: int(time.Second)}.Normalise())
			assert.Equal(t, timekit.Duration{Minute: 1}, timekit.Duration{Second: 60}.Normalise())
			assert.Equal(t, timekit.Duration{Hour: 1}, timekit.Duration{Minute: 60}.Normalise())
			assert.Equal(t, timekit.Duration{Day: 1}, timekit.Duration{Hour: 24}.Normalise())
			assert.Equal(t, timekit.Duration{Year: 1}, timekit.Duration{Month: 12}.Normalise())
		})

		s.When("delta is already normalised positive value", func(s *testcase.Spec) {
			delta.Let(s, func(t *testcase.T) timekit.Duration {
				return timekit.Duration{
					Year:       t.Random.IntBetween(0, math.MaxInt),
					Month:      t.Random.IntBetween(0, maxMonth),
					Day:        t.Random.IntBetween(0, 20), // days can't be normalised as a month is not a fixed amount of day.
					Hour:       t.Random.IntBetween(0, 23), // hour can't be normalised as a Day is not a fixed 24h.
					Minute:     t.Random.IntBetween(0, maxMinute),
					Second:     t.Random.IntBetween(0, maxSecond),
					Nanosecond: t.Random.IntBetween(0, maxNanosecond),
				}
			})
		})
	})
}

func ThenResultDeltaIsNormalised(s *testcase.Spec, act func(t *testcase.T) timekit.Duration) {
	s.Then("result has normalised time intervals in its fields", func(t *testcase.T) {
		assertDeltaIsNormalised(t, act(t))
	})
}

func assertDeltaIsNormalised(t testing.TB, got timekit.Duration) {
	var (
		maxNanosecond = int(math.Pow(10, 9))
		maxSecond     = 59
		maxMinute     = 59
		maxHour       = 23
	)
	assert.AnyOf(t, func(a *assert.A) {
		a.Case(func(t assert.It) { // when A is before B
			assert.True(t, 0 <= got.Year, assert.MessageF("Year: %v", got.Year))
			assert.True(t, 0 <= got.Month, assert.MessageF("Month: %v", got.Month))
			assert.True(t, 0 <= got.Day, assert.MessageF("Day: %v", got.Day))
			assert.True(t, got.Hour <= maxHour, assert.MessageF("Hour: %v", got.Hour))
			assert.True(t, got.Minute <= maxMinute, assert.MessageF("Minute: %v", got.Minute))
			assert.True(t, got.Second <= maxSecond, assert.MessageF("Second: %v", got.Second))
			assert.True(t, got.Nanosecond <= maxNanosecond, assert.MessageF("Nanosecond: %v", got.Nanosecond))
		})

		a.Case(func(t assert.It) { // when B is before A
			assert.True(t, got.Year <= 0, assert.MessageF("Year: %v", got.Year))
			assert.True(t, got.Month <= 0, assert.MessageF("Month: %v", got.Month))
			assert.True(t, got.Day <= 0, assert.MessageF("Day: %v", got.Day))
			assert.True(t, -maxHour <= got.Hour, assert.MessageF("Hour: %v", got.Hour))
			assert.True(t, -maxMinute <= got.Minute, assert.MessageF("Minute: %v", got.Minute))
			assert.True(t, -maxSecond <= got.Second, assert.MessageF("Second: %v", got.Second))
			assert.True(t, -maxNanosecond <= got.Nanosecond, assert.MessageF("Nanosecond: %v", got.Nanosecond))
		})
	})
}
