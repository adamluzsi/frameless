package timekit_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/timekit"
	"go.llib.dev/frameless/pkg/validate"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

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
		got := time.Time.Truncate(ref, timekit.Day)
		assert.Equal(t, exp, got)
	})
}

func TestShiftWeekday(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		weekday = let.OneOf(s, enum.Values[time.Weekday]()...)
		n       = let.IntB(s, 0, 7)
	)
	act := let.Act(func(t *testcase.T) time.Weekday {
		return timekit.ShiftWeekday(weekday.Get(t), n.Get(t))
	})

	s.Then("the result is a valid weekday", func(t *testcase.T) {
		assert.Contain(t, enum.Values[time.Weekday](), act(t))
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
		month = let.OneOf(s, enum.Values[time.Month]()...)
		n     = let.IntB(s, -12, 12)
	)
	act := let.Act(func(t *testcase.T) time.Month {
		return timekit.ShiftMonth(month.Get(t), n.Get(t))
	})

	s.Then("the result is a valid month", func(t *testcase.T) {
		assert.Contain(t, enum.Values[time.Month](), act(t))
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

func TestDiffMonth(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.DiffMonth(time.January, time.January), 0)
		assert.Equal(t, timekit.DiffMonth(time.January, time.February), 1)
		assert.Equal(t, timekit.DiffMonth(time.January, time.March), 2)
		assert.Equal(t, timekit.DiffMonth(time.January, time.April), 3)
		assert.Equal(t, timekit.DiffMonth(time.January, time.May), 4)
		assert.Equal(t, timekit.DiffMonth(time.January, time.June), 5)
		assert.Equal(t, timekit.DiffMonth(time.January, time.July), 6)
		assert.Equal(t, timekit.DiffMonth(time.January, time.August), 7)
		assert.Equal(t, timekit.DiffMonth(time.January, time.September), 8)
		assert.Equal(t, timekit.DiffMonth(time.January, time.October), 9)
		assert.Equal(t, timekit.DiffMonth(time.January, time.November), 10)
		assert.Equal(t, timekit.DiffMonth(time.January, time.December), 11)
		assert.Equal(t, timekit.DiffMonth(time.December, time.January), 1)
		assert.Equal(t, timekit.DiffMonth(time.October, time.January), 3)
	})
}

func TestDiffWeekday(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Monday), 0)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Tuesday), 1)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Wednesday), 2)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Thursday), 3)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Friday), 4)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Saturday), 5)
		assert.Equal(t, timekit.DiffWeekday(time.Monday, time.Sunday), 6)
	})
}

func TestDiffDay(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ref    = let.Var[time.Time](s, nil)
		target = let.Var[time.Time](s, nil)
	)
	act := let.Act(func(t *testcase.T) int {
		return timekit.DiffDay(ref.Get(t), target.Get(t))
	})

	s.When("ref time is before target time", func(s *testcase.Spec) {
		days := let.IntB(s, 1, 7)

		ref.Let(s, let.Time(s).Get)

		target.Let(s, func(t *testcase.T) time.Time {
			return ref.Get(t).Add(timekit.Day * time.Duration(days.Get(t)))
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
			return target.Get(t).Add(time.Duration(days.Get(t)) * timekit.Day)
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
	s.Describe("#NextRange", suite.specNextRange)
	s.Context("implements", func(s *testcase.Spec) {
		s.Context("timekit.Interval", suite.implementsInterval)
	})
}

func (spec ScheduleSpec) implementsInterval(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return t.Random.Time()
	})

	schedule := let.Var(s, func(t *testcase.T) timekit.Schedule {
		return timekit.Schedule{}
	})

	act := let.Act(func(t *testcase.T) time.Duration {
		return schedule.Get(t).UntilNext(ref.Get(t))
	})

	s.When("schedule is unspecified", func(s *testcase.Spec) {
		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			return timekit.Schedule{}
		})

		s.Then("it will yield back zero", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})
	})

	s.When("schedule defines day-time", func(s *testcase.Spec) {
		dayTime := let.Var(s, func(t *testcase.T) timekit.DayTime {
			return timekit.DayTime{
				Hour: t.Random.IntBetween(9, 12),
			}
		})

		duration := let.DurationBetween(s, time.Minute, time.Hour)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.DayTime = dayTime.Get(t)
			sch.Duration = duration.Get(t)
			return sch
		})

		s.And("ref time is at the day-time", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				return dayTime.Get(t).ToTimeRelTo(ref.Super(t))
			})

			s.Then("until is time is zero", func(t *testcase.T) {
				assert.Equal(t, 0, act(t))
			})
		})

		s.And("ref time is after the day-time (exl) but within the schedule's duration", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				from := dayTime.Get(t).ToTimeRelTo(ref.Super(t))
				return t.Random.TimeBetween(from.Add(1), from.Add(schedule.Get(t).Duration))
			})

			s.Then("time till next occurence received", func(t *testcase.T) {
				nextDayOccurence := dayTime.Get(t).ToTimeRelTo(ref.Get(t)).AddDate(0, 0, 1)

				assert.Equal(t, act(t), nextDayOccurence.Sub(ref.Get(t)))
			})
		})

		s.And("ref time is before the day-time", func(s *testcase.Spec) {
			diff := let.DurationBetween(s, time.Minute, time.Hour*3)

			ref.Let(s, func(t *testcase.T) time.Time {
				return dayTime.Get(t).ToTimeRelTo(ref.Super(t)).Add(diff.Get(t) * -1)
			})

			s.Then("time until the nearest occurence is retruned", func(t *testcase.T) {
				assert.Equal(t, act(t), diff.Get(t))
			})
		})

		s.And("ref time is after the end of the schedule's duration", func(s *testcase.Spec) {
			ref.Let(s, func(t *testcase.T) time.Time {
				from := dayTime.Get(t).ToTimeRelTo(ref.Super(t))
				return t.Random.TimeBetween(from.Add(1), from.Add(schedule.Get(t).Duration))
			})

			diff := let.DurationBetween(s, time.Minute, time.Hour*3)

			ref.Let(s, func(t *testcase.T) time.Time {
				return dayTime.Get(t).ToTimeRelTo(ref.Super(t)).Add(diff.Get(t))
			})

			s.Then("until is then point to the next day", func(t *testcase.T) {
				nextDayOccurence := dayTime.Get(t).ToTimeRelTo(ref.Get(t)).AddDate(0, 0, 1)

				assert.Equal(t, act(t), nextDayOccurence.Sub(ref.Get(t)))
			})
		})
	})
}

