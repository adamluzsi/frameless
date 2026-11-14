package localfs_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRepository_Locks(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		storage = let.Var(s, func(t *testcase.T) repositoryBackend {
			return repositoryBackend{FS: &memory.FileSystem{}, Dir: "entities"}
		})
		locker = let.Var(s, func(t *testcase.T) *repositoryLeaseLocker {
			return &repositoryLeaseLocker{base: memory.NewLocker()}
		})
		fsys = let.Var(s, func(t *testcase.T) *repositoryLockFS {
			return &repositoryLockFS{RepositoryFileSystem: storage.Get(t).fsys()}
		})
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			return &localfs.Repository[testent.Foo, testent.FooID]{FS: fsys.Get(t), Dir: storage.Get(t).Dir, Locker: locker.Get(t)}
		})
		parent = let.With[context.Context](s, lockerTestContext)
		ctx    = let.Var(s, func(t *testcase.T) context.Context { return parent.Get(t) })
		ent    = let.Var(s, func(t *testcase.T) *testent.Foo {
			return &testent.Foo{ID: "a", Foo: t.Random.UUID()}
		})
		tx = let.Var(s, func(t *testcase.T) context.Context {
			r := subject.Get(t)
			tx, err := r.BeginTx(parent.Get(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = r.RollbackTx(tx) })
			return tx
		})
	)

	s.Describe("#Save", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Save(ctx.Get(t), ent.Get(t)) }

		s.Then("releases its ownership after writing", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assertLockerAvailable(t, locker.Get(t))
		})

		s.When("the first transaction operation uses a cancellable child", func(s *testcase.Spec) {
			cancel := let.Var[context.CancelFunc](s, nil)
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, stop := context.WithCancel(tx.Get(t))
				cancel.Set(t, stop)
				t.Cleanup(stop)
				return ctx
			})
			s.Then("keeps ownership after that child is cancelled and until the outer commit", func(t *testcase.T) {
				assert.NoError(t, act(t))
				ownership := locker.Get(t).current()
				cancel.Get(t)()
				assert.NoError(t, ownership.ctx.Err())
				assertLockerBusy(t, locker.Get(t))
				assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
				assertLockerAvailable(t, locker.Get(t))
			})
		})

		s.When("an independent writer holds the lock", func(s *testcase.Spec) {
			owner := let.Var(s, func(t *testcase.T) context.Context {
				l := locker.Get(t)
				ctx, err := l.Lock(parent.Get(t))
				assert.NoError(t, err)
				t.Cleanup(func() { _ = l.Unlock(ctx) })
				return ctx
			}).EagerLoading(s)
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithTimeout(parent.Get(t), 50*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			})
			s.Then("cancellation stops acquisition without leaking a later lock", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), context.DeadlineExceeded)
				assert.NoError(t, locker.Get(t).Unlock(owner.Get(t)))
				assert.NoError(t, subject.Get(t).Save(parent.Get(t), ent.Get(t)))
			})
		})

		s.When("cancellation races with a successful acquisition", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
				r := subject.Super(t)
				callCtx, cancel := context.WithCancel(parent.Get(t))
				t.Cleanup(cancel)
				ctx.Set(t, callCtx)
				r.Locker = repositoryLockerHook{Locker: locker.Get(t), afterLock: cancel}
				return r
			})
			s.Then("does no file work and releases a raced acquisition", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), context.Canceled)
				assert.Empty(t, storage.Get(t).names(t))
				assertLockerAvailable(t, locker.Get(t))
			})
		})

		s.When("ownership is lost after checking the destination", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				l, name := locker.Get(t), string(ent.Get(t).ID)
				fsys.Get(t).setHook(func(op, path string) {
					if op == "stat" && filepath.Base(path) == name {
						l.current().lose()
					}
				})
			})
			s.Then("does not publish or leave temporary files", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrLockLost)
				assert.Empty(t, storage.Get(t).names(t))
			})
		})

		s.When("the operation is cancelled after opening its temporary file", func(s *testcase.Spec) {
			cancel := let.Var[context.CancelFunc](s, nil)
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, stop := context.WithCancel(parent.Get(t))
				cancel.Set(t, stop)
				t.Cleanup(stop)
				return ctx
			})
			s.Then("retains the lock through cleanup and does not publish", func(t *testcase.T) {
				ctx.Get(t)
				l, stop := locker.Get(t), cancel.Get(t)
				var cleaned, unlockedTooSoon atomic.Bool
				fsys.Get(t).setHook(func(op, path string) {
					if !strings.HasPrefix(filepath.Base(path), ".") {
						return
					}
					if op == "open" {
						stop()
					}
					if op == "remove" {
						cleaned.Store(true)
						unlockedTooSoon.Store(l.current().ctx.Err() != nil)
					}
				})
				assert.ErrorIs(t, act(t), context.Canceled)
				assert.True(t, cleaned.Load())
				assert.False(t, unlockedTooSoon.Load())
				assert.Empty(t, storage.Get(t).names(t))
				assertLockerAvailable(t, l)
			})
		})
	})

	s.Describe("#CommitTx", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).CommitTx(tx.Get(t)) }
		s.Before(func(t *testcase.T) { assert.NoError(t, subject.Get(t).Save(tx.Get(t), ent.Get(t))) })

		s.Then("publishes the staged value and releases ownership", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assertLockerAvailable(t, locker.Get(t))
			got, found, err := subject.Get(t).FindByID(parent.Get(t), ent.Get(t).ID)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, got, *ent.Get(t))
		})

		s.When("ownership was lost and another writer has saved", func(s *testcase.Spec) {
			winner := let.Var(s, func(t *testcase.T) testent.Foo {
				return testent.Foo{ID: ent.Get(t).ID, Foo: t.Random.UUID()}
			})
			s.Before(func(t *testcase.T) {
				locker.Get(t).current().lose()
				other := storage.Get(t).repository()
				other.Locker = locker.Get(t)
				value := winner.Get(t)
				assert.NoError(t, other.Save(parent.Get(t), &value))
			})
			s.Then("reports lock loss without overwriting the new owner", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrLockLost)
				got, found, err := subject.Get(t).FindByID(parent.Get(t), ent.Get(t).ID)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, got, winner.Get(t))
				assert.Equal(t, storage.Get(t).names(t), []string{"a"})
			})
		})

		s.When("ownership is lost during the final rename", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				l := locker.Get(t)
				fsys.Get(t).setHook(func(op, _ string) {
					if op == "rename" {
						l.current().lose()
					}
				})
			})
			s.Then("reports loss even when every rename has already completed", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrLockLost)
				assert.Equal(t, storage.Get(t).names(t), []string{"a"})
			})
		})

		s.When("ownership is lost during the first rename", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				second := testent.Foo{ID: "b", Foo: t.Random.UUID()}
				assert.NoError(t, subject.Get(t).Save(tx.Get(t), &second))
				l := locker.Get(t)
				fsys.Get(t).setHook(func(op, path string) {
					if op == "rename" && filepath.Base(path) == "a" {
						l.current().lose()
					}
				})
			})
			s.Then("stops before the next publication and cleans the remaining staging", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrLockLost)
				assert.Equal(t, storage.Get(t).names(t), []string{"a"}, "the in-flight rename cannot be fenced by guard.Locker")
			})
		})
	})

	s.Describe("#FindByID within a transaction", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			_, _, err := subject.Get(t).FindByID(tx.Get(t), ent.Get(t).ID)
			return err
		}
		s.When("the transaction lost its ownership", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Save(tx.Get(t), ent.Get(t)))
				locker.Get(t).current().lose()
			})
			s.Then("rejects further reads and removes staging", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrLockLost)
				t.Eventually(func(t *testcase.T) { assert.Empty(t, storage.Get(t).names(t)) })
				assert.False(t, subject.Get(t).InTx(tx.Get(t)), "a transaction rolled back after lock loss is no longer active")
			})
		})
	})

	s.Describe("#BeginTx", func(s *testcase.Spec) {
		act := func(t *testcase.T) (context.Context, error) { return subject.Get(t).BeginTx(ctx.Get(t)) }
		s.Then("ending a nested first-operation scope does not release the outer transaction lock", func(t *testcase.T) {
			outer, err := act(t)
			assert.NoError(t, err)
			r := subject.Get(t)
			t.Cleanup(func() { _ = r.RollbackTx(outer) })
			inner, err := r.BeginTx(outer)
			assert.NoError(t, err)
			assert.NoError(t, r.Save(inner, ent.Get(t)))
			ownership := locker.Get(t).current()
			assert.NoError(t, r.CommitTx(inner))
			assert.NoError(t, ownership.ctx.Err())
			assertLockerBusy(t, locker.Get(t))
			assert.NoError(t, r.CommitTx(outer))
		})
		s.Then("parent cancellation rolls back before releasing its held lock", func(t *testcase.T) {
			parent, cancel := context.WithCancel(parent.Get(t))
			defer cancel()
			ctx.Set(t, parent)
			tx, err := act(t)
			assert.NoError(t, err)
			assert.NoError(t, subject.Get(t).Save(tx, ent.Get(t)))
			ownership := locker.Get(t).current()
			var cleaned, unlockedTooSoon atomic.Bool
			fsys.Get(t).setHook(func(op, path string) {
				if op == "remove" && strings.HasPrefix(filepath.Base(path), ".") {
					cleaned.Store(true)
					unlockedTooSoon.Store(ownership.ctx.Err() != nil)
				}
			})
			cancel()
			assertLockerAvailable(t, locker.Get(t))
			assert.True(t, cleaned.Load())
			assert.False(t, unlockedTooSoon.Load())
			assert.Empty(t, storage.Get(t).names(t))
		})
	})
}

