package jobs_test

import (
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/testcase"
	"os"
	"testing"
	"time"
)

const blockCheckWaitTime = 42 * time.Millisecond

func StubSignalNotify(t *testcase.T, fn func(chan<- os.Signal, ...os.Signal)) {
	var (
		notify = internal.SignalNotify
		stop   = internal.SignalStop
	)
	t.Cleanup(func() {
		internal.SignalNotify = notify
		internal.SignalStop = stop
	})
	internal.SignalNotify = fn
	internal.SignalStop = func(chan<- os.Signal) {}
}

func StubShutdownTimeout(tb testing.TB, timeout time.Duration) {
	og := internal.JobGracefulShutdownTimeout
	tb.Cleanup(func() { internal.JobGracefulShutdownTimeout = og })
	internal.JobGracefulShutdownTimeout = timeout
}
