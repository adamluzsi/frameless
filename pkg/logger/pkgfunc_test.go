package logger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/stringcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/random"
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
		logger.Default.Level = logger.LevelDebug
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

func ExampleAsyncLogging() {
	ctx := context.Background()
	defer logger.AsyncLogging()()
	logger.Info(ctx, "this log message is written out asynchronously")
}

func TestAsyncLogging(t *testing.T) {
	out := logger.Stub(t)
	logger.Default.MessageKey = "msg"
	logger.Default.KeyFormatter = stringcase.ToPascal

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer logger.AsyncLogging()()

	logger.Info(ctx, "gsm", logger.Field("fieldKey", "value"))

	assert.Eventually(t, 3*time.Second, func(it assert.It) {
		it.Must.Contain(out.String(), `"Msg":"gsm"`)
		it.Must.Contain(out.String(), `"FieldKey":"value"`)
	})
}
