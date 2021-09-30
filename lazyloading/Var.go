package lazyloading

import (
	"fmt"
	"sync"
)

type Var struct {
	// Init will set the constructor block for the variable's value
	Init func() (interface{}, error)

	do struct {
		once sync.Once
	}
	value struct {
		init  sync.Once
		value interface{}
	}
}

func (i *Var) Value() (interface{}, error) {
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
func (i *Var) Do(init func() interface{}) interface{} {
	i.do.once.Do(func() { i.Init = func() (interface{}, error) { return init(), nil } })
	v, err := i.Value()
	if err != nil {
		panic(fmt.Errorf("invalid usage of .Do, with init block that yields error"))
	}
	return v
}

func (i *Var) DoErr(init func() (interface{}, error)) (interface{}, error) {
	i.do.once.Do(func() { i.Init = init })
	return i.Value()
}
