package postgresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/ports/comproto"
)

// Connection represent an open connection.
// Connection will respect the transaction state in the received context.Context.
type Connection interface {
	io.Closer
	comproto.OnePhaseCommitProtocol
	ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) Row
}

type Result interface {
	// RowsAffected returns the number of rows affected by an
	// update, insert, or delete. Not every database or database
	// driver may support this.
	RowsAffected() int64
}

type Rows interface {
	io.Closer
	// Err returns any error that occurred while reading.
	Err() error
	// Next prepares the next row for reading. It returns true if there is another
	// row and false if no more rows are available. It automatically closes rows
	// when all rows are read.
	Next() bool
	// Scan reads the values from the current row into dest values positionally.
	// dest can include pointers to core types, values implementing the Scanner
	// interface, and nil. nil will skip the value entirely. It is an error to
	// call Scan without first calling Next() and checking that it returned true.
	Scan(dest ...any) error
}

type Row interface {
	// Scan works the same as Rows. with the following exceptions. If no
	// rows were found it returns errNoRows. If multiple rows are returned it
	// ignores all but the first.
	Scan(dest ...any) error
}

func Connect(dsn string) (Connection, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, err
	}
	return &connectionManager{Pool: pool}, nil
}

type connectionManager struct{ Pool *pgxpool.Pool }

func (c *connectionManager) ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error) {
	connection, err := c.connection(ctx)
	if err != nil {
		return nil, err
	}
	return connection.ExecContext(ctx, query, args...)
}

func (c *connectionManager) QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	connection, err := c.connection(ctx)
	if err != nil {
		return nil, err
	}
	return connection.QueryContext(ctx, query, args...)
}

func (c *connectionManager) QueryRowContext(ctx context.Context, query string, args ...interface{}) Row {
	connection, err := c.connection(ctx)
	if err != nil {
		var r sql.Row
		srrv := reflect.ValueOf(&r)
		reflectkit.SetValue(srrv.Elem().FieldByName("err"), reflect.ValueOf(err))
		return srrv.Interface().(*sql.Row)
	}
	return connection.QueryRowContext(ctx, query, args...)
}

func (c *connectionManager) Close() error {
	c.Pool.Close()
	return nil
}

type conn interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) Row
}

// Connection returns the current context's sql connection.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (c *connectionManager) connection(ctx context.Context) (conn, error) {
	if tx, ok := c.lookupSqlTx(ctx); ok {
		return tx, nil
	}

	pool, err := c.getPgxPool(ctx)
	if err != nil {
		return nil, err
	}

	return pgxConnAdapter{C: pool}, nil
}

type ctxCMTxKey struct{}

type cmTx struct {
	parent *cmTx
	*pgxConnAdapter
	tx   pgx.Tx
	done bool
}

func (c *connectionManager) BeginTx(ctx context.Context) (context.Context, error) {
	tx := &cmTx{}

	if ptx, ok := c.lookupTx(ctx); ok {
		tx.parent = ptx
	}

	if tx.parent == nil {
		conn, err := c.getPgxPool(ctx)
		if err != nil {
			return nil, err
		}
		transaction, err := conn.Begin(ctx)
		if err != nil {
			return nil, err
		}
		tx.tx = transaction
		tx.pgxConnAdapter = &pgxConnAdapter{C: transaction}
	}

	return context.WithValue(ctx, ctxCMTxKey{}, tx), nil
}

func (c *connectionManager) CommitTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgresql transaction found in the current context`)
	}
	if tx.done {
		return sql.ErrTxDone
	}
	tx.done = true
	if tx.tx == nil {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		_ = tx.tx.Rollback(ctx)
		return err
	}
	return tx.tx.Commit(ctx)
}

func (c *connectionManager) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgresql transaction found in the current context`)
	}
	if tx.done {
		return sql.ErrTxDone
	}
	for {
		tx.done = true
		if tx.tx != nil {
			return errorkit.Merge(tx.tx.Rollback(ctx), ctx.Err())
		}
		if tx.parent != nil {
			tx = tx.parent
		}
	}
}

func (c *connectionManager) lookupTx(ctx context.Context) (*cmTx, bool) {
	tx, ok := ctx.Value(ctxCMTxKey{}).(*cmTx)
	return tx, ok
}

func (c *connectionManager) lookupSqlTx(ctx context.Context) (*cmTx, bool) {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return nil, false
	}
	for {
		if tx.tx != nil {
			return tx, true
		}
		tx = tx.parent
	}
}

// mutexPing used to guard the Ping pgxpool.Pool Ping call from concurrent access.
// For some reason it yields a race condition.
var mutexPing sync.Mutex

func (c *connectionManager) getPgxPool(ctx context.Context) (_ *pgxpool.Pool, rErr error) {
	defer errorkit.Recover(&rErr)
	var retryCount = 42

ping:

	err := func() error {
		mutexPing.Lock()
		defer mutexPing.Unlock()
		return c.Pool.Ping(ctx)
	}()

	if errors.Is(err, driver.ErrBadConn) && 0 < retryCount {
		// it could be a temporary error, recovery is still possible
		retryCount--
		goto ping
	}

	if err != nil {
		return nil, err
	}

	return c.Pool, nil
}

type pgxConnAdapter struct{ C pgxConn }

type pgxConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (ca pgxConnAdapter) ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error) {
	return ca.C.Exec(ctx, query, args...)
}

func (ca pgxConnAdapter) QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	rows, err := ca.C.Query(ctx, query, args...)
	return pgxRowsAdapter{Rows: rows}, err
}

func (ca pgxConnAdapter) QueryRowContext(ctx context.Context, query string, args ...interface{}) Row {
	return ca.C.QueryRow(ctx, query, args...)
}

type pgxRowsAdapter struct {
	pgx.Rows
}

func (a pgxRowsAdapter) Close() error {
	a.Rows.Close()
	// TODO: a.Rows.Err()
	return nil
}
