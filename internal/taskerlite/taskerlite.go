package taskerlite

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.llib.dev/frameless/internal/errorkitlite"
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

// Sequence is a construct that allows you to execute a list of Task sequentially.
// If any of the Task fails with an error, it breaks the sequential execution and the error is returned.
func Sequence[TFN GenericTask](tfns ...TFN) Task {
	return sequence(ToTasks[TFN](tfns)).Run
}

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
func Concurrence[TFN GenericTask](tfns ...TFN) Task {
	if len(tfns) == 1 {
		return ToTask[TFN](tfns[0])
	}
	return concurrence(ToTasks[TFN](tfns)).Run
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
	return errorkitlite.Merge(errs...)
}

type GenericTask interface {
	Task | *Runnable |
		func(context.Context) error |
		func(context.Context) |
		func() error |
		func()
}

func ToTask[TFN GenericTask](tfn TFN) Task {
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

func ToTasks[TFN GenericTask](tfns []TFN) []Task {
	var tasks []Task
	for _, t := range tfns {
		tasks = append(tasks, ToTask(t))
	}
	return tasks
}
