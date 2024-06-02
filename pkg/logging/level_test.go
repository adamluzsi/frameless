package logging_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/pkg/logging"
)

func TestLogger_level(t *testing.T) {
	assertHasLevel := func(tb testing.TB, buf *bytes.Buffer, level string) {
		tb.Helper()
		assert.Contain(tb, buf.String(), fmt.Sprintf(`"level":"%s"`, level))
	}

	assertDoesNotHave := func(tb testing.TB, buf *bytes.Buffer, level string) {
		tb.Helper()
		assert.NotContain(tb, buf.String(), fmt.Sprintf(`"level":"%s"`, level))
	}

	t.Run("when level is not set", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertDoesNotHave(t, buf, "debug")
		assertHasLevel(t, buf, "info")
		assertHasLevel(t, buf, "warn")
		assertHasLevel(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})

	t.Run("when level is DEBUG", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Level = logging.LevelDebug
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertHasLevel(t, buf, "debug")
		assertHasLevel(t, buf, "info")
		assertHasLevel(t, buf, "warn")
		assertHasLevel(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})

	t.Run("when level is INFO", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Level = logging.LevelInfo
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertDoesNotHave(t, buf, "debug")
		assertHasLevel(t, buf, "info")
		assertHasLevel(t, buf, "warn")
		assertHasLevel(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})

	t.Run("when level is WARN", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Level = logging.LevelWarn
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertDoesNotHave(t, buf, "debug")
		assertDoesNotHave(t, buf, "info")
		assertHasLevel(t, buf, "warn")
		assertHasLevel(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})

	t.Run("when level is ERROR", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Level = logging.LevelError
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertDoesNotHave(t, buf, "debug")
		assertDoesNotHave(t, buf, "info")
		assertDoesNotHave(t, buf, "warn")
		assertHasLevel(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})

	t.Run("when level is FATAL", func(t *testing.T) {
		var (
			ctx = context.Background()
			buf = &bytes.Buffer{}
			lgr = logging.Logger{Out: buf}
		)
		lgr.Level = logging.LevelFatal
		lgr.Debug(ctx, "")
		lgr.Info(ctx, "")
		lgr.Warn(ctx, "")
		lgr.Error(ctx, "")
		lgr.Fatal(ctx, "")

		assertDoesNotHave(t, buf, "debug")
		assertDoesNotHave(t, buf, "info")
		assertDoesNotHave(t, buf, "warn")
		assertDoesNotHave(t, buf, "error")
		assertHasLevel(t, buf, "fatal")
	})
}
