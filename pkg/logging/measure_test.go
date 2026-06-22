package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestDuration(t *testing.T) {
	s := testcase.NewSpec(t)

	// Use an identity key formatter so the configured key round-trips verbatim,
	// keeping this test focused on Duration rather than on key formatting.
	logger, buf := testcase.Let2(s, func(t *testcase.T) (*logging.Logger, logging.StubOutput) {
		l, out := logging.Stub(t)
		l.KeyFormatter = func(key string) string { return key }
		return l, out
	})

	var (
		key = let.Var(s, func(t *testcase.T) string {
			return t.Random.StringNWithCharset(t.Random.IntBetween(3, 7), random.CharsetAlpha())
		})
		since = let.Var(s, func(t *testcase.T) time.Time {
			return t.Random.Time()
		})
		unit = let.Var(s, func(t *testcase.T) time.Duration {
			return random.Pick(t.Random, 0,
				time.Hour, time.Minute, time.Second,
				time.Millisecond, time.Microsecond, time.Nanosecond)
		})
	)
	act := let.Act(func(t *testcase.T) logging.Detail {
		return logging.Duration(key.Get(t), since.Get(t), unit.Get(t))
	})

	// elapsed is the amount of time that passes between `since` and the moment the entry is rendered.
	elapsed := let.DurationBetween(s, time.Millisecond, 24*time.Hour)

	s.Before(func(t *testcase.T) {
		// freeze time to make it very easy to create sub arrangements
		// where we move forward by the expected time amount.
		// We start exactly at `since`, so the measured elapsed time begins at zero.
		timecop.Travel(t, since.Get(t), timecop.DeepFreeze)
	})

	// logDetail builds the detail while the clock still sits at `since`,
	// then advances the clock by `elapsed` and only afterwards emits the entry.
	// Building before advancing is what proves the measurement happens lazily, at logging time.
	logDetail := func(t *testcase.T) {
		t.Helper()
		detail := act(t)
		timecop.Travel(t, since.Get(t).Add(elapsed.Get(t)), timecop.DeepFreeze)
		logger.Get(t).Info(context.Background(), t.Random.String(), detail)
	}

	entryFields := func(t *testcase.T) map[string]any {
		t.Helper()
		var fields map[string]any
		assert.NoError(t, json.NewDecoder(bytes.NewReader(buf.Get(t).Bytes())).Decode(&fields))
		return fields
	}

	loggedValue := func(t *testcase.T) (any, bool) {
		t.Helper()
		logDetail(t)
		v, ok := entryFields(t)[key.Get(t)]
		return v, ok
	}

	s.Then("the elapsed time is logged under the provided key", func(t *testcase.T) {
		_, ok := loggedValue(t)
		assert.True(t, ok, assert.Message("expected a field to be present under the configured key"))
	})

	s.Then("it leaves the base log fields intact", func(t *testcase.T) {
		logDetail(t)
		fields := entryFields(t)
		assert.NotEmpty(t, fields["message"])
		assert.Equal[any](t, string(logging.LevelInfo), fields["level"])
		assert.NotEmpty(t, fields["timestamp"])
	})

	thenItLogs := func(s *testcase.Spec, expected func(t *testcase.T) any) {
		s.Then("it logs the elapsed time converted to the configured unit", func(t *testcase.T) {
			got, ok := loggedValue(t)
			assert.True(t, ok)
			assert.Equal[any](t, expected(t), got)
		})
	}

	s.Describe("the logged value", func(s *testcase.Spec) {
		// Numeric units decode back from JSON as float64, hence the float64 conversions below.

		s.When("the unit is zero", func(s *testcase.Spec) {
			unit.LetValue(s, 0)

			thenItLogs(s, func(t *testcase.T) any {
				return elapsed.Get(t).String()
			})
		})

		s.When("the unit is time.Hour", func(s *testcase.Spec) {
			unit.LetValue(s, time.Hour)

			thenItLogs(s, func(t *testcase.T) any {
				return elapsed.Get(t).Hours()
			})
		})

		s.When("the unit is time.Minute", func(s *testcase.Spec) {
			unit.LetValue(s, time.Minute)

			thenItLogs(s, func(t *testcase.T) any {
				return elapsed.Get(t).Minutes()
			})
		})

		s.When("the unit is time.Second", func(s *testcase.Spec) {
			unit.LetValue(s, time.Second)

			thenItLogs(s, func(t *testcase.T) any {
				return elapsed.Get(t).Seconds()
			})
		})

		s.When("the unit is time.Millisecond", func(s *testcase.Spec) {
			unit.LetValue(s, time.Millisecond)

			thenItLogs(s, func(t *testcase.T) any {
				return float64(elapsed.Get(t).Milliseconds())
			})
		})

		s.When("the unit is time.Microsecond", func(s *testcase.Spec) {
			unit.LetValue(s, time.Microsecond)

			thenItLogs(s, func(t *testcase.T) any {
				return float64(elapsed.Get(t).Microseconds())
			})
		})

		s.When("the unit is time.Nanosecond", func(s *testcase.Spec) {
			unit.LetValue(s, time.Nanosecond)

			thenItLogs(s, func(t *testcase.T) any {
				return float64(elapsed.Get(t).Nanoseconds())
			})
		})

		s.When("the unit is not one of the canonical units", func(s *testcase.Spec) {
			// any value other than 0 / hour / minute / second / milli / micro / nano
			// falls back to the human readable duration string.
			unit.LetValue(s, 90*time.Minute)

			thenItLogs(s, func(t *testcase.T) any {
				return elapsed.Get(t).String()
			})
		})
	})

	s.Describe("lazy evaluation", func(s *testcase.Spec) {
		unit.LetValue(s, 0)

		s.Then("the elapsed time is measured at logging time, not when the detail is created", func(t *testcase.T) {
			// create the detail while the clock is still at `since` (elapsed == 0)
			detail := act(t)

			// move the clock forward only AFTER the detail already exists
			timecop.Travel(t, since.Get(t).Add(elapsed.Get(t)), timecop.DeepFreeze)
			logger.Get(t).Info(context.Background(), t.Random.String(), detail)

			// the logged value reflects the time elapsed up to the log call, not zero
			assert.Equal[any](t, elapsed.Get(t).String(), entryFields(t)[key.Get(t)])
		})
	})

	s.When("no time has elapsed between the detail's creation and the logging", func(s *testcase.Spec) {
		elapsed.LetValue(s, 0)
		unit.LetValue(s, 0)

		s.Then(`it logs the zero duration ("0s")`, func(t *testcase.T) {
			got, ok := loggedValue(t)
			assert.True(t, ok)
			assert.Equal[any](t, time.Duration(0).String(), got)
		})
	})

	s.When("the since timestamp is in the future", func(s *testcase.Spec) {
		// the clock at logging time is *before* `since`, so the elapsed time is negative.
		elapsed.Let(s, func(t *testcase.T) time.Duration {
			return -1 * t.Random.DurationBetween(time.Second, time.Hour)
		})
		unit.LetValue(s, time.Second)

		s.Then("the logged duration is negative", func(t *testcase.T) {
			got, ok := loggedValue(t)
			assert.True(t, ok)
			assert.Equal[any](t, elapsed.Get(t).Seconds(), got)
			gotSeconds, ok := got.(float64)
			assert.True(t, ok)
			assert.True(t, gotSeconds < 0)
		})
	})

	s.When("the log entry's level is disabled", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			// LevelWarn disables the Info entry produced by logDetail.
			logger.Get(t).Level = logging.LevelWarn
		})

		s.Then("nothing is rendered, so the measurement is skipped entirely", func(t *testcase.T) {
			logDetail(t)
			assert.Empty(t, buf.Get(t).String())
		})
	})
}
