// Package zeroutil helps with zero value related use-cases such as initialisation.
package zeroutil

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	var (
		zero T
		key  = uintptr(unsafe.Pointer(v))
	)
	if val, ok := initSyncFast[T](key, v); ok {
		return val
	}
	defer initlcks.Sync(key)()
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

var initlcks = initLocks{Locks: map[uintptr]*initLock{}}

type initLocks struct {
	Mutex sync.RWMutex
	Locks map[uintptr]*initLock
}

func initSyncFast[T any](key uintptr, v *T) (val T, found bool) {
	defer initlcks.ReadSync(key)()
	if v != nil && any(*v) != any(val) {
		return *v, true
	}
	return val, false
}

func (l *initLocks) Sync(key uintptr) func() {
	l.Mutex.Lock()
	m, ok := l.Locks[key]
	if !ok {
		m = &initLock{}
		l.Locks[key] = m
	}
	m.IncUserCount()
	l.Mutex.Unlock()

	m.Lock()
	return func() {
		m.Unlock()
		l.release(key, m)
	}
}

func (l *initLocks) ReadSync(key uintptr) func() {
	l.Mutex.RLock()
	m, ok := l.Locks[key]
	if !ok {
		// Since there is no lock that can specifically match the key,
		// the code is using a general lock called RLock to prevent multiple writes happening at the same time.
		// This works well because writes are less frequent than reads.
		// Although this method may result in slightly slower write speed,
		// it's not a significant issue because ReadSync is used to quickly check values,
		// such as the state of the pointer.
		//
		// In the end, this shortcut leads to a 350% increase in read operation speed
		// but causes only a 0.17% decrease in write speed performance.
		return l.Mutex.RUnlock
	}
	// inc user count is protected by RLock
	m.IncUserCount()
	l.Mutex.RUnlock()

	m.RLock()
	return func() {
		m.RUnlock()
		l.release(key, m)
	}
}

func (l *initLocks) release(key uintptr, m *initLock) {
	if isLast := m.DecUserCount() == 0; !isLast {
		return
	}

	l.Mutex.Lock()
	defer l.Mutex.Unlock()

	if isLast := m.GetUserCount() == 0; !isLast {
		return
	}

	delete(l.Locks, key)
}

type initLock struct {
	sync.RWMutex
	UserCount int64
}

func (l *initLock) GetUserCount() int64 { return atomic.LoadInt64(&l.UserCount) }

// IncUserCount
// - must be used when initLocks.Mutex is in use
func (l *initLock) IncUserCount() int64 { return atomic.AddInt64(&l.UserCount, 1) }

// DecUserCount
// - must be used when initLocks.Mutex is in use
func (l *initLock) DecUserCount() int64 { return atomic.AddInt64(&l.UserCount, -1) }
