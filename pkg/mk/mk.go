package mk

import (
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

func New[T any]() *T {
	ptr := new(T)
	if d, ok := any(ptr).(defaults[T]); ok {
		*ptr = d.Default()
	}
	if i, ok := any(ptr).(initializable); ok {
		i.Init()
	}
	if rPTR := reflect.ValueOf(ptr); rPTR.Type().Elem().Kind() == reflect.Struct {
		initStructField(rPTR)
	}
	return ptr
}

var initInterface = reflect.TypeOf((*initializable)(nil)).Elem()

func ReflectNew(typ reflect.Type) reflect.Value {
	ptr := reflect.New(typ)

	if m := ptr.MethodByName("Default"); m.IsValid() {
		mType := m.Type()
		inCount := mType.NumIn()
		outCount := mType.NumOut()

		if inCount == 0 && outCount == 1 && mType.Out(0) == typ {
			ptr.Elem().Set(m.Call([]reflect.Value{})[0])
		}
	}

	if ptr.Type().Implements(initInterface) {
		ptr.MethodByName("Init").Call([]reflect.Value{})
	}

	if typ.Kind() == reflect.Struct {
		initStructField(ptr)
	}

	return ptr
}

func initStructField(ptr reflect.Value) {
	var NumField = ptr.Type().Elem().NumField()
	for i := 0; i < NumField; i++ {
		field := ptr.Elem().Field(i)

		if reflectkit.IsZero(field) {
			field.Set(ReflectNew(field.Type()).Elem())
		}
	}
}

type initializable interface {
	Init()
}

type defaults[T any] interface {
	Default() T
}
