package reflects

import (
	"reflect"
)

func BaseValueOf(i interface{}) reflect.Value {
	v := reflect.ValueOf(i)

	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}

	return v
}
