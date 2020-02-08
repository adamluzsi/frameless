package fixtures

import (
	"math/rand"
	"reflect"
)

func RandomElementFromSlice(slice interface{}) interface{} {
	s := reflect.ValueOf(slice)
	index := rand.New(randomSource).Intn(s.Len())
	return s.Index(index).Interface()
}
