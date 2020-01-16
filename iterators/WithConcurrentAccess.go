package iterators

import (
	"sync"
)

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this, that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess(i Interface) *ConcurrentAccessIterator {
	return &ConcurrentAccessIterator{Interface: i}
}

type ConcurrentAccessIterator struct {
	Interface
	mutex sync.Mutex
}

func (i *ConcurrentAccessIterator) Next() bool {
	i.mutex.Lock()
	return i.Interface.Next()
}

func (i *ConcurrentAccessIterator) Decode(ptr interface{}) error {
	defer i.mutex.Unlock()
	return i.Interface.Decode(ptr)
}
