package postgresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/runtimes"
	"github.com/adamluzsi/frameless/ports/comproto"
	_ "github.com/lib/pq" // side-effect loading
	"io"
)

type ConnectionManager interface {
	io.Closer
	comproto.OnePhaseCommitProtocol
	// Connection returns the current context's connection.
	// This can be a *sql.DB or if we are within a transaction, then an *sql.Tx
	Connection(ctx context.Context) (Connection, error)
}

type Connection interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func NewConnectionManagerWithDSN(dsn string) (ConnectionManager, error) {
	db, err := sql.Open(`postgres`, dsn)
	if err != nil {
		return nil, err
	}
	return &connectionManager{DB: db}, nil
}

func NewConnectionManagerWithDB(db *sql.DB) ConnectionManager {
	return &connectionManager{DB: db}
}

type connectionManager struct{ DB *sql.DB }

func (c *connectionManager) Close() error {
	return c.DB.Close()
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

	return client, nil
}

type ctxCMTxKey struct{}

type cmTx struct {
	parent *cmTx
	sqlTx  *sql.Tx
	done   bool
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
		sqlTx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		tx.sqlTx = sqlTx
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
	if tx.sqlTx == nil {
		return nil
	}
	return tx.sqlTx.Commit()
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
		if tx.sqlTx != nil {
			return tx.sqlTx.Rollback()
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

func (c *connectionManager) lookupSqlTx(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return nil, false
	}
	for {
		if tx.sqlTx != nil {
			return tx.sqlTx, true
		}
		tx = tx.parent
	}
}

func (c *connectionManager) getConnection(ctx context.Context) (_ *sql.DB, rErr error) {
	defer runtimes.Recover(&rErr)
	var retryCount = 42

ping:
	err := c.DB.PingContext(ctx)

	if errors.Is(err, driver.ErrBadConn) && 0 < retryCount {
		// it could be a temporary error, recovery is still possible
		retryCount--
		goto ping
	}

	if err != nil {
		return nil, err
	}

	return c.DB, nil
}
