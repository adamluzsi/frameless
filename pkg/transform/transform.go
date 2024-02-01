package transform

import "fmt"

func Must[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Errorf("transform.Must: %w", err))
	}
	return v
}

func Map[O, I any, FN mapFunc[O, I]](s []I, fn FN) ([]O, error) {
	var (
		out    []O
		mapper = toMapFunc[O, I](fn)
	)
	if s != nil {
		out = make([]O, 0, len(s))
	}
	for _, i := range s {
		o, err := mapper(i)
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
	return out, nil
}

func Reduce[O, I any, FN reduceFunc[O, I]](s []I, initial O, fn FN) (O, error) {
	var (
		result  = initial
		reducer = toReduceFunc[O, I](fn)
	)
	for _, i := range s {
		o, err := reducer(result, i)
		if err != nil {
			return result, err
		}
		result = o
	}
	return result, nil
}

type reduceFunc[O, I any] interface {
	func(O, I) O | func(O, I) (O, error)
}

func toReduceFunc[O, I any, FN reduceFunc[O, I]](m FN) func(O, I) (O, error) {
	switch fn := any(m).(type) {
	case func(O, I) O:
		return func(o O, i I) (O, error) {
			return fn(o, i), nil
		}
	case func(O, I) (O, error):
		return fn
	default:
		panic("unexpected")
	}
}

type mapFunc[O, I any] interface {
	func(I) O | func(I) (O, error)
}

func toMapFunc[O, I any, MF mapFunc[O, I]](m MF) func(I) (O, error) {
	switch fn := any(m).(type) {
	case func(I) O:
		return func(i I) (O, error) {
			return fn(i), nil
		}
	case func(I) (O, error):
		return fn
	default:
		panic("unexpected")
	}
}
