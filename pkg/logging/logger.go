// Package logger provides tooling for structured logging.
// With logger, you can use context to add logging details to your call stack.
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/stringcase"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
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
	Hijack HijackFunc

	outLock sync.Mutex

	strategy _LoggerStrategy

	TestingTB testingTB

	// FlushTimeout is a deadline time for async logging to tell how much time it should wait with batching.
	//
	// Default: 1s
	FlushTimeout time.Duration
}

type _LoggerStrategy struct {
	mutex    sync.RWMutex
	strategy strategy
}

type HijackFunc func(level Level, msg string, fields Fields)

func (l *Logger) Debug(ctx context.Context, msg string, ds ...Detail) {
	l.tb().Helper()
	l.log(ctx, LevelDebug, msg, ds)
}

func (l *Logger) Info(ctx context.Context, msg string, ds ...Detail) {
	l.tb().Helper()
	l.log(ctx, LevelInfo, msg, ds)
}

func (l *Logger) Warn(ctx context.Context, msg string, ds ...Detail) {
	l.tb().Helper()
	l.log(ctx, LevelWarn, msg, ds)
}

func (l *Logger) Error(ctx context.Context, msg string, ds ...Detail) {
	l.tb().Helper()
	l.log(ctx, LevelError, msg, ds)
}

func (l *Logger) Fatal(ctx context.Context, msg string, ds ...Detail) {
	l.tb().Helper()
	l.log(ctx, LevelFatal, msg, ds)
}

func (l *Logger) getKeyFormatter() func(string) string {
	if l.KeyFormatter != nil {
		return l.KeyFormatter
	}
	return stringcase.ToSnake
}

func (l *Logger) log(ctx context.Context, level Level, msg string, ds []Detail) {
	l.tb().Helper()
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
		Timestamp: clock.Now(),
	})
}

func (l *Logger) isHijacked(ctx context.Context, level Level, msg string, ds []Detail) bool {
	l.tb().Helper()
	if l.Hijack == nil {
		return false
	}
	var le = make(logEntry)
	for _, d := range getLoggingDetailsFromContext(ctx) {
		d.addTo(l, le)
	}
	for _, d := range ds {
		d.addTo(l, le)
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

type syncwriter struct {
	Writer io.Writer
	Locker sync.Locker
}

func (w *syncwriter) Write(p []byte) (n int, err error) {
	w.Locker.Lock()
	defer w.Locker.Unlock()
	return w.Writer.Write(p)
}

func (l *Logger) writer() io.Writer {
	var out io.Writer = os.Stdout
	if l.Out != nil {
		out = l.Out
	}
	return &syncwriter{
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
	for _, d := range getLoggingDetailsFromContext(event.Context) {
		d.addTo(l, le)
	}
	for _, ld := range event.Details {
		ld.addTo(l, le)
	}
	le[l.getLevelKey()] = event.Level
	le[l.getMessageKey()] = event.Message
	le[l.getTimestampKey()] = event.Timestamp.Format(time.RFC3339)
	return le
}

func (l *Logger) getTimestampKey() string {
	const timestampKey = "timestamp"
	return l.coalesceKey(l.TimestampKey, timestampKey)
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

func (l *Logger) Clone() *Logger {
	return &Logger{
		Out:          l.Out,
		Level:        l.Level,
		Separator:    l.Separator,
		MessageKey:   l.MessageKey,
		LevelKey:     l.LevelKey,
		TimestampKey: l.TimestampKey,
		MarshalFunc:  l.MarshalFunc,
		KeyFormatter: l.KeyFormatter,
		Hijack:       l.Hijack,
		strategy: _LoggerStrategy{
			strategy: l.strategy.strategy,
		},
	}
}

type testingTB interface {
	Helper()
	Cleanup(func())
	Log(args ...any)
}

func (l *Logger) tb() testingTB {
	if l.TestingTB != nil {
		return l.TestingTB
	}
	return fallbackTestingTB
}

var fallbackTestingTB = (*nullTestingTB)(nil)

type nullTestingTB struct{}

func (*nullTestingTB) Helper() {}

func (*nullTestingTB) Cleanup(func()) {}

func (*nullTestingTB) Log(...any) {}

// Stub the logger.Default and return the buffer where the logging output will be recorded.
// Stub will restore the logger.Default after the test.
func Stub(tb testingTB) (*Logger, StubOutput) {
	buf := &stubOutput{}
	l := &Logger{
		TestingTB: tb,
		Level:     LevelDebug,
		Out:       buf,
	}
	return l, buf
}

type StubOutput interface {
	io.Reader
	String() string
	Bytes() []byte
}

type stubOutput struct {
	m   sync.Mutex
	buf bytes.Buffer
}

func (o *stubOutput) Read(p []byte) (n int, err error) {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Read(p)
}

func (o *stubOutput) Write(p []byte) (n int, err error) {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Write(p)
}

func (o *stubOutput) String() string {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.String()
}

func (o *stubOutput) Bytes() []byte {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Bytes()
}
