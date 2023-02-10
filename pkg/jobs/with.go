package jobs

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/contexts"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/testcase/clock"
	"os"
	"syscall"
)

// WithShutdown will combine the start and stop/shutdown function into a single Job function.
// It supports a graceful shutdown period;
// upon reaching the deadline, it will cancel the context passed to the shutdown function.
// WithShutdown makes it easy to use components with graceful shutdown support as a Job, such as the http.Server.
//
//	jobs.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
func WithShutdown[StartFn, StopFn genericJob](start StartFn, stop StopFn) Job {
	return func(signal context.Context) error {
		serveErrChan := make(chan error, 1)
		go func() { serveErrChan <- ToJob(start)(signal) }()
		select {
		case err := <-serveErrChan:
			return err
		case <-signal.Done():
			break
		}
		ctx, cancel := context.WithTimeout(contexts.Detach(signal), internal.JobGracefulShutdownTimeout)
		defer cancel()
		return ToJob(stop)(ctx)
	}
}

// WithRepeat will keep repeating a given Job until shutdown is signaled.
// It is most suitable for Job(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[JFN genericJob](interval internal.Interval, jfn JFN) Job {
	return func(ctx context.Context) error {
		var job = ToJob(jfn)
		if err := job(ctx); err != nil {
			return err
		}
		var at = clock.TimeNow()
	repeat:
		for {
			select {
			case <-ctx.Done():
				break repeat
			case <-clock.After(interval.UntilNext(at)):
				if err := job(ctx); err != nil {
					return err
				}
				at = clock.TimeNow()
			}
		}
		return nil
	}
}

func OnError[JFN genericJob](jfn JFN, fn func(error) error) Job {
	job := ToJob(jfn)
	return func(ctx context.Context) error {
		err := job(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ctx.Err()) {
			return err
		}
		return fn(err)
	}
}

func WithSignalNotify[JFN genericJob](jfn JFN, shutdownSignals ...os.Signal) Job {
	job := ToJob(jfn)
	if len(shutdownSignals) == 0 {
		shutdownSignals = []os.Signal{
			os.Interrupt,
			syscall.SIGINT,
			syscall.SIGTERM,
		}
	}
	return func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ch := make(chan os.Signal)
		defer close(ch)

		internal.SignalNotify(ch, shutdownSignals...)
		defer internal.SignalStop(ch)

		go func() {
			for range ch {
				cancel()
			}
		}()

		err := job(ctx)
		if errors.Is(err, ctx.Err()) {
			return nil
		}

		return err
	}
}
