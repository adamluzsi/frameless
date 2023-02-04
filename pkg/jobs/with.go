package jobs

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/contexts"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/testcase/clock"
	"time"
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
		go func() { serveErrChan <- toJob(start)(signal) }()
		select {
		case err := <-serveErrChan:
			return err
		case <-signal.Done():
			break
		}
		ctx, cancel := context.WithTimeout(contexts.Detach(signal), internal.JobGracefulShutdownTimeout)
		defer cancel()
		return toJob(stop)(ctx)
	}
}

// WithRepeat will keep repeating a given Job until shutdown is signaled.
// It is most suitable for Job(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[JFN genericJob](interval time.Duration, jfn JFN) Job {
	return func(ctx context.Context) error {
		job := toJob(jfn)
		if err := job(ctx); err != nil {
			return err
		}
	repeat:
		for {
			select {
			case <-ctx.Done():
				break repeat
			case <-clock.After(interval):
				if err := job(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}
}
