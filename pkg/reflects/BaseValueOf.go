package reflects

import (
	"reflect"
)

func BaseValueOf(i interface{}) reflect.Value {
	return BaseValue(reflect.ValueOf(i))
}

func BaseValue(v reflect.Value) reflect.Value {
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}
