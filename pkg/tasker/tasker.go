// Package tasker provides utilities to background task management to achieve simplicity.
package tasker

import (
	"context"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/tasker/internal"
	"go.llib.dev/testcase/clock"
	"os"
	"sync"
	"syscall"
	"time"
)

// Task is the basic unit of tasker package, which represents an executable work.
//
// Task at its core is nothing more than a synchronous function.
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

func ToTask[TFN genericTask](tfn TFN) Task {
	switch v := any(tfn).(type) {
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
		panic(fmt.Sprintf("%T is not supported Task func", v))
	}
}

func toTasks[TFN genericTask](tfns []TFN) []Task {
	var tasks []Task
	for _, t := range tfns {
		tasks = append(tasks, ToTask(t))
	}
	return tasks
}

func Sequence[TFN genericTask](tfns ...TFN) Task {
	return sequence(toTasks[TFN](tfns)).Run
}

// Sequence is a construct that allows you to execute a list of Task sequentially.
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

// Concurrence is a construct that allows you to execute a list of Task concurrently.
// If any of the Task fails with an error, all Task will receive cancellation signal.
func Concurrence[TFN genericTask](tfns ...TFN) Task {
	return concurrence(toTasks[TFN](tfns)).Run
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
	return errorkit.Merge(errs...)
}

// WithShutdown will combine the start and stop/shutdown function into a single Task function.
// It supports a graceful shutdown period;
// upon reaching the deadline, it will cancel the context passed to the shutdown function.
// WithShutdown makes it easy to use components with graceful shutdown support as a Task, such as the http.Server.
//
//	tasker.WithShutdown(srv.ListenAndServe, srv.Shutdown)
func WithShutdown[StartFn, StopFn genericTask](start StartFn, stop StopFn) Task {
	startTask := ToTask(start)
	stopTask := ToTask(stop)
	return func(signal context.Context) error {
		serveErrChan := make(chan error, 1)
		go func() { serveErrChan <- startTask(signal) }()
		select {
		case <-signal.Done():
			break
		case err := <-serveErrChan:
			if err != nil {
				return err
			}
			break
		}
		ctx, cancel := context.WithTimeout(contextkit.Detach(signal), internal.GracefulShutdownTimeout)
		defer cancel()
		return stopTask(ctx)
	}
}

type Interval interface {
	UntilNext(lastRanAt time.Time) time.Duration
}

// WithRepeat will keep repeating a given Task until shutdown is signaled.
// It is most suitable for Task(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[TFN genericTask](interval Interval, tfn TFN) Task {
	return func(ctx context.Context) error {
		var task = ToTask(tfn)
		if err := task(ctx); err != nil {
			return err
		}
		var at = clock.TimeNow()
	repeat:
		for {
			select {
			case <-ctx.Done():
				break repeat
			case <-clock.After(interval.UntilNext(at)):
				if err := task(ctx); err != nil {
					return err
				}
				at = clock.TimeNow()
			}
		}
		return nil
	}
}

type genericErrorHandler interface {
	func(context.Context, error) error | func(error) error
}

func OnError[TFN genericTask, EHFN genericErrorHandler](tfn TFN, ehfn EHFN) Task {
	var erroHandler func(context.Context, error) error
	switch v := any(ehfn).(type) {
	case func(context.Context, error) error:
		erroHandler = v
	case func(error) error:
		erroHandler = func(ctx context.Context, err error) error { return v(err) }
	default:
		panic(fmt.Sprintf("%T is not supported Task func", v))
	}
	task := ToTask(tfn)
	return func(ctx context.Context) error {
		err := task(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ctx.Err()) {
			return err
		}
		return erroHandler(ctx, err)
	}
}

func IgnoreError[TFN genericTask](tfn TFN, errsToIgnore ...error) Task {
	task := ToTask(tfn)
	return func(ctx context.Context) error {
		err := task.Run(ctx)
		if len(errsToIgnore) == 0 {
			return nil
		}
		for _, ignore := range errsToIgnore {
			if errors.Is(err, ignore) {
				return nil
			}
		}
		return err
	}
}

func WithSignalNotify[TFN genericTask](tfn TFN, shutdownSignals ...os.Signal) Task {
	task := ToTask(tfn)
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

		err := task(ctx)
		if errors.Is(err, ctx.Err()) {
			return nil
		}

		return err
	}
}

// Main helps to manage concurrent background Tasks in your main.
// Each Task will run in its own goroutine.
// If any of the Task encounters a failure, the other tasker will receive a cancellation signal.
func Main[TFN genericTask](ctx context.Context, tasks ...TFN) error {
	return WithSignalNotify(Concurrence(tasks...))(ctx)
}

func Background[TFN genericTask](ctx context.Context, tasks ...TFN) func() error {
	ctx, cancel := context.WithCancel(ctx)
	output := make(chan error)
	go func() { output <- Concurrence(tasks...).Run(ctx) }()
	var out error
	return func() error {
		cancel()
		res, ok := <-output
		if ok {
			close(output)
			out = res
		}
		return out
	}
}
