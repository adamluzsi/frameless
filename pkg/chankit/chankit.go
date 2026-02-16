package chankit

func Collect[T any](src <-chan T) []T {
	var vs []T
	for v := range src {
		vs = append(vs, v)
	}
	return vs
}
