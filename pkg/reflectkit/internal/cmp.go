package internal

import (
	"reflect"

	"go.llib.dev/frameless/pkg/synckit"
)

type CompareFunc func(val, oth reflect.Value) (int, error)

var comparableTypes synckit.Map[reflect.Type, CompareFunc]

func ImplementsComparable(T reflect.Type) (CompareFunc, bool) {
	if T == nil {
		return nil, false
	}
	fn := comparableTypes.GetOrInit(T, func() CompareFunc {
		return mkComparableFuncFor(T)
	})
	return fn, fn != nil

}

func mkComparableFuncFor(T reflect.Type) CompareFunc {
	if T == nil {
		return nil
	}
	m, ok := T.MethodByName("Compare")
	if !ok {
		m, ok = T.MethodByName("Cmp")
		if !ok {
			return nil
		}
	}
	var mFuncType = m.Func.Type()
	if mFuncType.NumIn() != 2 {
		return nil
	}
	if valType := mFuncType.In(0); valType != T {
		// expected that the receiver is a T type
		return nil
	}
	if othType := mFuncType.In(1); othType != T {
		// expected that the other value is a T type
		// 	T#Compare(oth T) int
		//
		return nil
	}
	if mFuncType.NumOut() != 1 {
		return nil
	}
	if outType := mFuncType.Out(0); outType != intType {
		// expected that the first argument is the same type as the value itself.
		// 	T#Compare(oth T) int
		//
		return nil
	}
	return func(a, b reflect.Value) (int, error) {
		if err := ValidateComparedTypes(a, b); err != nil {
			return 0, err
		}
		//
		// T#Compare(oth T) int
		return int(m.Func.Call([]reflect.Value{a, b})[0].Int()), nil
	}
}

func ValidateComparedTypes(a, b reflect.Value) error {
	if a.Type() == b.Type() {
		return nil
	}
	return ErrTypeMismatch.F("comparison between %s and %s is not possible.", a.Type().String(), b.Type().String())
}
