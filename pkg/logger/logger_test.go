package logger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/pkg/teardown"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

var defaultKeyFormatter = stringkit.ToSnake

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
	logger.Info(ctx, "foo", logging.Fields{
		"userID":    42,
		"accountID": 24,
	})
}

func Test_pkgFuncSmoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze)
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
			msg := logging.Fields{}
			assert.NoError(t, dec.Decode(&msg))
			assert.NotEmpty(t, msg)
		}

		assert.Equal(t, expected, got)
	})

	t.Run("message, timestamp, level and all details are logged, including from context", func(t *testing.T) {
		buf := logger.Stub(t, func(l *logging.Logger) {
			l.Level = logging.LevelDebug
		})

		ctx := context.Background()
		ctx = logging.ContextWith(ctx, logging.Fields{"foo": "bar"})
		ctx = logging.ContextWith(ctx, logging.Fields{"bar": 42})

		logger.Info(ctx, "a", logging.Fields{"info": "level"})
		assert.Contains(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contains(t, buf.String(), `"info":"level"`)
		assert.Contains(t, buf.String(), `"foo":"bar"`)
		assert.Contains(t, buf.String(), `"message":"a"`)
		assert.Contains(t, buf.String(), `"bar":42`)
		assert.Contains(t, buf.String(), `"level":"info"`)

		t.Run("on all levels", func(t *testing.T) {
			logger.Debug(ctx, "b", logging.Fields{"debug": "level"})
			assert.Contains(t, buf.String(), `"message":"b"`)
			assert.Contains(t, buf.String(), `"debug":"level"`)
			logger.Warn(ctx, "c", logging.Fields{"warn": "level"})
			assert.Contains(t, buf.String(), `"message":"c"`)
			assert.Contains(t, buf.String(), `"level":"warn"`)
			assert.Contains(t, buf.String(), `"warn":"level"`)
			logger.Error(ctx, "d", logging.Fields{"error": "level"})
			assert.Contains(t, buf.String(), `"message":"d"`)
			assert.Contains(t, buf.String(), `"level":"error"`)
			assert.Contains(t, buf.String(), `"error":"level"`)
			logger.Fatal(ctx, "e", logging.Fields{"fatal": "level"})
			assert.Contains(t, buf.String(), `"message":"e"`)
			assert.Contains(t, buf.String(), `"level":"fatal"`)
			assert.Contains(t, buf.String(), `"fatal":"level"`)
		})
	})

	t.Run("keys can be configured", func(t *testing.T) {
		ctx := context.Background()

		var (
			messageKey   = rnd.UUID()
			timestampKey = rnd.UUID()
			levelKey     = rnd.UUID()
		)
		buf := logger.Stub(t, func(l *logging.Logger) {
			l.MessageKey = messageKey
			l.TimestampKey = timestampKey
			l.LevelKey = levelKey
		})

		logger.Info(ctx, "foo")
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(timestampKey), now.Format(time.RFC3339)))
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(messageKey), "foo"))
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`,
			defaultKeyFormatter(levelKey), "info"))
	})
}

func ExampleAsyncLogging() {
	ctx := context.Background()
	defer logger.AsyncLogging()()
	logger.Info(ctx, "this log message is written out asynchronously")
}

func TestAsyncLogging(t *testing.T) {
	out := logger.Stub(t, func(l *logging.Logger) {
		l.MessageKey = "msg"
		l.KeyFormatter = stringkit.ToPascal
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer logger.AsyncLogging()()

	logger.Info(ctx, "gsm", logging.Field("fieldKey", "value"))

	assert.Eventually(t, 3*time.Second, func(it testing.TB) {
		assert.Contains(it, out.String(), `"Msg":"gsm"`)
		assert.Contains(it, out.String(), `"FieldKey":"value"`)
	})
}

func ExampleStub() {
	var tb testing.TB
	buf := logger.Stub(tb) // stub will clean up after itself when the test is finished
	logger.Info(context.Background(), "foo")
	strings.Contains(buf.String(), "foo") // true
}

func TestStub(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		buf := logger.Stub(t)
		logger.Info(context.Background(), "hello")
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"hello"`, defaultKeyFormatter("message")))
	})
	t.Run("mutating", func(t *testing.T) {
		rnd := random.New(random.CryptoSeed{})
		buf := logger.Stub(t, func(l *logging.Logger) {
			l.MessageKey = "foo"
		})
		msg := rnd.UUID()
		logger.Info(context.Background(), msg)
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, "foo", msg))
	})
	t.Run("with optional HijackFunc", func(t *testing.T) {
		type Entry struct {
			Level   logging.Level
			Message string
			Fields  logging.Fields
		}
		var entries []Entry

		logger.Stub(t, func(l *logging.Logger) {
			l.MessageKey = "msg"
			l.LevelKey = "lvl"
			l.Level = logging.LevelDebug

			l.Hijack = func(level logging.Level, msg string, fields logging.Fields) {
				entries = append(entries, Entry{
					Level:   level,
					Message: msg,
					Fields:  fields,
				})
			}
		})

		ctx := logging.ContextWith(context.Background(), logging.Field("foo", 42))
		logger.Debug(ctx, "msg-1", logging.Field("bar", 24))
		logger.Info(ctx, "msg-2", logging.Field("baz", []int{1, 2, 3}))

		assert.Equal(t, len(entries), 2)
		assert.OneOf(t, entries, func(it testing.TB, got Entry) {
			assert.Equal(it, got.Level, logging.LevelDebug)
			assert.Equal(it, got.Message, "msg-1")
			assert.NotEmpty(it, got.Fields)
			assert.Equal(it, got.Fields["foo"], 42)
			assert.Equal(it, got.Fields["bar"], 24)
		})
	})
}

