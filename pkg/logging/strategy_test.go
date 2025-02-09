package logging_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var asyncLoggingEventually = assert.MakeRetry(3 * time.Second)

func ExampleLogger_AsyncLogging() {
	ctx := context.Background()
	l := logging.Logger{}
	defer l.AsyncLogging()()
	l.Info(ctx, "this log message is written out asynchronously")
}

func TestLogger_AsyncLogging(t *testing.T) {
	var (
		out = &bytes.Buffer{}
		m   sync.Mutex
	)
	l := logging.Logger{
		Out: &iokit.SyncWriter{
			Writer: out,
			Locker: &m,
		},
		FlushTimeout: time.Millisecond,
	}

	defer l.AsyncLogging()()

	l.MessageKey = "msg"
	l.KeyFormatter = stringkit.ToPascal
	l.Info(context.Background(), "gsm", logging.Field("fieldKey", "value"))

	asyncLoggingEventually.Assert(t, func(it assert.It) {
		m.Lock()
		logs := out.String()
		m.Unlock()

		it.Must.Contain(logs, `"Msg":"gsm"`)
		it.Must.Contain(logs, `"FieldKey":"value"`)
	})
}

func TestLogger_AsyncLogging_onCancellationAllMessageIsFlushed(t *testing.T) {
	var (
		out = &bytes.Buffer{}
		m   sync.Mutex
	)
	l := logging.Logger{Out: &iokit.SyncWriter{
		Writer: out,
		Locker: &m,
	}, FlushTimeout: time.Millisecond}

	defer l.AsyncLogging()()

	const sampling = 10
	for i := 0; i < sampling; i++ {
		l.Info(context.Background(), strconv.Itoa(i))
	}
	asyncLoggingEventually.Assert(t, func(it assert.It) {
		m.Lock()
		logs := out.String()
		m.Unlock()

		for i := 0; i < sampling; i++ {
			assert.Contain(it, logs, fmt.Sprintf(`"message":"%d"`, i))
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
		l := &logging.Logger{Out: out}
		defer b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(context.Background(), "msg")
		}
	})

	b.Run("async", func(b *testing.B) {
		l := &logging.Logger{Out: out}
		defer l.AsyncLogging()()
		assert.Waiter{WaitDuration: time.Millisecond}.Wait()

		defer b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(context.Background(), "msg")
		}
	})

	b.Run("sync with heavy concurrency", func(b *testing.B) {
		l := &logging.Logger{Out: out}
		makeConcurrentAccesses(b, l)

		defer b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(context.Background(), "msg")
		}
	})

	b.Run("async with heavy concurrency", func(b *testing.B) {
		l := &logging.Logger{Out: out}
		defer l.AsyncLogging()()
		assert.Waiter{WaitDuration: time.Millisecond}.Wait()
		makeConcurrentAccesses(b, l)

		defer b.StopTimer()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			l.Info(context.Background(), "msg")
		}
	})
}

func makeConcurrentAccesses(tb testing.TB, l *logging.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var ready int32
	go func() {
		blk := func() {
			l.Info(context.Background(), "msg")
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
