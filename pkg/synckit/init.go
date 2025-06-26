package synckit

import "sync"

func Init[T comparable, L *sync.RWMutex | *sync.Mutex | *sync.Once](l L, ptr *T, init func() T) T {
	var (
		v   T
		err error
	)
	switch lock := any(l).(type) {
	case *sync.Once:
		return initOnce(lock, ptr, init)
	case *sync.RWMutex:
		v, err = InitErr(lock, ptr, func() (T, error) {
			return init(), nil
		})
	case *sync.Mutex:
		v, err = InitErr(lock, ptr, func() (T, error) {
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

func InitErr[T comparable, L *sync.RWMutex | *sync.Mutex](l L, ptr *T, init func() (T, error)) (T, error) {
	switch m := any(l).(type) {
	case *sync.RWMutex:
		return initRWMutex(m, ptr, init)
	case *sync.Mutex:
		return initMutex(m, ptr, init)
	default:
		panic("not implemented")
	}
}

func initRWMutex[T comparable](rwm *sync.RWMutex, ptr *T, init func() (T, error)) (T, error) {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if rwm == nil {
		panic("[zerokit.InitWith]: nil sync.RWMutex pointer received")
	}
	rwm.RLock()
	if v := *ptr; !isZero(v) {
		rwm.RUnlock()
		return v, nil
	}
	rwm.RUnlock()
	rwm.Lock()
	defer rwm.Unlock()
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

func initMutex[T comparable](m *sync.Mutex, ptr *T, init func() (T, error)) (T, error) {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if m == nil {
		panic("[zerokit.InitWith]: nil sync.Mutex pointer received")
	}
	m.Lock()
	defer m.Unlock()
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

func initOnce[T comparable](o *sync.Once, ptr *T, init func() T) T {
	if ptr == nil {
		panic("[zerokit.InitWith]: nil value pointer received")
	}
	if o == nil {
		panic("[zerokit.InitWith]: nil sync.Once pointer received")
	}
	o.Do(func() { *ptr = init() })
	return *ptr
}

func isZero[T comparable](v T) bool {
	var zero T
	return v == zero
}