func ExampleTesting() {
	var tb testing.TB

	logger.Testing(tb)

	// somewhere in your application
	logger.Debug(context.Background(), "the logging message", logging.Field("bar", 24))

}

func TestTesting(t *testing.T) {
	t.Run("package level", func(t *testing.T) {
		logger.Stub(t)
		logger.Configure(func(l *logging.Logger) {
			l.MessageKey = "msg"
			l.LevelKey = "lvl"
		})

		var dtb TestingTBDouble
		logger.Testing(&dtb)

		ctx := logging.ContextWith(context.Background(), logging.Field("foo", 42))
		logger.Debug(ctx, "msg-1", logging.Field("bar", 24))
		logger.Info(ctx, "msg-2", logging.Field("baz", []int{1, 2, 3}))
		assert.OneOf(t, dtb.Logs, func(it testing.TB, got []any) {
			entry := fmt.Sprint(got...)
			assert.Contains(it, entry, "[debug] msg-1")
			assert.Contains(it, entry, `foo = 42`)
			assert.Contains(it, entry, `bar = 24`)
		})
		assert.OneOf(t, dtb.Logs, func(it testing.TB, got []any) {
			entry := fmt.Sprint(got...)
			assert.Contains(it, entry, "[info] msg-2")
			assert.Contains(it, entry, `foo = 42`)
			assert.Contains(it, entry, fmt.Sprintf(`baz = %s`, pp.Format([]int{1, 2, 3})))
		})
	})
	t.Run("individual log level", func(t *testing.T) {
		out := logger.Stub(t)

		var dtb TestingTBDouble
		logger.Testing(&dtb)

		ctx := logging.ContextWith(context.Background(), logging.Field("foo", 42))
		logger.Debug(ctx, "msg-1", logging.Field("bar", 24))
		logger.Info(ctx, "msg-2", logging.Field("baz", []int{1, 2, 3}))

		assert.OneOf(t, dtb.Logs, func(it testing.TB, got []any) {
			entry := fmt.Sprint(got...)
			assert.Contains(it, entry, "[debug] msg-1")
			assert.Contains(it, entry, `foo = 42`)
			assert.Contains(it, entry, `bar = 24`)
		})
		assert.OneOf(t, dtb.Logs, func(it testing.TB, got []any) {
			entry := fmt.Sprint(got...)
			assert.Contains(it, entry, "[info] msg-2")
			assert.Contains(it, entry, `foo = 42`)
			assert.Contains(it, entry, fmt.Sprintf(`baz = %s`, pp.Format([]int{1, 2, 3})))
		})

		assert.Empty(t, out.Bytes())
	})
}

func TestTesting_spike(t *testing.T) {
	ctx := context.Background()

	// this is ignored due to logging is disabled by default during the tests, to avoid polluting the test outputs
	logger.Debug(ctx, "ignored")

	logger.Testing(t)
	logger.Debug(ctx, "msg", logging.Field("bar", 24))
	logger.Info(ctx, "msg", logging.Field("bar", 24))
	logger.Warn(ctx, "msg", logging.Field("bar", 24))
	logger.Error(ctx, "msg", logging.Field("bar", 24))
	logger.Fatal(ctx, "msg", logging.Field("bar", 24))
	logger.Info(ctx, "fields", logging.Fields{
		"[]int":          []int{1, 2, 3},
		"map[string]int": map[string]int{"The answer is": 42},
	})
}

type TestingTBDouble struct {
	teardown.Teardown
	Logs [][]any
}

func (tb *TestingTBDouble) Helper() {}

func (tb *TestingTBDouble) Cleanup(f func()) {
	tb.Teardown.Defer(func() error {
		f()
		return nil
	})
}

func (tb *TestingTBDouble) Log(args ...any) {
	tb.Logs = append(tb.Logs, args)
}

func TestHijack(t *testing.T) {
	out := logger.Stub(t)
	var lastMessage string
	logger.Hijack(func(level logging.Level, msg string, fields logging.Fields) {
		assert.NotEmpty(t, level.String())
		assert.NotEmpty(t, msg)
		assert.NotEmpty(t, fields)
		assert.Equal(t, fields, logging.Fields{"key": "value"})
		lastMessage = msg
	})
	assert.Empty(t, lastMessage)
	logger.Info(context.Background(), "ok", logging.Field("key", "value"))
	assert.Equal(t, lastMessage, "ok")
	assert.Empty(t, out.String())
}
