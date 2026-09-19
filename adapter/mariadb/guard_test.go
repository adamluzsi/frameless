package mariadb_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/mariadb"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestLockerFactory(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) mariadb.LockerFactory[string] {
		factory := mariadb.LockerFactory[string]{Connection: GetConnection(t)}
		assert.Must(t).NoError(factory.Migrate(context.Background()))
		return factory
	})

	var (
		ctx = let.Var(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			return ctx
		})
		outerKey = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		innerKey = let.Var(s, func(t *testcase.T) string {
			return random.Unique(t.Random.UUID, outerKey.Get(t))
		})
		outerCtx = let.Var(s, func(t *testcase.T) context.Context {
			locker := subject.Get(t).LockerFor(outerKey.Get(t))
			lockedCtx, err := locker.Lock(ctx.Get(t))
			assert.Must(t).NoError(err)
			t.Cleanup(func() { _ = locker.Unlock(lockedCtx) })
			return lockedCtx
		})
	)

	s.Describe("#TryLock", func(s *testcase.Spec) {
		act := func(t *testcase.T) (context.Context, bool, error) {
			locker := subject.Get(t).NonBlockingLockerFor(innerKey.Get(t))
			lockedCtx, acquired, err := locker.TryLock(outerCtx.Get(t))
			if acquired {
				t.Cleanup(func() { _ = locker.Unlock(lockedCtx) })
			}
			return lockedCtx, acquired, err
		}

		s.Then("acquires and releases the inner lock without releasing or cancelling the outer lock", func(t *testcase.T) {
			innerCtx, acquired, err := act(t)
			assert.NoError(t, err)
			assert.True(t, acquired)
			assert.NotNil(t, innerCtx)

			locker := subject.Get(t).NonBlockingLockerFor(innerKey.Get(t))
			contenderCtx, acquired, err := locker.TryLock(ctx.Get(t))
			if acquired {
				t.Cleanup(func() { _ = locker.Unlock(contenderCtx) })
			}
			assert.NoError(t, err)
			assert.False(t, acquired, "the inner lock must be held independently of the outer lock")

			assert.NoError(t, locker.Unlock(innerCtx))
			assert.ErrorIs(t, innerCtx.Err(), context.Canceled)
			assert.NoError(t, outerCtx.Get(t).Err())

			outerLocker := subject.Get(t).NonBlockingLockerFor(outerKey.Get(t))
			contenderCtx, acquired, err = outerLocker.TryLock(ctx.Get(t))
			if acquired {
				t.Cleanup(func() { _ = outerLocker.Unlock(contenderCtx) })
			}
			assert.NoError(t, err)
			assert.False(t, acquired, "the outer lock must remain held after the inner lock is released")
		})

		s.When("the inner lock is already held outside the outer context", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				locker := subject.Get(t).LockerFor(innerKey.Get(t))
				lockedCtx, err := locker.Lock(ctx.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = locker.Unlock(lockedCtx) })
			})

			s.Then("does not report ownership of the unrelated lock", func(t *testcase.T) {
				lockedCtx, acquired, err := act(t)
				assert.NoError(t, err)
				assert.False(t, acquired)
				assert.Nil(t, lockedCtx)
				assert.NoError(t, outerCtx.Get(t).Err())
			})
		})
	})

	s.Describe("#Unlock", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).LockerFor(innerKey.Get(t)).Unlock(outerCtx.Get(t))
		}

		s.Then("rejects another lock's context without releasing or cancelling it", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), guard.ErrNoLock)
			assert.NoError(t, outerCtx.Get(t).Err())
		})
	})
}
