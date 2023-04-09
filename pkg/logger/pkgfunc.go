package logger

import (
	"context"
)

var Default Logger

func Debug(ctx context.Context, msg string, ds ...LoggingDetail) {
	Default.Debug(ctx, msg, ds...)
}

func Info(ctx context.Context, msg string, ds ...LoggingDetail) {
	Default.Info(ctx, msg, ds...)
}

func Warn(ctx context.Context, msg string, ds ...LoggingDetail) {
	Default.Warn(ctx, msg, ds...)
}

func Error(ctx context.Context, msg string, ds ...LoggingDetail) {
	Default.Error(ctx, msg, ds...)
}

func Fatal(ctx context.Context, msg string, ds ...LoggingDetail) {
	Default.Fatal(ctx, msg, ds...)
}
