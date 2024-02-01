package mapkit

import "fmt"

func Must[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Errorf("transform.Must: %w", err))
	}
	return v
}

// Map will do a mapping from an input type into an output type.
func Map[
	K comparable, V any,
	IK comparable, IV any,
	FN mapFunc[K, V, IK, IV],
](s map[IK]IV, fn FN) (map[K]V, error) {
	if s == nil {
		return nil, nil
	}
	var (
		out    = make(map[K]V)
		mapper = toMapFunc[K, V, IK, IV](fn)
	)
	for ik, iv := range s {
		ok, ov, err := mapper(ik, iv)
		if err != nil {
			return out, err
		}
		out[ok] = ov
	}
	return out, nil
}

// Reduce goes through the map value, combining elements using the reducer function.
func Reduce[
	O any,
	K comparable, V any,
	FN reduceFunc[O, K, V],
](s map[K]V, initial O, fn FN) (O, error) {
	var (
		result  = initial
		reducer = toReduceFunc[O, K, V](fn)
	)
	for k, v := range s {
		o, err := reducer(result, k, v)
		if err != nil {
			return result, err
		}
		result = o
	}
	return result, nil
}

// --------------------------------------------------------------------------------- //

type reduceFunc[O any, K comparable, V any] interface {
	func(O, K, V) O | func(O, K, V) (O, error)
}

func toReduceFunc[O any, K comparable, V any, FN reduceFunc[O, K, V]](m FN) func(O, K, V) (O, error) {
	switch fn := any(m).(type) {
	case func(O, K, V) O:
		return func(o O, k K, v V) (O, error) {
			return fn(o, k, v), nil
		}
	case func(O, K, V) (O, error):
		return fn
	default:
		panic("unexpected")
	}
}

type mapFunc[OK comparable, OV any, IK comparable, IV any] interface {
	func(IK, IV) (OK, OV) | func(IK, IV) (OK, OV, error)
}

func toMapFunc[OK comparable, OV any, IK comparable, IV any, MF mapFunc[OK, OV, IK, IV]](m MF) func(IK, IV) (OK, OV, error) {
	switch fn := any(m).(type) {
	case func(IK, IV) (OK, OV):
		return func(k IK, v IV) (OK, OV, error) {
			ok, ov := fn(k, v)
			return ok, ov, nil
		}
	case func(IK, IV) (OK, OV, error):
		return fn
	default:
		panic("unexpected")
	}
}
