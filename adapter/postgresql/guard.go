package postgresql

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/resilience"
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
	_, err := synckit.InitErr(&l.m, &l.owner, uuid.MakeV4)
	return err
}

const defaultExpiration = 30 * time.Second

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
	var rec, err = l.insLock(ctx)
	if err != nil {
		return nil, false, err
	}
	for lr, err := range l.getLocks(ctx) {
		if err != nil {
			return nil, false, err
		}
		if !lr.ID.Equal(rec.ID) { // if we are not the first in line, we bail
			return nil, false, l.deleteLock(ctx, rec)
		}
		break // we own it
	}
	return l.lockContext(ctx, rec), true, nil
}

func (l *Lock) Lock(ctx context.Context) (_ context.Context, rerr error) {
	if err := l.init(); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fmt.Errorf("missing context.Context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.isLockedAlready(ctx) {
		return ctx, nil
	}
	var rec, err = l.insLock(ctx)
	if err != nil {
		return nil, err
	}
waitingInQueue:
	for {
		for lr, err := range l.getLocks(ctx) {
			if err != nil {
				return nil, err
			}
			if !lr.ID.Equal(rec.ID) { // if we are not the first in line, we bail
				select {
				case <-ctx.Done():
					if err := l.deleteLock(ctx, rec); err != nil {
						return nil, err
					}
					return nil, ctx.Err()
				case <-clock.After(time.Second / 2):
					goto waitingInQueue
				}
			}
			break // we own it
		}
		break // all done we own it
	}
	return l.lockContext(ctx, rec), nil
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

type lockRecord struct {
	ID    uuid.UUID `uuid:"v7"`
	Owner uuid.UUID

	Name    string
	Expires time.Time
}

func (l *lockRecord) isExpired() bool {
	if l == nil {
		return true
	}
	if l.Expires.IsZero() {
		return true
	}
	return !l.Expires.After(clock.Now())
}

func (l *Lock) insLock(ctx context.Context) (*lockRecord, error) {
	if err := l.init(); err != nil {
		return nil, err
	}
	var id, err = uuid.MakeV7()
	if err != nil {
		return nil, fmt.Errorf("failed to create UUID V7")
	}
	var rec = &lockRecord{
		ID:      id,
		Owner:   l.owner,
		Name:    l.Name,
		Expires: l.getExpiresAt(),
	}
	var query = fmt.Sprintf(`INSERT INTO %s ("id", "owner", "name", "expires") VALUES ($1, $2, $3, $4)`, l.tableName())
	res, err := l.db().Exec(ctx, query, rec.ID, rec.Owner, rec.Name, rec.Expires)
	if err != nil {
		return nil, err
	}
	if n := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("failed to insert lock record")
	}
	return rec, err
}

func (l *Lock) getLocks(ctx context.Context) iter.Seq2[lockRecord, error] {
	return func(yield func(lockRecord, error) bool) {
		if err := l.init(); err != nil {
			var zero lockRecord
			yield(zero, err)
			return
		}
		if err := l.autoUnlock(ctx); err != nil {
			var zero lockRecord
			yield(zero, err)
			return
		}
		var queryString = fmt.Sprintf(`SELECT "id", "owner", "expires" FROM %s WHERE name = $1 ORDER BY id ASC`, l.tableName())
		var queryMany = flsql.QueryMany(l.Connection, ctx, func(s flsql.Scanner) (lockRecord, error) {
			var rec lockRecord
			rec.Name = l.Name
			err := s.Scan(&rec.ID, &rec.Owner, &rec.Expires)
			return rec, err
		}, queryString, l.Name)
		var now = clock.Now()
		queryMany = iterkit.Filter(queryMany, func(l lockRecord) bool {
			return l.Expires.After(now)
		})
		for rec, err := range queryMany {
			if !yield(rec, err) {
				return
			}
		}
	}
}

func (l *Lock) refreshLock(ctx context.Context, rec *lockRecord) error {
	if rec.isExpired() {
		return fmt.Errorf("already expired")
	}
	var (
		nextExpires = l.getExpiresAt()
		remaining   = rec.Expires.Sub(clock.Now())
		retry       = resilience.Waiter{Timeout: remaining}
		db          = l.db()
		err         error
	)
	var query = fmt.Sprintf(`UPDATE %s SET "expires" = $2 WHERE "id" = $1`, l.tableName())
	for range resilience.Retries(ctx, retry) {
		err = db.Ping(ctx)
		if err != nil {
			continue
		}
		_, err = l.db().Exec(ctx, query, rec.ID, nextExpires)
		if err != nil {
			return err
		}
		rec.Expires = nextExpires
	}
	return err
}

