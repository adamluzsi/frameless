package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

func Test_smoke(t *testing.T) {
	ctx := context.Background()
	l, buf := logging.Stub(t)
	l.KeyFormatter = stringkit.ToPascal

	// you can add details to context, thus every logging call using this context
	ctx = logging.ContextWith(ctx, logging.Fields{
		"foo": "bar",
		"baz": "qux",
	})

	// You can use your own Logger instance or the logger.Default logger instance if you plan to log to the STDOUT.
	l.Info(ctx, "foo", logging.Fields{
		"userID":    42,
		"accountID": 24,
	})

	t.Log(buf.String())
}

func TestLogger_smoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze)
	rnd := random.New(random.CryptoSeed{})

	t.Run("log methods accept nil context", func(t *testing.T) {
		buf := &bytes.Buffer{}
		l := logging.Logger{Out: buf, Level: logging.LevelDebug}
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
		l := logging.Logger{Out: buf}

		expected := rnd.Repeat(3, 7, func() {
			l.Info(ctx, rnd.String())
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

		t.Run("but marshaling can be configured through the MarshalFunc", func(t *testing.T) {
			ctx := context.Background()
			buf := &bytes.Buffer{}
			l := logging.Logger{Out: buf, MarshalFunc: func(a any) ([]byte, error) {
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
		l := logging.Logger{Out: buf}
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
			l := logging.Logger{Out: buf, Separator: "|"}
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
		l := logging.Logger{Out: buf, Level: logging.LevelDebug}

		ctx := context.Background()
		ctx = logging.ContextWith(ctx, logging.Fields{"foo": "bar"})
		ctx = logging.ContextWith(ctx, logging.Fields{"bar": 42})

		l.Info(ctx, "a", logging.Fields{"info": "level"})
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), `"info":"level"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"message":"a"`)
		assert.Contain(t, buf.String(), `"bar":42`)
		assert.Contain(t, buf.String(), `"level":"info"`)

		t.Log("on all levels")
		l.Debug(ctx, "b", logging.Fields{"debug": "level"})
		assert.Contain(t, buf.String(), `"message":"b"`)
		assert.Contain(t, buf.String(), `"debug":"level"`)
		l.Warn(ctx, "c", logging.Fields{"warn": "level"})
		assert.Contain(t, buf.String(), `"message":"c"`)
		assert.Contain(t, buf.String(), `"level":"warn"`)
		assert.Contain(t, buf.String(), `"warn":"level"`)
		l.Error(ctx, "d", logging.Fields{"error": "level"})
		assert.Contain(t, buf.String(), `"message":"d"`)
		assert.Contain(t, buf.String(), `"level":"error"`)
		assert.Contain(t, buf.String(), `"error":"level"`)
		l.Fatal(ctx, "e", logging.Fields{"fatal": "level"})
		assert.Contain(t, buf.String(), `"message":"e"`)
		assert.Contain(t, buf.String(), `"level":"fatal"`)
		assert.Contain(t, buf.String(), `"fatal":"level"`)
	})

	t.Run("keys can be configured", func(t *testing.T) {
		ctx := context.Background()
		buf := &bytes.Buffer{}
		l := logging.Logger{Out: buf}

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

		l := logging.Logger{}
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
			formatter = rnd.Pick([]func(string) string{
				stringkit.ToPascal,
				stringkit.ToCamel,
				stringkit.ToKebab,
				stringkit.ToSnake,
				stringkit.ToScreamingSnake,
			}).(func(string) string)
			l = logging.Logger{
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

		l.Info(ctx, "msg", logging.Field(exampleKey1, map[string]string{exampleKey2: "qux"}))
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
			l   = logging.Logger{Out: buf}
		)

		var (
			exampleKey1 = "Hello_World-HTTP"
			exampleKey2 = "HTTPFoo"
		)
		var (
			expectedKey1 = stringkit.ToSnake(exampleKey1)
			expectedKey2 = stringkit.ToSnake(exampleKey2)
		)

		l.Info(ctx, "msg", logging.Field(exampleKey1, map[string]string{exampleKey2: "qux"}))

		assert.Contain(t, buf.String(), expectedKey1)
		assert.Contain(t, buf.String(), expectedKey2)
	})

	t.Run("logging can be hijacked to support piping logging requests into other logging libraries", func(t *testing.T) {
		type LogEntry struct {
			Level   logging.Level
			Message string
			Fields  logging.Fields
		}
		var (
			ctx  = context.Background()
			logs = make([]LogEntry, 0)
			buf  = &bytes.Buffer{}
			l    = logging.Logger{
				Out: buf,
				Hijack: func(level logging.Level, msg string, fields logging.Fields) {
					logs = append(logs, LogEntry{Level: level, Message: msg, Fields: fields})
				},
				Level: logging.LevelFatal,
			}
		)

		ctx = logging.ContextWith(ctx,
			logging.Field("foo", "1"),
			logging.Field("bar", 2),
			logging.Field("baz", true))

		l.Info(ctx, "the info message", logging.Fields{"qux": "xuq"})
		l.Debug(ctx, "the debug message", logging.Fields{"qux": "xuq"})

		assert.Equal(t, 0, buf.Len())
		assert.Equal(t, 2, len(logs))

		assert.OneOf(t, logs, func(it assert.It, got LogEntry) {
			it.Must.Equal("the info message", got.Message)
			it.Must.Equal(logging.LevelInfo, got.Level)
			assert.Equal[any](it, "xuq", got.Fields["qux"])
			assert.Equal[any](it, "1", got.Fields["foo"])
			assert.Equal[any](it, 2, got.Fields["bar"])
			assert.Equal[any](it, true, got.Fields["baz"])
		})
	})
}

func TestLogger_concurrentUse(t *testing.T) {
	type LogEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	t.Run("default", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &TeeBuffer{}
			l   = logging.Logger{Out: buf}
		)

		write := func() { l.Info(ctx, "msg") }
		var writes = random.Slice(10000, func() func() { return write })

		testcase.Race(write, write, writes...)

		defer func() {
			if t.Failed() {
				t.Log("contains null char:", strings.Contains(buf.B.String(), "\\x00"))
			}
		}()

		dec := json.NewDecoder(buf)
		for dec.More() {
			var le LogEntry
			assert.NoError(t, dec.Decode(&le), assert.Message(pp.Format(buf.B.String())))
		}
	})

}

func TestLogger_Clone(t *testing.T) {
	var (
		buf = &bytes.Buffer{}
		lgr = logging.Logger{Out: buf}
		cln = lgr.Clone()
	)
	assert.Equal(t, &lgr, cln)
	ogLevel := lgr.Level
	cln.Level = logging.LevelDebug
	assert.Equal(t, ogLevel, lgr.Level)
	assert.Equal(t, logging.LevelDebug, cln.Level)
	assert.NotEqual(t, &lgr, cln)
	cln.Debug(context.Background(), "msg")
	assert.Contain(t, buf.String(), "msg")
}

type TeeBuffer struct {
	A, B iokit.Buffer
}

func (buf *TeeBuffer) Write(p []byte) (n int, err error) {
	n, err = buf.A.Write(p)
	_, _ = buf.B.Write(p)
	return
}

func (buf *TeeBuffer) Read(p []byte) (n int, err error) {
	return buf.A.Read(p)
}

func TestLogger_AsyncLogging_smoke(t *testing.T) {
	var (
		l   logging.Logger
		out = &bytes.Buffer{}
		m   sync.Mutex
	)
	l.Out = &iokit.SyncWriter{
		Writer: out,
		Locker: &m,
	}
	l.MessageKey = "msg"
	l.KeyFormatter = stringkit.ToPascal
	l.FlushTimeout = time.Millisecond
	defer l.AsyncLogging()()

	l.Info(context.Background(), "gsm", logging.Field("fieldKey", "value"))

	assert.Eventually(t, 3*time.Second, func(it assert.It) {
		m.Lock()
		defer m.Unlock()

		it.Must.Contain(out.String(), `"Msg":"gsm"`)
		it.Must.Contain(out.String(), `"FieldKey":"value"`)
	})
}
