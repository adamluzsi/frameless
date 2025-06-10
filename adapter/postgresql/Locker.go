package postgresql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
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

func (l Locker) TryLock(ctx context.Context) (_ context.Context, _ bool, rerr error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("missing context.Context")
	}

	if _, ok := lockctx.Lookup(ctx); ok {
		return ctx, true, nil
	}

	tx, err := l.beginLockTx(ctx)
	if err != nil {
		return nil, false, err
	}
	defer errorkit.FinishOnError(&rerr,
		func() { tx.Rollback(ctx) })

	_, err = tx.Exec(ctx, `SET LOCAL lock_timeout = 1;`)
	if err != nil {
		return nil, false, err
	}

	_, err = tx.Exec(ctx, queryLock, l.Name)
	if err != nil {
		const LockTimeoutErrorCode = "55P03"
		if pgErr, ok := errorkit.As[*pgconn.PgError](err); ok && pgErr.Code == LockTimeoutErrorCode {
			_ = tx.Rollback(ctx)
			return nil, false, nil
		}
		return nil, false, err
	}

	return l.lockContext(ctx, tx), true, nil
}

func (l Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("missing context.Context")
	}

	if _, ok := lockctx.Lookup(ctx); ok {
		return ctx, nil
	}

	tx, err := l.beginLockTx(ctx)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, queryLock, l.Name)
	if err != nil {
		return nil, err
	}

	return l.lockContext(ctx, tx), nil
}

func (l Locker) Unlock(ctx context.Context) error {
	if ctx == nil {
		return guard.ErrNoLock
	}
	lck, ok := lockctx.Lookup(ctx)
	if !ok {
		return guard.ErrNoLock
	}
	return lck.Unclock(ctx)
}

var lockctx contextkit.ValueHandler[ctxKeyLock, *lockerContext]

type ctxKeyLock struct{}

type lockerContext struct {
	onUnlock sync.Once
	tx       pgx.Tx

	Connection Connection
	cancel     func()
	ctx        context.Context
}

func (lck *lockerContext) Unclock(ctx context.Context) (rerr error) {
	lck.onUnlock.Do(func() {
		if err := lck.tx.Rollback(lck.ctx); err != nil {
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

func (l Locker) lockContext(ctx context.Context, tx pgx.Tx) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	lck := &lockerContext{
		ctx:        ctx,
		cancel:     cancel,
		Connection: l.Connection,
		tx:         tx,
	}
	context.AfterFunc(ctx, func() {
		_ = lck.Unclock(ctx)
	})
	return lockctx.ContextWith(ctx, lck)
}

func (l Locker) beginLockTx(ctx context.Context) (pgx.Tx, error) {
	return l.Connection.DB.Begin(ctx)
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
	return MakeMigrator(l.Connection, "frameless_locker_locks", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{UpQuery: queryCreateLockerTable},
		"2": flsql.MigrationStep[Connection]{UpQuery: queryRenameLockerTable},
	}).Migrate(ctx)
}

type LockerFactory[Key comparable] struct{ Connection Connection }

func (lf LockerFactory[Key]) Migrate(ctx context.Context) error {
	return Locker{Connection: lf.Connection}.Migrate(ctx)
}

func (lf LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	return Locker{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}

func (lf LockerFactory[Key]) NonBlockingLockerFor(key Key) guard.NonBlockingLocker {
	return Locker{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}
