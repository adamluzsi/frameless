package sysutil

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/contexts"
	"github.com/adamluzsi/frameless/pkg/sysutil/internal"
)

type Job func(context.Context) error

// JobWithShutdown will combine the start and stop/shutdown function into a single Job function.
// It supports a graceful shutdown period;
// upon reaching the deadline, it will cancel the context passed to the shutdown function.
// JobWithShutdown makes it easy to use components with graceful shutdown support as a Job, such as the http.Server.
//
//	sysutil.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
func JobWithShutdown[StartFn, StopFn genericJobFunc](start StartFn, stop StopFn) Job {
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

type genericJobFunc interface {
	func() error | func(context.Context) error
}

func toJob[Fn genericJobFunc](fn Fn) Job {
	switch fn := any(fn).(type) {
	case func(context.Context) error:
		return fn
	case func() error:
		return func(context.Context) error { return fn() }
	default:
		panic(fmt.Sprintf("%T is not supported Job func", fn))
	}
}
