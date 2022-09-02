package txs

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/teardown"
)

type Manager struct{ ContextKey any }

type transaction struct {
	parent        *transaction
	done          bool
	rollbackSteps teardown.Teardown
}

func (c Manager) Begin(ctx context.Context) context.Context {
	tx, err := c.BeginTx(ctx)
	if err != nil { // err should be nil unless an implementation mistake
		panic(err.Error())
	}
	return tx
}

func (c Manager) Finish(returnError *error, tx context.Context) {
	if *returnError != nil {
		_ = c.RollbackTx(tx)
		return
	}

	*returnError = c.CommitTx(tx)
}

func (c Manager) OnRollback(ctx context.Context, step func()) error {
	tx, ok := c.lookupParent(ctx)
	if !ok {
		return ErrNoTx
	}
	tx.rollbackSteps.Defer(step)
	return nil
}

func (c Manager) BeginTx(ctx context.Context) (context.Context, error) {
	parent, _ := c.lookup(ctx)
	return context.WithValue(ctx, c.getKey(), &transaction{parent: parent}), nil
}

func (c Manager) CommitTx(ctx context.Context) error {
	tx, ok := c.lookup(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	return nil
}

func (c Manager) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookup(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	if tx.parent != nil {
		return nil
	}
	tx.rollbackSteps.Finish()
	return nil
}

func (c Manager) lookup(ctx context.Context) (*transaction, bool) {
	tx, ok := ctx.Value(c.getKey()).(*transaction)
	return tx, ok
}

func (c Manager) lookupParent(ctx context.Context) (*transaction, bool) {
	tx, ok := ctx.Value(c.getKey()).(*transaction)
	if !ok {
		return nil, false
	}
	for tx.parent != nil {
		tx = tx.parent
	}
	return tx, ok
}

func (c Manager) getKey() any {
	var key = c.ContextKey
	if key == nil {
		type defaultContextKey struct{}
		key = defaultContextKey{}
	}
	return key
}
