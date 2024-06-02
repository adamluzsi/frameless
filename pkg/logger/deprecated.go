package logger

import "go.llib.dev/frameless/pkg/logging"

// ErrField
//
// Deprecated: use logging.ErrField
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
// DEPRECATED: Field in logger is deprecated, please use logging.Field instead.
func Field(key string, value any) logging.Detail {
	return logging.Field(key, value)
}

// Fields is a collection of field that you can add to your loggig record.
// It will enrich the log entry with a value in the key you gave.
//
// DEPRECATED: Fields in logger is deprecated, please use logging.Fields instead.
type Fields = logging.Fields
