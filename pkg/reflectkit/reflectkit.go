package reflectkit

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"go.llib.dev/frameless/pkg/errorkit"
)

func Cast[T any](v any) (T, bool) {
	var (
		typ = reflect.TypeOf(*new(T))
		val = reflect.ValueOf(v)
	)
	if !val.CanConvert(typ) {
		return *new(T), false
	}
	return val.Convert(typ).Interface().(T), true
}

func DerefType(T reflect.Type) (_ reflect.Type, depth int) {
	if T == nil {
		return T, depth
	}
	for ; T.Kind() == reflect.Pointer; depth++ {
		T = T.Elem()
	}
	return T, depth
}

func PointerOf(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.CanAddr() {
		return value.Addr()
	}
	ptr := reflect.New(value.Type())
	ptr.Elem().Set(value)
	return ptr
}

func BaseTypeOf(i any) reflect.Type {
	t := reflect.TypeOf(i)

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func BaseValueOf(i any) reflect.Value {
	return BaseValue(reflect.ValueOf(i))
}

func isBaseKind(kind reflect.Kind) bool {
	return kind != reflect.Pointer && kind != reflect.Interface
}

func BaseValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	for !isBaseKind(v.Kind()) {
		v = v.Elem()
	}
	return v
}

func baseTypeOfAny(v any) (reflect.Type, int) {
	var typ reflect.Type
	switch v := v.(type) {
	case reflect.Type:
		typ = v
	case reflect.Value:
		typ = v.Type()
	default:
		typ = reflect.TypeOf(v)
	}
	return DerefType(typ)
}

func SymbolicName(v any) string {
	typ, depth := baseTypeOfAny(v)
	return strings.Repeat("*", depth) + typ.String()
}

func FullyQualifiedName(v any) string {
	typ, depth := baseTypeOfAny(v)
	var name = typ.Name()
	if pkgPath := typ.PkgPath(); pkgPath != "" {
		name = fmt.Sprintf("%q.%s", pkgPath, name)
	}
	return strings.Repeat("*", depth) + name
}

func IsEmpty(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		return IsEmpty(val.Elem())
	case reflect.Slice, reflect.Map:
		if val.IsNil() {
			return true
		}
		return val.Len() == 0
	case reflect.Pointer:
		if val.IsNil() {
			return true
		}
		return IsEmpty(val.Elem())
	case reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return !val.IsValid() || val.IsZero()
	}
}

func IsZero(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return !val.IsValid() || val.IsZero()
	}
}

func IsNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return false
	}
}

// Link will make destination interface be linked with the src value.
func Link(src, ptr any) (err error) {
	vPtr := reflect.ValueOf(ptr)

	if vPtr.Kind() != reflect.Pointer {
		return errors.New(`pointer type destination expected`)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New(fmt.Sprint(recovered))
		}
	}()

	vPtr.Elem().Set(reflect.ValueOf(src))

	return nil
}

// SetValue will force set
func SetValue(variable, value reflect.Value) {
	if variable.CanSet() {
		variable.Set(value)
		return
	}
	reflect.NewAt(variable.Type(), unsafe.Pointer(variable.UnsafeAddr())).
		Elem().Set(value)
}

var anyInterface = reflect.TypeOf((*any)(nil)).Elem()

func TypeOf[T any](i ...T) reflect.Type {
	var typ = reflect.TypeOf((*T)(nil)).Elem()
	if typ == anyInterface && 0 < len(i) {
		for _, v := range i {
			if typeOfV := reflect.TypeOf(v); typeOfV != nil {
				return typeOfV
			}
		}
	}
	return typ
}

func ToValue(v any) reflect.Value {
	if rv, ok := v.(reflect.Value); ok {
		return rv
	}
	return reflect.ValueOf(v)
}

const ErrTypeMismatch errorkit.Error = "ErrTypeMismatch"

func LookupField[FieldID StructFieldID](rStruct reflect.Value, i FieldID) (reflect.StructField, reflect.Value, bool) {
	if rStruct.Kind() != reflect.Struct || !rStruct.IsValid() {
		return reflect.StructField{}, reflect.Value{}, false
	}

	var structField reflect.StructField
	switch i := any(i).(type) {
	case reflect.StructField:
		structField = i

	case int:
		structField = rStruct.Type().Field(i)

	case string:
		sf, ok := rStruct.Type().FieldByName(i)
		if !ok {
			return reflect.StructField{}, reflect.Value{}, false
		}
		structField = sf
	}

	field := rStruct.FieldByIndex(structField.Index)
	if !field.IsValid() {
		return structField, field, false
	}

	if af, ok := toAccessible(field); ok {
		field = af
	}

	return structField, field, field.IsValid()
}

type StructFieldID interface {
	/* StructField */ reflect.StructField | /*index*/ int | /* name */ string
}

func ToSettable(rv reflect.Value) (_ reflect.Value, ok bool) {
	if !rv.IsValid() {
		return reflect.Value{}, false
	}
	if rv.CanSet() {
		return rv, true
	}
	if rv.CanAddr() {
		if uv := reflect.NewAt(rv.Type(), rv.Addr().UnsafePointer()).Elem(); uv.CanInterface() {
			return uv, true
		}
	}
	return reflect.Value{}, false
}
