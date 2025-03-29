package iterkit

import (
	"io"
	"iter"

	"go.llib.dev/frameless/pkg/errorkit"
)

///////////////////////////////////////////// Pull Iter ///////////////////////////////////////////////////////

// PullIter define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type PullIter[V any] interface {
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Value returns the current value in the iterator.
	// The action should be repeatable without side effects.
	Value() V
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
	io.Closer
	// Err return the error cause.
	Err() error
}

func ToPullIter[T any](itr ErrSeq[T]) PullIter[T] {
	next, stop := iter.Pull2(itr)
	return &pullIter[T]{next: next, stop: stop}
}

func FromPullIter[T any](itr PullIter[T]) ErrSeq[T] {
	return Once2(func(yield func(T, error) bool) {
		defer itr.Close()
		for itr.Next() {
			if !yield(itr.Value(), nil) {
				return
			}
		}
		var zero T
		if err := itr.Err(); err != nil {
			if !yield(zero, err) {
				return
			}
		}
		if err := itr.Close(); err != nil {
			if !yield(zero, err) {
				return
			}
		}
	})
}

func CollectPullIter[T any](itr PullIter[T]) ([]T, error) {
	if itr == nil {
		return nil, nil
	}
	defer itr.Close()
	var vs []T
	for itr.Next() {
		vs = append(vs, itr.Value())
	}
	var errs []error
	if err := itr.Err(); err != nil {
		errs = append(errs, err)
	}
	if err := itr.Close(); err != nil {
		errs = append(errs, err)
	}
	return vs, errorkit.Merge(errs...)
}

// toIterSeqWithRelease
//
// Deprecated: use FromPullIter
func toIterSeqWithRelease[V any](i PullIter[V], errFuncs ...ErrFunc) (iter.Seq[V], ErrFunc) {
	var (
		returnError error
		errFunc     = func() error {
			return returnError
		}
	)
	return func(yield func(V) bool) {
		defer errorkit.Finish(&returnError, i.Err)
		defer errorkit.Finish(&returnError, i.Close)
		for i.Next() {
			if !yield(i.Value()) {
				break
			}
		}
	}, errorkit.MergeErrFunc(append(errFuncs, errFunc)...)
}

type pullIter[T any] struct {
	next func() (T, error, bool)
	stop func()
	val  T
	err  error
	done bool
}

func (i *pullIter[T]) Next() bool {
	if i.done {
		return false
	}
	v, err, ok := i.next()
	if !ok {
		return false
	}
	i.val = v
	i.err = err
	return true
}

func (i *pullIter[T]) Close() error {
	if i.done {
		return nil
	}
	i.done = true
	i.stop()
	return nil
}

func (i *pullIter[T]) Err() error {
	return i.err
}

func (i *pullIter[T]) Value() T {
	return i.val
}
