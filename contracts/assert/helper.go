package assert

import (
	"reflect"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

type T = interface{}

func newT(T interface{}) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

func newTFunc(T interface{}) func() interface{} {
	return func() interface{} { return newT(T) }
}

func toT(ent interface{}) frameless.T {
	return reflects.BaseValueOf(ent).Interface()
}

type CRD interface {
	frameless.Creator
	frameless.Finder
	frameless.Deleter
}
