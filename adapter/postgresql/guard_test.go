package postgresql_test

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/guard/guardcontract"
	"go.llib.dev/frameless/port/migration"
)

func ExampleLock() {
	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	l := postgresql.Lock{
		Name:       "my-lock",
		Connection: cm,
	}

	ctx, err := l.Lock(context.Background())
	if err != nil {
		panic(err)
	}

	if err := l.Unlock(ctx); err != nil {
		panic(err)
	}
}

var _ migration.Migratable = (*postgresql.Lock)(nil)

func TestLock(t *testing.T) {
	cm := GetConnection(t)

	l := postgresql.Lock{
		Name:       rnd.StringNC(5, random.CharsetAlpha()),
		Connection: cm,
		Expiration: 2 * time.Second,
	}
	assert.NoError(t, l.Migrate(context.Background()))

	conf := guardcontract.LockerConfig{
		MakeLockLost: func(t testing.TB, lockCtx context.Context) {
			// e.g. another instance's autoUnlock or a manual cleanup removes the lock record
			_, err := cm.DB.Exec(context.Background(), `DELETE FROM "frameless_locks" WHERE "name" = $1`, l.Name)
			assert.NoError(t, err)
		},
	}

	testcase.RunSuite(t,
		guardcontract.Locker(&l, conf),
		guardcontract.NonBlockingLocker(&l, conf),
	)
}

func ExampleLockerFactory() {
	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	lockerFactory := postgresql.LockerFactory[string]{Connection: cm}
	if err := lockerFactory.Migrate(context.Background()); err != nil {
		log.Fatal(err)
	}

	locker := lockerFactory.LockerFor("hello world")

	ctx, err := locker.Lock(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if err := locker.Unlock(ctx); err != nil {
		log.Fatal(err)
	}
}

var _ migration.Migratable = postgresql.LockerFactory[int]{}

func TestLockerFactory(t *testing.T) {
	ctx := context.Background()
	cm := GetConnection(t)

	const expiration = 2 * time.Second
	lockerFactoryStrKey := postgresql.LockerFactory[string]{Connection: cm, Expiration: expiration}
	assert.NoError(t, lockerFactoryStrKey.Migrate(ctx))
	guardcontract.LockerFactory[string](lockerFactoryStrKey).Test(t)

	lockerFactoryIntKey := postgresql.LockerFactory[int]{Connection: cm, Expiration: expiration}
	assert.NoError(t, lockerFactoryIntKey.Migrate(ctx))
	guardcontract.LockerFactory[int](lockerFactoryIntKey).Test(t)
}

func TestLock_TryLock_smoke(t *testing.T) {
	const timeout = 3 * time.Second
	c := GetConnection(t)
	l := postgresql.Lock{Name: rnd.Domain(), Connection: c}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, l.Migrate(ctx))

	var lockContext context.Context

	assert.Within(t, timeout, func(context.Context) {
		var acquired bool
		var err error
		lockContext, acquired, err = l.TryLock(ctx)
		assert.NoError(t, err)
		assert.True(t, acquired)
		assert.NotNil(t, lockContext)
		t.Cleanup(func() { l.Unlock(lockContext) })
	})

	assert.NotNil(t, lockContext)

	assert.Within(t, timeout, func(wictx context.Context) {
		lctx, acquired, err := l.TryLock(wictx)
		assert.NoError(t, err)
		assert.False(t, acquired)
		assert.Nil(t, lctx)
	})
}

// func TestLock_lockOwnershipLost(t *testing.T) {
// 	const timeout = 5 * time.Second
// 	ctx := context.Background()
// 	c := GetConnection(t)

// 	l := postgresql.Lock{
// 		Name:       rnd.Domain(),
// 		Connection: c,
// 		Expiration: 100 * time.Millisecond,
// 	}
// 	assert.NoError(t, l.Migrate(ctx))

