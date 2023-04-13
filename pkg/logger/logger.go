// Package logger provides tooling for structured logging.
// With logger, you can use context to add logging details to your call stack.
package logger

import (
	"context"
	"encoding/json"
	"github.com/adamluzsi/frameless/pkg/internal/zeroutil"
	"github.com/adamluzsi/frameless/pkg/pointer"
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

	outLock sync.Mutex

	strategy struct {
		mutex    sync.RWMutex
		strategy *strategy
	}
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
	l.getStrategy().Log(logEvent{
		Context:   ctx,
		Level:     level,
		Message:   msg,
		Details:   ds,
		Timestamp: clock.TimeNow(),
	})
}

func (l *Logger) logTo(out io.Writer, event logEvent) error {
	var (
		entry   = l.toLogEntry(event)
		bs, err = l.marshalFunc()(entry)
	)
	if err != nil {
		return err
	}
	_, err = out.Write(append(bs, []byte(l.separator())...))
	return err
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

type writer struct {
	Writer io.Writer
	Locker sync.Locker
}

func (w *writer) Write(p []byte) (n int, err error) {
	w.Locker.Lock()
	defer w.Locker.Unlock()
	return w.Writer.Write(p)
}

func (l *Logger) writer() io.Writer {
	var out io.Writer = os.Stdout
	if l.Out != nil {
		out = l.Out
	}
	return &writer{
		Writer: out,
		Locker: &l.outLock,
	}
}

func (l *Logger) marshalFunc() func(any) ([]byte, error) {
	if l.MarshalFunc != nil {
		return l.MarshalFunc
	}
	return json.Marshal
}

func (l *Logger) coalesceKey(key, defaultKey string) string {
	return l.getKeyFormatter()(zeroutil.Coalesce(key, defaultKey))
}

func (l *Logger) toLogEntry(event logEvent) logEntry {
	le := make(logEntry)
	le = le.Merge(getLoggingDetailsFromContext(event.Context, l))
	for _, ld := range event.Details {
		ld.addTo(l, le)
	}
	le[l.coalesceKey(l.LevelKey, levelDefaultKey)] = event.Level
	le[l.coalesceKey(l.MessageKey, messageDefaultKey)] = event.Message
	le[l.coalesceKey(l.TimestampKey, timestampKey)] = event.Timestamp.Format(time.RFC3339)
	return le
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

func (l *Logger) setStrategy(s strategy) {
	l.strategy.mutex.Lock()
	defer l.strategy.mutex.Unlock()
	l.strategy.strategy = &s
}

func (l *Logger) getStrategy() strategy {
	l.strategy.mutex.RLock()
	defer l.strategy.mutex.RUnlock()
	return pointer.Init[strategy](&l.strategy.strategy, func() strategy {
		return &syncLogger{Logger: l}
	})
}
