package slicekit

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Map will do a mapping from an input type into an output type.
func Map[O, I any, FN mapperFunc[O, I]](s []I, fn FN) ([]O, error) {
	if s == nil {
		return nil, nil
	}
	var (
		out    = make([]O, len(s))
		mapper = toMapperFunc[O, I](fn)
	)
	for index, v := range s {
		o, err := mapper(v)
		if err != nil {
			return out, err
		}
		out[index] = o
	}
	return out, nil
}

// Reduce iterates over a slice, combining elements using the reducer function.
func Reduce[O, I any, FN reducerFunc[O, I]](s []I, initial O, fn FN) (O, error) {
	var (
		result  = initial
		reducer = toReducerFunc[O, I](fn)
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

func Lookup[T any](vs []T, index int) (T, bool) {
	if index < 0 || len(vs)-1 < index {
		return *new(T), false
	}
	return vs[index], true
}

// Merge will merge every []T slice into a single one.
func Merge[T any](slices ...[]T) []T {
	var out []T
	for _, slice := range slices {
		out = append(out, slice...)
	}
	return out
}

// --------------------------------------------------------------------------------- //

type reducerFunc[O, I any] interface {
	func(O, I) O | func(O, I) (O, error)
}

func toReducerFunc[O, I any, FN reducerFunc[O, I]](m FN) func(O, I) (O, error) {
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

type mapperFunc[O, I any] interface {
	func(I) O | func(I) (O, error)
}

func toMapperFunc[O, I any, MF mapperFunc[O, I]](m MF) func(I) (O, error) {
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
