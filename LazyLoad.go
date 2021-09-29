package frameless

import "sync"

// LazyLoad
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
