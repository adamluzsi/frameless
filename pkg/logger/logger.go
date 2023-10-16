// Package logger provides tooling for structured logging.
// With logger, you can use context to add logging details to your call stack.
package logger

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/stringcase"
	"go.llib.dev/frameless/pkg/zerokit"
	"github.com/adamluzsi/testcase/clock"
)

type Logger struct {
	Out io.Writer

	// Level is the logging level.
	// The default Level is LevelInfo.
	Level Level

	Separator string

	MessageKey   string
	LevelKey     string
	TimestampKey string

	// MarshalFunc is used to serialise the logging message event.
	// When nil it defaults to JSON format.
	MarshalFunc func(any) ([]byte, error)
	// KeyFormatter will be used to format the logging field keys
	KeyFormatter func(string) string

	// Hijack will hijack the logging and instead of letting it logged out to the Out,
	// the logging will be done with the Hijack function.
	// This is useful if you want to use your own choice of logging,
	// but also packages that use this logging package.
	Hijack func(level Level, msg string, fields Fields)

	outLock sync.Mutex

	strategy struct {
		mutex    sync.RWMutex
		strategy strategy
	}
}

func (l *Logger) Debug(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	l.log(ctx, LevelDebug, msg, ds)
}

func (l *Logger) Info(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	l.log(ctx, LevelInfo, msg, ds)
}

func (l *Logger) Warn(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	l.log(ctx, LevelWarn, msg, ds)
}

func (l *Logger) Error(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	l.log(ctx, LevelError, msg, ds)
}

func (l *Logger) Fatal(ctx context.Context, msg string, ds ...LoggingDetail) {
	tb().Helper()
	l.log(ctx, LevelFatal, msg, ds)
}

func (l *Logger) getKeyFormatter() func(string) string {
	if l.KeyFormatter != nil {
		return l.KeyFormatter
	}
	return stringcase.ToSnake
}

func (l *Logger) log(ctx context.Context, level Level, msg string, ds []LoggingDetail) {
	tb().Helper()
	if l.isHijacked(ctx, level, msg, ds) {
		return
	}
	if !isLevelEnabled(l.getLevel(), level) {
		return
	}
	l.getStrategy().Log(logEvent{
		Context:   ctx,
		Level:     level,
		Message:   msg,
		Details:   ds,
		Timestamp: clock.TimeNow(),
	})
}

var overrideHijack func(l *Logger, level Level, msg string, fields Fields)

func withHijackOverride(fn func(l *Logger, level Level, msg string, fields Fields)) func() {
	previousHijack := overrideHijack
	overrideHijack = fn
	return func() {overrideHijack = previousHijack }
}

func (l *Logger) isHijacked(ctx context.Context, level Level, msg string, ds []LoggingDetail) bool {
	tb().Helper()
	if l.Hijack == nil && overrideHijack == nil {
		return false
	}
	var le = make(logEntry)
	for _, d := range getLoggingDetailsFromContext(ctx, l) {
		d.addTo(l, le)
	}
	for _, d := range ds {
		d.addTo(l, le)
	}
	if overrideHijack != nil {
		overrideHijack(l, level, msg, Fields(le))
		return true
	}
	l.Hijack(level, msg, Fields(le))
	return true
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
	return l.getKeyFormatter()(zerokit.Coalesce(key, defaultKey))
}

func (l *Logger) toLogEntry(event logEvent) logEntry {
	le := make(logEntry)
	for _, d := range getLoggingDetailsFromContext(event.Context, l) {
		d.addTo(l, le)
	}
	for _, ld := range event.Details {
		ld.addTo(l, le)
	}
	le[l.getLevelKey()] = event.Level
	le[l.getMessageKey()] = event.Message
	const timestampKey = "timestamp"
	le[l.coalesceKey(l.TimestampKey, timestampKey)] = event.Timestamp.Format(time.RFC3339)
	return le
}

func (l *Logger) getMessageKey() string {
	const messageDefaultKey = "message"
	return l.coalesceKey(l.MessageKey, messageDefaultKey)
}

func (l *Logger) getLevelKey() string {
	const levelDefaultKey = "level"
	return l.coalesceKey(l.LevelKey, levelDefaultKey)
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
	l.strategy.strategy = s
}

func (l *Logger) getStrategy() strategy {
	l.strategy.mutex.RLock()
	defer l.strategy.mutex.RUnlock()
	return zerokit.Init(&l.strategy.strategy, func() strategy {
		return &syncLogger{Logger: l}
	})
}

func (l *Logger) getLevel() Level {
	if l.Level != "" {
		return l.Level
	}
	return zerokit.Init[Level](&l.Level, func() Level {
		return defaultLevel
	})
}
