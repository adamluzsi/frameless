// Package zerokit helps with zero value related use-cases such as initialisation.
package zerokit

import (
	"fmt"
	"go.llib.dev/frameless/pkg/internal/pointersync"
	"reflect"
	"sync/atomic"
)

// Coalesce will return the first non-zero value from the provided values.
func Coalesce[T any](vs ...T) T {
	for _, v := range vs {
		if !isZero(v) {
			return v
		}
	}
	return *new(T)
}

// Init will initialise a zero value through its pointer (*T),
// If it's not set, it assigns a value to it based on the supplied initialiser.
// Init is safe to use concurrently, it has no race condition.
func Init[T any, I initialiser[T]](ptr *T, init I) T {
	if ptr == nil {
		panic(fmt.Sprintf("nil pointer exception with pointers.Init for %T", *new(T)))
	}
	if val, ok := initAtomic[T, I](ptr, init); ok {
		return val
	}
	if val, ok := initFastPath[T](ptr); ok {
		return val
	}
	var key = pointersync.Key(ptr)
	defer initLocks.Sync(key)()
	if ptr != nil && !isZero(*ptr) {
		return *ptr
	}
	*ptr = initialise[T, I](init)
	return *ptr
}

var initLocks = pointersync.NewLocks()

type initialiser[T any] interface {
	func() T | *T
}

func initialise[T any, IV initialiser[T]](init IV) T {
	switch dv := any(init).(type) {
	case func() T:
		return dv()
	case *T:
		return *dv
	default:
		var zero T
		return zero
	}
}

func initFastPath[T any](ptr *T) (_ T, ok bool) {
	defer initLocks.ReadSync(pointersync.Key(ptr))()
	v := *ptr
	return v, !isZero[T](v)
}

func initAtomic[T any, I initialiser[T]](ptr *T, init I) (_ T, _ bool) {
	switch tsPtr := any(ptr).(type) {
	case *int32:
		var zero int32
		if !atomic.CompareAndSwapInt32(tsPtr, zero, zero) {
			return *ptr, true
		}
		ok := atomic.CompareAndSwapInt32(tsPtr, zero, any(initialise[T, I](init)).(int32))
		return any(atomic.LoadInt32(tsPtr)).(T), ok
	case *int64:
		var zero int64
		if !atomic.CompareAndSwapInt64(tsPtr, zero, zero) {
			return *ptr, true
		}
		ok := atomic.CompareAndSwapInt64(tsPtr, zero, any(initialise[T, I](init)).(int64))
		return any(atomic.LoadInt64(tsPtr)).(T), ok
	}
	return
}

func isZero[T any](v T) (ok bool) {
	var zero T
	defer func() {
		if r := recover(); r == nil {
			return
		}
		ok = reflect.DeepEqual(v, zero)
	}()
	return any(v) == any(zero)
}
