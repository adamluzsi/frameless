package internal

type ErrorUnwrap interface {
	Unwrap() error
}
type ErrorAs interface {
	As(target any) bool
}

type ErrorIs interface {
	Is(target error) bool
}
