package postgresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/adamluzsi/frameless/ports/locks"
)

// Locker is a PG-based shared mutex implementation.
// It depends on the existence of the frameless_locker_locks table.
// Locker is safe to call from different application instances,
// ensuring that only one of them can hold the lock concurrently.
type Locker struct {
	Name string
	CM   ConnectionManager
}

const queryLock = `INSERT INTO frameless_locker_locks (name) VALUES ($1);`

func (l Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("missing context.Context")
	}

	if _, ok := l.lookup(ctx); ok {
		return ctx, nil
	}

	ctx, err := l.CM.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	connection, err := l.CM.Connection(ctx)
	if err != nil {
		return nil, err
	}

	_, err = connection.ExecContext(ctx, queryLock, l.Name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, lockerCtxKey{}, &lockerCtxValue{
		ctx:    ctx,
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

	if err := l.CM.RollbackTx(lck.ctx); err != nil {
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
		ctx    context.Context
	}
)

const queryCreateLockerTable = `
CREATE TABLE IF NOT EXISTS frameless_locker_locks (
    name TEXT PRIMARY KEY
);
`

var lockerMigrationConfig = MigratorConfig{
	Namespace: "frameless_locker_locks",
	Steps: []MigratorStep{
		MigrationStep{UpQuery: queryCreateLockerTable},
	},
}

func (l Locker) Migrate(ctx context.Context) error {
	return Migrator{CM: l.CM, Config: lockerMigrationConfig}.Up(ctx)
}

func (l Locker) lookup(ctx context.Context) (*lockerCtxValue, bool) {
	v, ok := ctx.Value(lockerCtxKey{}).(*lockerCtxValue)
	return v, ok
}

type LockerFactory[Key comparable] struct{ CM ConnectionManager }

func (lf LockerFactory[Key]) Migrate(ctx context.Context) error {
	return Locker{CM: lf.CM}.Migrate(ctx)
}

func (lf LockerFactory[Key]) LockerFor(key Key) locks.Locker {
	return Locker{Name: fmt.Sprintf("%T:%v", key, key), CM: lf.CM}
}
