package jobs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/testcase"
	"os"
	"testing"
	"time"
)

const blockCheckWaitTime = 42 * time.Millisecond

type (
	runnable     interface{ Run(context.Context) error }
	runnableFunc func(context.Context) error
)

func (fn runnableFunc) Run(ctx context.Context) error { return fn(ctx) }

var _ jobs.Job = runnableFunc(func(ctx context.Context) error { return nil }).Run

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
