// Package predicate
//
// This package provides a framework for expressing relational predicates
// in logical conditions that describe how values relate to one another.
// Types implementing these interfaces can be used in generic algorithms
// that require comparison or equality semantics.
//
// # Predicates
//
// A predicate is a function or condition that returns a boolean result about a property or relationship.
// The interfaces in this package define predicates that answer fundamental questions:
//
//   - Equatable: "Are these two values semantically equal?"
//   - Comparable: "How do these values order relative to each other?"
//
// These predicates are distinct from syntax-level equality (==) because they allow
// types to define semantic equivalence and ordering logic tailored to their domain.
package predicate

// Equatable defines custom equality semantics for a type.
//
// An implementation of Equatable allows a type to define how semantic equality
// is determined, distinct from Go's syntax-level equality operator (==).
// This is useful for encapsulated types, domain-specific values, or situations
// where reference equality differs from logical equivalence.
//
// # Semantic vs Syntactic Equality
//
// Go's == operator performs syntactic equality: for structs, it compares all
// fields directly. Equatable allows types to define semantic equality
// based on domain logic instead.
type Equatable[T any] interface {
	// Equal will perform semantic equality checking.
	Equal(oth T) bool
}

// Comparable defines how comparison can be implemented.
// An implementation of Comparable can be sorted by the routines in this package.
// The methods refer to elements of the underlying collection by integer index.
//
// Types implementing this interface must provide a Compare method that defines the ordering or equivalence of values.
// This pattern is useful when working with:
// - Custom user-defined types requiring comparison logic
// - Encapsulated values needing semantic comparisons
// - Comparison-agnostic systems (e.g., sorting algorithms)
type Comparable[T any] interface {
	// Compare returns:
	//   -1 if receiver is less than the argument,
	//    0 if they're equal, and
	//   +1 if receiver is greater.
	//
	// Think of the result of Compare like a seesaw:
	// The side that’s lower (touching the ground) represents the smaller value.
	// The side that’s up shows the larger value — it’s higher, so it’s greater.
	//
	// Implementors must ensure consistent ordering semantics.
	Compare(T) int
}

type ComparableShort[T any] interface {
	// Cmp compares x and y and returns:
	//   - -1 if x  < y;
	//   -  0 if x == y;
	//   - +1 if x  > y.
	//
	// x cmp y == x cmp y
	// x cmp (-y) == x
	// (-x) cmp y == y
	// (-x) cmp (-y) == -(x cmp y)
	//
	Cmp(T) int
}

// Zeroable defines whether a value represents the zero state for its type.
//
// A zero value is the natural "empty" or "unset" state of a type — analogous
// to Go's zero values, but expressed as a semantic predicate rather than
// syntactic inspection. This is useful for domain types where the zero state
// carries meaning beyond a field-by-field default.
//
// # Relation to Other Predicates
//
// Zeroable is a degenerate form of a predicate: rather than relating two values
// to one another (as Equatable and Comparable do), it expresses a unary
// condition — whether a value relates to the zero of its own type.
type Zeroable interface {
	// IsZero reports whether the receiver represents the zero value for its type.
	IsZero() bool
}
