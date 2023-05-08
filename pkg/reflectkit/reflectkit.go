package reflectkit

import (
	"errors"
	"fmt"
	"reflect"
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

func BaseTypeOf(i interface{}) reflect.Type {
	t := reflect.TypeOf(i)

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}

func BaseValueOf(i interface{}) reflect.Value {
	return BaseValue(reflect.ValueOf(i))
}

func BaseValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v 
	}
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}

func SymbolicName(e interface{}) string {
	return BaseTypeOf(e).String()
}

func FullyQualifiedName(e interface{}) string {
	t := BaseTypeOf(e)

	if t.PkgPath() == "" {
		return fmt.Sprintf("%s", t.Name())
	}

	return fmt.Sprintf("%q.%s", t.PkgPath(), t.Name())
}

func IsValueEmpty(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		return IsValueEmpty(val.Elem())
	case reflect.Slice, reflect.Map:
		if val.IsNil() {
			return true
		}
		return val.Len() == 0
	case reflect.Ptr:
		if val.IsNil() {
			return true
		}
		return IsValueEmpty(val.Elem())
	case reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return !val.IsValid() || val.IsZero()
	}
}

func IsValueNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return false
	}
}

// Link will make destination interface be linked with the src value.
func Link(src, ptr interface{}) (err error) {
	vPtr := reflect.ValueOf(ptr)

	if vPtr.Kind() != reflect.Ptr {
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