// 	lockCtx, err := l.Lock(ctx)
// 	assert.NoError(t, err)
// 	t.Cleanup(func() { _ = l.Unlock(lockCtx) })
// 	assert.NoError(t, lockCtx.Err())

// 	// simulate that the connection which holds the lock is lost
// 	pid, ok := postgresql.LockerContextConnectionPID(lockCtx, l.Name)
// 	assert.True(t, ok, "expected that the lock context knows about its connection")
// 	_, err = c.ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid)
// 	assert.NoError(t, err)

// 	assert.Within(t, timeout, func(ctx context.Context) {
// 		select {
// 		case <-lockCtx.Done():
// 		case <-ctx.Done():
// 		}
// 	}, "expected that the lock context is cancelled when the lock's ownership is lost")

// 	assert.Within(t, timeout, func(context.Context) {
// 		assert.ErrorIs(t, postgresql.ErrLockLost, l.Unlock(lockCtx),
// 			"expected that unlocking a lost lock tells that the lock ownership was lost")
// 	})

// 	assert.Within(t, timeout, func(context.Context) {
// 		otherLockCtx, acquired, err := l.TryLock(ctx)
// 		assert.NoError(t, err)
// 		assert.True(t, acquired, "expected that the lost lock is free to be acquired again")
// 		assert.NoError(t, l.Unlock(otherLockCtx))
// 	})
// }

// func TestLock_keepAlive(t *testing.T) {
// 	const timeout = 5 * time.Second
// 	ctx := context.Background()
// 	c := GetConnection(t)

// 	l := postgresql.Lock{
// 		Name:       rnd.Domain(),
// 		Connection: c,
// 		Expiration: 50 * time.Millisecond,
// 	}
// 	assert.NoError(t, l.Migrate(ctx))

// 	lockCtx, err := l.Lock(ctx)
// 	assert.NoError(t, err)
// 	t.Cleanup(func() { _ = l.Unlock(lockCtx) })

// 	// while the connection is healthy, the keep alive checking must not disturb the lock
// 	time.Sleep(500 * time.Millisecond)
// 	assert.NoError(t, lockCtx.Err())

// 	assert.Within(t, timeout, func(ctx context.Context) {
// 		_, acquired, err := l.TryLock(ctx)
// 		assert.NoError(t, err)
// 		assert.False(t, acquired, "expected that the lock is still held")
// 	})

// 	assert.NoError(t, l.Unlock(lockCtx))
// 	assert.Error(t, lockCtx.Err())
// }

// makeLockTestPool creates a dedicated, small pool so the tests can saturate it or trace its queries
// without affecting the shared test connection.
func makeLockTestPool(t *testing.T, maxConns int32, tracer pgx.QueryTracer) (*pgxpool.Pool, postgresql.Connection) {
	cfg, err := pgxpool.ParseConfig(DatabaseDSN(t))
	assert.NoError(t, err)
	cfg.MaxConns = maxConns
	if tracer != nil {
		cfg.ConnConfig.Tracer = tracer
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	assert.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, postgresql.MakeConnectionFromPGXPool(pool)
}

type lockRefreshCounter struct{ n atomic.Int64 }

func (c *lockRefreshCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, `UPDATE "frameless_locks" SET "expires"`) {
		c.n.Add(1)
	}
	return ctx
}

func (c *lockRefreshCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestLock_heldBeyondExpirationWhileThePoolIsBusy(t *testing.T) {
	const (
		maxConns   = 3
		expiration = time.Second
	)
	ctx := context.Background()
	pool, conn := makeLockTestPool(t, maxConns, nil)

	l := postgresql.Lock{Name: rnd.Domain(), Connection: conn, Expiration: expiration}
	assert.NoError(t, l.Migrate(ctx))

	lockCtx, err := l.Lock(ctx)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = l.Unlock(lockCtx) })

	// keep every pool connection busy, like a concurrent bulk load would
	loadCtx, stopLoad := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for range maxConns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for loadCtx.Err() == nil {
				_, _ = pool.Exec(loadCtx, "SELECT pg_sleep(0.2)")
			}
		}()
	}
	t.Cleanup(func() { stopLoad(); wg.Wait() })

	select {
	case <-lockCtx.Done():
	case <-time.After(3 * expiration):
	}

	assert.NoError(t, lockCtx.Err(), "expected that a held lock survives beyond its Expiration while the pool is busy")
	stopLoad()
	wg.Wait()
	assert.NoError(t, l.Unlock(lockCtx))
}

