package compare

// Interface defines how comparison can be implemented.
// An implementation of Interface can be sorted by the routines in this package.
// The methods refer to elements of the underlying collection by integer index.
//
// Types implementing this interface must provide a Compare method that defines the ordering or equivalence of values.
// This pattern is useful when working with:
// 1. Custom user-defined types requiring comparison logic
// 2. Encapsulated values needing semantic comparisons
// 3. Comparison-agnostic systems (e.g., sorting algorithms)
//
// Example usage:
//
//	type MyNumber int
//
//	func (m MyNumber) Compare(other MyNumber) int {
//		if m < other {
//			return -1
//		}
//		if other < m {
//			return +1
//		}
//		return 0
//	}
type Interface[T any] interface {
	// Compare returns:
	//   -1 if receiver is less than the argument,
	//    0 if they're equal, and
	//    +1 if receiver is greater.
	//
	// Think of the result of Compare like a seesaw:
	// The side that’s lower (touching the ground) represents the smaller value.
	// The side that’s up shows the larger value — it’s higher, so it’s greater.
	//
	// Implementors must ensure consistent ordering semantics.
	Compare(T) int
}

// IsEqual reports whether two values are equal based on their comparison result.
func IsEqual(cmp int) bool {
	return cmp == 0
}

// IsLess reports whether the receiver is less than another value.
func IsLess(cmp int) bool {
	return cmp < 0
}

// IsLessOrEqual reports whether the receiver is less than or equal to another value.
func IsLessOrEqual(cmp int) bool {
	return cmp <= 0
}

// IsGreater reports whether the receiver is greater than another value.
func IsGreater(cmp int) bool {
	return 0 < cmp
}

// IsGreaterOrEqual reports whether the receiver is greater than or equal to another value.
func IsGreaterOrEqual(cmp int) bool {
	return 0 <= cmp
}

// --- NUMBER ---//
type number interface {
	float32 | float64 |
		int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64
}

func Numbers[T number](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
