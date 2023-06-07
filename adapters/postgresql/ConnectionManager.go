package postgresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"github.com/adamluzsi/frameless/pkg/runtimekit"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"reflect"
)

type ConnectionManager interface {
	io.Closer
	comproto.OnePhaseCommitProtocol
	// Connection returns the current context's connection.
	// This can be a *sql.DB or if we are within a transaction, then an *sql.Tx
	Connection(ctx context.Context) (Connection, error)
	Connection
}

type Connection interface {
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

func NewConnectionManager(dsn string) (ConnectionManager, error) {
	conn, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, err
	}
	return &connectionManager{C: conn}, nil
}

// NewConnectionManagerWithDSN
//
// DEPRECATED: use NewConnectionManager instead
func NewConnectionManagerWithDSN(dsn string) (ConnectionManager, error) {
	return NewConnectionManager(dsn)
}

type connectionManager struct{ C *pgxpool.Pool }

func (c *connectionManager) ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error) {
	connection, err := c.Connection(ctx)
	if err != nil {
		return nil, err
	}
	return connection.ExecContext(ctx, query, args...)
}

func (c *connectionManager) QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	connection, err := c.Connection(ctx)
	if err != nil {
		return nil, err
	}
	return connection.QueryContext(ctx, query, args...)
}

func (c *connectionManager) QueryRowContext(ctx context.Context, query string, args ...interface{}) Row {
	connection, err := c.Connection(ctx)
	if err != nil {
		var r sql.Row
		srrv := reflect.ValueOf(&r)
		reflectkit.SetValue(srrv.Elem().FieldByName("err"), reflect.ValueOf(err))
		return srrv.Interface().(*sql.Row)
	}
	return connection.QueryRowContext(ctx, query, args...)
}

func (c *connectionManager) Close() error {
	c.C.Close()
	return nil
}

// Connection returns the current context's sql connection.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (c *connectionManager) Connection(ctx context.Context) (Connection, error) {
	if tx, ok := c.lookupSqlTx(ctx); ok {
		return tx, nil
	}

	client, err := c.getConnection(ctx)
	if err != nil {
		return nil, err
	}

	return pgxConnAdapter{C: client}, nil
}

type ctxCMTxKey struct{}

type cmTx struct {
	parent *cmTx
	pgxConnAdapter
	tx   pgx.Tx
	done bool
}

func (c *connectionManager) BeginTx(ctx context.Context) (context.Context, error) {
	tx := &cmTx{}

	if ptx, ok := c.lookupTx(ctx); ok {
		tx.parent = ptx
	}

	if tx.parent == nil {
		conn, err := c.getConnection(ctx)
		if err != nil {
			return nil, err
		}
		transaction, err := conn.Begin(ctx)
		if err != nil {
			return nil, err
		}
		tx.tx = transaction
		tx.pgxConnAdapter = pgxConnAdapter{C: transaction}
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

func (c *connectionManager) getConnection(ctx context.Context) (_ *pgxpool.Pool, rErr error) {
	defer runtimekit.Recover(&rErr)
	var retryCount = 42

ping:
	err := c.C.Ping(ctx)

	if errors.Is(err, driver.ErrBadConn) && 0 < retryCount {
		// it could be a temporary error, recovery is still possible
		retryCount--
		goto ping
	}

	if err != nil {
		return nil, err
	}

	return c.C, nil
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