func TestLock_keepAliveRefreshRateIsBounded(t *testing.T) {
	const expiration = time.Second
	ctx := context.Background()
	counter := &lockRefreshCounter{}
	_, conn := makeLockTestPool(t, 3, counter)

	l := postgresql.Lock{Name: rnd.Domain(), Connection: conn, Expiration: expiration}
	assert.NoError(t, l.Migrate(ctx))

	lockCtx, err := l.Lock(ctx)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = l.Unlock(lockCtx) })

	const periods = 2
	time.Sleep(periods * expiration)
	assert.NoError(t, lockCtx.Err())
	assert.NoError(t, l.Unlock(lockCtx))

	assert.True(t, counter.n.Load() < 10*periods,
		assert.MessageF("expected a bounded number of keep-alive refreshes per Expiration, got %d", counter.n.Load()))
}

func TestLock_nestedLocking(t *testing.T) {
	const timeout = 5 * time.Second
	ctx := context.Background()
	c := GetConnection(t)

	var (
		l1 = postgresql.Lock{Name: rnd.Domain(), Connection: c}
		l2 = postgresql.Lock{Name: random.Unique(rnd.Domain, l1.Name), Connection: c}
	)
	assert.NoError(t, l1.Migrate(ctx))

	outerCtx, err := l1.Lock(ctx)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = l1.Unlock(outerCtx) })

	innerCtx, err := l2.Lock(outerCtx)
	assert.NoError(t, err)

	// each Locker must see only its own lock in the lock context
	assert.Within(t, timeout, func(ctx context.Context) {
		_, acquired, err := l2.TryLock(ctx)
		assert.NoError(t, err)
		assert.False(t, acquired,
			"expected that the nested Lock call actually acquired the lock")
	})

	assert.NoError(t, l2.Unlock(innerCtx))
	assert.NoError(t, outerCtx.Err(),
		"unlocking a nested lock context must not cancel its parent lock context")

	assert.Within(t, timeout, func(context.Context) {
		_, acquired, err := l1.TryLock(ctx)
		assert.NoError(t, err)
		assert.False(t, acquired, "expected that the outer lock is still held")
	})

	assert.NoError(t, l1.Unlock(outerCtx))
}

func TestLock_Lock_interruptedByContextCancellation(t *testing.T) {
	const timeout = 5 * time.Second
	ctx := context.Background()
	c := GetConnection(t)

	l := postgresql.Lock{Name: rnd.Domain(), Connection: c}
	assert.NoError(t, l.Migrate(ctx))

	lockCtx, err := l.Lock(ctx)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = l.Unlock(lockCtx) })

	acquiredConns := c.DB.Stat().AcquiredConns()

	waitCtx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	assert.Within(t, timeout, func(context.Context) {
		_, err := l.Lock(waitCtx)
		assert.Error(t, err)
	})

	assert.Eventually(t, timeout, func(t testing.TB) {
		assert.Equal(t, acquiredConns, c.DB.Stat().AcquiredConns(),
			"expected that the interrupted lock acquisition releases its connection")
	})

	assert.NoError(t, l.Unlock(lockCtx))

	assert.Within(t, timeout, func(ctx context.Context) {
		otherLockCtx, err := l.Lock(ctx)
		assert.NoError(t, err, "expected that the interrupted lock acquisition doesn't leak the lock")
		assert.NoError(t, l.Unlock(otherLockCtx))
	})
}
