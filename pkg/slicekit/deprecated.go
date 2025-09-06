package slicekit

import "slices"

// Contains reports if a slice contains a given value.
//
// Deprecated: use slices.Contains
func Contains[T comparable](vs []T, v T) bool {
	return slices.Contains(vs, v)
}
