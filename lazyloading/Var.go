package lazyloading

import (
	"fmt"
	"sync"
)

type Var[T any] struct {
	// Init will set the constructor block for the variable's value
	Init func() (T, error)

	do struct {
		once sync.Once
	}
	value struct {
		init  sync.Once
		value T
	}
}

func (i *Var[T]) Value() (T, error) {
	var rErr error
	i.value.init.Do(func() {
		if i.Init == nil {
			panic(".Init is not set before .Value() is called")
		}
		i.value.value, rErr = i.Init()
	})
	if rErr != nil {
		i.value.init = sync.Once{}
	}
	return i.value.value, rErr
}

// Do set the .Init block with the received block, and then immediately retrieve the value.
// However it will only set .Init once, to provide an easy to use syntax sugar to the lazyloading.Var.
//
// When Var defined as a struct field, and used from the method of the struct with a pointer receiver,
// then it will streamline the lazy loading process for that struct field.
func (i *Var[T]) Do(init func() T) T {
	i.do.once.Do(func() { i.Init = func() (T, error) { return init(), nil } })
	v, err := i.Value()
	if err != nil {
		panic(fmt.Errorf("invalid usage of .Do, with init block that yields error"))
	}
	return v
}

func (i *Var[T]) DoErr(init func() (T, error)) (T, error) {
	i.do.once.Do(func() { i.Init = init })
	return i.Value()
}