func TestRepository_BorrowedLocker(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		dir     = let.Var(s, func(t *testcase.T) string { return t.TempDir() })
		locker  = let.Var(s, func(t *testcase.T) *localfs.Locker { return &localfs.Locker{Dir: dir.Get(t)} })
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			return &localfs.Repository[testent.Foo, testent.FooID]{Dir: dir.Get(t), Locker: locker.Get(t)}
		})
		owner = let.Var(s, func(t *testcase.T) context.Context {
			l := locker.Get(t)
			ctx, err := l.Lock(lockerTestContext(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = l.Unlock(ctx) })
			return ctx
		})
		ent = let.Var(s, func(t *testcase.T) *testent.Foo { return &testent.Foo{ID: "setting", Foo: t.Random.UUID()} })
	)
	s.Describe("#Save", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Save(owner.Get(t), ent.Get(t)) }
		s.Then("borrows ownership without cancelling or unlocking its caller", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, owner.Get(t).Err())
			assertLockerBusy(t, &localfs.Locker{Dir: dir.Get(t)})
		})
		s.When("an independent call is already waiting for the caller's lock", func(s *testcase.Spec) {
			owner.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithTimeout(owner.Super(t), 250*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			})
			s.Then("lets the owner re-enter without queuing behind its own waiter", func(t *testcase.T) {
				type waiterKey struct{}
				owner.Get(t)
				r := subject.Get(t)
				entered := make(chan struct{})
				r.Locker = repositoryLockerHook{Locker: locker.Get(t), beforeLock: func(ctx context.Context) {
					if ctx.Value(waiterKey{}) != nil {
						close(entered)
					}
				}}
				waitCtx, cancel := context.WithTimeout(context.WithValue(context.Background(), waiterKey{}, true), time.Second)
				defer cancel()
				done := make(chan error, 1)
				finished := make(chan struct{})
				value := testent.Foo{ID: "waiter", Foo: t.Random.UUID()}
				go func() { defer close(finished); done <- r.Save(waitCtx, &value) }()
				t.Cleanup(func() { cancel(); <-finished })
				assert.Within(t, time.Second, func(ctx context.Context) {
					select {
					case <-entered:
					case <-ctx.Done():
						assert.NoError(t, ctx.Err())
					}
				})
				assert.NoError(t, act(t))
				assert.NoError(t, owner.Get(t).Err())
				assert.NoError(t, locker.Get(t).Unlock(owner.Get(t)))
				assert.Within(t, time.Second, func(ctx context.Context) {
					select {
					case err := <-done:
						assert.NoError(t, err)
					case <-ctx.Done():
						assert.NoError(t, ctx.Err())
					}
				})
			})
		})
	})
	s.Describe("#Create concurrently under the same borrowed ownership", func(s *testcase.Spec) {
		act := func(t *testcase.T) []error {
			r, ctx, entity := subject.Get(t), owner.Get(t), *ent.Get(t)
			errs := make([]error, 8)
			var calls []func()
			for i := range errs {
				calls = append(calls, func() { value := entity; errs[i] = r.Create(ctx, &value) })
			}
			testcase.Race(calls...)
			return errs
		}
		s.Then("serialises the calls locally without releasing outer ownership", func(t *testcase.T) {
			var successes int
			for _, err := range act(t) {
				if err == nil {
					successes++
				} else {
					assert.ErrorIs(t, err, crud.ErrAlreadyExists)
				}
			}
			assert.Equal(t, successes, 1)
			assert.NoError(t, owner.Get(t).Err())
			assertLockerBusy(t, &localfs.Locker{Dir: dir.Get(t)})
		})
	})
	s.Describe("#CommitTx", func(s *testcase.Spec) {
		tx := let.Var(s, func(t *testcase.T) context.Context {
			r := subject.Get(t)
			tx, err := r.BeginTx(owner.Get(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = r.RollbackTx(tx) })
			assert.NoError(t, r.Save(tx, ent.Get(t)))
			return tx
		})
		act := func(t *testcase.T) error { return subject.Get(t).CommitTx(tx.Get(t)) }
		s.Then("finishes its transaction without releasing the caller lock", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, owner.Get(t).Err())
			assertLockerBusy(t, &localfs.Locker{Dir: dir.Get(t)})
		})
	})
}

