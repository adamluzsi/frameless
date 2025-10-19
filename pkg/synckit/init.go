package synckit

import (
	"fmt"
	"sync"
)

func Init[T comparable, L sync.RWMutex | sync.Mutex | sync.Once](l *L, ptr *T, init func() T) T {
	if ptr == nil {
		panic("[synckit.Init]: nil value pointer received")
	}
	if l == nil {
		panic(fmt.Sprintf("[synckit.Init]: nil %T received", l))
	}
	var (
		v   T
		err error
	)
	switch l := any(l).(type) {
	case *sync.Once:
		return initOnce(l, ptr, init)
	case *sync.RWMutex:
		v, err = initErrWRWL(l, ptr, func() (T, error) {
			return init(), nil
		})
	case *sync.Mutex:
		v, err = initErrWL(l, ptr, func() (T, error) {
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

func InitErr[T comparable, L sync.RWMutex | sync.Mutex](l *L, ptr *T, init func() (T, error)) (T, error) {
	if ptr == nil {
		panic("[synckit.InitErr]: nil value pointer received")
	}
	if l == nil {
		panic(fmt.Sprintf("[synckit.InitErr]: nil %T received", l))
	}
	switch m := any(l).(type) {
	case *sync.RWMutex:
		return initErrWRWL(m, ptr, init)
	case *sync.Mutex:
		return initErrWL(m, ptr, init)
	default:
		panic("not implemented")
	}
}

func InitErrWL[T comparable](l sync.Locker, ptr *T, init func() (T, error)) (T, error) {
	if rwl, ok := l.(RWLocker); ok {
		return initErrWRWL(rwl, ptr, init)
	}
	return initErrWL(l, ptr, init)
}

func initOnce[T comparable](o *sync.Once, ptr *T, init func() T) T {
	o.Do(func() { *ptr = init() })
	return *ptr
}

func isZero[T comparable](v T) bool {
	var zero T
	return v == zero
}

func initErrWRWL[T comparable, RWL RWLocker](rwl RWL, ptr *T, init func() (T, error)) (T, error) {
	rwl.RLock()
	if v := *ptr; !isZero(v) {
		rwl.RUnlock()
		return v, nil
	}
	rwl.RUnlock()
	rwl.Lock()
	defer rwl.Unlock()
	if v := *ptr; !isZero(v) {
		return v, nil
	}
	out, err := init()
	if err != nil {
		return out, err
	}
	*ptr = out
	return out, nil
}

func initErrWL[T comparable](l sync.Locker, ptr *T, init func() (T, error)) (T, error) {
	l.Lock()
	defer l.Unlock()
	if v := *ptr; !isZero(v) {
		return v, nil
	}
	out, err := init()
	if err != nil {
		return out, err
	}
	*ptr = out
	return out, nil
}
