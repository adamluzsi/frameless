package dscontract

import (
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
)

type mkFunc[T any] interface {
	func() T | func(testing.TB) T
}

func mk[T any, FN mkFunc[T]](tb testing.TB, fn FN) T {
	if fn == nil {
		return spechelper.MakeValue[T](tb)
	}
	switch function := any(fn).(type) {
	case func() T:
		return function()
	case func(testing.TB) T:
		return function(tb)
	default:
		panic("not-implemented")
	}
}
