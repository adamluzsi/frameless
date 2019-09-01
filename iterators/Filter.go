package iterators

import (
	"reflect"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

func Filter(i frameless.Iterator, selectorFunc interface{}) *FilterIterator {
	iter := &FilterIterator{src: i, filterFunc: selectorFunc}
	iter.init()
	return iter
}

type FilterIterator struct {
	src        frameless.Iterator
	filterFunc interface{}
	matcher    func(interface{}) bool

	next interface{}
	err  error
}

func (fi *FilterIterator) init() {
	// TODO: Check arity and types here, rather than dying badly elsewhere.
	v := reflect.ValueOf(fi.filterFunc)
	ft := v.Type()

	fi.matcher = func(arg interface{}) bool {
		var varg reflect.Value

		if arg != nil {
			varg = reflect.ValueOf(arg)
		} else {
			varg = reflect.Zero(ft.In(0))
		}

		vrets := v.Call([]reflect.Value{varg})

		if len(vrets) != 1 {
			panic(`Filter function expects one return value`)
		}

		isMatching, ok := vrets[0].Interface().(bool)

		if !ok {
			panic(`Filter matcher function must return boolean`)
		}

		return isMatching
	}
}

func (fi *FilterIterator) Close() error {
	return fi.src.Close()
}

func (fi *FilterIterator) Err() error {
	if fi.err != nil {
		return fi.err
	}

	return fi.src.Err()
}

func (fi *FilterIterator) Decode(e frameless.Entity) error {
	return reflects.Link(fi.next, e)
}

func (fi *FilterIterator) Next() bool {

	hasNext := fi.src.Next()

	if !hasNext {
		return false
	}

	if err := fi.src.Decode(&fi.next); err != nil {
		fi.err = err
		return false
	}

	if fi.matcher(fi.next) {
		return true
	}

	return fi.Next()

}
