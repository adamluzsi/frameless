package postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "github.com/lib/pq" // side-effect loading
)

func NewConnectionManager(dsn string) *ConnectionManager {
	return &ConnectionManager{DSN: dsn}
}

type ConnectionManager struct {
	DSN string

	mutex      sync.Mutex
	connection *sql.DB
}

type Connection interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// GetConnection returns the current context's sql connection.
// This can be a *sql.DB or if we within a transaction, then a *sql.Tx.
func (c *ConnectionManager) GetConnection(ctx context.Context) (Connection, error) {
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

func (c *ConnectionManager) BeginTx(ctx context.Context) (context.Context, error) {
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

func (c *ConnectionManager) CommitTx(ctx context.Context) error {
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

func (c *ConnectionManager) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgres tx in the given context`)
	}

	return tx.Rollback()
}

func (c *ConnectionManager) LookupTx(ctx context.Context) (Connection, bool) {
	return c.lookupTx(ctx)
}

func (c *ConnectionManager) lookupTx(ctx context.Context) (*ctxDefaultPoolTxValue, bool) {
	tx, ok := ctx.Value(ctxDefaultPoolTxKey{}).(*ctxDefaultPoolTxValue)
	return tx, ok
}

func (c *ConnectionManager) getConnection() (*sql.DB, error) {
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

func (c *ConnectionManager) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.connection == nil {
		return nil
	}
	return c.connection.Close()
}

type ctxMetaKey struct{}
type metaMap map[string]json.RawMessage

func (c *ConnectionManager) SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error) {
	bs, err := json.Marshal(value)
	if err != nil {
		return ctx, err
	}

	mm, ok := c.lookupMetaMap(ctx)
	if !ok {
		mm = make(metaMap)
		ctx = c.setMetaMap(ctx, mm)
	}
	mm[key] = bs

	return ctx, nil
}

func (c *ConnectionManager) setMetaMap(ctx context.Context, mm metaMap) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxMetaKey{}, mm)
}
func (c *ConnectionManager) lookupMetaMap(ctx context.Context) (metaMap, bool) {
	if ctx == nil {
		return nil, false
	}
	mm, ok := ctx.Value(ctxMetaKey{}).(metaMap)
	return mm, ok
}

func (c *ConnectionManager) LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error) {
	if ctx == nil {
		return false, nil
	}
	mm, ok := c.lookupMetaMap(ctx)
	if !ok {
		return false, nil
	}
	bs, ok := mm[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(bs, ptr)
}
