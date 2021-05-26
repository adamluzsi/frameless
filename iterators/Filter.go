package iterators

import (
	"reflect"

	"github.com/adamluzsi/frameless/reflects"
)

func Filter(i Interface, selectorFunc interface{}) Interface {
	iter := &filter{iterator: i, filterFunc: selectorFunc}
	iter.init()
	return iter
}

type filter struct {
	iterator   Interface
	filterFunc interface{}
	matcher    func(interface{}) bool
	rType      reflect.Type

	next interface{}
	err  error
}

func (fi *filter) init() {
	// TODO: Check arity and types here, rather than dying badly elsewhere.
	v := reflect.ValueOf(fi.filterFunc)
	ft := v.Type()

	if ft.NumIn() != 1 {
		panic(`invalid Filter function signature`)
	}

	fi.rType = ft.In(0)

	fi.matcher = func(arg interface{}) bool {
		var varg reflect.Value

		if arg != nil {
			varg = reflect.ValueOf(arg)
		} else {
			varg = reflect.Zero(ft.In(0))
		}

		vrets := v.Call([]reflect.Value{varg})

		const ErrSignatureMismatch = `Filter function expects only one return value: func(type T)(T) bool`

		if len(vrets) != 1 {
			panic(ErrSignatureMismatch)
		}

		isMatching, ok := vrets[0].Interface().(bool)

		if !ok {
			panic(ErrSignatureMismatch)
		}

		return isMatching
	}
}

func (fi *filter) Close() error {
	return fi.iterator.Close()
}

func (fi *filter) Err() error {
	if fi.err != nil {
		return fi.err
	}

	return fi.iterator.Err()
}

func (fi *filter) Decode(e interface{}) error {
	return reflects.Link(fi.next, e)
}

func (fi *filter) Next() bool {
	if !fi.iterator.Next() {
		return false
	}

	nextRV := reflect.New(fi.rType)
	if err := fi.iterator.Decode(nextRV.Interface()); err != nil {
		fi.err = err
		return false
	}

	fi.next = nextRV.Elem().Interface()
	if fi.matcher(fi.next) {
		return true
	}

	return fi.Next()
}
