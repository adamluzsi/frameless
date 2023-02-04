package jobs

import (
	"context"
	"fmt"
)

type Job func(context.Context) error

type genericJob interface {
	Job | func(context.Context) error | func() error | func()
}

func toJob[JFN genericJob](fn JFN) Job {
	switch fn := any(fn).(type) {
	case Job:
		return fn
	case func(context.Context) error:
		return fn
	case func() error:
		return func(context.Context) error { return fn() }
	case func():
		return func(context.Context) error { fn(); return nil }
	default:
		panic(fmt.Sprintf("%T is not supported Job func", fn))
	}
}
