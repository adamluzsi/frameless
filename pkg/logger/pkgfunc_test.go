package logger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"testing"
	"time"
)

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
			msg := logger.Details{}
			assert.NoError(t, dec.Decode(&msg))
			assert.NotEmpty(t, msg)
		}

		assert.Equal(t, expected, got)
	})

	t.Run("message, timestamp, level and all details are logged, including from context", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx := context.Background()
		ctx = logger.ContextWithDetails(ctx, logger.Details{"foo": "bar"})
		ctx = logger.ContextWithDetails(ctx, logger.Details{"bar": 42})

		logger.Info(ctx, "a", logger.Details{"info": "level"})
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), `"info":"level"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"message":"a"`)
		assert.Contain(t, buf.String(), `"bar":42`)
		assert.Contain(t, buf.String(), `"level":"info"`)

		t.Run("on all levels", func(t *testing.T) {
			logger.Debug(ctx, "b", logger.Details{"debug": "level"})
			assert.Contain(t, buf.String(), `"message":"b"`)
			assert.Contain(t, buf.String(), `"debug":"level"`)
			logger.Warn(ctx, "c", logger.Details{"warn": "level"})
			assert.Contain(t, buf.String(), `"message":"c"`)
			assert.Contain(t, buf.String(), `"level":"warn"`)
			assert.Contain(t, buf.String(), `"warn":"level"`)
			logger.Error(ctx, "d", logger.Details{"error": "level"})
			assert.Contain(t, buf.String(), `"message":"d"`)
			assert.Contain(t, buf.String(), `"level":"error"`)
			assert.Contain(t, buf.String(), `"error":"level"`)
			logger.Fatal(ctx, "e", logger.Details{"fatal": "level"})
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
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, logger.Default.TimestampKey, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, logger.Default.MessageKey, "foo"))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, logger.Default.LevelKey, "info"))
	})
}
