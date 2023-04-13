package logger_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/pkg/stringcase"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

var asyncLoggingEventually = assert.EventuallyWithin(3 * time.Second)

func ExampleLogger_AsyncLogging() {
	ctx := context.Background()
	l := logger.Logger{}
	go l.AsyncLogging(ctx) // not handling graceful shutdown with context cancellation
	l.Info(ctx, "this log message is written out asynchronously")
}

func TestLogger_AsyncLogging(t *testing.T) {
	out := &bytes.Buffer{}
	l := logger.Logger{Out: out}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.AsyncLogging(ctx)

	l.MessageKey = "msg"
	l.KeyFormatter = stringcase.ToPascal
	l.Info(ctx, "gsm", logger.Field("fieldKey", "value"))

	asyncLoggingEventually.Assert(t, func(it assert.It) {
		it.Must.Contain(out.String(), `"Msg":"gsm"`)
		it.Must.Contain(out.String(), `"FieldKey":"value"`)
	})
}

func TestLogger_AsyncLogging_cancelledContextStopsTheAsyncHandler(t *testing.T) {
	out := &bytes.Buffer{}
	l := logger.Logger{Out: out}

	var done int32
	assert.NotWithin(t, time.Second, func(ctx context.Context) {
		l.AsyncLogging(ctx)
		atomic.AddInt32(&done, 1)
	})
	assert.EventuallyWithin(5*time.Second).Assert(t, func(it assert.It) {
		it.Must.True(atomic.LoadInt32(&done) != 0)
	})

	t.Log("afterwards it is finshed")
	l.Info(nil, "foo")
	assert.Contain(t, out.String(), "foo")
}

func TestLogger_AsyncLogging_onCancellationAllMessageIsFlushed(t *testing.T) {
	out := &bytes.Buffer{}
	l := logger.Logger{Out: out}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.AsyncLogging(ctx)

	const sampling = 10
	for i := 0; i < sampling; i++ {
		l.Info(nil, strconv.Itoa(i))
	}
	cancel()
	asyncLoggingEventually.Assert(t, func(it assert.It) {
		for i := 0; i < sampling; i++ {
			assert.Contain(it, out.String(), fmt.Sprintf(`"message":"%d"`, i))
		}
	})
}

func BenchmarkLogger_AsyncLogging(b *testing.B) {
	tmpDir := b.TempDir()
	out, err := os.CreateTemp(tmpDir, "")
	if err != nil {
		b.Skip(err.Error())
	}

	b.Run("sync", func(b *testing.B) {
		l := &logger.Logger{Out: out}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(nil, "msg")
		}
	})

	b.Run("async", func(b *testing.B) {
		l := &logger.Logger{Out: out}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go l.AsyncLogging(ctx)
		assert.Waiter{WaitDuration: time.Millisecond}.Wait()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			l.Info(nil, "msg")
		}
	})

	b.Run("sync with heavy concurrency", func(b *testing.B) {
		l := &logger.Logger{Out: out}
		makeConcurrentAccesses(b, l)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(nil, "msg")
		}
	})

	b.Run("async with heavy concurrency", func(b *testing.B) {
		l := &logger.Logger{Out: out}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go l.AsyncLogging(ctx)
		assert.Waiter{WaitDuration: time.Millisecond}.Wait()
		makeConcurrentAccesses(b, l)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			l.Info(nil, "msg")
		}
	})
}

func makeConcurrentAccesses(tb testing.TB, l *logger.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var ready int32
	go func() {
		blk := func() {
			l.Info(nil, "msg")
		}
		more := random.Slice[func()](runtime.NumCPU()*10, func() func() { return blk })
		atomic.AddInt32(&ready, 1)
		func() {
			for {
				if ctx.Err() != nil {
					break
				}
				testcase.Race(blk, blk, more...)
			}
		}()
	}()
	for {
		if atomic.LoadInt32(&ready) != 0 {
			break
		}
	}
}
