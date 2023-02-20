package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogger_smoke(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())
	rnd := random.New(random.CryptoSeed{})

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
			msg := logger.Details{}
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
				l.Info(ctx, rnd.String())
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
		ctx = logger.ContextWithDetails(ctx, logger.Details{"foo": "bar"})
		ctx = logger.ContextWithDetails(ctx, logger.Details{"bar": 42})

		l.Info(ctx, "a", logger.Details{"info": "level"})
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), `"info":"level"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"message":"a"`)
		assert.Contain(t, buf.String(), `"bar":42`)
		assert.Contain(t, buf.String(), `"level":"info"`)

		t.Log("on all levels")
		l.Debug(ctx, "b", logger.Details{"debug": "level"})
		assert.Contain(t, buf.String(), `"message":"b"`)
		assert.Contain(t, buf.String(), `"debug":"level"`)
		l.Warn(ctx, "c", logger.Details{"warn": "level"})
		assert.Contain(t, buf.String(), `"message":"c"`)
		assert.Contain(t, buf.String(), `"level":"warn"`)
		assert.Contain(t, buf.String(), `"warn":"level"`)
		l.Error(ctx, "d", logger.Details{"error": "level"})
		assert.Contain(t, buf.String(), `"message":"d"`)
		assert.Contain(t, buf.String(), `"level":"error"`)
		assert.Contain(t, buf.String(), `"error":"level"`)
		l.Fatal(ctx, "e", logger.Details{"fatal": "level"})
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
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, l.TimestampKey, now.Format(time.RFC3339)))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, l.MessageKey, "foo"))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, l.LevelKey, "info"))
	})

	t.Run("by default, it will print into the stdout", func(t *testing.T) {
		ogSTDOUT := os.Stdout
		defer func() { os.Stdout = ogSTDOUT }()

		tmpDir := t.TempDir()
		tmpFile, err := os.CreateTemp(tmpDir, "out")
		assert.NoError(t, err)
		os.Stdout = tmpFile

		logger.Logger{}.Info(context.Background(), "msg")

		_, err = tmpFile.Seek(0, io.SeekStart)
		assert.NoError(t, err)

		bs, err := io.ReadAll(tmpFile)
		assert.NoError(t, err)
		assert.Contain(t, string(bs), `"message":"msg"`)
	})
}
