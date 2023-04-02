package logger

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
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

func Field(key string, value any) LoggingDetail {
	return Default.Field(key, value)
}

func ErrField(err error) LoggingDetail {
	if err == nil {
		return nullLoggingDetail{}
	}
	details := Details{
		"message": err.Error(),
	}
	if usrErr := (errorutil.UserError{}); errors.As(err, &usrErr) {
		details["code"] = usrErr.ID.String()
	}
	return Field("error", details)
}
