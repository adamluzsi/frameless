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
	var (
		zero T
		done bool
	)
	defer func() {
		if done {
			return
		}
		if recover() == nil {
			return
		}
		ok = reflectkit.Equal(v, zero)
	}()
	ok = any(v) == any(zero)
	done = true
	return ok
}

// Coalesce will return the first non-zero value from the provided values.
func Coalesce[T any](vs ...T) T {
	for _, v := range vs {
		if !IsZero(v) {
			return v
		}
	}
	var zero T
	return zero
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
func Init[T any, I *T | func() T](ptr *T, init I) T {
	got, _ := initErr[T, I](ptr, init)
	return got
}

// InitErr will initialise a zero value through its pointer (*T),
// If it's not set, it assigns a value to it based on the supplied initialiser function.
// InitErr is safe to use concurrently, it has no race condition.
//
// If the initialiser function encounters an InitErr failure,
// it will leave the provided *T pointer unassigned,
// allowing subsequent calls to attempt initialization again.
func InitErr[T any](ptr *T, init func() (T, error)) (T, error) {
	return initErr(ptr, init)
}

func initErr[T any, Init initialiser[T]](ptr *T, init Init) (T, error) {
	if ptr == nil {
		panic(fmt.Sprintf("nil pointer exception with pointers.Init for %T", (*T)(nil)))
	}
	if val, ok, err := initAtomic[T, Init](ptr, init); err != nil {
		return val, err
	} else if ok {
		return val, nil
	}
	if val, ok := initFastPath[T](ptr); ok {
		return val, nil
	}
	var key = pointerKey(ptr)
	l := initLocks.RWLocker(key)
	l.Lock()
	defer l.Unlock()
	if !IsZero(*ptr) { // ptr is already nil checked
		return *ptr, nil
	}
	if init == nil {
		var zero T
		return zero, nil
	}
	val, err := initialise[T, Init](init)
	if err != nil {
		return val, err
	}
	*ptr = val
	return *ptr, nil
}

func pointerKey[T any](ptr *T) uintptr {
	return uintptr(unsafe.Pointer(ptr))
}

var initLocks = synckit.RWLockerFactory[uintptr]{ReadOptimised: true}

type initialiser[T any] interface {
	noErrInitialiser[T] | func() (T, error)
}

type noErrInitialiser[T any] interface {
	*T | func() T
}

func initialise[T any, IV initialiser[T]](init IV) (T, error) {
	switch dv := any(init).(type) {
	case func() T:
		return dv(), nil
	case func() (T, error):
		return dv()
	case *T:
		return *dv, nil
	default:
		var zero T
		return zero, nil
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

func initAtomic[T any, Init initialiser[T]](ptr *T, init Init) (_ T, _ bool, _ error) {
	// TODO: function initialisers are not protected during the function call, only the assignment itself.
	switch tsPtr := any(ptr).(type) {
	case *int32:
		var zero int32
		if !atomic.CompareAndSwapInt32(tsPtr, zero, zero) {
			return *ptr, true, nil
		}
		v, err := initialise[T, Init](init)
		if err != nil {
			return v, false, err
		}
		ok := atomic.CompareAndSwapInt32(tsPtr, zero, any(v).(int32))
		return any(atomic.LoadInt32(tsPtr)).(T), ok, nil
	case *int64:
		var zero int64
		if !atomic.CompareAndSwapInt64(tsPtr, zero, zero) {
			return *ptr, true, nil
		}
		v, err := initialise[T, Init](init)
		if err != nil {
			return v, false, err
		}
		ok := atomic.CompareAndSwapInt64(tsPtr, zero, any(v).(int64))
		return any(atomic.LoadInt64(tsPtr)).(T), ok, nil
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

func InitWith[T any, L *sync.RWMutex | *sync.Mutex | *sync.Once](ptr *T, l L, init func() T) T {
	var (
		v   T
		err error
	)
	switch lock := any(l).(type) {
	case *sync.Once:
		return initWithOnce(ptr, lock, init)
	case *sync.RWMutex:
		v, err = InitErrWith(ptr, lock, func() (T, error) {
			return init(), nil
		})
	case *sync.Mutex:
		v, err = InitErrWith(ptr, lock, func() (T, error) {
			return init(), nil
		})
	default:
		panic("not-implemented")
	}
	if err != nil {
		panic(err)
	}
	return v
}

func InitErrWith[T any, L *sync.RWMutex | *sync.Mutex](ptr *T, l L, init func() (T, error)) (T, error) {
	switch l := any(l).(type) {
	case *sync.RWMutex:
		return initWithRWMutex(ptr, l, init)
	case *sync.Mutex:
		return initWithMutex(ptr, l, init)
	default:
		panic("not implemented")
	}
}

func initWithRWMutex[T any](ptr *T, rwm *sync.RWMutex, init func() (T, error)) (T, error) {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if rwm == nil {
		panic("[zerokit.InitWith]: nil sync.RWMutex pointer received")
	}
	rwm.RLock()
	if v := *ptr; !IsZero(v) {
		rwm.RUnlock()
		return v, nil
	}
	rwm.RUnlock()
	rwm.Lock()
	defer rwm.Unlock()
	if v := *ptr; !IsZero(v) {
		return v, nil
	}
	out, err := init()
	if err != nil {
		return out, err
	}
	*ptr = out
	return out, nil
}

func initWithMutex[T any](ptr *T, m *sync.Mutex, init func() (T, error)) (T, error) {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if m == nil {
		panic("[zerokit.InitWith]: nil sync.Mutex pointer received")
	}
	m.Lock()
	defer m.Unlock()
	if v := *ptr; !IsZero(v) {
		return v, nil
	}
	out, err := init()
	if err != nil {
		return out, err
	}
	*ptr = out
	return out, nil
}

func initWithOnce[T any](ptr *T, o *sync.Once, init func() T) T {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if o == nil {
		panic("[zerokit.InitWith]: nil sync.Once pointer received")
	}
	o.Do(func() { *ptr = init() })
	return *ptr
}
