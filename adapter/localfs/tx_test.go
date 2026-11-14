package localfs_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"uuid"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRepository_OnePhaseCommitProtocol(t *testing.T) {
	s := testcase.NewSpec(t)

	for _, backend := range repositoryBackends() {
		s.Context(backend.name, func(s *testcase.Spec) {
			specRepositoryTransactions(s, t, backend.make)
		})
	}
}

func specRepositoryTransactions(s *testcase.Spec, tb testing.TB, makeBackend func(testing.TB) repositoryBackend) {
	var (
		ctx     = let.Context(s)
		storage = let.Var(s, func(t *testcase.T) repositoryBackend { return makeBackend(t) })
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			return storage.Get(t).repository()
		})
	)

	repo := makeBackend(tb).repository()
	crudConfig := crudcontract.Config[testent.Foo, testent.FooID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
		OnePhaseCommit:  repo,
	}
	testcase.RunSuite(s,
		crudcontract.OnePhaseCommitProtocol[testent.Foo, testent.FooID](repo, repo, crudConfig),
		crudcontract.Creator[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.Saver[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.ByIDFinder[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.AllFinder[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.Updater[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.ByIDDeleter[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.AllDeleter[testent.Foo, testent.FooID](repo, crudConfig),
	)

	s.Context("with an ID which is not a string", func(s *testcase.Spec) {
		storage := makeBackend(tb)
		notes := &localfs.Repository[note, uuid.UUID]{
			FS:      storage.FS,
			Dir:     storage.Dir,
			FileExt: "json",
			MakeID:  func(context.Context) (uuid.UUID, error) { return uuid.NewV7(), nil },
		}
		noteCRUDConfig := crudcontract.Config[note, uuid.UUID]{OnePhaseCommit: notes}
		testcase.RunSuite(s,
			crudcontract.OnePhaseCommitProtocol[note, uuid.UUID](notes, notes, noteCRUDConfig),
			crudcontract.Creator[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.Saver[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.ByIDFinder[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.AllFinder[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.Updater[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.ByIDDeleter[note, uuid.UUID](notes, noteCRUDConfig),
			crudcontract.AllDeleter[note, uuid.UUID](notes, noteCRUDConfig),
		)
	})

	s.Describe("#BeginTx", func(s *testcase.Spec) {
		var (
			parent = let.Var(s, func(t *testcase.T) context.Context { return ctx.Get(t) })
			tx     = let.Var(s, func(t *testcase.T) context.Context {
				tx, err := subject.Get(t).BeginTx(parent.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = subject.Get(t).RollbackTx(tx) })
				return tx
			})
			ent = let.Var(s, func(t *testcase.T) *testent.Foo {
				return &testent.Foo{ID: "github.com", Foo: t.Random.StringN(40)}
			})
			other = let.Var(s, func(t *testcase.T) *testent.Foo {
				return &testent.Foo{ID: "other", Foo: t.Random.StringN(40)}
			})
		)
		// saveInTx makes the transaction lock the directory, as it is its first operation.
		saveInTx := func(t *testcase.T) {
			t.Helper()
			assert.Must(t).NoError(subject.Get(t).Save(tx.Get(t), ent.Get(t)))
		}

		s.Then("the changes are kept in a hidden file of the transaction, until the commit moves them into place", func(t *testcase.T) {
			saveInTx(t)
			names := storage.Get(t).names(t)
			assert.Equal(t, 1, len(names))
			staged, ok := strings.CutPrefix(names[0], ".github.com.")
			assert.True(t, ok, assert.Message(names[0]))
			_, err := uuid.Parse(staged)
			assert.NoError(t, err)

			assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
			assert.Equal(t, []string{"github.com"}, storage.Get(t).names(t))
			got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, *ent.Get(t), got)
		})

		s.Then("a rollback removes the files of the changes", func(t *testcase.T) {
			saveInTx(t)
			assert.NoError(t, subject.Get(t).RollbackTx(tx.Get(t)))
			assert.Empty(t, storage.Get(t).names(t))
		})

		s.Then("an update keeps the permission of the entity's file", func(t *testcase.T) {
			if storage.Get(t).native() && runtime.GOOS == "windows" {
				t.Skip("Windows has no permission bits, only a read-only attribute")
			}
			assert.NoError(t, subject.Get(t).Save(ctx.Get(t), ent.Get(t)))
			path := filepath.Join(storage.Get(t).Dir, "github.com")
			if storage.Get(t).native() {
				var root string
				switch fsys := storage.Get(t).FS.(type) {
				case localfs.FileSystem:
					root = fsys.RootPath
				case *localfs.FileSystem:
					root = fsys.RootPath
				}
				assert.NoError(t, os.Chmod(filepath.Join(root, path), 0o640))
			} else {
				// FileSystem has no Chmod; recreate the same contents with a different mode.
				content := storage.Get(t).readFile(t, "github.com")
				assert.NoError(t, storage.Get(t).fsys().Remove(path))
				assert.NoError(t, filesystem.WriteFile(storage.Get(t).fsys(), path, []byte(content), 0o640))
			}

			ent.Get(t).Foo = t.Random.StringN(40)
			assert.NoError(t, subject.Get(t).Update(tx.Get(t), ent.Get(t)))
			assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))

			assert.Equal(t, os.FileMode(0o640), storage.Get(t).stat(t, "github.com").Mode().Perm())
		})

		s.Then("until its first operation, the transaction doesn't hold back the changes from outside of it", func(t *testcase.T) {
			tx.Get(t)
			assert.NoError(t, receive(t, goSave(subject.Get(t), ctx.Get(t), other.Get(t))))
		})

		s.Then("changes from outside of the transaction wait until it ends", func(t *testcase.T) {
			saveInTx(t)
			done := goSave(subject.Get(t), ctx.Get(t), other.Get(t))
			assertWaits(t, done)

			assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
			assert.NoError(t, receive(t, done))
		})

		s.Then("changes through another repository of the same directory wait as well, as the directory is locked", func(t *testcase.T) {
			another := storage.Get(t).repository()
			if !storage.Get(t).native() {
				locker := memory.NewLocker()
				subject.Get(t).Locker = locker
				another.Locker = locker
			}
			saveInTx(t)
			done := goSave(another, ctx.Get(t), other.Get(t))
			assertWaits(t, done)

			assert.NoError(t, subject.Get(t).RollbackTx(tx.Get(t)))
			assert.NoError(t, receive(t, done))
			assert.Equal(t, []string{"other"}, storage.Get(t).names(t))
		})

		s.Then("waiting for the lock stops when the context of the change is done", func(t *testcase.T) {
			saveInTx(t)
			waitCtx, cancel := context.WithTimeout(ctx.Get(t), 50*time.Millisecond)
			defer cancel()
			assert.ErrorIs(t, context.DeadlineExceeded, subject.Get(t).Save(waitCtx, other.Get(t)))
		})

		s.Then("reads from outside of the transaction don't wait, and see the state from before the transaction", func(t *testcase.T) {
			original := *ent.Get(t)
			assert.NoError(t, subject.Get(t).Save(ctx.Get(t), &original))
			ent.Get(t).Foo = t.Random.StringN(40)
			saveInTx(t)

			type result struct {
				ent   testent.Foo
				found bool
				err   error
			}
			done := make(chan result, 1)
			go func() {
				got, found, err := subject.Get(t).FindByID(ctx.Get(t), original.ID)
				done <- result{ent: got, found: found, err: err}
			}()
			got := receive(t, done)
			assert.NoError(t, got.err)
			assert.True(t, got.found)
			assert.Equal(t, original, got.ent)
		})

		s.When("the context of the transaction is done before the transaction ends", func(s *testcase.Spec) {
			cancel := testcase.Let[func()](s, nil)
			parent.Let(s, func(t *testcase.T) context.Context {
				parent, cancelParent := context.WithCancel(ctx.Get(t))
				t.Defer(cancelParent)
				cancel.Set(t, cancelParent)
				return parent
			})

			s.Then("the transaction is rolled back, and it no longer holds back the changes from outside of it", func(t *testcase.T) {
				saveInTx(t)
				cancel.Get(t)()

				waitCtx, cancelWait := context.WithTimeout(ctx.Get(t), time.Second)
				defer cancelWait()
				assert.NoError(t, subject.Get(t).Save(waitCtx, other.Get(t)))
				assert.Equal(t, []string{"other"}, storage.Get(t).names(t))
				assert.ErrorIs(t, context.Canceled, subject.Get(t).CommitTx(tx.Get(t)))
			})
		})
	})
}

// note is an entity whose ID is not a string, so its Repository needs a MakeID,
// and its files are named after the text form of the ID (see encoding.TextMarshaler).
type note struct {
	ID   uuid.UUID `ext:"id"`
	Text string
}

func goSave(repo *localfs.Repository[testent.Foo, testent.FooID], ctx context.Context, ent *testent.Foo) <-chan error {
	done := make(chan error, 1)
	go func() { done <- repo.Save(ctx, ent) }()
	return done
}

func assertWaits[T any](t *testcase.T, done <-chan T) {
	t.Helper()
	select {
	case v := <-done:
		t.Fatalf("expected to wait, but it is done: %v", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func receive[T any](t *testcase.T, done <-chan T) T {
	t.Helper()
	select {
	case v := <-done:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("it is still waiting")
		var zero T
		return zero
	}
}
