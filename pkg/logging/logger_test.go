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

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

const defaultLevelKey = "level"
const defaultMessageKey = "message"
const defaultTimestampKey = "timestamp"

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

	t.Run("default field keys", func(t *testing.T) {
		now := rnd.Time().In(time.Local)
		timecop.Travel(t, now, timecop.Freeze)
		buf := &bytes.Buffer{}
		l := logging.Logger{Out: buf, Level: logging.LevelDebug}
		assert.Empty(t, l.LevelKey)
		assert.Empty(t, l.MessageKey)
		assert.Empty(t, l.TimestampKey)
		ctx := context.Background()

		l.Info(ctx, "msg")
		assert.Contains(t, buf.String(), fmt.Sprintf(`%q:"info"`, defaultLevelKey))
		assert.Contains(t, buf.String(), fmt.Sprintf(`%q:"msg"`, defaultMessageKey))
		assert.Contains(t, buf.String(), fmt.Sprintf(`%q:"%s"`, defaultTimestampKey, now.Format(time.RFC3339)))
	})

	t.Run("log methods accept nil context", func(t *testing.T) {
		buf := &bytes.Buffer{}
		l := logging.Logger{Out: buf, Level: logging.LevelDebug}
		var nilContext context.Context = nil
		l.Debug(nilContext, "Debug")
		l.Info(nilContext, "Info")
		l.Warn(nilContext, "Warn")
		l.Error(nilContext, "Error")
		l.Fatal(nilContext, "Fatal")
		assert.Contains(t, buf.String(), "Debug")
		assert.Contains(t, buf.String(), "Info")
		assert.Contains(t, buf.String(), "Warn")
		assert.Contains(t, buf.String(), "Error")
		assert.Contains(t, buf.String(), "Fatal")
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
				assert.Contains(t, fmt.Sprintf("%#v", a), "msg")
				return []byte("Hello, world!"), nil
			}}
			l.Info(ctx, "msg")
			assert.Contains(t, buf.String(), "Hello, world!")
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
		assert.Contains(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contains(t, buf.String(), `"info":"level"`)
		assert.Contains(t, buf.String(), `"foo":"bar"`)
		assert.Contains(t, buf.String(), `"message":"a"`)
		assert.Contains(t, buf.String(), `"bar":42`)
		assert.Contains(t, buf.String(), `"level":"info"`)

		t.Log("on all levels")
		l.Debug(ctx, "b", logging.Fields{"debug": "level"})
		assert.Contains(t, buf.String(), `"message":"b"`)
		assert.Contains(t, buf.String(), `"debug":"level"`)
		l.Warn(ctx, "c", logging.Fields{"warn": "level"})
		assert.Contains(t, buf.String(), `"message":"c"`)
		assert.Contains(t, buf.String(), `"level":"warn"`)
		assert.Contains(t, buf.String(), `"warn":"level"`)
		l.Error(ctx, "d", logging.Fields{"error": "level"})
		assert.Contains(t, buf.String(), `"message":"d"`)
		assert.Contains(t, buf.String(), `"level":"error"`)
		assert.Contains(t, buf.String(), `"error":"level"`)
		l.Fatal(ctx, "e", logging.Fields{"fatal": "level"})
		assert.Contains(t, buf.String(), `"message":"e"`)
		assert.Contains(t, buf.String(), `"level":"fatal"`)
		assert.Contains(t, buf.String(), `"fatal":"level"`)
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
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.TimestampKey), now.Format(time.RFC3339)))
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.MessageKey), "foo"))
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, fm(l.LevelKey), "info"))
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
		assert.Contains(t, string(bs), `"message":"msg"`)
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
		assert.Contains(t, buf.String(), expectedKey1)
		assert.Contains(t, buf.String(), expectedKey2)
		assert.Contains(t, buf.String(), expectedKey3)
		assert.Contains(t, buf.String(), expectedKey4)
		assert.Contains(t, buf.String(), expectedKey5)
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

		assert.Contains(t, buf.String(), expectedKey1)
		assert.Contains(t, buf.String(), expectedKey2)
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
				Hijack: func(ctx context.Context, level logging.Level, msg string, fields logging.Fields) {
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

		assert.OneOf(t, logs, func(it testing.TB, got LogEntry) {
			assert.Equal(it, "the info message", got.Message)
			assert.Equal(it, logging.LevelInfo, got.Level)
			assert.Equal(it, "xuq", got.Fields["qux"])
			assert.Equal(it, "1", got.Fields["foo"])
			assert.Equal(it, 2, got.Fields["bar"])
			assert.Equal(it, true, got.Fields["baz"])
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
	assert.Contains(t, buf.String(), "msg")
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

	assert.Eventually(t, 3*time.Second, func(it testing.TB) {
		m.Lock()
		defer m.Unlock()

		assert.Contains(it, out.String(), `"Msg":"gsm"`)
		assert.Contains(it, out.String(), `"FieldKey":"value"`)
	})
}

func TestLogger_Log(t *testing.T) {
	s := testcase.NewSpec(t)

	type Message struct {
		Context context.Context
		Level   logging.Level
		Message string
		Fields  logging.Fields
	}

	messages := let.VarOf[[]Message](s, nil)

	logger := let.Var(s, func(t *testcase.T) *logging.Logger {
		return &logging.Logger{
			Hijack: func(ctx context.Context, level logging.Level, msg string, fields logging.Fields) {
				testcase.Append(t, messages, Message{Context: ctx, Level: level, Message: msg, Fields: fields})
			},
			KeyFormatter: stringkit.ToSnake,
		}
	})

	type ctxKey struct{}

	var (
		ctxVal  = let.String(s)
		Context = let.Var(s, func(t *testcase.T) context.Context {
			return context.WithValue(t.Context(), ctxKey{}, ctxVal.Get(t))
		})
		level   = let.OneOf(s, enum.Values[logging.Level]()...)
		message = let.Var(s, func(t *testcase.T) string {
			return t.Random.String()
		})
		details = let.VarOf[[]logging.Detail](s, nil)
	)
	act := let.Act0(func(t *testcase.T) {
		logger.Get(t).Log(Context.Get(t), level.Get(t), message.Get(t), details.Get(t)...)
	})

	s.Then("log entry is made using the inputs", func(t *testcase.T) {
		act(t)

		assert.NotEmpty(t, messages.Get(t))
		assert.OneOf(t, messages.Get(t), func(tb testing.TB, msg Message) {
			assert.Equal(tb, msg, Message{
				Context: Context.Get(t),
				Level:   level.Get(t),
				Message: message.Get(t),
				Fields:  logging.Fields{},
			})
		})
	})

	s.When("detail is given", func(s *testcase.Spec) {
		details.Let(s, func(t *testcase.T) []logging.Detail {
			return []logging.Detail{
				logging.Field("foo", "1"),
				logging.Fields{"bar": 2, "baz": "three"},
			}
		})

		s.Then("logging entry is generated", func(t *testcase.T) {
			act(t)

			assert.NotEmpty(t, messages.Get(t))
			assert.OneOf(t, messages.Get(t), func(tb testing.TB, msg Message) {
				assert.NotNil(tb, msg.Context)
				assert.Equal[any](t, msg.Context.Value(ctxKey{}), ctxVal.Get(t))
				assert.Equal(tb, msg.Level, level.Get(t))
				assert.Equal(tb, msg.Message, message.Get(t))
				assert.Equal(tb, msg.Fields, logging.Fields{"foo": "1", "bar": 2, "baz": "three"})
			})
		})

		s.And("a detail key is not aligned with the logoutput", func(s *testcase.Spec) {
			details.Let(s, func(t *testcase.T) []logging.Detail {
				return append(details.Super(t), logging.Field("Qux", logging.Fields{
					"Abc": "a",
					"Bcd": "b",
					"Dce": "b",
				}))
			})

			s.Then("the outcome logging fields will have formatted logs", func(t *testcase.T) {
				act(t)

				assert.NotEmpty(t, messages.Get(t))
				assert.OneOf(t, messages.Get(t), func(tb testing.TB, msg Message) {
					assert.Equal(tb, msg.Fields, logging.Fields{
						"foo": "1",
						"bar": 2,
						"baz": "three",
						"qux": map[string]any{
							"abc": "a",
							"bcd": "b",
							"dce": "b",
						},
					})

				})
			})
		})

		s.And("the details contain a mapped struct type", func(s *testcase.Spec) {
			type T struct {
				Foo string
				Bar int
				Baz bool
			}

			s.Before(func(t *testcase.T) {
				t.Cleanup(logging.RegisterType[T](func(ctx context.Context, v T) logging.Detail {
					return logging.Fields{
						"FOO": v.Foo,
						"BAR": v.Bar,
						"BAZ": v.Baz,
						"QUX": logging.Fields{
							"FOO": v.Foo,
							"BAR": v.Bar,
							"BAZ": v.Baz,
						},
						"quux": map[string]any{
							"FOO": v.Foo,
							"BAR": v.Bar,
							"BAZ": v.Baz,
						},
					}
				}))
			})

			v := let.Var(s, func(t *testcase.T) T {
				return T{
					Foo: t.Random.HexN(5),
					Bar: t.Random.Int(),
					Baz: t.Random.Bool(),
				}
			})

			details.Let(s, func(t *testcase.T) []logging.Detail {
				return append(details.Super(t), logging.Field("val", v.Get(t)))
			})

			s.Then("the outcome logging fields will contain the field", func(t *testcase.T) {
				act(t)

				assert.NotEmpty(t, messages.Get(t))
				assert.OneOf(t, messages.Get(t), func(tb testing.TB, msg Message) {
					assert.NotNil(t, msg.Fields)

					val, ok := msg.Fields["val"].(map[string]any)
					assert.True(t, ok)
					assert.NotNil(t, val)
					assert.Equal[any](t, val["foo"], v.Get(t).Foo)
					assert.Equal[any](t, val["bar"], v.Get(t).Bar)
					assert.Equal[any](t, val["baz"], v.Get(t).Baz)

					qux, ok := val["qux"].(map[string]any)
					assert.True(t, ok)
					assert.NotNil(t, qux)
					assert.Equal[any](t, qux["foo"], v.Get(t).Foo)
					assert.Equal[any](t, qux["bar"], v.Get(t).Bar)
					assert.Equal[any](t, qux["baz"], v.Get(t).Baz)
				})
			})
		})
	})
}
