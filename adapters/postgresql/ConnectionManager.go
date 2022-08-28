package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sync"

	_ "github.com/lib/pq" // side-effect loading
)

type ConnectionManager interface {
	io.Closer
	// Connection returns the current context's connection.
	// This can be a *sql.DB or if we are within a transaction, then an *sql.Tx
	Connection(ctx context.Context) (Connection, error)

	BeginTx(ctx context.Context) (context.Context, error)
	CommitTx(ctx context.Context) error
	RollbackTx(ctx context.Context) error
}

type Connection interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func NewConnectionManager(dsn string) ConnectionManager {
	return &connectionManager{DSN: dsn}
}

type connectionManager struct {
	DSN string

	mutex      sync.Mutex
	connection *sql.DB
}

func (c *connectionManager) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.connection == nil {
		return nil
	}
	return c.connection.Close()
}

// Connection returns the current context's sql connection.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (c *connectionManager) Connection(ctx context.Context) (Connection, error) {
	if tx, ok := c.lookupTx(ctx); ok {
		return tx, nil
	}

	client, err := c.getConnection()
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

	conn, err := c.getConnection()
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

func (c *connectionManager) getConnection() (*sql.DB, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	setConnection := func() error {
		db, err := sql.Open(`postgres`, c.DSN)
		if err != nil {
			return err
		}
		c.connection = db
		return nil
	}
	if c.connection == nil {
		if err := setConnection(); err != nil {
			return nil, err
		}
	}
	if err := c.connection.Ping(); err != nil {
		if err := setConnection(); err != nil {
			return nil, err
		}
	}
	return c.connection, nil
}
