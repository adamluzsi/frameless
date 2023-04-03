// Package logger provides tooling for structured logging.
// With logger, you can use context to add logging details to your call stack.
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/stringcase"
	"github.com/adamluzsi/testcase/clock"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	Out io.Writer

	Separator string

	MessageKey   string
	LevelKey     string
	TimestampKey string

	// MarshalFunc is used to serialise the logging message event.
	// When nil it defaults to JSON format.
	MarshalFunc func(any) ([]byte, error)
	// KeyFormatter will be used to format the logging field keys
	KeyFormatter func(string) string

	m sync.Mutex
}

const (
	levelDefaultKey   = "level"
	messageDefaultKey = "message"
	timestampKey      = "timestamp"
)

func (l *Logger) Debug(ctx context.Context, msg string, ds ...LoggingDetail) {
	l.log(ctx, levelDebug, msg, ds)
}

func (l *Logger) Info(ctx context.Context, msg string, ds ...LoggingDetail) {
	l.log(ctx, levelInfo, msg, ds)
}

func (l *Logger) Warn(ctx context.Context, msg string, ds ...LoggingDetail) {
	l.log(ctx, levelWarn, msg, ds)
}

func (l *Logger) Error(ctx context.Context, msg string, ds ...LoggingDetail) {
	l.log(ctx, levelError, msg, ds)
}

func (l *Logger) Fatal(ctx context.Context, msg string, ds ...LoggingDetail) {
	l.log(ctx, levelFatal, msg, ds)
}

func (l *Logger) getKeyFormatter() func(string) string {
	if l.KeyFormatter != nil {
		return l.KeyFormatter
	}
	return stringcase.ToSnake
}

func (l *Logger) log(ctx context.Context, level loggingLevel, msg string, ds []LoggingDetail) {
	entry := l.toLogEntry(ctx, level, msg, ds)
	bs, err := l.marshalFunc()(entry)
	l.m.Lock()
	defer l.m.Unlock()
	if err != nil {
		fmt.Println("ERROR", "framless/pkg/logger", "Logger.MarshalFunc", err.Error())
		return
	}
	if _, err := l.writer().Write(bs); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "ERROR", "framless/pkg/logger", "Logger.Out", err.Error())
		return
	}
	if _, err := l.writer().Write([]byte(l.separator())); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "framless/pkg/logger", "Logger.Out", err.Error())
		return
	}
}

type loggingLevel string

func (ll loggingLevel) String() string { return string(ll) }

const (
	levelDebug loggingLevel = "debug"
	levelInfo  loggingLevel = "info"
	levelWarn  loggingLevel = "warn"
	levelError loggingLevel = "error"
	levelFatal loggingLevel = "fatal"
)

func (l *Logger) writer() io.Writer {
	if l.Out != nil {
		return l.Out
	}
	return os.Stdout
}

func (l *Logger) marshalFunc() func(any) ([]byte, error) {
	if l.MarshalFunc != nil {
		return l.MarshalFunc
	}
	return json.Marshal
}

func (l *Logger) coalesceKey(key, defaultKey string) (rKey string) {
	defer func() { rKey = l.getKeyFormatter()(rKey) }()
	if key != "" {
		return key
	}
	return defaultKey
}

func (l *Logger) toLogEntry(ctx context.Context, level loggingLevel, msg string, lds []LoggingDetail) logEntry {
	d := make(logEntry)
	d = d.Merge(getLoggingDetailsFromContext(ctx))
	for _, ld := range lds {
		ld.addTo(d)
	}
	d[l.coalesceKey(l.LevelKey, levelDefaultKey)] = level
	d[l.coalesceKey(l.MessageKey, messageDefaultKey)] = msg
	d[l.coalesceKey(l.TimestampKey, timestampKey)] = clock.TimeNow().Format(time.RFC3339)
	return d
}

func (l *Logger) separator() string {
	if l.Separator != "" {
		return l.Separator
	}
	switch os.PathSeparator {
	case '/':
		return "\n"
	case '\\':
		return "\r\n"
	default:
		return "\n"
	}
}
