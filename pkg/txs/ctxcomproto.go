package txs

import (
	"context"
)

type contextKey struct{}

func Begin(ctx context.Context) (context.Context, error) {
	if err := checkContext(ctx); err != nil {
		return ctx, err
	}
	ctx, cancel := context.WithCancel(ctx)
	parent, _ := lookupTx(ctx)
	return context.WithValue(ctx, contextKey{}, &transaction{
		parent: parent,
		cancel: cancel,
	}), nil
}

func Finish(returnError *error, tx context.Context) {
	if *returnError == nil {
		*returnError = Commit(tx)
		return
	}
	rbErr := Rollback(tx)
	if rbErr == nil {
		return
	}
	*returnError = &txRollbackError{
		Cause: *returnError,
		Err:   rbErr,
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

func OnRollback[StepFn func() | func() error](ctx context.Context, step StepFn) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	switch fn := any(step).(type) {
	case func():
		return tx.OnRollback(func() error { fn(); return nil })
	case func() error:
		return tx.OnRollback(fn)
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
