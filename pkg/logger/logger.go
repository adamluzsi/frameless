package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/testcase/clock"
	"io"
	"os"
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
}

const (
	levelDefaultKey   = "level"
	messageDefaultKey = "message"
	timestampKey      = "timestamp"
)

func (l Logger) Debug(ctx context.Context, msg string, ds ...Details) {
	l.log(ctx, levelDebug, msg, ds)
}

func (l Logger) Info(ctx context.Context, msg string, ds ...Details) {
	l.log(ctx, levelInfo, msg, ds)
}

func (l Logger) Warn(ctx context.Context, msg string, ds ...Details) {
	l.log(ctx, levelWarn, msg, ds)
}

func (l Logger) Error(ctx context.Context, msg string, ds ...Details) {
	l.log(ctx, levelError, msg, ds)
}

func (l Logger) Fatal(ctx context.Context, msg string, ds ...Details) {
	l.log(ctx, levelFatal, msg, ds)
}

func (l Logger) log(ctx context.Context, level loggingLevel, msg string, ds []Details) {
	entry := l.toLogEntry(ctx, level, msg, ds)
	bs, err := l.marshalFunc()(entry)
	if err != nil {
		fmt.Println("ERROR", "framless/pkg/logger", "Logger.MarshalFunc", err.Error())
		return
	}
	if _, err := l.writer().Write(bs); err != nil {
		fmt.Println("ERROR", "framless/pkg/logger", "Logger.Out", err.Error())
		return
	}
	if _, err := l.writer().Write([]byte(l.separator())); err != nil {
		fmt.Println("ERROR", "framless/pkg/logger", "Logger.Out", err.Error())
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

func (l Logger) writer() io.Writer {
	if l.Out != nil {
		return l.Out
	}
	return os.Stdout
}

func (l Logger) marshalFunc() func(any) ([]byte, error) {
	if l.MarshalFunc != nil {
		return l.MarshalFunc
	}
	return json.Marshal
}

func (l Logger) coalesceKey(key, defaultKey string) string {
	if key != "" {
		return key
	}
	return defaultKey
}

func (l Logger) toLogEntry(ctx context.Context, level loggingLevel, msg string, ds []Details) Details {
	d := make(Details)
	d.Merge(getDetailsFromContext(ctx))
	for _, oth := range ds {
		d.Merge(oth)
	}
	d[l.coalesceKey(l.LevelKey, levelDefaultKey)] = level
	d[l.coalesceKey(l.MessageKey, messageDefaultKey)] = msg
	d[l.coalesceKey(l.TimestampKey, timestampKey)] = clock.TimeNow().Format(time.RFC3339)
	return d
}

func (l Logger) separator() string {
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
