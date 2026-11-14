package localfs_test

import (
	"context"
	"io"
	"io/fs"
	"testing"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRepository_Restaging(t *testing.T) {
	s := testcase.NewSpec(t)

	for _, backend := range repositoryBackends() {
		s.Context(backend.name, func(s *testcase.Spec) {
			var (
				storage = let.Var(s, func(t *testcase.T) repositoryBackend { return backend.make(t) })
				faults  = let.Var(s, func(t *testcase.T) *repositoryFaultFS {
					return &repositoryFaultFS{RepositoryFileSystem: storage.Get(t).fsys()}
				})
				subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
					r := storage.Get(t).repository()
					r.FS = faults.Get(t)
					return r
				})
				ctx = let.Context(s)
				tx  = let.Var(s, func(t *testcase.T) context.Context {
					r := subject.Get(t)
					tx, err := r.BeginTx(ctx.Get(t))
					assert.NoError(t, err)
					t.Cleanup(func() { _ = r.RollbackTx(tx) })
					return tx
				})
				original = let.Var(s, func(t *testcase.T) testent.Foo {
					return testent.Foo{ID: "setting", Foo: "original-" + t.Random.StringN(40)}
				})
				first = let.Var(s, func(t *testcase.T) testent.Foo {
					return testent.Foo{ID: original.Get(t).ID, Foo: "first-" + t.Random.StringN(40)}
				})
				next = let.Var(s, func(t *testcase.T) *testent.Foo {
					return &testent.Foo{ID: original.Get(t).ID, Foo: "next-" + t.Random.StringN(40)}
				})
				hadOriginal = let.Var(s, func(t *testcase.T) bool { return true })
			)
			assertEntity := func(t *testcase.T, ctx context.Context, expected testent.Foo) {
				t.Helper()
				got, found, err := subject.Get(t).FindByID(ctx, expected.ID)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, got, expected)
			}
			assertOriginal := func(t *testcase.T) {
				t.Helper()
				got, found, err := subject.Get(t).FindByID(ctx.Get(t), original.Get(t).ID)
				assert.NoError(t, err)
				assert.Equal(t, found, hadOriginal.Get(t))
				if hadOriginal.Get(t) {
					assert.Equal(t, got, original.Get(t))
				} else {
					assert.Empty(t, got)
				}
			}

			for _, operation := range []struct {
				name  string
				write func(*localfs.Repository[testent.Foo, testent.FooID], context.Context, *testent.Foo) error
			}{
				{"#Save an already staged entity", (*localfs.Repository[testent.Foo, testent.FooID]).Save},
				{"#Update an already staged entity", (*localfs.Repository[testent.Foo, testent.FooID]).Update},
			} {
				s.Describe(operation.name, func(s *testcase.Spec) {
					act := func(t *testcase.T) error { return operation.write(subject.Get(t), tx.Get(t), next.Get(t)) }
					names := let.Var(s, func(t *testcase.T) []string { return storage.Get(t).names(t) })
					s.Before(func(t *testcase.T) {
						if hadOriginal.Get(t) {
							ent := original.Get(t)
							assert.NoError(t, subject.Get(t).Save(ctx.Get(t), &ent))
						}
						ent := first.Get(t)
						assert.NoError(t, subject.Get(t).Save(tx.Get(t), &ent))
						names.Get(t)
					})

					s.Then("replaces the staged value without extra files and commits only the latest value", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assertEntity(t, tx.Get(t), *next.Get(t))
						assertOriginal(t)
						assert.Equal(t, storage.Get(t).names(t), names.Get(t))
						assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
						assertEntity(t, ctx.Get(t), *next.Get(t))
						assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
					})

					for _, failure := range []struct {
						name string
						err  error
						set  func(*repositoryFaultFS, error)
					}{
						{"opening fails", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.openErr = err }},
						{"writing fails after a partial write", io.ErrShortWrite, func(f *repositoryFaultFS, err error) { f.writeErr = err }},
						{"closing fails", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.closeErr = err }},
						{"renaming fails", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.renameErr = err }},
					} {
						s.When(failure.name, func(s *testcase.Spec) {
							s.Before(func(t *testcase.T) { failure.set(faults.Get(t), failure.err) })

							check := func(t *testcase.T) {
								assert.ErrorIs(t, act(t), failure.err)
								failure.set(faults.Get(t), nil)
								assertEntity(t, tx.Get(t), first.Get(t))
								assertOriginal(t)
								assert.Equal(t, storage.Get(t).names(t), names.Get(t), "the earlier stage must survive without temporary leftovers")
								assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
								assertEntity(t, ctx.Get(t), first.Get(t))
								assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
							}
							s.Then("preserves the earlier staged value for reading and committing", check)

							s.Then("can still roll back without changing committed contents or leaving temporary files", func(t *testcase.T) {
								assert.ErrorIs(t, act(t), failure.err)
								failure.set(faults.Get(t), nil)
								assert.NoError(t, subject.Get(t).RollbackTx(tx.Get(t)))
								assertOriginal(t)
								assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
							})

							s.Then("can retry the write and commit the replacement", func(t *testcase.T) {
								assert.ErrorIs(t, act(t), failure.err)
								failure.set(faults.Get(t), nil)
								assert.NoError(t, act(t))
								assertEntity(t, tx.Get(t), *next.Get(t))
								assert.Equal(t, storage.Get(t).names(t), names.Get(t))
								assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
								assertEntity(t, ctx.Get(t), *next.Get(t))
								assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
							})

							s.And("the entity did not exist before the transaction", func(s *testcase.Spec) {
								hadOriginal.LetValue(s, false)

								s.Then("preserves the newly staged entity for reading and committing", check)
							})
						})
					}
				})
			}

			for _, operation := range []struct {
				name  string
				write func(*localfs.Repository[testent.Foo, testent.FooID], context.Context, *testent.Foo) error
			}{
				{"#Save an entity deleted in the transaction", (*localfs.Repository[testent.Foo, testent.FooID]).Save},
				{"#Create an entity deleted in the transaction", (*localfs.Repository[testent.Foo, testent.FooID]).Create},
			} {
				s.Describe(operation.name, func(s *testcase.Spec) {
					act := func(t *testcase.T) error { return operation.write(subject.Get(t), tx.Get(t), next.Get(t)) }
					s.Before(func(t *testcase.T) {
						ent := original.Get(t)
						assert.NoError(t, subject.Get(t).Save(ctx.Get(t), &ent))
						assert.NoError(t, subject.Get(t).DeleteByID(tx.Get(t), ent.ID))
					})

					s.Then("replaces the pending deletion with the recreated entity", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assertEntity(t, tx.Get(t), *next.Get(t))
						assertOriginal(t)
						assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
						assertEntity(t, ctx.Get(t), *next.Get(t))
						assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
					})

					for _, failure := range []struct {
						name string
						err  error
						set  func(*repositoryFaultFS, error)
					}{
						{"opening fails", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.openErr = err }},
						{"writing fails after a partial write", io.ErrShortWrite, func(f *repositoryFaultFS, err error) { f.writeErr = err }},
						{"closing fails", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.closeErr = err }},
					} {
						s.When(failure.name, func(s *testcase.Spec) {
							s.Before(func(t *testcase.T) { failure.set(faults.Get(t), failure.err) })

							s.Then("preserves the pending deletion for reading and committing", func(t *testcase.T) {
								assert.ErrorIs(t, act(t), failure.err)
								failure.set(faults.Get(t), nil)
								got, found, err := subject.Get(t).FindByID(tx.Get(t), original.Get(t).ID)
								assert.NoError(t, err)
								assert.False(t, found)
								assert.Empty(t, got)
								assertOriginal(t)
								assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
								assert.NoError(t, subject.Get(t).CommitTx(tx.Get(t)))
								got, found, err = subject.Get(t).FindByID(ctx.Get(t), original.Get(t).ID)
								assert.NoError(t, err)
								assert.False(t, found)
								assert.Empty(t, got)
								assert.Empty(t, storage.Get(t).names(t))
							})
						})
					}
				})
			}
		})
	}
}
