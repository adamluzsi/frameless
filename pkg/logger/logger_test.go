package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/pkg/stringcase"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func Test_smoke(t *testing.T) {
	ctx := context.Background()
	buf := logger.Stub(t)
	logger.Default.KeyFormatter = stringcase.ToPascal

	// you can add details to context, thus every logging call using this context
	ctx = logger.ContextWith(ctx, logger.Fields{
		"foo": "bar",
		"baz": "qux",
	})

	// You can use your own Logger instance or the logger.Default logger instance if you plan to log to the STDOUT.
	logger.Info(ctx, "foo", logger.Fields{
		"userID":    42,
		"accountID": 24,
	})

	t.Log(buf.String())
}

func TestLogger_smoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())
	rnd := random.New(random.CryptoSeed{})

	t.Run("log methods accept nil context", func(t *testing.T) {
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}
		l.Debug(nil, "Debug")
		l.Info(nil, "Info")
		l.Warn(nil, "Warn")
		l.Error(nil, "Error")
		l.Fatal(nil, "Fatal")
		assert.Contain(t, buf.String(), "Debug")
		assert.Contain(t, buf.String(), "Info")
		assert.Contain(t, buf.String(), "Warn")
		assert.Contain(t, buf.String(), "Error")
		assert.Contain(t, buf.String(), "Fatal")
	})

	t.Run("output is a valid JSON by default", func(t *testing.T) {
		ctx := context.Background()
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}

		expected := rnd.Repeat(3, 7, func() {
			l.Info(ctx, rnd.String())
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

		t.Run("but marshaling can be configured through the MarshalFunc", func(t *testing.T) {
			ctx := context.Background()
			buf := &bytes.Buffer{}
			l := logger.Logger{Out: buf, MarshalFunc: func(a any) ([]byte, error) {
				assert.NotEmpty(t, a)
				assert.Contain(t, fmt.Sprintf("%#v", a), "msg")
				return []byte("Hello, world!"), nil
			}}
			l.Info(ctx, "msg")
			assert.Contain(t, buf.String(), "Hello, world!")
		})
	})

	t.Run("log entries split by lines", func(t *testing.T) {
		ctx := context.Background()
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}
		expected := rnd.Repeat(3, 7, func() {
			l.Info(ctx, rnd.String())
		})
		gotEntries := strings.Split(buf.String(), "\n")
		if li := len(gotEntries) - 1; gotEntries[li] == "" {
			// remove the last empty line
			gotEntries = gotEntries[:li]
		}
		assert.Equal(t, expected, len(gotEntries))

		t.Run("but if the separator is configured", func(t *testing.T) {
			buf := &bytes.Buffer{}
			l := logger.Logger{Out: buf, Separator: "|"}
			expected := rnd.Repeat(3, 7, func() {
				l.Info(ctx, rnd.UUID())
			})
			gotEntries := strings.Split(buf.String(), "|")
			if li := len(gotEntries) - 1; gotEntries[li] == "" {
				// remove the last empty line
				gotEntries = gotEntries[:li]
			}
			assert.Equal(t, expected, len(gotEntries))
		})
	})

	t.Run("message, timestamp, level and all details are logged, including from context", func(t *testing.T) {
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}

		ctx := context.Background()
		ctx = logger.ContextWith(ctx, logger.Fields{"foo": "bar"})
		ctx = logger.ContextWith(ctx, logger.Fields{"bar": 42})

		l.Info(ctx, "a", logger.Fields{"info": "level"})
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), `"info":"level"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"message":"a"`)
		assert.Contain(t, buf.String(), `"bar":42`)
		assert.Contain(t, buf.String(), `"level":"info"`)

		t.Log("on all levels")
		l.Debug(ctx, "b", logger.Fields{"debug": "level"})
		assert.Contain(t, buf.String(), `"message":"b"`)
		assert.Contain(t, buf.String(), `"debug":"level"`)
		l.Warn(ctx, "c", logger.Fields{"warn": "level"})
		assert.Contain(t, buf.String(), `"message":"c"`)
		assert.Contain(t, buf.String(), `"level":"warn"`)
		assert.Contain(t, buf.String(), `"warn":"level"`)
		l.Error(ctx, "d", logger.Fields{"error": "level"})
		assert.Contain(t, buf.String(), `"message":"d"`)
		assert.Contain(t, buf.String(), `"level":"error"`)
		assert.Contain(t, buf.String(), `"error":"level"`)
		l.Fatal(ctx, "e", logger.Fields{"fatal": "level"})
		assert.Contain(t, buf.String(), `"message":"e"`)
		assert.Contain(t, buf.String(), `"level":"fatal"`)
		assert.Contain(t, buf.String(), `"fatal":"level"`)
	})

	t.Run("keys can be configured", func(t *testing.T) {
		ctx := context.Background()
		buf := &bytes.Buffer{}
		l := logger.Logger{Out: buf}

		l.MessageKey = rnd.UUID()
		l.TimestampKey = rnd.UUID()
		l.LevelKey = rnd.UUID()

		l.Info(ctx, "foo")
		fm := defaultKeyFormatter
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.TimestampKey), now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.MessageKey), "foo"))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.LevelKey), "info"))
	})

	t.Run("by default, it will print into the stdout", func(t *testing.T) {
		ogSTDOUT := os.Stdout
		defer func() { os.Stdout = ogSTDOUT }()

		tmpDir := t.TempDir()
		tmpFile, err := os.CreateTemp(tmpDir, "out")
		assert.NoError(t, err)
		os.Stdout = tmpFile

		l := logger.Logger{}
		l.Info(context.Background(), "msg")

		_, err = tmpFile.Seek(0, io.SeekStart)
		assert.NoError(t, err)

		bs, err := io.ReadAll(tmpFile)
		assert.NoError(t, err)
		assert.Contain(t, string(bs), `"message":"msg"`)
	})

	t.Run("logging key string format is consistent based based on the supplied KeyFormatter", func(t *testing.T) {
		var (
			ctx       = context.Background()
			buf       = &bytes.Buffer{}
			formatter = rnd.SliceElement([]func(string) string{
				stringcase.ToPascal,
				stringcase.ToCamel,
				stringcase.ToKebab,
				stringcase.ToSnake,
				stringcase.ToScreamingSnake,
			}).(func(string) string)
			l = logger.Logger{
				Out:          buf,
				KeyFormatter: formatter,
			}
		)

		var (
			exampleKey1 = "Hello_World-HTTP"
			exampleKey2 = "HTTPFoo"
		)
		l.LevelKey = "lvl-key"
		l.MessageKey = "message_key"
		l.TimestampKey = "tsKey"

		var (
			expectedKey1 = formatter(exampleKey1)
			expectedKey2 = formatter(exampleKey2)
			expectedKey3 = formatter(l.MessageKey)
			expectedKey4 = formatter(l.TimestampKey)
			expectedKey5 = formatter(l.LevelKey)
		)

		l.Info(ctx, "msg", l.Field(exampleKey1, map[string]string{exampleKey2: "qux"}))
		assert.Contain(t, buf.String(), expectedKey1)
		assert.Contain(t, buf.String(), expectedKey2)
		assert.Contain(t, buf.String(), expectedKey3)
		assert.Contain(t, buf.String(), expectedKey4)
		assert.Contain(t, buf.String(), expectedKey5)
	})

	t.Run("logging key string format is consistent even in absence of KeyFormatter, with snake_case format", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			l   = logger.Logger{Out: buf}
		)

		var (
			exampleKey1 = "Hello_World-HTTP"
			exampleKey2 = "HTTPFoo"
		)
		var (
			expectedKey1 = stringcase.ToSnake(exampleKey1)
			expectedKey2 = stringcase.ToSnake(exampleKey2)
		)

		l.Info(ctx, "msg", l.Field(exampleKey1, map[string]string{exampleKey2: "qux"}))

		assert.Contain(t, buf.String(), expectedKey1)
		assert.Contain(t, buf.String(), expectedKey2)
	})
}

func TestLogger_concurrentUse(t *testing.T) {
	var (
		ctx = context.Background()
		buf = logger.Stub(t)
	)

	write := func() {
		logger.Info(ctx, "msg")
	}

	var writes = random.Slice(10000, func() func() { return write })

	testcase.Race(write, write, writes...)

	type LogEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}

	dec := json.NewDecoder(buf)
	for dec.More() {
		var le LogEntry
		assert.NoError(t, dec.Decode(&le))
	}
}
