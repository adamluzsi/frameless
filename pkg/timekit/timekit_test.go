package timekit_test

import (
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

func Test_timeDate_contract(t *testing.T) {
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

func TestAvailabilityPolicy(t *testing.T) {
	SchedulingSpec{}.Test(t)
}

type SchedulingSpec struct{}

func (suite SchedulingSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe("#Next", suite.specNext)

	// s.Describe("#Near", func(s *testcase.Spec) {
	// 	// SpecAvailabilityPolicy_Near(s, scheduling)
	// })
}

func (spec SchedulingSpec) specNext(s *testcase.Spec) {
	var ref = let.Var(s, func(t *testcase.T) time.Time {
		return time.Date(2000, 1, 2, 3, 4, 5, 6, time.Local)
	})

	scheduling := let.Var(s, func(t *testcase.T) timekit.Scheduling {
		return timekit.Scheduling{}
	})

	act := let.Act2(func(t *testcase.T) (timekit.Range, bool) {
		rng, ok := scheduling.Get(t).Next(ref.Get(t))
		t.OnFail(func() {
			t.LogPretty("now", ref.Get(t))
			t.LogPretty(scheduling.Get(t))
			t.LogPretty(rng, ok)
		})
		return rng, ok
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, validate.Value(scheduling.Get(t)), "sanity check")
	})

	s.Test("A zero Schedule should still be valid and yield a next occurence related to a reference time", func(t *testcase.T) {
		rng, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, rng)
		assert.True(t, ref.Get(t).Before(rng.From))
	})

	s.Test("A zero Schedule means 0-7/0-24", func(t *testcase.T) {
		rng, ok := act(t)
		assert.True(t, ok)
		assert.NotEmpty(t, rng)
		t.Log("so the next occurence will be the next day related to the reference time")
		assert.Equal(t, ref.Get(t).Year(), rng.From.Year())
		assert.Equal(t, ref.Get(t).Month(), rng.From.Month())
		assert.Equal(t, ref.Get(t).Day(), rng.From.Day()+1)
	})

	s.When(".From is provided", func(s *testcase.Spec) {
		from := let.VarOf(s, timekit.DayTime{Hour: 12, Minute: 00})

		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			sch := scheduling.Super(t)
			sch.From = from.Get(t)
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

		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			sch := scheduling.Super(t)
			sch.Month = pointer.Of(month.Get(t))
			return sch
		})

		s.Test("month will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				rng, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, rng)
				assert.Equal(t, rng.From.Month(), month.Get(t))
				ref.Set(t, rng.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the weekday", func(s *testcase.Spec) {
		weekday := let.OneOf[time.Weekday](s, timekit.Weekdays()...)

		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			sch := scheduling.Super(t)
			sch.Weekday = pointer.Of(weekday.Get(t))
			return sch
		})

		s.Test("weekday will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Weekdays()), func() {
				rng, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, rng)
				assert.Equal(t, rng.From.Weekday(), weekday.Get(t))
				ref.Set(t, rng.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the day", func(s *testcase.Spec) {
		day := let.IntB(s, 1, 25)

		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			sch := scheduling.Super(t)
			sch.Day = pointer.Of(day.Get(t))
			return sch
		})

		s.Test("day will be used as a constraing to retrieve the next occurence", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				rng, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, rng)
				assert.Equal(t, rng.From.Day(), day.Get(t))
				ref.Set(t, rng.Till.AddDate(0, 0, 1))
			})
		})
	})

	s.When("schedule defines the location", func(s *testcase.Spec) {
		location := let.Var(s, func(t *testcase.T) *time.Location {
			timeGMT, err := time.LoadLocation("GMT")
			assert.NoError(t, err)
			return random.Pick(t.Random, time.Local, time.UTC, timeGMT)
		})

		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			sch := scheduling.Super(t)
			sch.Location = location.Get(t)
			return sch
		})

		s.Test("location is used as part of the next occurence calculation", func(t *testcase.T) {
			t.Random.Repeat(3, len(timekit.Months()), func() {
				rng, ok := act(t)
				assert.True(t, ok)
				assert.NotEmpty(t, rng)
				assert.Equal(t, rng.From.Location(), location.Get(t))
				ref.Set(t, rng.Till.AddDate(0, 0, 1))
			})
		})

		// TODO add more coverage for edge cases with location.
	})

	s.When("current time is within the availability policy", func(s *testcase.Spec) {
		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 15, 30, 45),
			}
			return timekit.Scheduling{
				From: from,

				Till: timekit.DayTime{
					Hour:   from.Hour + 8,
					Minute: random.Pick(t.Random, 0, 15, 30, 45),
				},

				Location: time.Local,
			}
		})

		ref.Let(s, func(t *testcase.T) time.Time {
			super := ref.Super(t)
			sch := scheduling.Get(t)
			return time.Date(super.Year(), super.Month(), super.Day(),
				t.Random.IntBetween(sch.From.Hour, sch.Till.Hour),
				t.Random.IntBetween(sch.From.Minute, 59),
				t.Random.IntBetween(0, 59), 0, time.Local)
		})

		s.Then("the next occurence is returned", func(t *testcase.T) {
			got, ok := act(t)
			assert.True(t, ok)

			rtm := ref.Get(t)
			rtm = rtm.AddDate(0, 0, 0) // next occurence from next day
			year, month, day := rtm.Date()
			nextDay := day + 1

			assert.Equal(t, timekit.Range{
				From: scheduling.Get(t).From.ToDate(year, month, nextDay, scheduling.Get(t).Location),
				Till: scheduling.Get(t).Till.ToDate(year, month, nextDay, scheduling.Get(t).Location),
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
		scheduling.Let(s, func(t *testcase.T) timekit.Scheduling {
			from := timekit.DayTime{
				Hour:   t.Random.IntBetween(7, 10),
				Minute: random.Pick(t.Random, 0, 30),
			}

			weekday := timekit.ShiftWeekday(ref.Get(t).Weekday(), 1)

			return timekit.Scheduling{
				Weekday: &weekday,

				From: from,

				Till: timekit.DayTime{
					Hour:   from.Hour + 8,
					Minute: random.Pick(t.Random, 0, 30),
				},

				Location: ref.Get(t).Location(),
			}
		})

		s.Then("next occurence is given back", func(t *testcase.T) {
			rng, ok := act(t)
			assert.True(t, ok)

			loc := scheduling.Get(t).Location
			expectedDay := ref.Get(t).AddDate(0, 0, 1).In(loc)
			year, month, day := expectedDay.Date()

			assert.Equal(t, rng, timekit.Range{
				From: scheduling.Get(t).From.ToDate(year, month, day, loc),
				Till: scheduling.Get(t).Till.ToDate(year, month, day, loc),
			})
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
