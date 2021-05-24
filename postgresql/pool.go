package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

type SinglePool struct {
	DSN string

	mutex      sync.Mutex
	connection *sql.DB
}

func (c *SinglePool) GetDSN() string {
	return c.DSN
}

func (c *SinglePool) GetClient(ctx context.Context) (SQLClient, func(), error) {
	free := func() {} // free is not used because this is a single Pool
	if tx, ok := c.lookupTx(ctx); ok {
		return tx, free, nil
	}

	client, err := c.getConnection()
	if err != nil {
		return nil, nil, err
	}

	return client, free, nil
}

type (
	ctxDefaultPoolTxKey   struct{}
	ctxDefaultPoolTxValue struct {
		depth int
		*sql.Tx
	}
)

func (c *SinglePool) BeginTx(ctx context.Context) (context.Context, error) {
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

func (c *SinglePool) CommitTx(ctx context.Context) error {
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

func (c *SinglePool) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgres tx in the given context`)
	}

	return tx.Rollback()
}

func (c *SinglePool) LookupTx(ctx context.Context) (SQLClient, bool) {
	return c.lookupTx(ctx)
}

func (c *SinglePool) lookupTx(ctx context.Context) (*ctxDefaultPoolTxValue, bool) {
	tx, ok := ctx.Value(ctxDefaultPoolTxKey{}).(*ctxDefaultPoolTxValue)
	return tx, ok
}

func (c *SinglePool) getConnection() (*sql.DB, error) {
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
