package convkit

// ParseWith
//
// Deprecated: use UnmarshalWith instead
func ParseWith[T any](parser func(data string) (T, error)) Option {
	return UnmarshalWith(func(data []byte) (T, error) {
		return parser(string(data))
	})
}
