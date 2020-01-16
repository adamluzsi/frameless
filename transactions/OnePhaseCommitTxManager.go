package transactions

import (
	"context"
	"sync"
)

func NewOnePhaseCommitTxManager() *OnePhaseCommitTxManager {
	return &OnePhaseCommitTxManager{}
}

// OnePhaseCommitTxManager serves as a context based transaction manager
// to solve the problem of having a resource across many different supplier implementations in one transaction.
//	e.g.: many function works implements different goals on top of a database, and the they should act like one unit during an ineractor call.
//
// OnePhaseCommitTxManager does not guarantee that handled resources act together,
// in case one commit fails, there is no guarantee that units will be rolled back.
// handled transactions interpreted as independent resources.
// For linking commit/rollback lifecycle management in resources into one unit, see TwoPhaseCommitTxManager.
type OnePhaseCommitTxManager struct {
	contextAccessors      []*defaultContextAccessor
	contextAccessorsMutex sync.Mutex
}

type OnePhaseCommitAdapter interface {
	// BeginTx initiates a transaction block, that is,
	// all statements after a BEGIN command will be executed in a single transaction
	// until an explicit COMMIT or ROLLBACK is given.
	BeginTx(ctx context.Context) (tx interface{}, err error)
	// Commit commits the current transaction.
	// All changes made by the transaction become visible to others and are guaranteed to be durable if a crash occurs.
	Commit(tx interface{}) error
	// Rollback rolls back the current transaction and causes all the updates made by the transaction to be discarded.
	Rollback(tx interface{}) error
}

func (txm *OnePhaseCommitTxManager) RegisterAdapter(adapter OnePhaseCommitAdapter) ContextAccessor {
	txm.contextAccessorsMutex.Lock()
	defer txm.contextAccessorsMutex.Unlock()
	var ctxAcc = &defaultContextAccessor{
		txStackKey: adapter,
		adapter:    adapter,
	}
	txm.contextAccessors = append(txm.contextAccessors, ctxAcc)
	return ctxAcc
}

func (txm *OnePhaseCommitTxManager) ContextWithTransactionManagement(ctx context.Context) (context.Context, Handler) {
	ctx = contextWithTransactions(ctx)
	return ctx, &onePhaseCommitTxManagerHandler{
		txm: txm,
		ctx: ctx,
	}
}

type onePhaseCommitTxManagerHandler struct {
	txm *OnePhaseCommitTxManager
	ctx context.Context
}

func (h onePhaseCommitTxManagerHandler) Commit() error {
	if len(h.txm.contextAccessors) == 0 {
		return nil
	}
	var errs []error
	for _, ctxAccessor := range h.txm.contextAccessors {
		if err := ctxAccessor.GetHandler(h.ctx).Commit(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return MultiError{Errors: errs}
	}
	return nil
}

func (h onePhaseCommitTxManagerHandler) Rollback() error {
	if len(h.txm.contextAccessors) == 0 {
		return nil
	}
	var errs []error
	for _, ctxAccessor := range h.txm.contextAccessors {
		if err := ctxAccessor.GetHandler(h.ctx).Rollback(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return MultiError{Errors: errs}
	}
	return nil
}
