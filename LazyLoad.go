package frameless

import "sync"

// LazyLoad allows a value to be lazy evaluated when it is actually used.
func LazyLoad(init func() interface{}) func() interface{} {
	var (
		once  sync.Once
		value interface{}
	)
	return func() interface{} {
		once.Do(func() { value = init() })
		return value
	}
}
