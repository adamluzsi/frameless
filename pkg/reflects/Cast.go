package reflects

import "reflect"

func Cast[T any](v any) (T, bool) {
	rv := reflect.ValueOf(v)

	targetType := reflect.TypeOf(*new(T))
	if rv.CanConvert(targetType) {
		rv.Convert(targetType)
		return rv.Convert(targetType).Interface().(T), true
	}

	val, ok := rv.Interface().(T)
	return val, ok
}
