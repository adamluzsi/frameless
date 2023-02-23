// Package tasks provides utilities to background job management to achieve simplicity.
package tasks

import (
	"context"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/contexts"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/tasks/internal"
	"github.com/adamluzsi/testcase/clock"
	"os"
	"sync"
	"syscall"
)

// Task is the basic unit of tasks package, that represents an executable work.
//
// Task at its core, is nothing more than a synchronous function.
// Working with synchronous functions removes the complexity of thinking about how to run your application.
// Your components become more stateless and focus on the domain rather than the lifecycle management.
// This less stateful approach can help to make testing your Task also easier.
type Task func(context.Context) error

// Run method supplies Runnable interface for Task.
func (fn Task) Run(ctx context.Context) error { return fn(ctx) }

type Runnable interface{ Run(context.Context) error }

type genericTask interface {
	Task | *Runnable |
		func(context.Context) error |
		func(context.Context) |
		func() error |
		func()
}

func ToTask[JFN genericTask](fn JFN) Task {
	switch v := any(fn).(type) {
	case Task:
		return v
	case func(context.Context) error:
		return v
	case func(context.Context):
		return func(ctx context.Context) error { v(ctx); return nil }
	case func() error:
		return func(context.Context) error { return v() }
	case func():
		return func(context.Context) error { v(); return nil }
	case *Runnable:
		return (*v).Run
	default:
		panic(fmt.Sprintf("%T is not supported Job func", v))
	}
}

func toTasks[TASK genericTask](ts []TASK) []Task {
	var tasks []Task
	for _, t := range ts {
		tasks = append(tasks, ToTask(t))
	}
	return tasks
}

func Sequence[TASK genericTask](ts ...TASK) Task {
	return sequence(toTasks[TASK](ts)).Run
}

// Sequence is construct that allows you to execute a list of Task sequentially.
// If any of the Task fails with an error, it breaks the sequential execution and the error is returned.
type sequence []Task

func (s sequence) Run(ctx context.Context) error {
	for _, task := range s {
		if err := task(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Concurrence is construct that allows you to execute a list of Task concurrently.
// If any of the Task fails with an error, all Task will receive cancellation signal.
func Concurrence[TASK genericTask](ts ...TASK) Task {
	return concurrence(toTasks[TASK](ts)).Run
}

type concurrence []Task

func (c concurrence) Run(ctx context.Context) error {
	var (
		wwg, cwg sync.WaitGroup
		errs     []error
		errCh    = make(chan error, len(c))
	)
	ctx, cancelDueToError := context.WithCancel(ctx)
	defer cancelDueToError()

	wwg.Add(len(c))
	for _, task := range c {
		go func(t Task) {
			defer wwg.Done()
			errCh <- t(ctx)
		}(task)
	}

	cwg.Add(1)
	go func() {
		defer cwg.Done()
		for err := range errCh {
			if err == nil { // shutdown with no error is OK
				continue
			}

			cancelDueToError() // if one fails, all will shut down

			if errors.Is(err, context.Canceled) { // we don't report back context cancellation error
				continue
			}

			errs = append(errs, err)
		}
	}()

	wwg.Wait()
	close(errCh)
	cwg.Wait()
	return errorutil.Merge(errs...)
}

// WithShutdown will combine the start and stop/shutdown function into a single Task function.
// It supports a graceful shutdown period;
// upon reaching the deadline, it will cancel the context passed to the shutdown function.
// WithShutdown makes it easy to use components with graceful shutdown support as a Task, such as the http.Server.
//
//	tasks.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
func WithShutdown[StartFn, StopFn genericTask](start StartFn, stop StopFn) Task {
	return func(signal context.Context) error {
		serveErrChan := make(chan error, 1)
		go func() { serveErrChan <- ToTask(start)(signal) }()
		select {
		case err := <-serveErrChan:
			return err
		case <-signal.Done():
			break
		}
		ctx, cancel := context.WithTimeout(contexts.Detach(signal), internal.JobGracefulShutdownTimeout)
		defer cancel()
		return ToTask(stop)(ctx)
	}
}

// WithRepeat will keep repeating a given Task until shutdown is signaled.
// It is most suitable for Task(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[JFN genericTask](interval internal.Interval, jfn JFN) Task {
	return func(ctx context.Context) error {
		var job = ToTask(jfn)
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

func OnError[JFN genericTask](jfn JFN, fn func(error) error) Task {
	job := ToTask(jfn)
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

func WithSignalNotify[JFN genericTask](jfn JFN, shutdownSignals ...os.Signal) Task {
	job := ToTask(jfn)
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

// Main helps to manage concurrent background Jobs in your main.
// Each Task will run in its own goroutine.
// If any of the Task encounters a failure, the other tasks will receive a cancellation signal.
func Main(ctx context.Context, tasks ...Task) error {
	return WithSignalNotify(Concurrence(tasks...))(ctx)
}