func (spec ScheduleSpec) specNextRange(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return time.Date(2000, 1, 2, 3, 4, 5, 6, time.Local)
	})

	schedule := let.Var(s, func(t *testcase.T) timekit.Schedule {
		return timekit.Schedule{}
	})

	act := let.Act2(func(t *testcase.T) (timekit.Range, bool) {
		t.OnFail(func() {
			t.LogPretty("ref", ref.Get(t))
			t.LogPretty("schedule", schedule.Get(t))
		})
		var (
			next timekit.Range
			ok   bool
		)
		assert.Within(t, 3*time.Second, func(ctx context.Context) {
			next, ok = schedule.Get(t).NextRange(ref.Get(t))
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
		assert.Equal(t, got.From.Year(), ref.Get(t).Year())
		assert.Equal(t, got.From.Month(), ref.Get(t).Month())
		assert.Equal(t, got.From.Day(), ref.Get(t).Day()+1)
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
		month := let.OneOf[time.Month](s, timekit.Months()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Month = pointer.Of(month.Get(t))
			return sch
		})

		s.Test("month will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Month(), month.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the weekday", func(s *testcase.Spec) {
		weekday := let.OneOf[time.Weekday](s, timekit.Weekdays()...)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Weekday = pointer.Of(weekday.Get(t))
			return sch
		})

		s.Test("weekday will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Weekdays()), func() {
				got, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, got)
				assert.Equal(t, got.From.Weekday(), weekday.Get(t))
				ref.Set(t, got.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the day", func(s *testcase.Spec) {
		day := let.IntB(s, 1, 25)

		schedule.Let(s, func(t *testcase.T) timekit.Schedule {
			sch := schedule.Super(t)
			sch.Day = pointer.Of(day.Get(t))
			return sch
		})

		s.Test("day will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
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
				Duration: random.Pick[time.Duration](t.Random, 0, 15, 30, 45) * timekit.Minute,
				Location: time.Local,
			}
		})

		ref.Let(s, func(t *testcase.T) time.Time {
			super := ref.Super(t)
			sch := schedule.Get(t)
			return time.Date(super.Year(), super.Month(), super.Day(),
				t.Random.IntBetween(sch.DayTime.Hour, int(sch.Duration/timekit.Hour)),
				t.Random.IntBetween(sch.DayTime.Minute, 59),
				t.Random.IntBetween(0, 59), 0, time.Local)
		})

		s.Then("the next occurence is returned", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)

			from := schedule.Get(t).DayTime.
				ToTimeRelTo(ref.Get(t)).
				AddDate(0, 0, 1)

			assert.Equal(t, timekit.Range{
				From: from,
				Till: from.Add(schedule.Get(t).Duration),
			}, got)
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
