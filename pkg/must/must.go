// Package must is a syntax sugar package to make the use of `Must` functions.
//
// The `must` package provides an easy way to make functions panic on error.
// This is typically used at the global variable scope where returning an error is inconvenient
// and meaningful error recovery isn't possible due to it being a programming error.
// For example, the two variant functions behave the same:
//
//	must.Must(regexp.Compile(`regexp`))
//	regexp.Must(regexp.Compile(`regexp`)
//
// Dot import can be used since the package is intentionally kept small and focused on this specific topic:
//
//	Must(regexp.Compile(`regexp`))
package must

// Must is a syntax sugar to express things like must.Must(regexp.Compile(`regexp`))
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func Must0(err error) {
	if err != nil {
		panic(err)
	}
}

func Must2[A, B any](a A, b B, err error) (A, B) {
	if err != nil {
		panic(err)
	}
	return a, b
}

func Must3[A, B, C any](a A, b B, c C, err error) (A, B, C) {
	if err != nil {
		panic(err)
	}
	return a, b, c
}

func Must4[A, B, C, D any](a A, b B, c C, d D, err error) (A, B, C, D) {
	if err != nil {
		panic(err)
	}
	return a, b, c, d
}

func Must5[A, B, C, D, E any](a A, b B, c C, d D, e E, err error) (A, B, C, D, E) {
	if err != nil {
		panic(err)
	}
	return a, b, c, d, e
}

const ErrNotOK = "ErrNotOK"

func OK[T any](v T, ok bool) T {
	if !ok {
		panic(ErrNotOK)
	}
	return v
}

func OK0(ok bool) {
	if !ok {
		panic(ErrNotOK)
	}
}

func OK2[A, B any](a A, b B, ok bool) (A, B) {
	if !ok {
		panic(ErrNotOK)
	}
	return a, b
}

func OK3[A, B, C any](a A, b B, c C, ok bool) (A, B, C) {
	if !ok {
		panic(ErrNotOK)
	}
	return a, b, c
}

func OK4[A, B, C, D any](a A, b B, c C, d D, ok bool) (A, B, C, D) {
	if !ok {
		panic(ErrNotOK)
	}
	return a, b, c, d
}

func OK5[A, B, C, D, E any](a A, b B, c C, d D, e E, ok bool) (A, B, C, D, E) {
	if !ok {
		panic(ErrNotOK)
	}
	return a, b, c, d, e
}
