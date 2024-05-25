package txs

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/contextkit"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/teardown"
)

////////////////////////////////////////////////////// TRANSACTION /////////////////////////////////////////////////////

type transaction struct {
	parent    *transaction
	done      bool
	context   context.Context
	cancel    func()
	rollbacks teardown.Teardown
}

func (tx *transaction) OnRollback(fn func(context.Context) error) error {
	if tx.done {
		return ErrTxDone
	}
	tx.rollbacks.Defer(func() error {
		return fn(tx.context)
	})
	return nil
}

func (tx *transaction) Commit() error {
	if err := tx.finish(); err != nil {
		return err
	}
	if tx.parent != nil {
		return tx.parent.OnRollback(func(context.Context) error {
			return tx.rollbacks.Finish()
		})
	}
	return nil
}

func (tx *transaction) Rollback() (rErr error) {
	if tx.done {
		return ErrTxDone
	}
	defer func() { rErr = errorkit.Merge(rErr, tx.finish()) }()
	defer func() {
		if tx.parent == nil {
			return
		}
		rErr = errorkit.Merge(rErr, tx.parent.Rollback())
	}()
	return tx.rollbacks.Finish()
}

func (tx *transaction) isDone() bool {
	return tx.done
}

func (tx *transaction) finish() error {
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	tx.cancel()
	return nil
}

//////////////////////////////////////////////////////// ERRORS ////////////////////////////////////////////////////////

const (
	ErrTxDone errorkit.Error = "transaction is already finished"
	ErrNoCtx  errorkit.Error = "context.Context not given"
	ErrNoTx   errorkit.Error = "no transaction present in the current context"
)

type TxRollbackError struct {
	Err   error
	Cause error
}

func (err *TxRollbackError) Error() string {
	return fmt.Sprintf("%s (rollback: %s)", err.Cause, err.Err)
}

func (err *TxRollbackError) Unwrap() error {
	return err.Cause
}

//////////////////////////////////////////////////////// CONTEXT ///////////////////////////////////////////////////////

type contextKey struct{}

func Begin(ctx context.Context) (context.Context, error) {
	if err := checkContext(ctx); err != nil {
		return ctx, err
	}
	parent, _ := lookupTx(ctx)
	subctx, cancel := context.WithCancel(ctx)
	return context.WithValue(subctx, contextKey{}, &transaction{
		parent:  parent,
		context: contextkit.Detach(ctx),
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
	*returnError = &TxRollbackError{
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
