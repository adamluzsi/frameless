package txs

import (
	"context"
)

type contextKey struct{}

func Begin(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, contextKey{}, &cascadingTx{
		baseTx: baseTx{
			parent: getTx(ctx),
			cancel: cancel,
		},
	})
}

func BeginIsolated(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, contextKey{}, &baseTx{
		parent: getTx(ctx),
		cancel: cancel,
	})
}

func Finish(returnError *error, tx context.Context) {
	if *returnError == nil {
		*returnError = CommitTx(tx)
		return
	}
	rbErr := RollbackTx(tx)
	if rbErr == nil {
		return
	}
	*returnError = &txRollbackError{
		Cause: *returnError,
		Err:   rbErr,
	}
}

func CommitTx(ctx context.Context) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	return tx.Commit()
}

func RollbackTx(ctx context.Context) error {
	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	return tx.Rollback()
}

func OnRollback(ctx context.Context, step func()) error {

	tx, ok := lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	return tx.OnRollback(step)
}

func getTx(ctx context.Context) transaction {
	tx, _ := lookupTx(ctx)
	return tx
}

func lookupTx(ctx context.Context) (transaction, bool) {
	tx, ok := ctx.Value(contextKey{}).(transaction)
	return tx, ok
}
