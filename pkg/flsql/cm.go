package flsql

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/reflectkit"
)

// ConnectionAdapter is generic implementation to handle query interactions which are aware of trasnactions within the context.
//
// Example:
//
//	type Connection = flsql.ConnectionAdapter[*sql.DB, *sql.Tx]
type ConnectionAdapter[DB, TX any] struct {
	// DB is the underlying Database type to access.
	// It is ideal if ConnectionAdapter used as the Connection type implementation, but you need access to not exposed functionalities.
	//
	// 		type Connection flsql.ConnectionAdapter[*sql.DB, *sql.Tx]
	//
	DB DB
	// TxAdapter provides the mapping for a native driver specific TX type to be usable as a Queryable.
	TxAdapter func(tx TX) Queryable
	// DBAdapter provides the mapping for a native driver specific DB type to be usable as a Queryable.
	DBAdapter func(db DB) Queryable
	// BeginFunc is a function that must create a new transaction that is also a connection.
	BeginFunc func(ctx context.Context) (TX, error)
	// CommitFunc is a function that must commit a given transaction.
	CommitFunc func(ctx context.Context, tx TX) error
	// Rollback is a function that must rollback a given transaction.
	Rollback func(ctx context.Context, tx TX) error

	// OnClose [optional] is used to implement the io.Closer.
	// If The ConnectionAdapter needs to close something,
	// then this function can be used for that.
	//
	// default: DB.Close()
	OnClose func() error
	// ErrTxDone is the error returned when the transaction is already finished.
	// ErrTxDone is an optional field.
	//
	// default: sql.ErrTxDone
	ErrTxDone error
}

func (c ConnectionAdapter[DB, TX]) Close() error {
	if c.OnClose != nil {
		return c.OnClose()
	}
	if closer, ok := any(c.DB).(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Connection returns the current context's sql connection.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (c ConnectionAdapter[DB, TX]) connection(ctx context.Context) Queryable {
	if c.TxAdapter == nil {
		// panicking here is the idiomatic way
		panic("flsql.ConnectionAdapter implementation error, missing TxAdapter")
	}
	if tx, ok := c.lookupRootTx(ctx); ok {
		// TODO: add done tx connection here
		return c.TxAdapter(*tx.tx)
	}
	if c.DBAdapter == nil {
		// panicking here is the idiomatic way
		panic("flsql.ConnectionAdapter implementation error, missing DBAdapter")
	}
	return c.DBAdapter(c.DB)
}

type txInContext[TX any] struct {
	parent *txInContext[TX]
	tx     *TX
	done   bool
}

func (c ConnectionAdapter[DB, TX]) BeginTx(ctx context.Context) (context.Context, error) {
	tx := &txInContext[TX]{}

	if ptx, ok := c.lookupTx(ctx); ok {
		tx.parent = ptx
	}

	if tx.parent == nil {
		transaction, err := c.BeginFunc(ctx)
		if err != nil {
			return nil, err
		}
		tx.tx = &transaction
	}

	return context.WithValue(ctx, ctxKeyForContextTxHandler[TX]{}, tx), nil
}

func (c ConnectionAdapter[DB, TX]) CommitTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no transaction found in the current context`)
	}
	if tx.done {
		return fmt.Errorf("CommitTx: %w", c.txDoneErr())
	}
	tx.done = true
	if tx.tx == nil {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return errorkit.Merge(err, c.Rollback(ctx, *tx.tx))
	}
	return c.CommitFunc(ctx, *tx.tx)
}

func (c ConnectionAdapter[DB, TX]) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgresql transaction found in the current context`)
	}
	if tx.done {
		return fmt.Errorf("RollbackTx: %w", c.txDoneErr())
	}
	for {
		tx.done = true
		if tx.tx != nil {
			return errorkit.Merge(c.Rollback(ctx, *tx.tx), ctx.Err())
		}
		if tx.parent != nil {
			tx = tx.parent
		}
	}
}

func (c ConnectionAdapter[DB, TX]) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	conn := c.connection(ctx)
	c.debugLogExec(ctx, conn, "ExecContext", query, args)
	return conn.ExecContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	conn := c.connection(ctx)
	c.debugLogExec(ctx, conn, "QueryContext", query, args)
	return conn.QueryContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	conn := c.connection(ctx)
	c.debugLogExec(ctx, conn, "QueryRowContext", query, args)
	return conn.QueryRowContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) txDoneErr() error {
	if c.ErrTxDone != nil {
		return c.ErrTxDone
	}
	return sql.ErrTxDone
}

type ctxKeyForContextTxHandler[T any] struct{}

func (c ConnectionAdapter[DB, TX]) lookupTx(ctx context.Context) (*txInContext[TX], bool) {
	tx, ok := ctx.Value(ctxKeyForContextTxHandler[TX]{}).(*txInContext[TX])
	return tx, ok
}

func (c ConnectionAdapter[DB, TX]) debugLogExec(ctx context.Context, conn Queryable, method string, query string, args []any) {
	logger.Debug(ctx, "QueryableAdapter", logging.LazyDetail(func() logging.Detail {
		fs := logging.Fields{
			"method":     method,
			"connection": reflectkit.SymbolicName(conn),
			"query":      query,
			"args":       args,
		}
		if _, ok := c.lookupTx(ctx); ok {
			fs["in-transaction"] = ok
		}
		return fs
	}))
}

func (c ConnectionAdapter[DB, TX]) lookupRootTx(ctx context.Context) (*txInContext[TX], bool) {
	tx, ok := c.lookupTx(ctx)
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

type QueryableAdapter[T any] struct {
	V T

	ExecFunc     func(ctx context.Context, query string, args ...any) (Result, error)
	QueryFunc    func(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRowFunc func(ctx context.Context, query string, args ...any) Row
}

func (a QueryableAdapter[T]) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return a.ExecFunc(ctx, query, args...)
}

func (a QueryableAdapter[T]) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return a.QueryFunc(ctx, query, args...)
}

func (a QueryableAdapter[T]) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return a.QueryRowFunc(ctx, query, args...)
}

// SQLConnectionAdapter is a built-in ConnectionAdapter usage for the stdlib sql.DB/sql.Tx.
// This can be used with any sql driver that integartes with the sql stdlib.
func SQLConnectionAdapter(db *sql.DB) ConnectionAdapter[*sql.DB, *sql.Tx] {
	return ConnectionAdapter[*sql.DB, *sql.Tx]{
		DB: db,

		DBAdapter: QueryableSQL[*sql.DB],
		TxAdapter: QueryableSQL[*sql.Tx],

		BeginFunc: func(ctx context.Context) (*sql.Tx, error) {
			// TODO: integrate begin tx options
			return db.BeginTx(ctx, nil)
		},

		CommitFunc: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Commit()
		},

		Rollback: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Rollback()
		},
	}
}
