package postgresql

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/testcase/clock"
)

// Lock is a PG-based shared mutex implementation.
// It depends on the existence of the frameless_locker_locks table.
// Lock is safe to call from different application instances,
// ensuring that only one of them can hold the lock concurrently.
type Lock struct {
	Name       string
	Connection Connection
	// Expiration is the time duration in which the lock expires
	// if the control of the is lost for an unexpected reason.
	//
	// Default: 30s
	Expiration time.Duration

	owner uuid.UUID

	m sync.RWMutex
}

func (l *Lock) init() error {
	if _, err := synckit.InitErr(&l.m, &l.owner, uuid.MakeV4); err != nil {
		return err
	}
	return nil
}

const defaultExpiration = 30 * time.Second

const queryLock = `INSERT INTO "frameless_locks" ("name", "owner", "expires") VALUES ($1, $2, $3)`
const queryUnlock = `DELETE FROM "frameless_locks" WHERE "name" = $1 AND "owner" = $2`

func (l *Lock) TryLock(ctx context.Context) (_ context.Context, _ bool, rerr error) {
	if err := l.init(); err != nil {
		return nil, false, err
	}
	if ctx == nil {
		return nil, false, fmt.Errorf("missing context.Context")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if l.isLockedAlready(ctx) {
		return ctx, true, nil
	}
	var expiresAt = l.getExpiresAt()
	var _, err = l.Connection.DB.Exec(ctx, queryLock, l.Name, l.owner, expiresAt)
	if isErrAlreadyExists(err) {
		return nil, false, nil
	}
	return l.lockContext(ctx), true, nil
}

func isErrAlreadyExists(err error) bool {
	return false
}

func (l *Lock) Lock(ctx context.Context) (context.Context, error) {
	if err := l.init(); err != nil {
		return nil, err
	}
	for {
		lockContext, acquired, err := l.TryLock(ctx)
		if err != nil {
			return ctx, err
		}
		if !acquired { // maybe use notify/subscribe instead of sleep
			runtime.Gosched()
			clock.Sleep(time.Millisecond)
			continue
		}
		return lockContext, nil
	}
}

func (l *Lock) Unlock(ctx context.Context) error {
	if err := l.init(); err != nil {
		return err
	}
	if ctx == nil {
		return guard.ErrNoLock
	}
	lck, ok := ctx.Value(ctxKeyLock{Name: l.Name}).(*lockContext)
	if !ok {
		return guard.ErrNoLock
	}
	return lck.Unlock(ctx)
}

func (l *Lock) getExpiresAt() time.Time {
	var now = clock.Now()
	if l.Expiration != 0 {
		return now.Add(l.Expiration)
	}
	return now.Add(defaultExpiration)
}

func (l *Lock) isLockedAlready(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(ctxKeyLock{Name: l.Name}).(*lockContext)
	return ok
}

type ctxKeyLock struct{ Name string }

type lockContext struct {
	Lock *Lock

	cancel func()
	ctx    context.Context

	onUnlock  sync.Once
	unlockErr error
}

func (lck *lockContext) Unlock(ctx context.Context) error {
	if err := lck.Lock.init(); err != nil {
		return err
	}
	lck.onUnlock.Do(func() {
		_, lck.unlockErr = lck.Lock.Connection.DB.Exec(context.WithoutCancel(ctx), queryUnlock, lck.Lock.Name, lck.Lock.owner.String())
		// if err := lck.tx.Rollback(lck.ctx); err != nil {
		// 	if driver.ErrBadConn == err && ctx.Err() != nil {
		// 		lck.unlockErr = ctx.Err()
		// 		return
		// 	}
		// 	lck.unlockErr = err
		// 	return
		// }
		lck.cancel()
	})
	return lck.unlockErr
}

func (l *Lock) lockContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	lck := &lockContext{
		ctx:    ctx,
		cancel: cancel,
		Lock:   l,
	}
	context.AfterFunc(ctx, func() {
		_ = lck.Unlock(ctx)
	})
	return context.WithValue(ctx, ctxKeyLock{Name: l.Name}, lck)
}

func (l *Lock) Migrate(ctx context.Context) error {
	err := MakeMigrator(l.Connection, "frameless_locker_locks", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_locker_locks ( name TEXT PRIMARY KEY );",
			DownQuery: `DROP TABLE IF EXISTS frameless_locker_locks`,
		},
		"2": flsql.MigrationStep[Connection]{
			UpQuery: `ALTER TABLE "frameless_locker_locks" RENAME TO "frameless_guard_locks";` + "\n" +
				`CREATE VIEW "frameless_locker_locks" AS SELECT * FROM "frameless_guard_locks";`,
			DownQuery: `DROP VIEW IF EXISTS "frameless_locker_locks";` + "\n" +
				`ALTER TABLE "frameless_guard_locks" RENAME TO "frameless_locker_locks";`,
		},
	}).MigrateDown(ctx, "")
	if err != nil {
		return err
	}
	return MakeMigrator(l.Connection, "frameless_locks", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{
			UpQuery:   `CREATE TABLE IF NOT EXISTS "frameless_locks" ( name TEXT PRIMARY KEY, owner text, expires TIMESTAMPTZ NOT NULL )`,
			DownQuery: `DROP TABLE IF EXISTS "frameless_locks"`,
		},
	}).Migrate(ctx)
}

type LockerFactory[Key comparable] struct{ Connection Connection }

func (lf LockerFactory[Key]) Migrate(ctx context.Context) error {
	return (&Lock{Connection: lf.Connection}).Migrate(ctx)
}

func (lf LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	return &Lock{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}

func (lf LockerFactory[Key]) NonBlockingLockerFor(key Key) guard.NonBlockingLocker {
	return &Lock{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}

const ErrLockLost errorkit.Error = "ErrLockLost"
