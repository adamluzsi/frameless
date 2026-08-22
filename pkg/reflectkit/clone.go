package reflectkit

import (
	"reflect"
	"unsafe"

	"go.llib.dev/frameless/pkg/reflectkit/reftree"
)

// CloneT is convenience syntax sugar for Clone.
func CloneT[T any](v T) T {
	return Clone(reflect.ValueOf(v)).Interface().(T)
}

// Clone recursively creates a deep copy of value and returns it as a fresh reflect.Value.
func Clone(value reflect.Value) reflect.Value {
	return clone(&reftree.RecursionGuard{}, value)
}

func clone(g *reftree.RecursionGuard, value reflect.Value) reflect.Value {
	if g.Seen(value) {
		// The value takes part in a reference cycle, so rather than recursing
		// into it, we alias the original back. It has to be made exported,
		// because the caller passes the result to reflect.Value.Set,
		// which rejects values reached through an unexported field.
		return toExported(value)
	}
	if v, ok := toAccessible(value); ok {
		value = v
	}
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(clone(g, value.Elem()))
		return out

	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		var i = reflect.New(value.Type()).Elem()
		// i is a settable, nil interface Value. Boxing a value into it
		// is done via i.Set, not i.Elem().Set: i.Elem() on a nil
		// interface returns the zero Value, which cannot be Set into.
		// We deep-copy the boxed value first; if it participates in a
		// pointer cycle, the recursion guard in clone() aliases the
		// original back, which is the correct cycle-breaking behavior.
		i.Set(clone(g, value.Elem()))
		return i

	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		for i := range value.NumField() {
			dst := out.Field(i)
			var ok bool
			dst, ok = ToSettable(dst)
			if !ok {
				continue
			}
			dst.Set(clone(g, value.Field(i)))
		}
		return out

	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(clone(g, value.Index(i)))
		}
		return out

	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			out.SetMapIndex(clone(g, key), clone(g, value.MapIndex(key)))
		}
		return out

	case reflect.Chan:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		var chanType = value.Type()
		if chanType.ChanDir() != reflect.BothDir {
			var chType = reflect.ChanOf(reflect.BothDir, chanType.Elem())
			var ch = reflect.MakeChan(chType, value.Cap())
			return ch.Convert(chanType)
		}
		return reflect.MakeChan(chanType, value.Cap())

	default:
		if value.CanInterface() {
			return reflect.ValueOf(value.Interface())
		}
		// Unexported, addressable leaf: copy via unsafe so we don't have
		// to call Interface() on it.
		if value.CanAddr() {
			uv := reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
			if uv.CanInterface() {
				return reflect.ValueOf(uv.Interface())
			}
		}
		return reflect.Zero(value.Type())
	}
}
