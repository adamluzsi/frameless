package fixtures

import (
	"math/rand"
	"reflect"
	"time"
)

var randomSelectFromMapRandom = rand.NewSource(time.Now().Unix())

func RandomKeyFromMap(anyMap interface{}) interface{} {
	s := reflect.ValueOf(anyMap)
	index := rand.New(randomSelectFromMapRandom).Intn(s.Len())
	return s.MapKeys()[index].Interface()
}
