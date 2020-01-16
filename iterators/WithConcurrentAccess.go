package iterators

import (
	"sync"
)

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this, that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess(i Iterator) *ConcurrentAccessIterator {
	return &ConcurrentAccessIterator{Iterator: i}
}

type ConcurrentAccessIterator struct {
	Iterator
	mutex sync.Mutex
}

func (i *ConcurrentAccessIterator) Next() bool {
	i.mutex.Lock()
	return i.Iterator.Next()
}

func (i *ConcurrentAccessIterator) Decode(ptr interface{}) error {
	defer i.mutex.Unlock()
	return i.Iterator.Decode(ptr)
}
