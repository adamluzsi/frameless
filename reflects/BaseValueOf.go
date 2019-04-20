package reflects

import (
	"fmt"
	"reflect"
)

func BaseValueOf(i interface{}) reflect.Value {
	v := reflect.ValueOf(i)

	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fmt.Println(v.Type().Kind())

	return v
}
