package reflectkit

import (
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
)

func toAccessible(rv reflect.Value) (reflect.Value, bool) {
	if isAccessible(rv) {
		return rv, true
	}
	if !rv.IsValid() {
		return reflect.Value{}, false
	}
	if sf, ok := ToSettable(rv); ok {
		return sf, true
	}
	if rv.CanUint() {
		return reflect.ValueOf(rv.Uint()).Convert(rv.Type()), true
	}
	if rv.CanInt() {
		return reflect.ValueOf(rv.Int()).Convert(rv.Type()), true
	}
	if rv.CanFloat() {
		return reflect.ValueOf(rv.Float()).Convert(rv.Type()), true
	}
	if rv.CanComplex() {
		return reflect.ValueOf(rv.Complex()).Convert(rv.Type()), true
	}
	switch rv.Kind() {
	case reflect.String:
		return reflect.ValueOf(rv.String()).Convert(rv.Type()), true
	case reflect.Map:
		m := reflect.MakeMap(rv.Type())
		for _, key := range rv.MapKeys() {
			key, ok := toAccessible(key)
			if !ok {
				continue
			}
			value, ok := toAccessible(rv.MapIndex(key))
			if !ok {
				continue
			}
			m.SetMapIndex(key, value)
		}
		return m, true
	case reflect.Slice:
		slice := reflect.MakeSlice(rv.Type(), 0, rv.Len())
		for i, l := 0, rv.Len(); i < l; i++ {
			v, ok := toAccessible(rv.Index(i))
			if !ok {
				continue
			}
			slice = reflect.Append(slice, v)
		}
		return slice, true
	case reflect.Interface:
		return toAccessible(rv.Elem())
	case reflect.Pointer:
		if rv.IsNil() {
			return rv, false
		}
		out, ok := toAccessible(rv.Elem())
		if !ok {
			return reflect.Value{}, false
		}
		ptr := reflect.New(rv.Type().Elem())
		ptr.Elem().Set(out)
		return ptr, true
	}
	return reflect.Value{}, false
}

func isAccessible(rv reflect.Value) (is bool) {
	defer errorkit.RecoverWith(func(r any) { is = false })

	if !rv.CanInterface() {
		return false
	}

	rv.Interface()
	return true
}
