package postgresql

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
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

var _ guard.Unlocker = (*Lock)(nil)
var _ guard.Locker = (*Lock)(nil)
var _ guard.NonBlockingLocker = (*Lock)(nil)

func (l *Lock) init() error {
	_, err := synckit.InitErr(&l.m, &l.owner, uuid.MakeV4)
	return err
}

const defaultExpiration = 30 * time.Second

const queryUnlock = `DELETE FROM "frameless_locks" WHERE "id" = $1`

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
	first, err := l.firstInLine(ctx)
	if err != nil {
		return nil, false, err
	}
	if first == nil || !first.ID.Equal(rec.ID) { // if we are not the first in line, we bail
		return nil, false, l.deleteLock(ctx, rec)
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
	for {
		first, err := l.firstInLine(ctx)
		if err != nil {
			return nil, err
		}
		if first != nil && first.ID.Equal(rec.ID) {
			break // we own it
		}
		select {
		case <-ctx.Done():
			if err := l.deleteLock(ctx, rec); err != nil {
				return nil, err
			}
			return nil, ctx.Err()
		case <-clock.After(min(time.Second/2, l.getExpiration()/3)):
		}
		// while waiting in the queue, our record must not expire,
		// else autoUnlock removes it, and we would take the lock
		// without being first in line, next to the current holder.
		rec, err = l.keepQueued(ctx, rec)
		if err != nil {
			return nil, err
		}
	}
	return l.lockContext(ctx, rec), nil
}

// firstInLine returns the first non-expired lock record in the queue, or nil if the queue is empty.
func (l *Lock) firstInLine(ctx context.Context) (*lockRecord, error) {
	for lr, err := range l.getLocks(ctx) {
		return lr, err
	}
	return nil, nil
}

// keepQueued extends the expiry of a queued lock record.
// If the record is already gone, it re-enqueues with a new record at the end of the line.
func (l *Lock) keepQueued(ctx context.Context, rec *lockRecord) (*lockRecord, error) {
	var query = fmt.Sprintf(`UPDATE %s SET "expires" = $2 WHERE "id" = $1`, l.tableName())
	nextExpires := l.getExpiresAt()
	res, err := l.db().Exec(ctx, query, rec.ID, nextExpires)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return l.insLock(ctx)
	}
	rec.expiresMu.Lock()
	rec.Expires = nextExpires
	rec.expiresMu.Unlock()
	return rec, nil
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

	Name string

	// expiresMu guards the mutable Expires field which is updated by the
	// keepAlive goroutine and read from the test goroutine when checking
	// re-entrant locks. Without it, the race detector flags a data race
	// and a racing read can observe a torn/zero value, which can cause
	// re-entrant Lock calls to take the slow path and hang the test.
	expiresMu sync.Mutex
	Expires   time.Time
}

