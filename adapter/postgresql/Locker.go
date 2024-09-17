package postgresql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/migration"
)

// Locker is a PG-based shared mutex implementation.
// It depends on the existence of the frameless_locker_locks table.
// Locker is safe to call from different application instances,
// ensuring that only one of them can hold the lock concurrently.
type Locker struct {
	Name       string
	Connection Connection
}

const queryLock = `INSERT INTO frameless_locker_locks (name) VALUES ($1);`

func (l Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("missing context.Context")
	}

	if _, ok := l.lookup(ctx); ok {
		return ctx, nil
	}

	ctx, err := l.Connection.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.Connection.ExecContext(ctx, queryLock, l.Name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	lck := &lockerCtxValue{
		ctx:        ctx,
		cancel:     cancel,
		Connection: l.Connection,
	}
	context.AfterFunc(ctx, func() {
		_ = lck.Unclock(ctx)
	})
	return context.WithValue(ctx, lockerCtxKey{}, lck), nil
}

func (l Locker) Unlock(ctx context.Context) error {
	if ctx == nil {
		return guard.ErrNoLock
	}
	lck, ok := l.lookup(ctx)
	if !ok {
		return guard.ErrNoLock
	}
	return lck.Unclock(ctx)
}

type (
	lockerCtxKey   struct{}
	lockerCtxValue struct {
		onUnlock   sync.Once
		Connection Connection
		done       bool
		cancel     func()
		ctx        context.Context
	}
)

func (lck *lockerCtxValue) Unclock(ctx context.Context) (rerr error) {
	lck.onUnlock.Do(func() {
		if err := lck.Connection.RollbackTx(lck.ctx); err != nil {
			if driver.ErrBadConn == err && ctx.Err() != nil {
				rerr = ctx.Err()
				return
			}
			rerr = err
			return
		}
		lck.cancel()
	})
	return rerr
}

const queryCreateLockerTable = `
CREATE TABLE IF NOT EXISTS frameless_locker_locks (
    name TEXT PRIMARY KEY
);
`

const queryRenameLockerTable = `
ALTER TABLE "frameless_locker_locks" RENAME TO "frameless_guard_locks";
CREATE VIEW "frameless_locker_locks" AS SELECT * FROM "frameless_guard_locks";
`

func (l Locker) Migrate(ctx context.Context) error {
	return makeMigrator(l.Connection, "frameless_locker_locks", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{UpQuery: queryCreateLockerTable},
		"2": flsql.MigrationStep[Connection]{UpQuery: queryRenameLockerTable},
	}).Migrate(ctx)
}

func (l Locker) lookup(ctx context.Context) (*lockerCtxValue, bool) {
	v, ok := ctx.Value(lockerCtxKey{}).(*lockerCtxValue)
	return v, ok
}

type LockerFactory[Key comparable] struct{ Connection Connection }

func (lf LockerFactory[Key]) Migrate(ctx context.Context) error {
	return Locker{Connection: lf.Connection}.Migrate(ctx)
}

func (lf LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	return Locker{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}
