package lazyloading

import "sync"

type Var struct {
	// Init will set the constructor block for the variable's value
	Init func() interface{}
	do   struct {
		once sync.Once
	}
	value struct {
		init  sync.Once
		value interface{}
	}
}

func (i *Var) Value() interface{} {
	i.value.init.Do(func() {
		if i.Init == nil {
			panic("lazyloading.Init usage error, .Init should be called before .Value")
		}

		i.value.value = i.Init()
	})
	return i.value.value
}

// Do set the .Init block with the received block, and then immediately retrieve the value.
// However it will only set .Init once, to provide an easy to use syntax sugar to the lazyloading.Var.
//
// When Var defined as a struct field, and used from the method of the struct with a pointer receiver,
// then it will streamline the lazy loading process for that struct field.
func (i *Var) Do(init func() interface{}) interface{} {
	i.do.once.Do(func() { i.Init = init })
	return i.Value()
}
