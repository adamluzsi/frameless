package errorkit

var (
	_ ErrorUnwrap = &tagError{}
	_ ErrorUnwrap = withContext{}
	_ ErrorUnwrap = withDetail{}
)

type ErrorUnwrap interface {
	Unwrap() error
}

type ErrorAs interface {
	As(target any) bool
}

type ErrorIs interface {
	Is(target error) bool
}
