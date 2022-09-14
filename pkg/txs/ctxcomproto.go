package txs

import (
	"context"
)

type contextKey struct{}

func Begin(ctx context.Context) (context.Context, error) {
	if err := checkContext(ctx); err != nil {
		return ctx, err
	}
	parent, _ := lookupTx(ctx)
	subctx, cancel := context.WithCancel(ctx)
	return context.WithValue(subctx, contextKey{}, &transaction{
		parent:  parent,
		context: ctx,
		cancel:  cancel,
	}), nil
}

func Finish(returnError *error, tx context.Context) {
	if *returnError == nil {
		*returnError = Commit(tx)
		return
	}
	rollbackErr := Rollback(tx)
	if rollbackErr == nil ||
		rollbackErr == ErrTxDone {
		return
	}
	*returnError = &txRollbackError{
		Cause: *returnError,
		Err:   rollbackErr,
	}
}

func Commit(ctx context.Context) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.isDone() {
		return ErrTxDone
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func Rollback(ctx context.Context) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.isDone() {
		return ErrTxDone
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	return tx.Rollback()
}

type onRollbackStepFn interface {
	func(ctx context.Context) error | func()
}

func OnRollback[StepFn onRollbackStepFn](ctx context.Context, step StepFn) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	switch fn := any(step).(type) {
	case func(context.Context) error:
		return tx.OnRollback(fn)
	case func() error:
		return tx.OnRollback(func(context.Context) error { return fn() })
	case func():
		return tx.OnRollback(func(context.Context) error { fn(); return nil })
	default:
		panic("not implemented")
	}
}

func lookupTx(ctx context.Context) (*transaction, bool) {
	tx, ok := ctx.Value(contextKey{}).(*transaction)
	return tx, ok
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNoCtx
	}
	return ctx.Err()
}
