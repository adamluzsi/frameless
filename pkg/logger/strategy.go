package logger

import (
	"bytes"
	"context"
	"go.llib.dev/frameless/pkg/runtimekit"
	"go.llib.dev/frameless/pkg/units"
	"go.llib.dev/frameless/pkg/zerokit"
	"runtime"
	"sync"
	"time"
)

type strategy interface {
	Log(event logEvent)
}

type logEvent struct {
	Context   context.Context
	Level     Level
	Message   string
	Timestamp time.Time
	Details   []LoggingDetail
}

type syncLogger struct{ Logger *Logger }

func (s *syncLogger) Log(event logEvent) {
	_ = s.Logger.logTo(s.Logger.writer(), event)
}

// AsyncLogging will change the logging strategy from sync to async.
// This makes the log calls such as Logger.Info not wait on io operations.
// The AsyncLogging function call is where the async processing will happen,
// You should either run it in a separate goroutine, or use it with the tasker package.
// After the AsyncLogging returned, the logger returns to log synchronously.
func (l *Logger) AsyncLogging() func() {
	var st = &asyncLogger{Logger: l, Stream: make(chan logEvent, 128)}

	var LogEventConsumerWG sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		LogEventConsumerWG.Add(1)
		go func() {
			defer LogEventConsumerWG.Done()
			st.LogEventConsumer()
		}()
	}

	var OutputWriterWG sync.WaitGroup
	OutputWriterWG.Add(1)
	go func() {
		defer OutputWriterWG.Done()
		st.OutputWriter()
	}()

	prevStrategy := l.getStrategy()
	l.setStrategy(st)

	return func() {
		l.setStrategy(prevStrategy)
		close(st.Stream)
		LogEventConsumerWG.Wait()
		close(st.getBatchedEvents())
		OutputWriterWG.Wait()
	}
}

type asyncLogger struct {
	Logger *Logger
	Stream chan logEvent

	batchedEvents chan []logEvent
}

func (s *asyncLogger) Log(event logEvent) {
	defer runtimekit.OnRecover(func(any) {
		s.Logger.logTo(s.Logger.writer(), event)
	})
	s.Stream <- event
}

func (s *asyncLogger) LogEventConsumer() {
	const (
		batchSize    = 512
		batchTimeout = time.Second
	)
	var (
		batch []logEvent
		timer = time.NewTimer(batchTimeout)
	)
	defer timer.Stop()
	flush := func() {
		if 0 < len(batch) {
			s.getBatchedEvents() <- batch
			batch = nil
		}
	}
wrk:
	for {
		timer.Reset(batchTimeout)
		select {
		case event, ok := <-s.Stream:
			if !ok {
				flush()
				break wrk
			}

			batch = append(batch, event)
			if batchSize <= len(batch) {
				flush()
			}

		case <-timer.C:
			flush()
		}
	}
}

// OutputWriter will accept batched logging events and write it into the logging output
// Having two output writer helps to have at least one receiver for the batched events
// but at the cost of random disorder between logging entries.
func (s *asyncLogger) OutputWriter() {
	const (
		bufSize      = 256 * units.Kibibyte
		flushTimeout = time.Second
	)
	var (
		buf    bytes.Buffer
		output = s.Logger.writer()
		flush  = func() { _, _ = buf.WriteTo(output) }
		timer  = time.NewTimer(flushTimeout)
	)
	defer timer.Stop()
wrk:
	for {
		timer.Reset(flushTimeout)
		select {
		case be, ok := <-s.getBatchedEvents():
			if !ok {
				flush()
				break wrk
			}
			for _, event := range be {
				_ = s.Logger.logTo(&buf, event)
			}
			if bufSize <= buf.Len() {
				flush()
			}

		case <-timer.C:
			if 0 < buf.Len() {
				flush()
			}
		}
	}
}

func (s *asyncLogger) getBatchedEvents() chan []logEvent {
	return zerokit.Init(&s.batchedEvents, func() chan []logEvent {
		return make(chan []logEvent)
	})
}
