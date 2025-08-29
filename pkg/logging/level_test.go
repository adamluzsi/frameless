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
		assert.Contains(tb, buf.String(), fmt.Sprintf(`"level":"%s"`, level))
	}

	assertDoesNotHave := func(tb testing.TB, buf *bytes.Buffer, level string) {
		tb.Helper()
		assert.NotContains(tb, buf.String(), fmt.Sprintf(`"level":"%s"`, level))
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

func TestLevel_Less(t *testing.T) {
	// Debug
	assert.False(t, logging.LevelDebug.Less(logging.LevelDebug))
	assert.True(t, logging.LevelDebug.Less(logging.LevelInfo))
	assert.True(t, logging.LevelDebug.Less(logging.LevelWarn))
	assert.True(t, logging.LevelDebug.Less(logging.LevelError))
	assert.True(t, logging.LevelDebug.Less(logging.LevelFatal))
	// Info
	assert.False(t, logging.LevelInfo.Less(logging.LevelDebug))
	assert.False(t, logging.LevelInfo.Less(logging.LevelInfo))
	assert.True(t, logging.LevelInfo.Less(logging.LevelWarn))
	assert.True(t, logging.LevelInfo.Less(logging.LevelError))
	assert.True(t, logging.LevelInfo.Less(logging.LevelFatal))
	// Warn
	assert.False(t, logging.LevelWarn.Less(logging.LevelDebug))
	assert.False(t, logging.LevelWarn.Less(logging.LevelInfo))
	assert.False(t, logging.LevelWarn.Less(logging.LevelWarn))
	assert.True(t, logging.LevelWarn.Less(logging.LevelError))
	assert.True(t, logging.LevelWarn.Less(logging.LevelFatal))
	// Error
	assert.False(t, logging.LevelError.Less(logging.LevelDebug))
	assert.False(t, logging.LevelError.Less(logging.LevelInfo))
	assert.False(t, logging.LevelError.Less(logging.LevelWarn))
	assert.False(t, logging.LevelError.Less(logging.LevelError))
	assert.True(t, logging.LevelError.Less(logging.LevelFatal))
	// Fatal
	assert.False(t, logging.LevelFatal.Less(logging.LevelDebug))
	assert.False(t, logging.LevelFatal.Less(logging.LevelInfo))
	assert.False(t, logging.LevelFatal.Less(logging.LevelWarn))
	assert.False(t, logging.LevelFatal.Less(logging.LevelError))
	assert.False(t, logging.LevelFatal.Less(logging.LevelFatal))
}

func TestLevel_Can(t *testing.T) {
	// Debug
	assert.True(t, logging.LevelDebug.Can(logging.LevelDebug))
	assert.True(t, logging.LevelDebug.Can(logging.LevelInfo))
	assert.True(t, logging.LevelDebug.Can(logging.LevelWarn))
	assert.True(t, logging.LevelDebug.Can(logging.LevelError))
	assert.True(t, logging.LevelDebug.Can(logging.LevelFatal))
	// Info
	assert.False(t, logging.LevelInfo.Can(logging.LevelDebug))
	assert.True(t, logging.LevelInfo.Can(logging.LevelInfo))
	assert.True(t, logging.LevelInfo.Can(logging.LevelWarn))
	assert.True(t, logging.LevelInfo.Can(logging.LevelError))
	assert.True(t, logging.LevelInfo.Can(logging.LevelFatal))
	// Warn
	assert.False(t, logging.LevelWarn.Can(logging.LevelDebug))
	assert.False(t, logging.LevelWarn.Can(logging.LevelInfo))
	assert.True(t, logging.LevelWarn.Can(logging.LevelWarn))
	assert.True(t, logging.LevelWarn.Can(logging.LevelError))
	assert.True(t, logging.LevelWarn.Can(logging.LevelFatal))
	// Error
	assert.False(t, logging.LevelError.Can(logging.LevelDebug))
	assert.False(t, logging.LevelError.Can(logging.LevelInfo))
	assert.False(t, logging.LevelError.Can(logging.LevelWarn))
	assert.True(t, logging.LevelError.Can(logging.LevelError))
	assert.True(t, logging.LevelError.Can(logging.LevelFatal))
	// Fatal
	assert.False(t, logging.LevelFatal.Can(logging.LevelDebug))
	assert.False(t, logging.LevelFatal.Can(logging.LevelInfo))
	assert.False(t, logging.LevelFatal.Can(logging.LevelWarn))
	assert.False(t, logging.LevelFatal.Can(logging.LevelError))
	assert.True(t, logging.LevelFatal.Can(logging.LevelFatal))
}
