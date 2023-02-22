package lazyload

import "sync"

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
	once  sync.Once
}

func (i *Var[T]) Set(v T) {
	i.once.Do(func() {})
	i.value = v
}

func (i *Var[T]) Get(inits ...func() T) T {
	if i.Init == nil && 0 < len(inits) {
		for _, init := range inits {
			i.Init = init
			break
		}
	}
	if i.Init != nil {
		i.once.Do(func() { i.value = i.Init() })
	}
	return i.value
}
