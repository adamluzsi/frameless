package mk

import (
	"reflect"

	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
)

// New will make a new T and call Init function recursively on it if it is implemented.
func New[T any]() *T {
	ptr := new(T)
	typ := reflectkit.TypeOf[T]()
	if typ.Kind() == reflect.Struct {
		refPtr := reflect.ValueOf(ptr)
		initStruct(refPtr.Elem()) // TODO: test .Elem() rrequired
	}
	if i, ok := any(ptr).(initializable); ok {
		i.Init()
	}
	return ptr
}

// ReflectNew will make a new T and call Init function recursively on it if it is implemented.
func ReflectNew(typ reflect.Type) reflect.Value {
	ptr := reflect.New(typ)
	reflectInit(ptr)
	return ptr
}

func reflectInit(v reflect.Value) {
	switch v.Type().Kind() {
	case reflect.Struct:
		initStruct(v)

	case reflect.Pointer:
		reflectInit(v.Elem())
		callInit(v)

	default:
		callInit(v)

	}
}

func callInit(val reflect.Value) bool {
	if !val.Type().Implements(initInterface) {
		return false
	}
	val.MethodByName("Init").Call([]reflect.Value{})
	return true
}

type initializable interface{ Init() }

var initInterface = reflect.TypeOf((*initializable)(nil)).Elem()

func initStruct(rStruct reflect.Value) {
	if err := defaultTag.HandleStruct(rStruct); err != nil {
		panic(err)
	}
	for _, value := range reflectkit.OverStruct(rStruct) {
		reflectInit(value.Addr())
	}
}

var defaultTag = reflectkit.TagHandler[func() (reflect.Value, error)]{
	Name: "default",
	Parse: func(sf reflect.StructField, tag string) (func() (reflect.Value, error), error) {
		if reflectkit.IsMutableType(sf.Type) {
			return func() (reflect.Value, error) { return parseDefaultValue(sf, tag) }, nil
		}
		val, err := parseDefaultValue(sf, tag)
		if err != nil {
			return nil, err
		}
		return func() (reflect.Value, error) { return val, nil }, nil
	},
	Use: func(sf reflect.StructField, field reflect.Value, getDefault func() (reflect.Value, error)) error {
		if !reflectkit.IsZero(field) {
			return nil
		}
		val, err := getDefault()
		if err != nil {
			return err
		}
		field, ok := reflectkit.ToSettable(field)
		if !ok { // unsettable values are ignored
			return nil
		}
		field.Set(val)
		return nil
	},
	ForceCache: true,
}

func parseDefaultValue(sf reflect.StructField, raw string) (reflect.Value, error) {
	val, err := convkit.ParseReflect(sf.Type, raw)
	if err != nil {
		const format = "%s field's default value is not a valid %s type: %w"
		return val, interr.ImplementationError.F(format, sf.Name, sf.Type, err)
	}
	return val, nil
}
