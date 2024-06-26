package reflectkit

import (
	"errors"
	"fmt"
	"reflect"
	"unsafe"
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

func BaseType(typ reflect.Type) (_ reflect.Type, depth int) {
	if typ == nil {
		return typ, depth
	}
	for ; typ.Kind() == reflect.Pointer; depth++ {
		typ = typ.Elem()
	}
	return typ, depth
}

func PointerOf(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
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

func BaseValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	for v.Type().Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v
}

func SymbolicName(e any) string {
	return BaseTypeOf(e).String()
}

func FullyQualifiedName(e any) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%q.%s", t.PkgPath(), t.Name())
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

func IsValueNil(val reflect.Value) bool {
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

func TypeOf[T any](i ...T) reflect.Type {
	for _, v := range i {
		return reflect.TypeOf(v)
	}
	return reflect.TypeOf((*T)(nil)).Elem()
}
