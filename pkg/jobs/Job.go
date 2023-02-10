package jobs

import (
	"context"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"sync"
)

// Job is the basic unit of jobs package, that represents an executable work.
//
// Job at its core, is nothing more than a synchronous function.
// Working with synchronous functions removes the complexity of thinking about how to run your application.
// Your components become more stateless and focus on the domain rather than the lifecycle management.
// This less stateful approach can help to make testing your Job also easier.
type Job func(context.Context) error

type Runnable interface{ Run(context.Context) error }

type genericJob interface {
	Job | func(context.Context) error | func() error | func() | Sequence | Concurrence | *Runnable
}

func ToJob[JFN genericJob](fn JFN) Job {
	switch v := any(fn).(type) {
	case Job:
		return v
	case func(context.Context) error:
		return v
	case func() error:
		return func(context.Context) error { return v() }
	case func():
		return func(context.Context) error { v(); return nil }
	case Sequence:
		return v.Run
	case Concurrence:
		return v.Run
	case *Runnable:
		return (*v).Run
	default:
		panic(fmt.Sprintf("%T is not supported Job func", v))
	}
}

// Sequence is construct that allows you to execute a list of Job sequentially.
// If any of the Job fails with an error, it breaks the sequential execution and the error is returned.
type Sequence []Job

func (s Sequence) Run(ctx context.Context) error {
	for _, job := range s {
		if err := job(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Concurrence is construct that allows you to execute a list of Job concurrently.
// If any of the Job fails with an error, all Job will receive cancellation signal.
type Concurrence []Job

func (c Concurrence) Run(ctx context.Context) error {
	var (
		wwg, cwg sync.WaitGroup
		errs     []error
		errCh    = make(chan error, len(c))
	)
	ctx, cancelDueToError := context.WithCancel(ctx)
	defer cancelDueToError()

	wwg.Add(len(c))
	for _, job := range c {
		go func(j Job) {
			defer wwg.Done()
			errCh <- j(ctx)
		}(job)
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
