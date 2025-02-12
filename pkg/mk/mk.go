package mk

import (
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
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
	var (
		rStructType = ptr.Type().Elem()
		NumField    = rStructType.NumField()
	)
	for i := 0; i < NumField; i++ {
		var (
			field              = ptr.Elem().Field(i)
			defVal, hasDefault = defaultValue(rStructType, rStructType.Field(i))
		)
		if reflectkit.IsZero(field) {
			if hasDefault {
				field.Set(defVal)
			} else {
				field.Set(ReflectNew(field.Type()).Elem())
			}
		}
	}
}

const tagNameDefault = "default"

var defaultTagCache synckit.Map[reflectkit.StructFieldID, reflect.Value]

func defaultValue(rStructType reflect.Type, sf reflect.StructField) (reflect.Value, bool) {
	id := reflectkit.ToStructFieldID(rStructType, sf)
	tag, ok := sf.Tag.Lookup(tagNameDefault)
	if !ok {
		return reflect.Value{}, false
	}
	if len(tag) == 0 {
		return reflect.Value{}, false
	}
	return defaultTagCache.GetOrInit(id, func() reflect.Value {
		val, err := convkit.ParseReflect(sf.Type, tag)
		if err != nil {
			panic(fmt.Sprintf("%s#%s's default value is incorrect for %s type: %q", id.Path, id.Name, id.Type, tag))
		}
		return val
	}), true
}
