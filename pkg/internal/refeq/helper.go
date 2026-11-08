package refeq

import (
	"reflect"
	"unsafe"

	"go.llib.dev/frameless/internal/errorkitlite"
)

const ErrTypeMismatch errorkitlite.Error = "ErrTypeMismatch"

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

func Accessible(rv reflect.Value) reflect.Value {
	if rv, ok := TryToMakeAccessible(rv); ok {
		return rv
	}
	return rv
}

func TryToMakeAccessible(rv reflect.Value) (reflect.Value, bool) {
	if rv.CanInterface() {
		return rv, true
	}
	if rv.CanAddr() {
		uv := reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem()
		if uv.CanInterface() {
			return uv, true
		}
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
			key, ok := TryToMakeAccessible(key)
			if !ok {
				continue
			}
			value, ok := TryToMakeAccessible(rv.MapIndex(key))
			if !ok {
				continue
			}
			m.SetMapIndex(key, value)
		}
		return m, true
	case reflect.Slice:
		slice := reflect.MakeSlice(rv.Type(), 0, rv.Len())
		for i, l := 0, rv.Len(); i < l; i++ {
			v, ok := TryToMakeAccessible(rv.Index(i))
			if !ok {
				continue
			}
			slice = reflect.Append(slice, v)
		}
		return slice, true
	}
	return reflect.Value{}, false
}

// refMem remembers the pairs of references which are under comparison already,
// so the comparison of self-referencing values terminates.
// A pair met again is taken as equal there, since its actual result is decided where it was met first.
//
// Only references (pointers, maps and slices) are remembered, as only they are able to close a cycle.
// Any other value has no identity which could be remembered:
// a struct shares its address with its first field,
// and a copy of a value gets a new address each time it is made.
//
// A pair is remembered, rather than each side on its own,
// because a reference which is at more than one place on one side
// can be paired with a different value at each of those places on the other side.
type refMem struct{ visited map[refPair]struct{} }

type refPair struct {
	typ reflect.Type
	// p1 and p2 are unsafe.Pointer values, not uintptr ones,
	// so the garbage collector keeps the remembered memory regions,
	// and their addresses can't be reused by an other value during the comparison.
	p1, p2 unsafe.Pointer
	// len tells apart the re-slices of the same backing array, like s[:1] and s[:2].
	len int
}

// TryVisit reports whether the pair of v1 and v2 is not yet under comparison,
// and remembers it if it is a pair of references.
// v1 and v2 are expected to be valid and of the same type.
func (i *refMem) TryVisit(v1, v2 reflect.Value) (ok bool) {
	key, ok := refPairOf(v1, v2)
	if !ok {
		return true
	}
	if i.visited == nil {
		i.visited = make(map[refPair]struct{})
	}
	if _, ok := i.visited[key]; ok {
		return false
	}
	i.visited[key] = struct{}{}
	return true
}

// refPairOf returns the identity of a pair of references.
// An empty or zero sized reference has nothing which could reference back to it,
// and Go doesn't give it a distinct address either, so it has no identity.
func refPairOf(v1, v2 reflect.Value) (refPair, bool) {
	switch v1.Kind() {
	case reflect.Pointer:
		if v1.IsNil() || v2.IsNil() || v1.Type().Elem().Size() == 0 {
			return refPair{}, false
		}
		return refPair{typ: v1.Type(), p1: v1.UnsafePointer(), p2: v2.UnsafePointer()}, true

	case reflect.Map:
		if v1.Len() == 0 || v2.Len() == 0 {
			return refPair{}, false
		}
		return refPair{typ: v1.Type(), p1: v1.UnsafePointer(), p2: v2.UnsafePointer()}, true

	case reflect.Slice:
		if v1.Len() == 0 || v2.Len() == 0 || v1.Type().Elem().Size() == 0 {
			return refPair{}, false
		}
		return refPair{typ: v1.Type(), p1: v1.UnsafePointer(), p2: v2.UnsafePointer(), len: v1.Len()}, true

	default:
		return refPair{}, false
	}
}

var (
	boolType = reflect.TypeFor[bool]()
	intType  = reflect.TypeFor[int]()
)

func ptrOf(v reflect.Value) reflect.Value {
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	return ptr
}