func (l *lockRecord) isExpired() bool {
	if l == nil {
		return true
	}
	l.expiresMu.Lock()
	expires := l.Expires
	l.expiresMu.Unlock()
	if expires.IsZero() {
		return true
	}
	return !expires.After(clock.Now())
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

func (l *Lock) getLocks(ctx context.Context) iter.Seq2[*lockRecord, error] {
	return func(yield func(*lockRecord, error) bool) {
		if err := l.init(); err != nil {
			yield(nil, err)
			return
		}
		if err := l.autoUnlock(ctx); err != nil {
			yield(nil, err)
			return
		}
		var queryString = fmt.Sprintf(`SELECT "id", "owner", "expires" FROM %s WHERE name = $1 ORDER BY id ASC`, l.tableName())
		var queryMany = flsql.QueryMany(l.Connection, ctx, func(s flsql.Scanner) (*lockRecord, error) {
			var rec lockRecord
			rec.Name = l.Name
			err := s.Scan(&rec.ID, &rec.Owner, &rec.Expires)
			return &rec, err
		}, queryString, l.Name)
		var now = clock.Now()
		queryMany = iterkit.Filter(queryMany, func(l *lockRecord) bool {
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
	var (
		query   = fmt.Sprintf(`UPDATE %s SET "expires" = $2 WHERE "id" = $1`, l.tableName())
		lastErr error
	)
	for ctx.Err() == nil && !rec.isExpired() {
		rec.expiresMu.Lock()
		deadline := rec.Expires
		rec.expiresMu.Unlock()
		// the next expiry is computed per attempt, so time spent waiting for a connection isn't lost from the lease
		nextExpires := l.getExpiresAt()
		attemptCtx, cancel := context.WithDeadline(ctx, deadline)
		res, err := l.db().Exec(attemptCtx, query, rec.ID, nextExpires)
		cancel()
		if err == nil {
			if res.RowsAffected() == 0 {
				return fmt.Errorf("%w: lock record no longer exists", ErrLockLost)
			}
			rec.expiresMu.Lock()
			rec.Expires = nextExpires
			rec.expiresMu.Unlock()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
		case <-clock.After(l.getExpiration() / 20):
		}
	}
	if ctx.Err() != nil {
		return nil // regular unlock or parent context cancellation
	}
	return fmt.Errorf("%w: lock refresh failed before expiry: %v", ErrLockLost, lastErr)
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
	return clock.Now().Add(l.getExpiration())
}

func (l *Lock) getExpiration() time.Duration {
	if l.Expiration != 0 {
		return l.Expiration
	}
	return defaultExpiration
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

	cancel func(error)
	ctx    context.Context
	// keepAlive refreshes the lock record's expiry while the lock is held.
	keepAlive synckit.Job

	onUnlock  sync.Once
	unlockErr error
}

func (lck *lockContext) Unlock(ctx context.Context) error {
	if err := lck.Lock.init(); err != nil {
		return err
	}
	lck.onUnlock.Do(func() {
		// stop refreshing before the record is removed,
		// else a refresh racing with the removal would report our own unlock as a lost lock.
		lck.keepAlive.Cancel()
		_ = lck.keepAlive.Wait()
		// delete only this acquisition's record; a late Unlock of an earlier lock context must not release a newer lock
		_, lck.unlockErr = lck.Lock.Connection.DB.Exec(context.WithoutCancel(ctx), queryUnlock, lck.Record.ID)
		if lck.unlockErr == nil {
			if cause := context.Cause(lck.ctx); errors.Is(cause, ErrLockLost) {
				lck.unlockErr = cause
			} else {
				lck.unlockErr = ctx.Err()
			}
		}
		lck.cancel(nil)
	})
	return lck.unlockErr
}

func (l *Lock) lockContext(ctx context.Context, lr *lockRecord) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	lck := &lockContext{
		ctx:    ctx,
		cancel: cancel,
		Lock:   l,
		Record: lr,
	}
	lck.keepAlive = synckit.Go(ctx, func(ctx context.Context) error {
		ticker := clock.NewTicker(l.getExpiration() / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := l.refreshLock(ctx, lr); err != nil {
					cancel(err)
					return err
				}
			}
		}
	})
	context.AfterFunc(ctx, func() {
		lck.keepAlive.Cancel()
		_ = lck.keepAlive.Wait()
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

type LockerFactory[K any] struct {
	Connection Connection
	// Expiration is passed to the issued Lock values.
	//
	// Default: 30s
	Expiration time.Duration
}

func (lf LockerFactory[K]) Migrate(ctx context.Context) error {
	return (&Lock{Connection: lf.Connection}).Migrate(ctx)
}

func (lf LockerFactory[K]) LockerFor(key K) *Lock {
	return &Lock{Name: lf.nameFor(key), Connection: lf.Connection, Expiration: lf.Expiration}
}

func (lf LockerFactory[K]) NonBlockingLockerFor(key K) guard.NonBlockingLocker {
	return lf.LockerFor(key)
}

// ErrLockLost is an alias of guard.ErrLockLost.
const ErrLockLost = guard.ErrLockLost

func (lf LockerFactory[K]) nameFor(key K) string {
	switch key := any(key).(type) {
	case fmt.Stringer:
		return key.String()
	case string:
		return key
	default:
		// Numeric and other primitive keys must be rendered through fmt.Sprint
		// so values that exceed the Unicode range (e.g. large random ints) do
		// not silently collapse to U+FFFD. Using reflect.ValueOf(key).Convert(stringType)
		// would interpret the int as a rune and produce the same name for every
		// out-of-range key, breaking lock isolation.
		return fmt.Sprint(key)
	}
}
