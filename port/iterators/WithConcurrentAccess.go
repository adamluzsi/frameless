package iterators

import (
	"sync"
)

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this is that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess[T any](i Iterator[T]) Iterator[T] {
	return &concurrentAccessIterator[T]{Iterator: i}
}

type concurrentAccessIterator[T any] struct {
	Iterator[T]

	mutex sync.Mutex
}

func (i *concurrentAccessIterator[T]) Next() bool {
	i.mutex.Lock()
	return i.Iterator.Next()
}

func (i *concurrentAccessIterator[T]) Value() T {
	defer i.mutex.Unlock()
	return i.Iterator.Value()
}
