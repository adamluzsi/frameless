package transactions

import (
	"context"
	"sync"
)

type transactionsCtxKey struct{}

type genericAdapter interface {
	BeginTx(ctx context.Context) (txPtr interface{}, err error)
	Commit(txPtr interface{}) error
	Rollback(txPtr interface{}) error
}

type defaultContextAccessor struct {
	txStackKey interface{}
	adapter    genericAdapter
	mutex      sync.Mutex
}

func contextWithTransactions(ctx context.Context) context.Context {
	return context.WithValue(ctx, transactionsCtxKey{}, &stack{})
}

func (acc *defaultContextAccessor) FromContext(ctx context.Context) (interface{}, StepFinalizer, error) {
	txs, ok := ctx.Value(transactionsCtxKey{}).(*stack)

	// if no defaultManagerContextTransactions found, fallback to null object pattern and provide a handler
	// that do the commit in the end of the current scope.
	// Since there is no transaction management involved with the current context.
	if !ok {
		tx, err := acc.adapter.BeginTx(ctx)
		if err != nil {
			return nil, nil, err
		}
		return tx, nullObjectStepFinalizer{
			adapter: acc.adapter,
			tx:      tx,
		}, err
	}

	tx, ok := txs.Lookup(acc.txStackKey)

	if !ok {
		acc.mutex.Lock()
		defer acc.mutex.Unlock()

		// verify that we are the goroutine that acquired the lock first.
		tx, ok = txs.Lookup(acc.txStackKey)
		if !ok { // then we need to initialize the tx
			var err error
			tx, err = acc.adapter.BeginTx(ctx)
			if err != nil {
				return nil, nil, err
			}
			txs.Push(acc.txStackKey, tx)
		}

	}

	return tx, nopStepFinalizer{}, nil
}

type nopStepFinalizer struct{}

func (n nopStepFinalizer) Done() error {
	return nil
}

type nullObjectStepFinalizer struct {
	adapter genericAdapter
	tx      interface{}
}

func (n nullObjectStepFinalizer) Done() error {
	return n.adapter.Commit(n.tx)
}

func (acc *defaultContextAccessor) GetHandler(ctx context.Context) Handler {
	return &defaultContextAccessorHandler{
		ctx:        ctx,
		txStackKey: acc.txStackKey,
		adapter:    acc.adapter,
	}
}

type defaultContextAccessorHandler struct {
	ctx        context.Context
	txStackKey interface{}
	adapter    genericAdapter
}

func (h *defaultContextAccessorHandler) lookupTxPtr() (interface{}, bool) {
	return h.ctx.Value(transactionsCtxKey{}).(*stack).Lookup(h.txStackKey)
}

func (h *defaultContextAccessorHandler) Commit() error {
	txPtr, ok := h.lookupTxPtr()
	if !ok {
		return nil
	}
	return h.adapter.Commit(txPtr)
}

func (h *defaultContextAccessorHandler) Rollback() error {
	txPtr, ok := h.lookupTxPtr()
	if !ok {
		return nil
	}
	return h.adapter.Rollback(txPtr)
}
