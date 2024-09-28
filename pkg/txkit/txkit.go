package txkit

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/teardown"
)

const (
	ErrTxDone errorkit.Error = "transaction is already finished"
	ErrNoCtx  errorkit.Error = "context.Context not given"
	ErrNoTx   errorkit.Error = "no transaction present in the current context"
)

type contextTransaction struct {
	context   context.Context
	rollbacks teardown.Teardown
	done      bool
}

func (tx *contextTransaction) OnRollback(fn func(context.Context) error) error {
	if tx.done {
		return ErrTxDone
	}
	tx.rollbacks.Defer(func() error {
		return fn(tx.context)
	})
	return nil
}

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

var m = Manager[any, contextTransaction, any]{
	Begin: func(ctx context.Context, _ *any) (*contextTransaction, error) {
		return &contextTransaction{
			context:   contextkit.WithoutCancel(ctx),
			rollbacks: teardown.Teardown{},
		}, nil
	},
	Commit: func(ctx context.Context, tx *contextTransaction) error {
		tx.done = true
		return nil
	},
	Rollback: func(ctx context.Context, tx *contextTransaction) error {
		tx.done = true
		return tx.rollbacks.Finish()
	},
	ErrTxDone: ErrTxDone,
}

func Begin(ctx context.Context) (context.Context, error) {
	return m.BeginTx(ctx)
}

func Finish(returnError *error, tx context.Context) {
	if *returnError == nil {
		*returnError = Commit(tx)
		return
	}
	rollbackErr := Rollback(tx)
	if rollbackErr == nil ||
		rollbackErr == ErrTxDone ||
		errors.Is(rollbackErr, ErrTxDone) {
		return
	}
	*returnError = &TxRollbackError{
		Cause: *returnError,
		Err:   rollbackErr,
	}
}

func Commit(ctx context.Context) error {
	return m.CommitTx(ctx)
}

func Rollback(ctx context.Context) error {
	return m.RollbackTx(ctx)
}

type onRollbackStepFn interface {
	func(ctx context.Context) error | func()
}

func OnRollback[StepFn onRollbackStepFn](ctx context.Context, step StepFn) error {
	tx, ok := m.LookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.done {
		return ErrTxDone
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

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Manager is generic implementation to handle query interactions which are aware of trasnactions within the context.
//
// Example:
//
//	type Connection = txkit.Manager[*sql.DB, *sql.Tx]
//
// Type Arguments:
// - DB: the main connection that
type Manager[DB, TX, Queryable any] struct {
	// DB  is the underlying Database type to access.
	// It is ideal if ConnectionAdapter used as the Connection type implementation, but you need access to not exposed functionalities.
	//
	// 		type Connection txkit.Manager[*sql.DB, *sql.Tx]
	//
	DB *DB
	// TxAdapter provides the mapping for a native driver specific TX type to be usable as a Queryable.
	TxAdapter func(tx *TX) Queryable
	// DBAdapter provides the mapping for a native driver specific DB type to be usable as a Queryable.
	DBAdapter func(db *DB) Queryable
	// Begin is a function that must create a new transaction that is also a connection.
	Begin func(ctx context.Context, db *DB) (*TX, error)
	// Commit is a function that must commit a given transaction.
	Commit func(ctx context.Context, tx *TX) error
	// Rollback is a function that must rollback a given transaction.
	Rollback func(ctx context.Context, tx *TX) error
	// ErrTxDone is the error returned when the transaction is already finished.
	// ErrTxDone is an optional field.
	//
	// default: txkit.ErrTxDone
	ErrTxDone error
	// OnClose [optional] is used to implement the io.Closer.
	// If The ConnectionAdapter needs to close something,
	// then this function can be used for that.
	//
	// default: DB.Close()
	OnClose func() error
}

func (m Manager[DB, TX, Queryable]) Close() error {
	if m.OnClose != nil {
		return m.OnClose()
	}
	if closer, ok := any(m.DB).(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type txInContext[TX any] struct {
	parent *txInContext[TX]
	tx     *TX
	done   bool
	cancel func()
}

func (m Manager[DB, TX, Queryable]) BeginTx(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}

	tx := &txInContext[TX]{}

	if ptx, ok := m.lookupTx(ctx); ok {
		tx.parent = ptx
	}

	if tx.parent == nil {
		transaction, err := m.Begin(ctx, m.DB)
		if err != nil {
			return nil, err
		}
		tx.tx = transaction
	}

	ctx, cancel := context.WithCancel(ctx)
	tx.cancel = cancel

	return context.WithValue(ctx, ctxKeyForContextTxHandler[TX]{}, tx), nil
}

func (m Manager[DB, TX, Queryable]) CommitTx(ctx context.Context) error {
	tx, ok := m.lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.done {
		return fmt.Errorf("CommitTx: %w", m.txDoneErr())
	}
	tx.done = true
	defer tx.cancel()
	if tx.tx == nil {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return errorkit.Merge(err, m.Rollback(ctx, tx.tx))
	}
	return m.Commit(ctx, tx.tx)
}

func (m Manager[DB, TX, Queryable]) RollbackTx(ctx context.Context) error {
	tx, ok := m.lookupTx(ctx)
	if !ok {
		return ErrNoTx
	}
	if tx.done {
		return m.txDoneErr()
	}
	for {
		tx.done = true
		// defer in loop is intentional here
		// defer context cancellation after all the rollback call is executed
		defer tx.cancel()
		if tx.tx != nil {
			rberr := m.Rollback(contextkit.WithoutCancel(ctx), tx.tx)
			ctxErr := ctx.Err()
			return errorkit.Merge(rberr, ctxErr)
		}
		if tx.parent != nil {
			tx = tx.parent
		}
	}
}

func (m Manager[DB, TX, Queryable]) LookupTx(ctx context.Context) (*TX, bool) {
	tx, ok := m.lookupRootTx(ctx)
	if !ok {
		return nil, false
	}
	return tx.tx, true
}

// Connection returns the current context's sql Q.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (m Manager[DB, TX, Queryable]) Q(ctx context.Context) Queryable {
	// Panics in this function the idiomatic way,
	// as it can only happen when the manager constructed incorrectly,
	// and it is impossible to recover from this during runtime.
	if m.TxAdapter == nil {
		panic("txkit.Manager implementation error, missing TxAdapter")
	}
	if tx, ok := m.lookupRootTx(ctx); ok {
		// TODO: add done tx connection here
		return m.TxAdapter(tx.tx)
	}
	if m.DBAdapter == nil {
		// panicking here is the idiomatic way
		panic("txkit.Manager implementation error, missing DBAdapter")
	}
	return m.DBAdapter(m.DB)
}

func (m Manager[DB, TX, Queryable]) txDoneErr() error {
	if m.ErrTxDone != nil {
		return m.ErrTxDone
	}
	return ErrTxDone
}

type ctxKeyForContextTxHandler[T any] struct{}

func (m Manager[DB, TX, Queryable]) lookupTx(ctx context.Context) (*txInContext[TX], bool) {
	tx, ok := ctx.Value(ctxKeyForContextTxHandler[TX]{}).(*txInContext[TX])
	return tx, ok
}

func (m Manager[DB, TX, Queryable]) lookupRootTx(ctx context.Context) (*txInContext[TX], bool) {
	tx, ok := m.lookupTx(ctx)
	if !ok {
		return nil, false
	}
	for {
		if tx.tx != nil {
			return tx, true
		}
		if tx.parent == nil {
			return tx, false
		}
		tx = tx.parent
	}
}
