package zeroutil

func Coalesce[T any](vs ...T) T {
	zeroValue := any(*new(T))
	for _, v := range vs {
		if any(v) != zeroValue {
			return v
		}
	}
	return zeroValue.(T)
}
