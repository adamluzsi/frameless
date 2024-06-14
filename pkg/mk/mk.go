package mk

import (
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

// New will make a new T and call Init function recursively on it if it is implemented.
func New[T any]() *T {
	ptr := new(T)
	if reflect.TypeOf((*T)(nil)).Elem().Kind() == reflect.Struct {
		refPtr := reflect.ValueOf(ptr)
		initStructField(refPtr)
	}
	if i, ok := any(ptr).(initializable); ok {
		i.Init()
	}
	return ptr
}

// ReflectNew will make a new T and call Init function recursively on it if it is implemented.
func ReflectNew(typ reflect.Type) reflect.Value {
	ptr := reflect.New(typ)

	if typ.Kind() == reflect.Struct {
		initStructField(ptr)
	}

	if ptr.Type().Implements(initInterface) {
		ptr.MethodByName("Init").Call([]reflect.Value{})
	}

	return ptr
}

type initializable interface{ Init() }

var initInterface = reflect.TypeOf((*initializable)(nil)).Elem()

func initStructField(ptr reflect.Value) {
	var NumField = ptr.Type().Elem().NumField()
	for i := 0; i < NumField; i++ {
		field := ptr.Elem().Field(i)

		if reflectkit.IsZero(field) {
			field.Set(ReflectNew(field.Type()).Elem())
		}
	}
}
