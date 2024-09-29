package flsql

import (
	"context"
	"database/sql"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/txkit"
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

func (c ConnectionAdapter[DB, TX]) txm() txkit.Manager[DB, TX, Queryable] {
	return txkit.Manager[DB, TX, Queryable]{
		Begin:     c.Begin,
		Commit:    c.Commit,
		Rollback:  c.Rollback,
		ErrTxDone: c.txDoneErr(),
		DB:        c.DB,
		TxAdapter: c.TxAdapter,
		DBAdapter: c.DBAdapter,
		OnClose:   c.OnClose,
	}
}

func (c ConnectionAdapter[DB, TX]) Close() error {
	return c.txm().Close()
}

type txInContext[TX any] struct {
	parent *txInContext[TX]
	tx     *TX
	done   bool
	cancel func()
}

func (c ConnectionAdapter[DB, TX]) BeginTx(ctx context.Context) (context.Context, error) {
	return c.txm().BeginTx(ctx)
}

func (c ConnectionAdapter[DB, TX]) CommitTx(ctx context.Context) error {
	return c.txm().CommitTx(ctx)
}

func (c ConnectionAdapter[DB, TX]) RollbackTx(ctx context.Context) error {
	return c.txm().RollbackTx(ctx)
}

func (c ConnectionAdapter[DB, TX]) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	conn := c.txm().Q(ctx)
	c.debugLogExec(ctx, conn, "ExecContext", query, args)
	return conn.ExecContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	conn := c.txm().Q(ctx)
	c.debugLogExec(ctx, conn, "QueryContext", query, args)
	return conn.QueryContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	conn := c.txm().Q(ctx)
	c.debugLogExec(ctx, conn, "QueryRowContext", query, args)
	return conn.QueryRowContext(ctx, query, args...)
}

func (c ConnectionAdapter[DB, TX]) txDoneErr() error {
	if c.ErrTxDone != nil {
		return c.ErrTxDone
	}
	return sql.ErrTxDone
}

func (c ConnectionAdapter[DB, TX]) debugLogExec(ctx context.Context, conn Queryable, method string, query string, args []any) {
	logger.Debug(ctx, "QueryableAdapter", logging.LazyDetail(func() logging.Detail {
		fs := logging.Fields{
			"method":     method,
			"connection": reflectkit.SymbolicName(conn),
			"query":      query,
			"args":       args,
		}

		if _, ok := c.txm().LookupTx(ctx); ok {
			fs["in-transaction"] = ok
		}
		return fs
	}))
}

type QueryableAdapter struct {
	ExecFunc     func(ctx context.Context, query string, args ...any) (Result, error)
	QueryFunc    func(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRowFunc func(ctx context.Context, query string, args ...any) Row
}

func (a QueryableAdapter) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return a.ExecFunc(ctx, query, args...)
}

func (a QueryableAdapter) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return a.QueryFunc(ctx, query, args...)
}

func (a QueryableAdapter) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return a.QueryRowFunc(ctx, query, args...)
}

// SQLConnectionAdapter is a built-in ConnectionAdapter usage for the stdlib sql.DB/sql.Tx.
// This can be used with any sql driver that integartes with the sql stdlib.
func SQLConnectionAdapter(db *sql.DB) ConnectionAdapter[sql.DB, sql.Tx] {
	return ConnectionAdapter[sql.DB, sql.Tx]{
		DB: db,

		DBAdapter: QueryableSQL[*sql.DB],
		TxAdapter: QueryableSQL[*sql.Tx],

		Begin: func(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
			// TODO: integrate begin tx options
			return db.BeginTx(ctx, nil)
		},

		Commit: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Commit()
		},

		Rollback: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Rollback()
		},
	}
}
