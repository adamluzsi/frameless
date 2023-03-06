package lazyload

import (
	"sync"
)

// Make allows a value to be lazy evaluated when it is actually used.
func Make[T any](init func() T) func() T {
	var (
		once  sync.Once
		value T
	)
	return func() T {
		once.Do(func() { value = init() })
		return value
	}
}

type Var[T any] struct {
	Init func() T

	value T
	done  bool
	lock  sync.RWMutex
}

func (i *Var[T]) Set(v T) {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.value = v
	i.done = true
}

func (i *Var[T]) Get(inits ...func() T) T {
	if v, ok := i.lookup(); ok {
		return v
	}
	i.init(inits)
	v, _ := i.lookup()
	return v
}

func (i *Var[T]) Reset() {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.value = *new(T)
	i.done = false
}

func (i *Var[T]) init(inits []func() T) {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.done {
		return
	}
	var init func() T
	init = i.Init
	for _, fn := range inits {
		init = fn
		break
	}
	if init == nil {
		return
	}
	i.value = init()
	i.done = true
}

func (i *Var[T]) lookup() (T, bool) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	return i.value, i.done
}
