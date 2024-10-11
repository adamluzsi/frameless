// Package zerokit helps with zero value related use-cases such as initialisation.
package zerokit

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"go.llib.dev/frameless/pkg/reflectkit"
	synckit "go.llib.dev/frameless/pkg/synckit"
)

// IsZero will report whether the value is zero or not.
func IsZero[T any](v T) (ok bool) {
	var zero T
	defer func() {
		if recover() == nil {
			return
		}
		ok = reflectkit.Equal(v, zero)
	}()
	return any(v) == any(zero)
}

// Coalesce will return the first non-zero value from the provided values.
func Coalesce[T any](vs ...T) T {
	for _, v := range vs {
		if !IsZero(v) {
			return v
		}
	}
	return *new(T)
}

// V is a type that can initialise itself upon access (V.Get).
// Map, Slice, Chan types are made, while primitive types returned as zero value.
// Pointer types are made with an initialised value.
//
// V is not thread safe, it just makes initialisation at type level in struct fields more convenient.
// The average cost for using V is low, see the benchmark for more
type V[T any] struct {
	value T
	init  sync.Once
}

func (i *V[T]) Set(v T) {
	i.init.Do(func() {})
	i.value = v
}

func (i *V[T]) Get() T {
	i.init.Do(func() { i.value = mk(reflectkit.TypeOf[T]()).(T) })
	return i.value
}

func (i *V[T]) Ptr() *T {
	i.Get()
	return &i.value
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
	var key = pointerKey(ptr)
	l := initLocks.RWLocker(key)
	l.Lock()
	defer l.Unlock()
	if !IsZero(*ptr) { // ptr is already nil checked
		return *ptr
	}
	*ptr = initialise[T, I](init)
	return *ptr
}

func pointerKey[T any](ptr *T) uintptr {
	return uintptr(unsafe.Pointer(ptr))
}

var initLocks = synckit.RWLockerFactory[uintptr]{ReadOptimised: true}

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
	key := pointerKey(ptr)
	l := initLocks.RWLocker(key)
	l.RLock()
	defer l.RUnlock()
	v := *ptr
	return v, !IsZero[T](v)
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

type canInit interface {
	Init()
}

var canInitType = reflectkit.TypeOf[canInit]()

func mk(typ reflect.Type) any {
	if reflect.PointerTo(typ).Implements(canInitType) {
		ptr := reflect.New(typ)
		ptr.MethodByName("Init").Call([]reflect.Value{})
		return ptr.Elem().Interface()
	}
	switch typ.Kind() {
	case reflect.Slice:
		return reflect.MakeSlice(typ, 0, 0).Interface()
	case reflect.Map:
		return reflect.MakeMap(typ).Interface()
	case reflect.Chan:
		return reflect.MakeChan(typ, 0).Interface()
	case reflect.Pointer:
		ptr := reflect.New(typ.Elem())
		ptr.Elem().Set(reflect.ValueOf(mk(typ.Elem())))
		return ptr.Interface()
	default:
		return reflect.New(typ).Elem().Interface()
	}
}
