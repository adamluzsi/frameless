package transactions

import "context"

// split independent resource use from 2 phase commit based bundled resources

type Manager interface {
	ContextWithTransactionManagement(ctx context.Context) (context.Context, Handler)
}

type Handler interface {
	Commit() error
	Rollback() error
}

type ContextAccessor interface {
	FromContext(ctx context.Context) (tx interface{}, sf StepFinalizer, err error)
}

type ContextAccessorFunc func(ctx context.Context) (tx interface{}, sf StepFinalizer, err error)

func (fn ContextAccessorFunc) FromContext(ctx context.Context) (tx interface{}, sf StepFinalizer, err error) {
	return fn(ctx)
}

type StepFinalizer interface {
	Done() error
}

type StepFunc func() error

func (fn StepFunc) Done() error {
	return fn()
}
