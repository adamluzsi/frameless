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
	if tx, ok := c.lookupTx(ctx); ok {
		return tx, nil
	}

	client, err := c.getConnection(ctx)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type (
	ctxDefaultPoolTxKey   struct{}
	ctxDefaultPoolTxValue struct {
		depth int
		*sql.Tx
	}
)

func (c *connectionManager) BeginTx(ctx context.Context) (context.Context, error) {
	if tx, ok := c.lookupTx(ctx); ok && tx.Tx != nil {
		tx.depth++
		return ctx, nil
	}

	conn, err := c.getConnection(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	return context.WithValue(ctx, ctxDefaultPoolTxKey{}, &ctxDefaultPoolTxValue{Tx: tx}), nil
}

func (c *connectionManager) CommitTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgresql transaction found in the current context`)
	}

	if tx.depth > 0 {
		tx.depth--
		return nil
	}

	return tx.Commit()
}

func (c *connectionManager) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgres comproto in the given context`)
	}

	return tx.Rollback()
}

func (c *connectionManager) lookupTx(ctx context.Context) (*ctxDefaultPoolTxValue, bool) {
	tx, ok := ctx.Value(ctxDefaultPoolTxKey{}).(*ctxDefaultPoolTxValue)
	return tx, ok
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
