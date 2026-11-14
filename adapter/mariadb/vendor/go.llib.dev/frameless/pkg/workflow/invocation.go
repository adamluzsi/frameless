package workflow

import (
	"context"
	"fmt"
	"reflect"
)

// invocationArgs accepts a validated function signature whose first parameter is
// context.Context. Variadic arguments may be individual values or one trailing
// slice; a matching slice takes precedence when both interpretations are valid.
func invocationArgs(fn reflect.Type, ctx context.Context, input []any) ([]reflect.Value, error) {
	fixed := fn.NumIn() - 1
	if fn.IsVariadic() {
		fixed--
	}
	if len(input) < fixed || (!fn.IsVariadic() && len(input) != fixed) {
		return nil, fmt.Errorf("input count mismatch: got %d, signature %s requires %d fixed inputs", len(input), fn, fixed)
	}

	args := make([]reflect.Value, 1, fn.NumIn())
	args[0] = reflect.ValueOf(&ctx).Elem()
	for i := 0; i < fixed; i++ {
		value, err := invocationValue(input[i], fn.In(i+1))
		if err != nil {
			return nil, fmt.Errorf("input #%d: %w", i, err)
		}
		args = append(args, value)
	}
	if !fn.IsVariadic() {
		return args, nil
	}

	variadicType := fn.In(fn.NumIn() - 1)
	if len(input) == fixed+1 && input[fixed] != nil {
		if packed, err := invocationValue(input[fixed], variadicType); err == nil {
			return append(args, packed), nil
		}
	}

	// An untyped nil is an individual optional argument, not an omitted slice.
	// A typed nil slice can be used to explicitly supply a nil variadic slice.
	optional := reflect.Zero(variadicType)
	if count := len(input) - fixed; count > 0 {
		optional = reflect.MakeSlice(variadicType, count, count)
		for i, value := range input[fixed:] {
			rv, err := invocationValue(value, variadicType.Elem())
			if err != nil {
				return nil, fmt.Errorf("input #%d: %w", fixed+i, err)
			}
			optional.Index(i).Set(rv)
		}
	}
	return append(args, optional), nil
}

func invocationValue(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		switch target.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
			return reflect.Zero(target), nil
		default:
			return reflect.Value{}, fmt.Errorf("nil is not assignable to %s", target)
		}
	}
	rv := reflect.ValueOf(value)
	if rv.Type().AssignableTo(target) {
		return rv, nil
	}
	// CanConvert, unlike Type.ConvertibleTo, also checks value-dependent
	// constraints such as the length of a slice converted to an array.
	if rv.CanConvert(target) {
		return rv.Convert(target), nil
	}
	return reflect.Value{}, fmt.Errorf("%s is not assignable or convertible to %s", rv.Type(), target)
}

func invoke(fn reflect.Value, args []reflect.Value) []reflect.Value {
	if fn.Type().IsVariadic() {
		return fn.CallSlice(args)
	}
	return fn.Call(args)
}
