package internal

import (
	"reflect"

	"go.llib.dev/frameless/pkg/teardown"
)

func Equal(v1, v2 reflect.Value) bool {
	return reflectDeepEqual(&refMem{visited: make(map[uintptr]struct{})}, v1, v2)
}

func RegisterIsEqual(typ reflect.Type, rfn func(v1, v2 reflect.Value) bool) {
	isEqualFuncRegister[typ] = rfn
}

var isEqualFuncRegister = map[reflect.Type]func(v1, v2 reflect.Value) bool{}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func reflectDeepEqual(m *refMem, v1, v2 reflect.Value) (iseq bool) {
	if !m.TryVisit(v1, v2) {
		return true // probably OK since we already visited it
	}
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}
	if eq, ok := tryEqualityMethods(v1, v2); ok {
		return eq
	}

	switch v1.Kind() {
	case reflect.Struct:
		return equalStruct(m, v1, v2)

	case reflect.Pointer:
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		return reflectDeepEqual(m, v1.Elem(), v2.Elem())

	case reflect.Array:
		// TODO: check if array with different length are considered as the same type
		for i := 0; i < v1.Len(); i++ {
			if eq := reflectDeepEqual(m, v1.Index(i), v2.Index(i)); !eq {
				return eq
			}
		}
		return true

	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		// Special case for []byte, which is common.
		if v1.Type().Elem().Kind() == reflect.Uint8 {
			return string(v1.Bytes()) == string(v2.Bytes())
		}
		for i := 0; i < v1.Len(); i++ {
			if eq := reflectDeepEqual(m, v1.Index(i), v2.Index(i)); !eq {
				return eq
			}
		}
		return true

	case reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return reflectDeepEqual(m, v1.Elem(), v2.Elem())

	case reflect.Map:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		for _, k := range v1.MapKeys() {
			val1 := v1.MapIndex(k)
			val2 := v2.MapIndex(k)
			if !val1.IsValid() || !val2.IsValid() {
				return false
			}
			if eq := reflectDeepEqual(m, val1, val2); !eq {
				return eq
			}
		}
		return true

	case reflect.Func:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		return false

	case reflect.Chan:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		if v1.Cap() == 0 {
			return reflect.DeepEqual(v1.Interface(), v2.Interface())
		}
		if v1.Cap() != v2.Cap() ||
			v1.Len() != v2.Len() {
			return false
		}

		var (
			ln = v1.Len()
			td = &teardown.Teardown{}
		)
		defer func() { _ = td.Finish() }()
		for i := 0; i < ln; i++ {
			v1x, v1OK := v1.Recv()
			if v1OK {
				td.Defer(func() error {
					v1.Send(v1x)
					return nil
				})
			}
			v2x, v2OK := v1.Recv()
			if v2OK {
				td.Defer(func() error {
					v2.Send(v2x)
					return nil
				})
			}
			if v1OK != v2OK {
				return false
			}
			if eq := reflectDeepEqual(m, v1x, v2x); !eq {
				return eq
			}
		}
		return true

	default:
		return reflect.DeepEqual(
			Accessible(v1).Interface(),
			Accessible(v2).Interface())
	}
}

