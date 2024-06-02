package logger

import (
	"context"

	"go.llib.dev/frameless/pkg/logging"
)

// ErrField
//
// DEPRECATED: functionality is moved to logging package, use it from there.
func ErrField(err error) logging.Detail {
	return logging.ErrField(err)
}

// LogWithTB is backward compatibility.
//
// DEPRECATED: use logger.Testing instead
var LogWithTB = Testing

// Field creates a single key value pair based logging detail.
// It will enrich the log entry with a value in the key you gave.
//
// DEPRECATED: functionality is moved to logging package, use it from there.
func Field(key string, value any) logging.Detail {
	return logging.Field(key, value)
}

// Fields is a collection of field that you can add to your loggig record.
// It will enrich the log entry with a value in the key you gave.
//
// DEPRECATED: functionality is moved to logging package, use it from there.
type Fields = logging.Fields

// ContextWith
//
// DEPRECATED: functionality is moved to logging package, use it from there.
func ContextWith(ctx context.Context, lds ...logging.Detail) context.Context {
	return logging.ContextWith(ctx, lds...)
}
