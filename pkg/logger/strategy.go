package logger

import (
	"bytes"
	"context"
	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/frameless/pkg/runtimes"
	"github.com/adamluzsi/frameless/pkg/tasker"
	"time"
)

type strategy interface {
	Log(event logEvent)
}

type logEvent struct {
	Context   context.Context
	Level     loggingLevel
	Message   string
	Timestamp time.Time
	Details   []LoggingDetail
}

type syncLogger struct{ Logger *Logger }

func (s *syncLogger) Log(event logEvent) {
	s.Logger.logTo(s.Logger.writer(), event)
}

// AsyncLogging will change the logging strategy from sync to async.
// This makes the log calls such as Logger.Info not wait on io operations.
// The AsyncLogging function call is where the async processing will happen,
// You should either run it in a separate goroutine, or use it with the tasker package.
// After the AsyncLogging returned, the logger returns to log synchronously.
func (l *Logger) AsyncLogging(ctx context.Context) {
	prevStrategy := l.getStrategy()
	defer func() { l.setStrategy(prevStrategy) }()
	s := &asyncLogger{Logger: l, Stream: make(chan logEvent, 128)}
	l.setStrategy(s)
	_ = tasker.Concurrence(
		s.LogEventBatcher,
		s.OutputWriter,
		s.OutputWriter,
	).Run(ctx)
}

type asyncLogger struct {
	Logger *Logger
	Stream chan logEvent

	batchedEvents *chan []logEvent
}

func (s *asyncLogger) Log(event logEvent) {
	defer runtimes.OnRecover(func(any) {
		s.Logger.logTo(s.Logger.writer(), event)
	})
	s.Stream <- event
}

func (s *asyncLogger) LogEventBatcher(ctx context.Context) {
	defer close(s.getBatchedEvents())
	go func() {
		<-ctx.Done()
		close(s.Stream)
	}()

	const (
		batchSize    = 512
		batchTimeout = time.Second
	)
	var (
		batch []logEvent
		timer = time.NewTimer(batchTimeout)
	)

	defer timer.Stop()

	var done bool
	for !done {
		timer.Reset(batchTimeout)
		var flush bool

		select {
		case event, ok := <-s.Stream:
			if !ok {
				done = true
				flush = true
				break
			}
			batch = append(batch, event)
			flush = batchSize <= len(batch)

		case <-timer.C:
			flush = true
		}

		if flush {
			s.getBatchedEvents() <- batch
			batch = nil
		}
	}
}

// OutputWriter will accept batched logging events and write it into the logging output
// Having two output writer helps to have at least one receiver for the batched events
// but at the cost of random disorder between logging entries.
func (s *asyncLogger) OutputWriter(ctx context.Context) {
	const (
		bufSize      = 4096
		flushTimeout = time.Second
	)
	var (
		buf   bytes.Buffer
		timer = time.NewTimer(flushTimeout)
	)
	defer timer.Stop()

	var done bool
	for !done {
		timer.Reset(flushTimeout)
		var flush bool
		select {
		case be, ok := <-s.getBatchedEvents():
			if !ok {
				done = true
				flush = true
				break
			}
			for _, event := range be {
				_ = s.Logger.logTo(&buf, event)
			}
			flush = bufSize <= buf.Len()
		case <-timer.C:
			flush = 0 < buf.Len()
		}
		if flush {
			_, _ = buf.WriteTo(s.Logger.writer())
		}
	}
}

func (s *asyncLogger) getBatchedEvents() chan []logEvent {
	return *pointer.Init(&s.batchedEvents, func() chan []logEvent {
		return make(chan []logEvent)
	})
}
