package localfs_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/guard/guardcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestLocker_guardcontract(t *testing.T) {
	s := testcase.NewSpec(t)
	subject := &localfs.Locker{FS: localfs.FileSystem{RootPath: t.TempDir()}, Dir: "."}
	guardcontract.Locker(subject, guardcontract.LockerConfig{
		MakeContext: lockerTestContext,
		// Native locks have no lease and their handles are private. There is no
		// supported way to revoke a live acquisition for MakeLockLost.
	}).Spec(s)
}

func TestLocker(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		root = let.Var(s, func(t *testcase.T) string {
			root := t.TempDir()
			assert.Must(t).NoError(os.Mkdir(filepath.Join(root, "settings"), 0o700))
			return root
		})
		ctx     = let.With[context.Context](s, lockerTestContext)
		subject = let.Var(s, func(t *testcase.T) *localfs.Locker {
			return &localfs.Locker{FS: localfs.FileSystem{RootPath: root.Get(t)}, Dir: "settings"}
		})
		other = let.Var(s, func(t *testcase.T) *localfs.Locker {
			l := subject.Get(t)
			return &localfs.Locker{FS: l.FS, Dir: l.Dir}
		})
	)

	s.Describe("#Lock", func(s *testcase.Spec) {
		act := func(t *testcase.T) (context.Context, error) {
			l := subject.Get(t)
			lockCtx, err := l.Lock(ctx.Get(t))
			if err == nil {
				t.Cleanup(func() { _ = l.Unlock(lockCtx) })
			}
			return lockCtx, err
		}

		s.Then("separate instances wait for the same directory and acquire it after release", func(t *testcase.T) {
			lockCtx, err := act(t)
			assert.NoError(t, err)
			l, waitCtx := other.Get(t), ctx.Get(t)
			waiter := assert.NotWithin(t, 50*time.Millisecond, func(context.Context) {
				lockCtx, err := l.Lock(waitCtx)
				assert.NoError(t, err)
				assert.NoError(t, l.Unlock(lockCtx))
			})
			assert.NoError(t, subject.Get(t).Unlock(lockCtx))
			assert.Within(t, time.Second, func(context.Context) { waiter.Wait() })
		})

		s.Then("jailed and unjailed instances coordinate on the opened directory", func(t *testcase.T) {
			lockCtx, err := act(t)
			assert.NoError(t, err)
			l := &localfs.Locker{Dir: filepath.Join(root.Get(t), "settings")}
			assertLockerBusy(t, l)
			assert.NoError(t, subject.Get(t).Unlock(lockCtx))
			assertLockerAvailable(t, l)
		})

		s.Then("different directories do not share ownership or block each other", func(t *testcase.T) {
			lockCtx, err := act(t)
			assert.NoError(t, err)
			assert.NoError(t, os.Mkdir(filepath.Join(root.Get(t), "other"), 0o700))
			l := &localfs.Locker{FS: subject.Get(t).FS, Dir: "other"}
			otherCtx, err := l.Lock(lockCtx)
			assert.NoError(t, err)
			t.Cleanup(func() { _ = l.Unlock(otherCtx) })
			assertLockerBusy(t, &localfs.Locker{FS: l.FS, Dir: l.Dir})
			assert.NoError(t, l.Unlock(otherCtx))
			assert.NoError(t, lockCtx.Err(), "unlocking a nested, different lock must not cancel its parent")
		})

		s.Then("a cancelled waiter does not leak ownership or prevent a later acquisition", func(t *testcase.T) {
			lockCtx, err := act(t)
			assert.NoError(t, err)
			assertLockerBusy(t, other.Get(t))
			assert.NoError(t, subject.Get(t).Unlock(lockCtx))
			assertLockerAvailable(t, other.Get(t))
		})

		s.Then("a cancelled waiter on the same instance does not leak its turn", func(t *testcase.T) {
			lockCtx, err := act(t)
			assert.NoError(t, err)
			assertLockerBusy(t, subject.Get(t))
			assert.NoError(t, subject.Get(t).Unlock(lockCtx))
			assertLockerAvailable(t, subject.Get(t))
		})

		s.Then("the opened directory stays locked after its path is renamed", func(t *testcase.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows can prevent renaming an open directory")
			}
			lockCtx, err := act(t)
			assert.NoError(t, err)
			assert.NoError(t, os.Rename(filepath.Join(root.Get(t), "settings"), filepath.Join(root.Get(t), "moved")))
			l := &localfs.Locker{FS: subject.Get(t).FS, Dir: "moved"}
			assertLockerBusy(t, l)
			assert.NoError(t, subject.Get(t).Unlock(lockCtx))
			assertLockerAvailable(t, l)
		})

		s.When("FS is the zero value", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *localfs.Locker {
				return &localfs.Locker{Dir: filepath.Join(root.Get(t), "settings")}
			})

			s.Then("Dir can be an absolute unjailed path", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.NoError(t, err)
				assertLockerBusy(t, other.Get(t))
				assert.NoError(t, subject.Get(t).Unlock(lockCtx))
				assertLockerAvailable(t, other.Get(t))
			})
		})

		s.When("a second path is a symlink to the same directory", func(s *testcase.Spec) {
			other.Let(s, func(t *testcase.T) *localfs.Locker {
				if runtime.GOOS == "windows" {
					t.Skip("creating symbolic links may require privileges on Windows")
				}
				assert.Must(t).NoError(os.Symlink("settings", filepath.Join(root.Get(t), "alias")))
				return &localfs.Locker{FS: subject.Get(t).FS, Dir: "alias"}
			})

			s.Then("the alias contends for the same lock", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.NoError(t, err)
				assertLockerBusy(t, other.Get(t))
				assert.NoError(t, subject.Get(t).Unlock(lockCtx))
				assertLockerAvailable(t, other.Get(t))
			})
		})

		s.When("Dir does not exist", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *localfs.Locker {
				return &localfs.Locker{FS: subject.Super(t).FS, Dir: "missing"}
			})

			s.Then("it reports not-exist without creating the directory", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.ErrorIs(t, err, os.ErrNotExist)
				assert.Nil(t, lockCtx)
				_, err = subject.Get(t).FS.Stat("missing")
				assert.ErrorIs(t, err, os.ErrNotExist)
			})
		})

		s.When("Dir is a regular file", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *localfs.Locker {
				assert.Must(t).NoError(os.WriteFile(filepath.Join(root.Get(t), "file"), nil, 0o600))
				return &localfs.Locker{FS: subject.Super(t).FS, Dir: "file"}
			})

			s.Then("it rejects the non-directory", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.Error(t, err)
				assert.Nil(t, lockCtx)
			})
		})

		for _, dir := range []string{"..", "absolute", "symlink"} {
			s.When("Dir escapes the jail via "+dir, func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *localfs.Locker {
					l := subject.Super(t)
					switch dir {
					case "..":
						l.Dir = ".."
					case "absolute":
						l.Dir = t.TempDir()
					case "symlink":
						if runtime.GOOS == "windows" {
							t.Skip("creating symbolic links may require privileges on Windows")
						}
						assert.Must(t).NoError(os.Symlink(t.TempDir(), filepath.Join(root.Get(t), "outside")))
						l.Dir = "outside"
					}
					return l
				})

				s.Then("it denies access instead of locking outside FS", func(t *testcase.T) {
					lockCtx, err := act(t)
					assert.ErrorIs(t, err, os.ErrPermission)
					assert.Nil(t, lockCtx)
				})
			})
		}

		s.When("context is nil", func(s *testcase.Spec) {
			ctx.LetValue(s, nil)

			s.Then("it returns an error rather than panicking", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.Error(t, err)
				assert.Nil(t, lockCtx)
			})
		})

		s.When("an old ownership is reused without its cancellation", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				l := subject.Get(t)
				lockCtx, err := l.Lock(ctx.Super(t))
				assert.Must(t).NoError(err)
				assert.Must(t).NoError(l.Unlock(lockCtx))
				return context.WithoutCancel(lockCtx)
			})

			s.Then("it rejects the ended acquisition rather than claiming re-entry", func(t *testcase.T) {
				lockCtx, err := act(t)
				assert.ErrorIs(t, err, guard.ErrNoLock)
				assert.Nil(t, lockCtx)
				assertLockerAvailable(t, other.Get(t))
			})
		})
	})

	s.Describe("#Unlock", func(s *testcase.Spec) {
		var (
			cancel = let.Var[context.CancelCauseFunc](s, nil)
			owner  = let.Var(s, func(t *testcase.T) context.Context {
				parent, stop := context.WithCancelCause(ctx.Get(t))
				cancel.Set(t, stop)
				t.Cleanup(func() { stop(nil) })
				l := subject.Get(t)
				lockCtx, err := l.Lock(parent)
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = l.Unlock(lockCtx) })
				return lockCtx
			})
			input = let.Var(s, func(t *testcase.T) context.Context { return owner.Get(t) })
		)
		act := func(t *testcase.T) error { return subject.Get(t).Unlock(input.Get(t)) }

		s.Then("late duplicate unlocks cannot release a newer acquisition", func(t *testcase.T) {
			assert.NoError(t, act(t))
			l := subject.Get(t)
			lockCtx, err := l.Lock(ctx.Get(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = l.Unlock(lockCtx) })
			assert.NoError(t, act(t))
			assertLockerBusy(t, other.Get(t))
		})

		s.Then("concurrent unlocks release the acquisition exactly once", func(t *testcase.T) {
			owner.Get(t)
			var first, second error
			testcase.Race(func() { first = act(t) }, func() { second = act(t) })
			assert.NoError(t, first)
			assert.NoError(t, second)
			assert.ErrorIs(t, owner.Get(t).Err(), context.Canceled)
			assertLockerAvailable(t, other.Get(t))
		})

		s.When("the parent context is cancelled", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				owner.Get(t)
				cancel.Get(t)(nil)
			})

			s.Then("it retains exclusion for cleanup until explicit Unlock, then reports cancellation", func(t *testcase.T) {
				assert.ErrorIs(t, owner.Get(t).Err(), context.Canceled)
				assertLockerBusy(t, other.Get(t))
				assert.ErrorIs(t, act(t), context.Canceled)
				assertLockerAvailable(t, other.Get(t))
			})

			s.And("cleanup detaches cancellation from the context", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) context.Context { return context.WithoutCancel(owner.Get(t)) })

				s.Then("it releases synchronously without a cancellation error", func(t *testcase.T) {
					assertLockerBusy(t, other.Get(t))
					assert.NoError(t, act(t))
					assertLockerAvailable(t, other.Get(t))
				})
			})
		})

		s.When("an enclosing lock signals lost ownership", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				owner.Get(t)
				cancel.Get(t)(guard.ErrLockLost)
			})

			s.Then("it preserves the lock-loss cause while releasing its own lock", func(t *testcase.T) {
				assert.ErrorIs(t, context.Cause(owner.Get(t)), guard.ErrLockLost)
				assertLockerBusy(t, other.Get(t))
				err := act(t)
				assert.ErrorIs(t, err, guard.ErrLockLost)
				assert.False(t, errors.Is(err, context.Canceled))
				assertLockerAvailable(t, other.Get(t))
			})
		})

		s.When("ownership belongs to a separate instance for the same directory", func(s *testcase.Spec) {
			input.Let(s, func(t *testcase.T) context.Context {
				l := other.Get(t)
				lockCtx, err := l.Lock(ctx.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = l.Unlock(lockCtx) })
				return lockCtx
			})

			s.Then("it reports no ownership and leaves the actual holder locked", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrNoLock)
				assertLockerBusy(t, subject.Get(t))
			})
		})

		s.When("context is nil", func(s *testcase.Spec) {
			input.LetValue(s, nil)

			s.Then("it reports no ownership", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), guard.ErrNoLock)
			})
		})
	})
}

func lockerTestContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func assertLockerBusy(t testing.TB, l guard.Locker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	lockCtx, err := l.Lock(ctx)
	if err == nil {
		defer l.Unlock(lockCtx)
	}
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, lockCtx)
}

func assertLockerAvailable(t testing.TB, l guard.Locker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lockCtx, err := l.Lock(ctx)
	assert.NoError(t, err)
	assert.NoError(t, l.Unlock(lockCtx))
}

func TestLocker_process(t *testing.T) {
	s := testcase.NewSpec(t)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("this platform only supports in-process locking")
	}
	var subject = let.Var(s, func(t *testcase.T) *localfs.Locker {
		return &localfs.Locker{Dir: t.TempDir()}
	})
	act := func(t *testcase.T) (context.Context, error) {
		return subject.Get(t).Lock(lockerTestContext(t))
	}

	s.Test("a separate process cannot acquire the directory until its holder unlocks", func(t *testcase.T) {
		l := subject.Get(t)
		lockCtx, err := act(t)
		assert.NoError(t, err)
		t.Cleanup(func() { _ = l.Unlock(lockCtx) })
		exe, err := os.Executable()
		assert.NoError(t, err)
		cmd := exec.CommandContext(lockerTestContext(t), exe, "-test.run=^TestLocker_processHelper$", "-test.timeout=5s")
		cmd.Env = append(os.Environ(), "FRAMELESS_LOCALFS_LOCK_TEST_DIR="+l.Dir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdout, err := cmd.StdoutPipe()
		assert.NoError(t, err)
		assert.NoError(t, cmd.Start())
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
		reader := bufio.NewReader(stdout)
		assert.Within(t, 2*time.Second, func(context.Context) {
			line, err := reader.ReadString('\n')
			assert.NoError(t, err)
			assert.Equal(t, line, "waiting\n")
		})
		waiter := assert.NotWithin(t, 50*time.Millisecond, func(context.Context) {
			line, err := reader.ReadString('\n')
			assert.NoError(t, err)
			assert.Equal(t, line, "locked\n")
		})
		assert.NoError(t, l.Unlock(lockCtx))
		assert.Within(t, 2*time.Second, func(context.Context) { waiter.Wait() })
		assert.NoError(t, cmd.Wait(), assert.Message(stderr.String()))
	})
}

func TestLocker_processHelper(t *testing.T) {
	s := testcase.NewSpec(t)
	dir := os.Getenv("FRAMELESS_LOCALFS_LOCK_TEST_DIR")
	if dir == "" {
		t.Skip("only run in the lock contention subprocess")
	}
	subject := &localfs.Locker{Dir: dir}
	s.Test("acquire the parent process's directory", func(t *testcase.T) {
		// Confirm contention before signalling readiness, so a delayed child
		// cannot make the parent's blocking assertion pass without trying Lock.
		assertLockerBusy(t, subject)
		_, err := fmt.Fprintln(os.Stdout, "waiting")
		assert.NoError(t, err)
		lockCtx, err := subject.Lock(lockerTestContext(t))
		assert.NoError(t, err)
		defer subject.Unlock(lockCtx)
		_, err = fmt.Fprintln(os.Stdout, "locked")
		assert.NoError(t, err)
		assert.NoError(t, subject.Unlock(lockCtx))
	})
}
