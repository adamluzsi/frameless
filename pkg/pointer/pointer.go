package pointer

import (
	"fmt"
	"sync"
	"unsafe"
)

// Of takes the pointer of a value.
func Of[T any](v T) *T { return &v }

// Deref will return the referenced value,
// or if the pointer has no value,
// then it returns with the zero value.
func Deref[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

// Init will initialise a variable **T by first checking its value,
// and if it's not set, it assigns a default value to it.
// Init is safe to use concurrently, it has no race condition.
func Init[T any, IV initialiser[T]](v **T, init IV) *T {
	if v == nil {
		panic(fmt.Sprintf("nil pointer exception with pointers.Init for %T", *new(T)))
	}
	if val, ok := initFastPath(v); ok {
		return val
	}
	defer initsync(v)()
	if *v != nil {
		return *v
	}
	switch dv := any(init).(type) {
	case func() T:
		*v = Of(dv())
	case func() *T:
		*v = dv()
	case *T:
		*v = Of(*dv)
	}
	return *v
}

type initialiser[T any] interface {
	func() T | func() *T | *T
}

func initFastPath[T any](v **T) (*T, bool) {
	// When we use the global read lock, we prevent any writes to any pointer value.
	// Although it may seem like a slower approach, over using a lock specific to the pointer
	// but getting a lock specific lock would still require global locking.
	// Additionally, this method is efficient and won't cause any live locking issues,
	// because Go's mutex implementation handles lock requests in a first-in, first-out (FIFO) order.
	// Therefore, using the global lock in this way is a good solution.
	initlcks.Mutex.RLock()
	defer initlcks.Mutex.RUnlock()
	if *v != nil {
		return *v, true
	}
	return nil, false
}

var initlcks = struct {
	Mutex sync.RWMutex
	Locks map[uintptr]*sync.Mutex
}{Locks: map[uintptr]*sync.Mutex{}}

func initsync[T any](v **T) func() {
	key := uintptr(unsafe.Pointer(v))
	initlcks.Mutex.Lock()
	m, ok := initlcks.Locks[key]
	if !ok {
		m = &sync.Mutex{}
		initlcks.Locks[key] = m
	}
	initlcks.Mutex.Unlock()
	m.Lock()
	return func() {
		m.Unlock()
		if ok {
			return
		}
		initlcks.Mutex.Lock()
		defer initlcks.Mutex.Unlock()
		delete(initlcks.Locks, key)
	}
}