func TestRepository_LockRoot(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		root = let.Var(s, func(t *testcase.T) string {
			if runtime.GOOS == "windows" {
				t.Skip("symbolic links may require privileges on Windows")
			}
			root := t.TempDir()
			assert.NoError(t, os.Mkdir(filepath.Join(root, "a"), 0o700))
			assert.NoError(t, os.Mkdir(filepath.Join(root, "b"), 0o700))
			assert.NoError(t, os.Symlink("a", filepath.Join(root, "alias")))
			return root
		})
		fsys    = let.Var(s, func(t *testcase.T) localfs.FileSystem { return localfs.FileSystem{RootPath: root.Get(t)} })
		locker  = let.Var(s, func(t *testcase.T) *localfs.Locker { return &localfs.Locker{FS: fsys.Get(t), Dir: "alias"} })
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			l := locker.Get(t)
			path := filepath.Join(root.Get(t), "alias")
			return &localfs.Repository[testent.Foo, testent.FooID]{FS: fsys.Get(t), Dir: "alias", Locker: repositoryLockerHook{
				Locker: l,
				afterLock: func() {
					assert.NoError(t, os.Remove(path))
					assert.NoError(t, os.Symlink("b", path))
				},
			}}
		})
		ctx = let.With[context.Context](s, lockerTestContext)
		ent = let.Var(s, func(t *testcase.T) *testent.Foo { return &testent.Foo{ID: "setting", Foo: t.Random.UUID()} })
	)
	s.Describe("#Save", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Save(ctx.Get(t), ent.Get(t)) }
		s.Then("uses the decorated native lock root rather than resolving the retargeted alias", func(t *testcase.T) {
			other := &localfs.Locker{FS: fsys.Get(t), Dir: "b"}
			owner, err := other.Lock(ctx.Get(t))
			assert.NoError(t, err)
			defer other.Unlock(owner)
			assert.NoError(t, act(t))
			_, err = fsys.Get(t).Stat("a/setting")
			assert.NoError(t, err)
			_, err = fsys.Get(t).Stat("b/setting")
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
		s.When("the caller already owns the native lock", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				l := locker.Get(t)
				owner, err := l.Lock(ctx.Super(t))
				assert.NoError(t, err)
				t.Cleanup(func() { _ = l.Unlock(owner) })
				return owner
			})
			s.Then("borrows the pinned root through the decorator without closing it", func(t *testcase.T) {
				ctx.Get(t)
				assert.NoError(t, act(t))
				assert.NoError(t, ctx.Get(t).Err())
				_, err := fsys.Get(t).Stat("a/setting")
				assert.NoError(t, err)
				assertLockerBusy(t, &localfs.Locker{FS: fsys.Get(t), Dir: "a"})
			})
		})
	})
	s.Describe("#CommitTx", func(s *testcase.Spec) {
		tx := let.Var(s, func(t *testcase.T) context.Context {
			r := subject.Get(t)
			tx, err := r.BeginTx(ctx.Get(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = r.RollbackTx(tx) })
			assert.NoError(t, r.Save(tx, ent.Get(t)))
			return tx
		})
		act := func(t *testcase.T) error { return subject.Get(t).CommitTx(tx.Get(t)) }
		s.Then("publishes into the original pinned root after the alias changes", func(t *testcase.T) {
			assert.NoError(t, act(t))
			_, err := fsys.Get(t).Stat("a/setting")
			assert.NoError(t, err)
			_, err = fsys.Get(t).Stat("b/setting")
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})
}

// This decorator gives a real in-memory lock cancellation-sensitive ownership,
// like a leased backend, with deterministic revocation for the repository specs.
type repositoryLeaseLocker struct {
	base   *memory.Lock
	mu     sync.Mutex
	latest *repositoryLease
}

type repositoryLease struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	base   context.Context

	once     sync.Once
	err      error
	released chan struct{}
}

