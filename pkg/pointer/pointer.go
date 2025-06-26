package pointer

import "fmt"

// Of takes the pointer of a value.
func Of[T any](v T) *T { return &v }

// Deref will return the referenced value,
// or if the pointer has no value,
// then it returns with the zero value.
func Deref[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func Link[T any](src T, dst any) error {
	if dst == nil {
		return fmt.Errorf("missing *%T dst argument", src)
	}
	ptr, ok := dst.(*T)
	if !ok {
		return fmt.Errorf("incorrect ptr type, expected *%T but got %T", src, dst)
	}
	if ptr == nil {
		return fmt.Errorf("nil *%T pointer received", src)
	}
	*ptr = src
	return nil
}
