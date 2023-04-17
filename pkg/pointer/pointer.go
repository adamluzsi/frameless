package pointer

// Of takes the pointer of a value.
func Of[T any](v T) *T { return &v }

// Deref will return the referenced value,
// or if the pointer has no value,
// then it returns with the zero value.
func Deref[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}
