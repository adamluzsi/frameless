package zeroutil

import (
	"fmt"
	"sync"
	"unsafe"
)

// Coalesce will return the first non-zero value from the provided values.
func Coalesce[T any](vs ...T) T {
	zeroValue := any(*new(T))
	for _, v := range vs {
		if any(v) != zeroValue {
			return v
		}
	}
	return zeroValue.(T)
}

// Init will initialise a zero value through its pointer (*T),
// If it's not set, it assigns a value to it based on the supplied initialiser.
// Init is safe to use concurrently, it has no race condition.
func Init[T any, IV initialiser[T]](v *T, init IV) T {
	if v == nil {
		panic(fmt.Sprintf("nil pointer exception with pointers.Init for %T", *new(T)))
	}
	var zero T
	if v == nil {
		panic(fmt.Sprintf("nil pointer exception with pointers.Init for %T", *new(T)))
	}
	if val, ok := initFastPath(v); ok {
		return val
	}
	defer initsync(v)()
	if v != nil && any(*v) != any(zero) {
		return *v
	}
	switch dv := any(init).(type) {
	case func() T:
		*v = dv()
	case func() *T:
		if nv := dv(); nv != nil {
			*v = *nv
		}

	case *T:
		*v = *dv
	}
	return *v
}

type initialiser[T any] interface {
	func() T | *T
}

func initFastPath[T any](v *T) (val T, found bool) {
	// When we use the global read lock, we prevent any writes to any pointer value.
	// Although it may seem like a slower approach, over using a lock specific to the pointer
	// but getting a lock specific lock would still require global locking.
	// Additionally, this method is efficient and won't cause any live locking issues,
	// because Go's mutex implementation handles lock requests in a first-in, first-out (FIFO) order.
	// Therefore, using the global lock in this way is a good solution.
	initlcks.Mutex.RLock()
	defer initlcks.Mutex.RUnlock()
	if v != nil && any(*v) != any(val) {
		return *v, true
	}
	return val, false
}

var initlcks = struct {
	Mutex sync.RWMutex
	Locks map[uintptr]*sync.Mutex
}{Locks: map[uintptr]*sync.Mutex{}}

func initsync[T any](v *T) func() {
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
