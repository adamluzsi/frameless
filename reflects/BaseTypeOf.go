package reflects

import "reflect"

func BaseTypeOf(i interface{}) reflect.Type {
	t := reflect.TypeOf(i)

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}
