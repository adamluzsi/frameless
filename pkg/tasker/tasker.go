// Package tasker provides utilities to background task management to achieve simplicity.
package tasker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/tasker/internal"
	"go.llib.dev/testcase/clock"
)

// Task is the basic unit of tasker package, which represents an executable work.
//
// Task at its core is nothing more than a synchronous function.
// Working with synchronous functions removes the complexity of thinking about how to run your application.
// Your components become more stateless and focus on the domain rather than the lifecycle management.
// This less stateful approach can help to make testing your Task also easier.
type Task = taskerlite.Task

type Runnable = taskerlite.Runnable

type genericTask = taskerlite.GenericTask

func ToTask[TFN genericTask](tfn TFN) Task {
	return taskerlite.ToTask[TFN](tfn)
}

func toTasks[TFN genericTask](tfns []TFN) []Task {
	return taskerlite.ToTasks[TFN](tfns)
}

// Sequence is a construct that allows you to execute a list of Task sequentially.
// If any of the Task fails with an error, it breaks the sequential execution and the error is returned.
func Sequence[TFN genericTask](tfns ...TFN) Task {
	return taskerlite.Sequence[TFN](tfns...)
}

// Concurrence is a construct that allows you to execute a list of Task concurrently.
// If any of the Task fails with an error, all Task will receive cancellation signal.
func Concurrence[TFN genericTask](tfns ...TFN) Task {
	return taskerlite.Concurrence[TFN](tfns...)
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
		var (
			serveErrChan = make(chan error)
			done         = make(chan struct{})
		)
		defer close(done)
		go func() {
			err := startTask(signal)
			select {
			case serveErrChan <- err:
			case <-done:
			}
		}()
		select {
		case <-signal.Done():
			break
		case err := <-serveErrChan:
			if err != nil {
				return err
			}
			break
		}
		ctx, cancel := context.WithTimeout(contextkit.WithoutCancel(signal), internal.GracefulShutdownTimeout)
		defer cancel()
		return stopTask(ctx)
	}
}

// WithRepeat will keep repeating a given Task until shutdown is signaled.
// It is most suitable for Task(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[TFN genericTask](interval Interval, tfn TFN) Task {
	return func(ctx context.Context) error {
		var task = ToTask(tfn)
		if err := task(ctx); err != nil {
			return err
		}
		var at = clock.Now()
	repeat:
		for {
			select {
			case <-ctx.Done():
				break repeat
			case <-clock.After(interval.UntilNext(at)):
				if err := task(ctx); err != nil {
					return err
				}
				at = clock.Now()
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

		signalint.Notify(ch, shutdownSignals...)
		defer signalint.Stop(ch)

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
func Main[TFN genericTask](ctx context.Context, tfns ...TFN) error {
	if len(tfns) == 0 {
		return nil
	}
	return WithSignalNotify(Concurrence(tfns...))(ctx)
}

type bgjob interface {
	Alive() bool
	// Join() error
	Stop() error
}

const ErrAlive errorkit.Error = "ErrAlive"
