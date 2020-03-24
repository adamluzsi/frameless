package fixtures

import (
	"reflect"
)

func RandomElementFromSlice(slice interface{}) interface{} {
	s := reflect.ValueOf(slice)
	index := rnd.Intn(s.Len())
	return s.Index(index).Interface()
}