func equalStruct(m *refMem, v1 reflect.Value, v2 reflect.Value) bool {
	var (
		v1cptr = reflect.New(v1.Type())
		v2cptr = reflect.New(v2.Type())
	)
	if v1c, ok := TryToMakeAccessible(v1); ok {
		v1cptr.Elem().Set(reflect.ValueOf(v1c.Interface()))
	}
	if v2c, ok := TryToMakeAccessible(v2); ok {
		v2cptr.Elem().Set(reflect.ValueOf(v2c.Interface()))
	}
	for i, n := 0, v1.NumField(); i < n; i++ {
		f1, ok := TryToMakeAccessible(v1.Field(i))
		if !ok {
			continue
		}
		f2, ok := TryToMakeAccessible(v2.Field(i))
		if !ok {
			continue
		}
		if eq := reflectDeepEqual(m, f1, f2); !eq {
			return eq
		}
		// we zero out fields that we already checked,
		// so we can repsect if something should be compared not through `==`
		// but with a method comparison based approach (Cmp/Compare/Equal)
		var zero = reflect.New(f1.Type()).Elem()
		if cf, ok := ToSettable(v1cptr.Elem().Field(i)); ok {
			cf.Set(zero)
		}
		if cf, ok := ToSettable(v2cptr.Elem().Field(i)); ok {
			cf.Set(zero)
		}
	}
	if T := v1cptr.Type().Elem(); T.Comparable() && v1cptr.Elem().CanInterface() {
		// Check equality of the remaining unexported fields
		// The fields that could be already confirmed to be equal,
		// has already set to zero value state as part of the previous iteration,
		// so what left is the unexported fields that we want to check.
		//
		// Specification:
		// 	Struct types are comparable if all their field types are comparable.
		// 	Two struct values are equal if their corresponding non-blank field values are equal.
		// 	The fields are compared in source order, and comparison stops as soon as two field values differ (or all fields have been compared).
		return v1cptr.Elem().Interface() == v2cptr.Elem().Interface()
	}
	return true
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func tryEqualityMethods(v1, v2 reflect.Value) (isEqual, ok bool) {
	defer func() { recover() }()
	if v1.Type() != v2.Type() {
		return false, false
	}
	if eqfn, ok := isEqualFuncRegister[v1.Type()]; ok {
		return eqfn(v1, v2), true
	}
	if eq, ok := tryEquatable(v1, v2); ok {
		return eq, ok
	}
	if eq, ok := tryComparableEqual(v1, v2); ok {
		return eq, ok
	}
	return false, false
}

func tryEquatable(v1, v2 reflect.Value) (bool, bool) {
	for _, methodName := range []string{"Equal", "IsEqual"} {
		if eq, ok := tryIsEqualMethod(methodName, v1, v2); ok {
			return eq, true
		}
		if eq, ok := tryIsEqualMethod(methodName, ptrOf(v1), v2); ok {
			return eq, true
		}
	}
	return false, false
}

func tryIsEqualMethod(methodName string, v1, v2 reflect.Value) (bool, bool) {
	method := v1.MethodByName(methodName)
	if method == (reflect.Value{}) {
		return false, false
	}

	methodType := method.Type()

	if methodType.NumIn() != 1 {
		return false, false
	}

	if methodType.In(0) != v2.Type() {
		return false, false
	}

	if numOut := methodType.NumOut(); !(numOut == 1 || numOut == 2) {
		return false, false
	}

	switch methodType.NumOut() {
	case 1:
		if methodType.Out(0) != boolType {
			return false, false
		}
	default:
		return false, false
	}

	result := method.Call([]reflect.Value{v2})
	return result[0].Bool(), true
}

func tryComparableEqual(v1, v2 reflect.Value) (bool, bool) {
	if eq, ok := tryCmpEqual(v1, v2); ok {
		return eq, ok
	}
	if eq, ok := tryCmpEqual(ptrOf(v1), v2); ok {
		return eq, ok
	}
	return false, false
}

func tryCmpEqual(v1 reflect.Value, v2 reflect.Value) (bool, bool) {
	if cmpFunc, ok := ImplementsComparable(v1.Type()); ok {
		if cmp, err := cmpFunc(v1, v2); err == nil {
			return cmp == 0, true
		}
	}
	method := v1.MethodByName("Cmp")
	if method == (reflect.Value{}) {
		return false, false
	}
	methodType := method.Type()
	if methodType.NumIn() != 1 {
		return false, false
	}
	if methodType.In(0) != v2.Type() {
		return false, false
	}
	if methodType.NumOut() != 1 {
		return false, false
	}
	if methodType.Out(0) != intType {
		return false, false
	}
	result := method.Call([]reflect.Value{v2})
	return result[0].Int() == 0, true
}
