package reflects

import (
	"reflect"
)

func Name(i interface{}) string {
	t := reflect.TypeOf(i)

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.Name()
}
