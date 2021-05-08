package postgresql

import (
	"context"
	"database/sql"
	"fmt"
)

type DefaultPool struct {
	DSN string

	connection *sql.DB
}

func (c *DefaultPool) GetDSN() string {
	return c.DSN
}

func (c *DefaultPool) GetClient(ctx context.Context) (SQLClient, func(), error) {
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

func (c *DefaultPool) BeginTx(ctx context.Context) (context.Context, error) {
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

func (c *DefaultPool) CommitTx(ctx context.Context) error {
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

func (c *DefaultPool) RollbackTx(ctx context.Context) error {
	tx, ok := c.lookupTx(ctx)
	if !ok {
		return fmt.Errorf(`no postgres tx in the given context`)
	}

	return tx.Rollback()
}

func (c *DefaultPool) LookupTx(ctx context.Context) (SQLClient, bool) {
	return c.lookupTx(ctx)
}

func (c *DefaultPool) lookupTx(ctx context.Context) (*ctxDefaultPoolTxValue, bool) {
	tx, ok := ctx.Value(ctxDefaultPoolTxKey{}).(*ctxDefaultPoolTxValue)
	return tx, ok
}

func (c *DefaultPool) setConnection() error {
	db, err := sql.Open(`postgres`, c.DSN)
	if err != nil {
		return err
	}
	c.connection = db
	return nil
}

func (c *DefaultPool) getConnection() (*sql.DB, error) {
	if c.connection == nil {
		if err := c.setConnection(); err != nil {
			return nil, err
		}
	}
	if err := c.connection.Ping(); err != nil {
		if err := c.setConnection(); err != nil {
			return nil, err
		}
	}
	return c.connection, nil
}
