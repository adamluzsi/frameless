package iterators

import (
	"context"
)

// Paginate will create an Iterator[T] which can be used like any other iterator,
// Under the hood the "more" function will be used to dynamically retrieve more values
// when the previously called values are already used up.
//
// If the more function has a hard-coded true for the "has next page" return value,
// then the pagination will interpret an empty result as "no more pages left".
func Paginate[T any](
	ctx context.Context,
	more func(ctx context.Context, offset int) (values []T, hasNext bool, _ error),
) Iterator[T] {
	return &paginator[T]{
		Context: ctx,
		More:    more,
	}
}

type paginator[T any] struct {
	// Context is the iteration context.
	Context context.Context
	// Offset is the current offset at which the next More will be called.
	Offset int
	// More is the function that meant to retrieve values for iteration.
	// It gets Offset which is used for pagination.
	More func(ctx context.Context, offset int) (_ []T, hasNext bool, _ error)

	value T
	err   error

	buffer []T
	index  int

	done   bool
	noMore bool
}

func (i *paginator[T]) Next() bool {
	if i.done || i.err != nil {
		return false
	}
	if !(i.index < len(i.buffer)) {
		vs, err := i.more()
		if err != nil {
			i.err = err
			return false
		}
		if len(vs) == 0 {
			i.done = true
			return false
		}
		i.index = 0
		i.buffer = vs
	}

	i.value = i.buffer[i.index]
	i.index++
	return true
}

func (i *paginator[T]) Close() error { i.done = true; return nil }
func (i *paginator[T]) Err() error   { return i.err }
func (i *paginator[T]) Value() T     { return i.value }

func (i *paginator[T]) more() ([]T, error) {
	if i.noMore {
		return nil, nil
	}
	vs, hasMore, err := i.More(i.Context, i.Offset)
	if err != nil {
		return nil, err
	}
	if 0 < len(vs) {
		i.Offset += len(vs)
	}
	if hasMore && len(vs) == 0 {
		// when hasMore is true but the result is empty,
		// then it is treated as a NoMore,
		// to enable easy implementations for those cases,
		// where the developer just wants to use a hard-coded true for this value.
		return nil, nil
	}
	i.noMore = !hasMore
	return vs, nil
}
