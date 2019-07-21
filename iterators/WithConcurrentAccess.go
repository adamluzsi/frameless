package iterators

import (
	"sync"

	"github.com/adamluzsi/frameless"
)

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this, that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess(i frameless.Iterator) *ConcurrentAccessIterator {
	return &ConcurrentAccessIterator{Iterator: i}
}

type ConcurrentAccessIterator struct {
	frameless.Iterator
	mutex sync.Mutex
}

func (i *ConcurrentAccessIterator) Next() bool {
	i.mutex.Lock()
	return i.Iterator.Next()
}

func (i *ConcurrentAccessIterator) Decode(ptr frameless.Entity) error {
	defer i.mutex.Unlock()
	return i.Iterator.Decode(ptr)
}
