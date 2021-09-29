package lazyloading

import (
	"reflect"
	"sync"
)

type Values struct {
	init  sync.Once
	mutex sync.Mutex
	cache map[reflect.Type][]struct{ k, v interface{} }
}

func (ll *Values) values() map[reflect.Type][]struct{ k, v interface{} } {
	ll.init.Do(func() { ll.cache = make(map[reflect.Type][]struct{ k, v interface{} }) })
	return ll.cache
}

// Get will return a value
func (ll *Values) Get(key interface{}, init func() interface{}) interface{} {
	ll.mutex.Lock()
	defer ll.mutex.Unlock()
	var (
		vs    = ll.values()
		rt    = reflect.TypeOf(key)
		value interface{}
		found bool
	)
	if _, ok := vs[rt]; !ok {
		vs[rt] = make([]struct{ k, v interface{} }, 0)
	}
	for _, v := range vs[rt] {
		if v.k == key {
			value = v.v
			found = true
			break
		}
	}
	if !found {
		value = init()
		vs[rt] = append(vs[rt], struct{ k, v interface{} }{k: key, v: value})
	}
	return value
}
