package reflects

import (
	"reflect"
)

func New(structAsType interface{}) interface{} {
	return reflect.New(BaseTypeOf(structAsType)).Interface()
}
