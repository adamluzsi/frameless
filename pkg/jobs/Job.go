package jobs

import (
	"context"
	"fmt"
)

// Job is the basic unit of jobs package, that represents an executable work.
//
// Job at its core, is nothing more than a synchronous function.
// Working with synchronous functions removes the complexity of thinking about how to run your application.
// Your components become more stateless and focus on the domain rather than the lifecycle management.
// This less stateful approach can help to make testing your Job also easier.
type Job func(context.Context) error

type genericJob interface {
	Job | func(context.Context) error | func() error | func()
}

func ToJob[JFN genericJob](fn JFN) Job {
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
