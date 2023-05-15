package logger

import (
	"context"
)

var Default Logger

func AsyncLogging() func() { return Default.AsyncLogging() }

func Debug(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	Default.Debug(ctx, msg, ds...)
}

func Info(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	Default.Info(ctx, msg, ds...)
}

func Warn(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	Default.Warn(ctx, msg, ds...)
}

func Error(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	Default.Error(ctx, msg, ds...)
}

func Fatal(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	Default.Fatal(ctx, msg, ds...)
}
