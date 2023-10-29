package logger_test

import (
	"bytes"
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/teardown"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
	"strings"
	"testing"
)

func ExampleStub() {
	var tb testing.TB
	buf := logger.Stub(tb) // stub will clean up after itself when the test is finished
	logger.Info(nil, "foo")
	strings.Contains(buf.String(), "foo") // true
}

func TestStub(t *testing.T) {
	var og logger.Logger // enforce variable type to guarantee pass by value copy
	og = logger.Default  // pass by value copy
	ogOut := logger.Default.Out
	t.Run("", func(t *testing.T) {
		buf := logger.Stub(t)
		l2 := logger.Default
		assert.NotEqual(t, og, l2)
		logger.Default.Info(context.Background(), "hello")
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"hello"`, defaultKeyFormatter("message")))
	})
	t.Run("mutating", func(t *testing.T) {
		rnd := random.New(random.CryptoSeed{})
		buf := logger.Stub(t)
		l2 := logger.Default
		assert.NotEqual(t, og, l2)
		logger.Default.MessageKey = rnd.UUID()
		msg := rnd.UUID()
		logger.Default.Info(context.Background(), msg)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, defaultKeyFormatter(logger.Default.MessageKey), msg))
	})
	assert.Equal(t, og, logger.Default, "logger has been restored")
	assert.Equal(t, ogOut, og.Out)
}

func ExampleLogWithTB() {
	var tb testing.TB

	logger.LogWithTB(tb)

	// somewhere in your application
	logger.Debug(nil, "the logging message", logger.Field("bar", 24))

}

func TestLogWithTB(t *testing.T) {
	t.Run("package level", func(t *testing.T) {
		logger.Stub(t)
		logger.Default.MessageKey = "msg"
		logger.Default.LevelKey = "lvl"

		var dtb TestingTBDouble
		logger.LogWithTB(&dtb)

		ctx := logger.ContextWith(context.Background(), logger.Field("foo", 42))
		logger.Debug(ctx, "msg-1", logger.Field("bar", 24))
		logger.Info(ctx, "msg-2", logger.Field("baz", []int{1, 2, 3}))
		assert.OneOf(t, dtb.Logs, func(it assert.It, got []any) {
			entry := fmt.Sprint(got...)
			it.Must.Contain(entry, "[debug] msg-1")
			it.Must.Contain(entry, `foo = 42`)
			it.Must.Contain(entry, `bar = 24`)
		})
		assert.OneOf(t, dtb.Logs, func(it assert.It, got []any) {
			entry := fmt.Sprint(got...)
			it.Must.Contain(entry, "[info] msg-2")
			it.Must.Contain(entry, `foo = 42`)
			it.Must.Contain(entry, fmt.Sprintf(`baz = %s`, pp.Format([]int{1, 2, 3})))
		})
	})
	t.Run("individual log level", func(t *testing.T) {
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}
		l.MessageKey = "msg"
		l.LevelKey = "lvl"

		var dtb TestingTBDouble
		logger.LogWithTB(&dtb)

		ctx := logger.ContextWith(context.Background(), logger.Field("foo", 42))
		l.Debug(ctx, "msg-1", logger.Field("bar", 24))
		l.Info(ctx, "msg-2", logger.Field("baz", []int{1, 2, 3}))

		assert.OneOf(t, dtb.Logs, func(it assert.It, got []any) {
			entry := fmt.Sprint(got...)
			it.Must.Contain(entry, "[debug] msg-1")
			it.Must.Contain(entry, `foo = 42`)
			it.Must.Contain(entry, `bar = 24`)
		})
		assert.OneOf(t, dtb.Logs, func(it assert.It, got []any) {
			entry := fmt.Sprint(got...)
			it.Must.Contain(entry, "[info] msg-2")
			it.Must.Contain(entry, `foo = 42`)
			it.Must.Contain(entry, fmt.Sprintf(`baz = %s`, pp.Format([]int{1, 2, 3})))
		})

		assert.Empty(t, buf.Len())
	})

	t.Run("with optional HijackFunc", func(t *testing.T) {
		logger.Stub(t)
		logger.Default.MessageKey = "msg"
		logger.Default.LevelKey = "lvl"

		type Entry struct {
			Level   logger.Level
			Message string
			Fields  logger.Fields
		}
		var entries []Entry

		var dtb TestingTBDouble
		logger.LogWithTB(&dtb, func(level logger.Level, msg string, fields logger.Fields) {
			entries = append(entries, Entry{
				Level:   level,
				Message: msg,
				Fields:  fields,
			})
		})

		ctx := logger.ContextWith(context.Background(), logger.Field("foo", 42))
		logger.Debug(ctx, "msg-1", logger.Field("bar", 24))
		logger.Info(ctx, "msg-2", logger.Field("baz", []int{1, 2, 3}))

		assert.Empty(t, dtb.Logs)
		assert.Equal(t, len(entries), 2)
		assert.OneOf(t, entries, func(it assert.It, got Entry) {
			it.Must.Equal(got.Level, logger.LevelDebug)
			it.Must.Equal(got.Message, "msg-1")
			it.Must.NotEmpty(got.Fields)
			it.Must.Equal(got.Fields["foo"], 42)
			it.Must.Equal(got.Fields["bar"], 24)
		})
	})
}

func TestLogWithTB_spike(t *testing.T) {
	logger.Debug(nil, "ignored")
	logger.LogWithTB(t)
	logger.Debug(nil, "msg", logger.Field("bar", 24))
	logger.Info(nil, "msg", logger.Field("bar", 24))
	logger.Warn(nil, "msg", logger.Field("bar", 24))
	logger.Error(nil, "msg", logger.Field("bar", 24))
	logger.Fatal(nil, "msg", logger.Field("bar", 24))
	logger.Info(nil, "fields", logger.Fields{
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
