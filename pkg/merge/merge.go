package merge

func Slice[T any](vss ...[]T) []T {
	var out []T
	for _, vs := range vss {
		out = append(out, vs...)
	}
	return out
}

func Map[K comparable, V any](vss ...map[K]V) map[K]V {
	var out = make(map[K]V)
	for _, vs := range vss {
		for k, v := range vs {
			out[k] = v
		}
	}
	return out
}
