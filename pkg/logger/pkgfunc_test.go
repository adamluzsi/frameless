package logger_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"testing"
	"time"
)

func ExampleDebug() {
	ctx := context.Background()
	logger.Debug(ctx, "foo")
}

func ExampleInfo() {
	ctx := context.Background()
	logger.Info(ctx, "foo")
}

func ExampleWarn() {
	ctx := context.Background()
	logger.Warn(ctx, "foo")
}

func ExampleError() {
	ctx := context.Background()
	logger.Error(ctx, "foo")
}

func ExampleFatal() {
	ctx := context.Background()
	logger.Fatal(ctx, "foo")
}

func Example_withDetails() {
	ctx := context.Background()
	logger.Info(ctx, "foo", logger.Fields{
		"userID":    42,
		"accountID": 24,
	})
}

func Test_pkgFuncSmoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())
	rnd := random.New(random.CryptoSeed{})

	t.Run("output is a valid JSON by default", func(t *testing.T) {
		ctx := context.Background()
		buf := logger.Stub(t)

		expected := rnd.Repeat(3, 7, func() {
			logger.Info(ctx, rnd.String())
		})

		dec := json.NewDecoder(buf)

		var got int
		for dec.More() {
			got++
			msg := logger.Fields{}
			assert.NoError(t, dec.Decode(&msg))
			assert.NotEmpty(t, msg)
		}

		assert.Equal(t, expected, got)
	})

	t.Run("message, timestamp, level and all details are logged, including from context", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx := context.Background()
		ctx = logger.ContextWith(ctx, logger.Fields{"foo": "bar"})
		ctx = logger.ContextWith(ctx, logger.Fields{"bar": 42})

		logger.Info(ctx, "a", logger.Fields{"info": "level"})
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), `"info":"level"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"message":"a"`)
		assert.Contain(t, buf.String(), `"bar":42`)
		assert.Contain(t, buf.String(), `"level":"info"`)

		t.Run("on all levels", func(t *testing.T) {
			logger.Debug(ctx, "b", logger.Fields{"debug": "level"})
			assert.Contain(t, buf.String(), `"message":"b"`)
			assert.Contain(t, buf.String(), `"debug":"level"`)
			logger.Warn(ctx, "c", logger.Fields{"warn": "level"})
			assert.Contain(t, buf.String(), `"message":"c"`)
			assert.Contain(t, buf.String(), `"level":"warn"`)
			assert.Contain(t, buf.String(), `"warn":"level"`)
			logger.Error(ctx, "d", logger.Fields{"error": "level"})
			assert.Contain(t, buf.String(), `"message":"d"`)
			assert.Contain(t, buf.String(), `"level":"error"`)
			assert.Contain(t, buf.String(), `"error":"level"`)
			logger.Fatal(ctx, "e", logger.Fields{"fatal": "level"})
			assert.Contain(t, buf.String(), `"message":"e"`)
			assert.Contain(t, buf.String(), `"level":"fatal"`)
			assert.Contain(t, buf.String(), `"fatal":"level"`)
		})
	})

	t.Run("keys can be configured", func(t *testing.T) {
		ctx := context.Background()
		buf := logger.Stub(t)
		logger.Default.MessageKey = rnd.UUID()
		logger.Default.TimestampKey = rnd.UUID()
		logger.Default.LevelKey = rnd.UUID()

		logger.Info(ctx, "foo")
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(logger.Default.TimestampKey), now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(logger.Default.MessageKey), "foo"))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(logger.Default.LevelKey), "info"))
	})
}

func ExampleErrField() {
	ctx := context.Background()
	err := errors.New("boom")

	logger.Error(ctx, "task failed successfully", logger.ErrField(err))
}

func TestErrField(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("plain error", func(t *testing.T) {
		buf := logger.Stub(t)
		expErr := rnd.Error()
		logger.Info(nil, "boom", logger.ErrField(expErr))
		assert.Contain(t, buf.String(), `"error":{`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
	t.Run("nil error", func(t *testing.T) {
		buf := logger.Stub(t)
		logger.Info(nil, "boom", logger.ErrField(nil))
		assert.NotContain(t, buf.String(), `"error"`)
	})
	t.Run("when err is a user error", func(t *testing.T) {
		buf := logger.Stub(t)
		const message = "The answer"
		const code = "42"
		var expErr error
		expErr = errorutil.UserError{ID: code, Message: message}
		expErr = fmt.Errorf("err: %w", expErr)
		d := logger.ErrField(expErr)
		logger.Info(nil, "boom", d)
		assert.Contain(t, buf.String(), `"error":{`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"code":%q`, code))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
}