func (l *repositoryLeaseLocker) Lock(ctx context.Context) (context.Context, error) {
	if state, ok := ctx.Value(l).(*repositoryLease); ok {
		if err := state.ctx.Err(); err != nil {
			return nil, err
		}
		return ctx, nil
	}
	base, err := l.base.Lock(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(base)
	state := &repositoryLease{cancel: cancel, base: base, released: make(chan struct{})}
	ctx = context.WithValue(ctx, l, state)
	state.ctx = ctx
	l.mu.Lock()
	l.latest = state
	l.mu.Unlock()
	context.AfterFunc(ctx, func() { _ = l.Unlock(ctx) })
	return ctx, nil
}

func (l *repositoryLeaseLocker) Unlock(ctx context.Context) error {
	state, ok := ctx.Value(l).(*repositoryLease)
	if !ok {
		return guard.ErrNoLock
	}
	state.once.Do(func() {
		state.err = errors.Join(context.Cause(state.ctx), l.base.Unlock(context.WithoutCancel(state.base)))
		state.cancel(nil)
		close(state.released)
	})
	return state.err
}

func (l *repositoryLeaseLocker) current() *repositoryLease {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.latest
}

func (l *repositoryLease) lose() {
	l.cancel(guard.ErrLockLost)
	<-l.released
}

type repositoryLockerHook struct {
	guard.Locker
	beforeLock func(context.Context)
	afterLock  func()
}

func (l repositoryLockerHook) Lock(ctx context.Context) (context.Context, error) {
	if l.beforeLock != nil {
		l.beforeLock(ctx)
	}
	ctx, err := l.Locker.Lock(ctx)
	if err == nil && l.afterLock != nil {
		l.afterLock()
	}
	return ctx, err
}

type repositoryLockFS struct {
	localfs.RepositoryFileSystem
	hook atomic.Pointer[func(string, string)]
}

func (f *repositoryLockFS) setHook(hook func(string, string)) { f.hook.Store(&hook) }
func (f *repositoryLockFS) after(op, name string) {
	if hook := f.hook.Load(); hook != nil {
		(*hook)(op, name)
	}
}
func (f *repositoryLockFS) Stat(name string) (fs.FileInfo, error) {
	info, err := f.RepositoryFileSystem.Stat(name)
	f.after("stat", name)
	return info, err
}
func (f *repositoryLockFS) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	file, err := f.RepositoryFileSystem.OpenFile(name, flag, perm)
	if err == nil {
		f.after("open", name)
	}
	return file, err
}
func (f *repositoryLockFS) Rename(oldpath, newpath string) error {
	err := f.RepositoryFileSystem.Rename(oldpath, newpath)
	if err == nil {
		f.after("rename", newpath)
	}
	return err
}
func (f *repositoryLockFS) Remove(name string) error {
	f.after("remove", name)
	return f.RepositoryFileSystem.Remove(name)
}