func (l *Lock) deleteLock(ctx context.Context, rec *lockRecord) error {
	if rec == nil {
		return errorkit.F("nil %T received", rec)
	}
	var query = fmt.Sprintf(`DELETE FROM %s WHERE "id" = $1`, l.tableName())
	_, err := l.db().Exec(context.WithoutCancel(ctx), query, rec.ID)
	// if error occurs, autoUnlock will clean up our mess after the lock is already expired
	return err
}

func (l *Lock) autoUnlock(ctx context.Context) error {
	var query = fmt.Sprintf(`DELETE FROM %s WHERE "expires" < $1`, l.tableName())
	res, err := l.db().Exec(ctx, query, clock.Now())
	if err != nil {
		return err
	}
	logger.Debug(ctx, l.tableName()+" auto unlock", logging.LazyDetail(func() logging.Detail {
		return logging.Field("removed", res.RowsAffected())
	}))
	return nil
}

func (l *Lock) db() *pgxpool.Pool {
	return l.Connection.DB
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
	lc, ok := ctx.Value(ctxKeyLock{Name: l.Name}).(*lockContext)
	if !ok {
		return false
	}
	if lc.Record == nil {
		return false
	}
	if lc.Record.isExpired() {
		return false
	}
	return true
}

type ctxKeyLock struct{ Name string }

type lockContext struct {
	Lock   *Lock
	Record *lockRecord

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
		if lck.unlockErr == nil {
			lck.unlockErr = ctx.Err()
		}
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

func (l *Lock) lockContext(ctx context.Context, lr *lockRecord) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	lck := &lockContext{
		ctx:    ctx,
		cancel: cancel,
		Lock:   l,
		Record: lr,
	}
	keepAlive := synckit.Go(ctx, func(ctx context.Context) error {
		for ctx.Err() == nil {
			if err := l.refreshLock(ctx, lr); err != nil {
				cancel()
				return err
			}
		}
		return nil
	})
	context.AfterFunc(ctx, func() {
		keepAlive.Cancel()
		_ = keepAlive.Wait()
		_ = lck.Unlock(ctx)
	})
	return context.WithValue(ctx, ctxKeyLock{Name: l.Name}, lck)
}

func (l *Lock) tableName() string {
	const name = "frameless_locks"
	return pgx.Identifier{name}.Sanitize()
}

func (l *Lock) Migrate(ctx context.Context) error {
	if err := l.legacyMigrate(ctx); err != nil {
		return err
	}
	var tableName = l.tableName()
	return MakeMigrator(l.Connection, "frameless_locks", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{
			UpQuery: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (`, tableName) +
				`id uuid PRIMARY KEY,` + // UUID v7
				`name TEXT,` +
				`owner text,` +
				`expires TIMESTAMPTZ NOT NULL` +
				`)`,
			DownQuery: fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tableName),
		},
	}).Migrate(ctx)
}

func (l *Lock) legacyMigrate(ctx context.Context) error {
	return MakeMigrator(l.Connection, "frameless_locker_locks", migration.Steps[Connection]{
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
}

type LockerFactory[Key any] struct{ Connection Connection }

func (lf LockerFactory[Key]) Migrate(ctx context.Context) error {
	return (&Lock{Connection: lf.Connection}).Migrate(ctx)
}

func (lf LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	return &Lock{Name: lf.nameFor(key), Connection: lf.Connection}
}

func (lf LockerFactory[Key]) NonBlockingLockerFor(key Key) guard.NonBlockingLocker {
	return &Lock{Name: fmt.Sprintf("%T:%v", key, key), Connection: lf.Connection}
}

const ErrLockLost errorkit.Error = "ErrLockLost"

var stringType = reflect.TypeFor[string]()

func (lf LockerFactory[Key]) nameFor(key Key) string {
	switch key := any(key).(type) {
	case fmt.Stringer:
		return key.String()
	case string:
		return key
	default:
		if reflect.TypeFor[T]().ConvertibleTo(stringType) {
			return reflect.ValueOf(key).Convert(stringType).Interface().(string)
		}
		return fmt.Sprintf("%v", key)
	}
}
