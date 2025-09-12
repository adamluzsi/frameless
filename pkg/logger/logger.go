package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"go.llib.dev/frameless/pkg/internal/testcheck"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase/pp"
)

func Debug(ctx context.Context, msg string, ds ...logging.Detail) {
	tb(logger.TestingTB).Helper()
	logger.Debug(ctx, msg, ds...)
}

func Info(ctx context.Context, msg string, ds ...logging.Detail) {
	tb(logger.TestingTB).Helper()
	logger.Info(ctx, msg, ds...)
}

func Warn(ctx context.Context, msg string, ds ...logging.Detail) {
	tb(logger.TestingTB).Helper()
	logger.Warn(ctx, msg, ds...)
}

func Error(ctx context.Context, msg string, ds ...logging.Detail) {
	tb(logger.TestingTB).Helper()
	logger.Error(ctx, msg, ds...)
}

func Fatal(ctx context.Context, msg string, ds ...logging.Detail) {
	tb(logger.TestingTB).Helper()
	logger.Fatal(ctx, msg, ds...)
}

func AsyncLogging() func() { return logger.AsyncLogging() }

func Hijack(fn logging.HijackFunc) {
	Configure(func(l *logging.Logger) { l.Hijack = fn })
}

type ConfigurationFunc func(*logging.Logger)

func Configure(blk ConfigurationFunc) struct{} {
	blk(logger)
	return struct{}{}
}

// Stub the logger.Default and return the buffer where the logging output will be recorded.
// Stub will restore the logger.Default after the test.
// optionally, the stub logger can be further configured by passing a configuration function block
func Stub(tb testingTB, optionalConfiguration ...ConfigurationFunc) logging.StubOutput {
	tb.Helper()

	original := logger
	tb.Cleanup(func() { logger = original })

	l, out := logging.Stub(tb)
	logger = logger.Clone()
	logger.Out = l.Out
	logger.Level = logging.LevelDebug
	logger.TestingTB = tb

	for _, configure := range optionalConfiguration {
		configure(logger)
	}

	return out
}

// Testing pipes all application log generated during the test execution through the testing.TB's Log method.
// Testing meant to help debugging your application during your TDD flow.
func Testing(tb testingTB) {
	tb.Helper()
	Stub(tb, func(l *logging.Logger) {
		tb.Helper()
		l.Hijack = testingTBLogger(tb)
	})
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var logger *logging.Logger = &logging.Logger{}

var defaultLevel logging.Level = logging.LevelInfo

func init() {
	if testcheck.IsDuringTestRun() {
		logger.Out = io.Discard
	}
	if level, ok := lookupLevelFromENV(); ok {
		defaultLevel = level
	}
	logger.Level = defaultLevel
}

func lookupLevelFromENV() (logging.Level, bool) {
	for _, envKey := range []string{"LOG_LEVEL", "LOGGER_LEVEL", "LOGGING_LEVEL"} {
		if raw, ok := os.LookupEnv(envKey); ok {
			if level, ok := envToLevel[strings.ToLower(raw)]; ok {
				return level, ok
			}
		}
	}
	return "", false
}

var envToLevel = map[string]logging.Level{
	"debug":    logging.LevelDebug,
	"info":     logging.LevelInfo,
	"warn":     logging.LevelWarn,
	"error":    logging.LevelError,
	"fatal":    logging.LevelFatal,
	"critical": logging.LevelFatal,

	"d": logging.LevelDebug,
	"i": logging.LevelInfo,
	"w": logging.LevelWarn,
	"e": logging.LevelError,
	"f": logging.LevelFatal,
	"c": logging.LevelFatal,
}

type testingTB interface {
	Helper()
	Cleanup(func())
	Log(args ...any)
}

func testingTBLogger(tb testingTB) func(ctx context.Context, lvl logging.Level, msg string, fields logging.Fields) {
	tb.Helper()
	return func(ctx context.Context, lvl logging.Level, msg string, fields logging.Fields) {
		tb.Helper()
		var parts []string
		parts = append(parts, fmt.Sprintf("[%s] %s", lvl.String(), msg))
		for k, v := range fields {
			parts = append(parts, fmt.Sprintf("%s = %s", k, pp.Format(v)))
		}
		tb.Log(strings.Join(parts, "\n"))
	}
}

func tb(tb testingTB) testingTB {
	if tb != nil {
		return tb
	}
	return fallbackTestingTB
}

var fallbackTestingTB = (*nullTestingTB)(nil)

type nullTestingTB struct{}

func (*nullTestingTB) Helper()        {}
func (*nullTestingTB) Cleanup(func()) {}
func (*nullTestingTB) Log(...any)     {}
