package fixtures

import (
	"math/rand"
	"reflect"
	"time"
)

var randomSelectFromSliceRandom = rand.NewSource(time.Now().Unix())

func RandomSelectFromSlice(slice interface{}) interface{} {
	s := reflect.ValueOf(slice)
	index := rand.New(randomSelectFromSliceRandom).Intn(s.Len())
	return s.Index(index).Interface()
}
