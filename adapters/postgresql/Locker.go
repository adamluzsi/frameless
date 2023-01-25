package postgresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/adamluzsi/frameless/ports/locks"
)

// Locker is a PG-based shared mutex implementation.
// It depends on the existence of the frameless_postgres_locker_locks table.
// Locker is safe to call from different application instances,
// ensuring that only one of them can hold the lock concurrently.
type Locker struct {
	Name string
	DB   *sql.DB
}

const queryLock = `INSERT INTO frameless_postgres_locker_locks (name) VALUES ($1);`

func (l Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("missing context.Context")
	}

	if _, ok := l.lookup(ctx); ok {
		return ctx, nil
	}

	tx, err := l.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, queryLock, l.Name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, lockerCtxKey{}, &lockerCtxValue{
		tx:     tx,
		cancel: cancel,
	}), nil
}

func (l Locker) Unlock(ctx context.Context) error {
	if ctx == nil {
		return locks.ErrNoLock
	}
	lck, ok := l.lookup(ctx)
	if !ok {
		return locks.ErrNoLock
	}
	if lck.done {
		return nil
	}
	if err := lck.tx.Rollback(); err != nil {
		if driver.ErrBadConn == err && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	lck.done = true
	err := ctx.Err()
	lck.cancel()
	return err
}

type (
	lockerCtxKey   struct{}
	lockerCtxValue struct {
		tx     *sql.Tx
		done   bool
		cancel func()
	}
)

const queryCreateLockerTable = `
CREATE TABLE IF NOT EXISTS frameless_postgres_locker_locks (
    name TEXT PRIMARY KEY
);
`

var lockerMigrationConfig = MigratorConfig{
	Namespace: "frameless/postgresql.Locker",
	Steps: []MigratorStep{
		MigrationStep{UpQuery: queryCreateLockerTable},
	},
}

func (l Locker) Migrate(ctx context.Context) error {
	return Migrator{DB: l.DB, Config: lockerMigrationConfig}.Up(ctx)
}

func (l Locker) lookup(ctx context.Context) (*lockerCtxValue, bool) {
	v, ok := ctx.Value(lockerCtxKey{}).(*lockerCtxValue)
	return v, ok
}
