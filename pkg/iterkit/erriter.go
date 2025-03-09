package iterkit

import (
	"context"
	"iter"
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/tasker"
)

///////////////////////////////////////////////// ErrIter //////////////////////////////////////////////////

// ErrIter is an iterator that can tell if a currently returned value has an issue or not.
type ErrIter[T any] = iter.Seq2[T, error]

// ToErrIter will turn a iter.Seq[T] into an iter.Seq2[T, error] iterator,
// and use the error function to yield potential issues with the iteration.
func ToErrIter[T any](i iter.Seq[T], errFuncs ...ErrFunc) ErrIter[T] {
	return func(yield func(T, error) bool) {
		for v := range i {
			if !yield(v, nil) {
				return
			}
		}
		if 0 < len(errFuncs) {
			errFunc := errorkit.MergeErrFunc(errFuncs...)
			if err := errFunc(); err != nil {
				var zero T
				yield(zero, errFunc())
			}
		}
	}
}

// FromErrIter will split an iter.Seq2[T, error] iterator into a iter.Seq[T] iterator plus an error retrival func.
func FromErrIter[T any](i ErrIter[T]) (iter.Seq[T], ErrFunc) {
	var m sync.RWMutex
	var errors []error
	return func(yield func(T) bool) {
			m.Lock()
			errors = nil
			m.Unlock()
			for v, err := range i {
				if err != nil {
					m.Lock()
					errors = append(errors, err)
					m.Unlock()
					continue
				}
				if !yield(v) {
					return
				}
			}
		},
		func() error {
			m.RLock()
			defer m.RUnlock()
			return errorkit.Merge(errors...)
		}
}

func CollectErrIter[T any](i iter.Seq2[T, error]) ([]T, error) {
	if i == nil {
		return nil, nil
	}
	var (
		vs   []T
		errs []error
	)
	for v, err := range i {
		if err == nil {
			vs = append(vs, v)
		} else {
			errs = append(errs, err)
		}
	}
	return vs, errorkit.Merge(errs...)
}

// OnErrIterValue will apply a iterator pipeline on a given ErrIter
func OnErrIterValue[To any, From any](itr ErrIter[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) ErrIter[To] {
	return func(yield func(To, error) bool) {
		var g tasker.JobGroup[tasker.Manual]
		defer g.Stop()

		var (
			in   = make(chan From)
			out  = make(chan To)
			errs = make(chan error)
		)

		g.Go(func(ctx context.Context) error {
			defer close(errs)
			defer close(in)

		listening:
			for from, err := range itr {
				if err != nil {
					select {
					case errs <- err:
						continue listening
					case <-ctx.Done():
						break listening
					}
				}

				select {
				case in <- from:
					continue listening
				case <-ctx.Done():
					break listening
				}
			}

			return nil
		})

		g.Go(func(ctx context.Context) error {
			defer close(out)
			var transformPipeline iter.Seq[To] = pipeline(Chan(in))

		feeding:
			for output := range transformPipeline {
				select {
				case out <- output:
					continue feeding
				case <-ctx.Done():
					break feeding
				}
			}

			return nil
		})

	pushing:
		for {
			select {
			case output, ok := <-out:
				if !ok {
					break pushing
				}
				if !yield(output, nil) {
					break pushing
				}
			case err, ok := <-errs:
				if !ok {
					// close(errs) happen earlier than close(out)
					// so we need to collect the remaining values
					output, ok := <-out
					if !ok {
						break pushing
					}
					if !yield(output, nil) {
						break pushing
					}
					continue pushing
				}
				var zero To
				if !yield(zero, err) {
					break pushing
				}
			}
		}
	}
}
