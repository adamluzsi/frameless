package lazyloading

import (
	"sync"
)

// Func allows a value to be lazy evaluated when it is actually used.
func Func(init func() interface{}) func() interface{} {
	var (
		once  sync.Once
		value interface{}
	)
	return func() interface{} {
		once.Do(func() { value = init() })
		return value
	}
}
