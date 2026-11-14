package localfs_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

type repositoryBackend struct {
	FS  localfs.RepositoryFileSystem
	Dir string
}

func repositoryBackends() []struct {
	name string
	make func(testing.TB) repositoryBackend
} {
	return []struct {
		name string
		make func(testing.TB) repositoryBackend
	}{
		{"default localfs", func(t testing.TB) repositoryBackend {
			return repositoryBackend{Dir: t.TempDir()}
		}},
		{"memory", func(t testing.TB) repositoryBackend {
			b := repositoryBackend{FS: &memory.FileSystem{}, Dir: filepath.Join("nested", "entities")}
			assert.NoError(t, filesystem.MkdirAll(b.FS, b.Dir, 0o700))
			return b
		}},
		{"jailed localfs value", func(t testing.TB) repositoryBackend {
			b := repositoryBackend{FS: localfs.FileSystem{RootPath: t.TempDir()}, Dir: filepath.Join("nested", "entities")}
			assert.NoError(t, filesystem.MkdirAll(b.FS, b.Dir, 0o700))
			return b
		}},
		{"jailed localfs pointer", func(t testing.TB) repositoryBackend {
			b := repositoryBackend{FS: &localfs.FileSystem{RootPath: t.TempDir()}, Dir: filepath.Join("nested", "entities")}
			assert.NoError(t, filesystem.MkdirAll(b.FS, b.Dir, 0o700))
			return b
		}},
	}
}

func (b repositoryBackend) repository() *localfs.Repository[testent.Foo, testent.FooID] {
	return &localfs.Repository[testent.Foo, testent.FooID]{FS: b.FS, Dir: b.Dir}
}

func (b repositoryBackend) fsys() localfs.RepositoryFileSystem {
	if b.FS == nil {
		return localfs.FileSystem{}
	}
	return b.FS
}

func (b repositoryBackend) native() bool {
	switch b.FS.(type) {
	case nil, localfs.FileSystem, *localfs.FileSystem:
		return true
	default:
		return false
	}
}

func (b repositoryBackend) readFile(t testing.TB, name string) string {
	t.Helper()
	data, err := filesystem.ReadFile(b.fsys(), filepath.Join(b.Dir, name))
	assert.NoError(t, err)
	return string(data)
}

func (b repositoryBackend) writeFile(t testing.TB, name, content string) {
	t.Helper()
	path := filepath.Join(b.Dir, name)
	assert.NoError(t, filesystem.MkdirAll(b.fsys(), filepath.Dir(path), 0o755))
	assert.NoError(t, filesystem.WriteFile(b.fsys(), path, []byte(content), 0o644))
}

func (b repositoryBackend) stat(t testing.TB, name string) fs.FileInfo {
	t.Helper()
	info, err := b.fsys().Stat(filepath.Join(b.Dir, name))
	assert.NoError(t, err)
	return info
}

func (b repositoryBackend) names(t testing.TB) []string {
	t.Helper()
	entries, err := filesystem.ReadDir(b.fsys(), b.Dir)
	assert.NoError(t, err)
	var names []string
	for _, entry := range entries {
		if b.native() && runtime.GOOS == "windows" && entry.Name() == ".lock" {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

func TestRepository_FS(t *testing.T) {
	s := testcase.NewSpec(t)

	for _, backend := range repositoryBackends() {
		s.Context(backend.name, func(s *testcase.Spec) {
			var (
				storage = let.Var(s, func(t *testcase.T) repositoryBackend { return backend.make(t) })
				subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
					return storage.Get(t).repository()
				})
				ctx = let.Context(s)
				ent = let.Var(s, func(t *testcase.T) *testent.Foo {
					return &testent.Foo{ID: "setting", Foo: t.Random.StringN(40)}
				})
			)

			s.Describe("#Save", func(s *testcase.Spec) {
				act := func(t *testcase.T) error { return subject.Get(t).Save(ctx.Get(t), ent.Get(t)) }

				s.Then("replacing an entity preserves its existing file mode", func(t *testcase.T) {
					if storage.Get(t).native() && runtime.GOOS == "windows" {
						t.Skip("Windows has no permission bits, only a read-only attribute")
					}
					data, err := jsonkit.Codec{}.Marshal(*ent.Get(t))
					assert.NoError(t, err)
					assert.NoError(t, filesystem.WriteFile(storage.Get(t).fsys(), filepath.Join(storage.Get(t).Dir, "setting"), data, 0o640))
					mode := storage.Get(t).stat(t, "setting").Mode().Perm()
					ent.Get(t).Foo = t.Random.StringN(40)

					assert.NoError(t, act(t))
					assert.Equal(t, storage.Get(t).stat(t, "setting").Mode().Perm(), mode)
					got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
					assert.NoError(t, err)
					assert.True(t, found)
					assert.Equal(t, got, *ent.Get(t))
					assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
				})

				s.When("Dir is not set", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
						r := subject.Super(t)
						r.Dir = ""
						return r
					})

					s.Then("still rejects the operation with an injected filesystem", func(t *testcase.T) {
						assert.Error(t, act(t))
						assert.Empty(t, storage.Get(t).names(t))
					})
				})

				s.When("Dir has missing nested parents", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
						r := subject.Super(t)
						r.Dir = filepath.Join(r.Dir, "missing", "settings")
						return r
					})

					s.Then("creates the directory and entity in the selected filesystem", func(t *testcase.T) {
						_, err := storage.Get(t).fsys().Stat(subject.Get(t).Dir)
						assert.ErrorIs(t, err, fs.ErrNotExist)
						assert.NoError(t, act(t))
						info, err := storage.Get(t).fsys().Stat(subject.Get(t).Dir)
						assert.NoError(t, err)
						assert.True(t, info.IsDir())
						data, err := filesystem.ReadFile(storage.Get(t).fsys(), filepath.Join(subject.Get(t).Dir, "setting"))
						assert.NoError(t, err)
						assert.NotEmpty(t, data)
						got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
						assert.NoError(t, err)
						assert.True(t, found)
						assert.Equal(t, got, *ent.Get(t))
					})
				})
			})

			s.Context("with non-entity directory entries", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					subject.Get(t).FileExt = "json"
					assert.NoError(t, subject.Get(t).Save(ctx.Get(t), ent.Get(t)))
					storage.Get(t).writeFile(t, ".hidden.json", "not JSON")
					storage.Get(t).writeFile(t, "README.md", "not JSON")
					storage.Get(t).writeFile(t, "subdir.json/child.json", "not JSON")
				})

				s.Describe("#FindAll", func(s *testcase.Spec) {
					act := func(t *testcase.T) ([]testent.Foo, error) {
						return iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
					}

					s.Then("ignores hidden files, other extensions and directories with the entity extension", func(t *testcase.T) {
						got, err := act(t)
						assert.NoError(t, err)
						assert.Equal(t, got, []testent.Foo{*ent.Get(t)})
					})
				})

				s.Describe("#DeleteAll", func(s *testcase.Spec) {
					act := func(t *testcase.T) error { return subject.Get(t).DeleteAll(ctx.Get(t)) }

					s.Then("leaves all non-entity entries and their contents intact", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal(t, storage.Get(t).names(t), []string{".hidden.json", "README.md", "subdir.json"})
						for _, name := range []string{".hidden.json", "README.md", "subdir.json/child.json"} {
							assert.Equal(t, storage.Get(t).readFile(t, name), "not JSON")
						}
					})
				})
			})

			s.Describe("#Create concurrently with the same ID", func(s *testcase.Spec) {
				repositories := let.Var(s, func(t *testcase.T) []*localfs.Repository[testent.Foo, testent.FooID] {
					return []*localfs.Repository[testent.Foo, testent.FooID]{subject.Get(t)}
				})
				type result struct {
					entity testent.Foo
					err    error
				}
				act := func(t *testcase.T) []result {
					repos := repositories.Get(t)
					callCtx, cancel := context.WithTimeout(ctx.Get(t), 5*time.Second)
					defer cancel()
					results := make([]result, 8)
					var calls []func()
					for i := range results {
						results[i].entity = testent.Foo{ID: ent.Get(t).ID, Foo: t.Random.UUID()}
						calls = append(calls, func() { results[i].err = repos[i%len(repos)].Create(callCtx, &results[i].entity) })
					}
					testcase.Race(calls...)
					return results
				}
				check := func(t *testcase.T) {
					var winners []testent.Foo
					for _, result := range act(t) {
						if result.err == nil {
							winners = append(winners, result.entity)
						} else {
							assert.ErrorIs(t, result.err, crud.ErrAlreadyExists)
						}
					}
					assert.Equal(t, len(winners), 1)
					got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
					assert.NoError(t, err)
					assert.True(t, found)
					assert.Equal(t, got, winners[0], "losing creates must not overwrite the winner")
					assert.Equal(t, storage.Get(t).names(t), []string{"setting"}, "no temporary files may remain")
				}

				s.Then("has one winner without overwrites or temporary leftovers", check)

				s.When("separate repositories share the same storage", func(s *testcase.Spec) {
					repositories.Let(s, func(t *testcase.T) []*localfs.Repository[testent.Foo, testent.FooID] {
						first, second := subject.Get(t), storage.Get(t).repository()
						if !storage.Get(t).native() {
							first.Locker = memory.NewLocker()
							second.Locker = first.Locker
						}
						return []*localfs.Repository[testent.Foo, testent.FooID]{first, second}
					})

					s.Then("still has one winner without overwrites or temporary leftovers", check)
				})
			})

			s.Context("with filesystem failures", func(s *testcase.Spec) {
				faults := let.Var(s, func(t *testcase.T) *repositoryFaultFS {
					return &repositoryFaultFS{RepositoryFileSystem: storage.Get(t).fsys()}
				})
				subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
					r := subject.Super(t)
					r.FS = faults.Get(t)
					return r
				})
				s.Before(func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Save(ctx.Get(t), ent.Get(t)))
				})

				for _, failure := range []struct {
					name string
					err  error
					set  func(*repositoryFaultFS, error)
				}{
					{"opening is denied", fs.ErrPermission, func(f *repositoryFaultFS, err error) { f.openErr = err }},
					{"reading fails", io.ErrUnexpectedEOF, func(f *repositoryFaultFS, err error) { f.readErr = err }},
				} {
					s.When(failure.name, func(s *testcase.Spec) {
						s.Before(func(t *testcase.T) { failure.set(faults.Get(t), failure.err) })

						s.Describe("#FindByID", func(s *testcase.Spec) {
							act := func(t *testcase.T) (testent.Foo, bool, error) {
								return subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
							}

							s.Then("returns the underlying error, not a missing entity", func(t *testcase.T) {
								got, found, err := act(t)
								assert.ErrorIs(t, err, failure.err)
								assert.False(t, found)
								assert.Empty(t, got)
							})
						})

						s.Describe("#FindAll", func(s *testcase.Spec) {
							act := func(t *testcase.T) ([]testent.Foo, error) {
								return iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
							}

							s.Then("yields the underlying error", func(t *testcase.T) {
								_, err := act(t)
								assert.ErrorIs(t, err, failure.err)
							})
						})
					})
				}

				s.Describe("#Save", func(s *testcase.Spec) {
					act := func(t *testcase.T) error { return subject.Get(t).Save(ctx.Get(t), ent.Get(t)) }

					for _, failure := range []struct {
						name string
						set  func(*repositoryFaultFS)
					}{
						{"opening is denied", func(f *repositoryFaultFS) { f.openErr = fs.ErrPermission }},
						{"rename is denied", func(f *repositoryFaultFS) { f.renameErr = fs.ErrPermission }},
					} {
						s.When(failure.name, func(s *testcase.Spec) {
							s.Before(func(t *testcase.T) { failure.set(faults.Get(t)) })

							s.Then("preserves the old contents and removes temporary files", func(t *testcase.T) {
								original := storage.Get(t).readFile(t, "setting")
								ent.Get(t).Foo = t.Random.StringN(40)
								assert.ErrorIs(t, act(t), fs.ErrPermission)
								assert.Equal(t, storage.Get(t).readFile(t, "setting"), original)
								assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
							})
						})
					}
				})

				s.Describe("#CommitTx", func(s *testcase.Spec) {
					tx := let.Var(s, func(t *testcase.T) context.Context {
						tx, err := subject.Get(t).BeginTx(ctx.Get(t))
						assert.NoError(t, err)
						t.Cleanup(func() { _ = subject.Get(t).RollbackTx(tx) })
						return tx
					})
					act := func(t *testcase.T) error { return subject.Get(t).CommitTx(tx.Get(t)) }

					s.When("rename fails while committing", func(s *testcase.Spec) {
						s.Before(func(t *testcase.T) {
							ent.Get(t).Foo = t.Random.StringN(40)
							assert.NoError(t, subject.Get(t).Save(tx.Get(t), ent.Get(t)))
							faults.Get(t).renameErr = fs.ErrPermission
						})

						s.Then("reports the error, cleans up staging and releases the lock", func(t *testcase.T) {
							original := storage.Get(t).readFile(t, "setting")
							assert.ErrorIs(t, act(t), fs.ErrPermission)
							assert.Equal(t, storage.Get(t).readFile(t, "setting"), original)
							assert.Equal(t, storage.Get(t).names(t), []string{"setting"})
							faults.Get(t).renameErr = nil
							assert.Within(t, time.Second, func(ctx context.Context) {
								assert.NoError(t, subject.Get(t).Save(ctx, ent.Get(t)))
							})
						})
					})
				})
			})
		})
	}
}

func TestRepository_FSJail(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		root = let.Var(s, func(t *testcase.T) string { return t.TempDir() })
		fsys = let.Var[localfs.RepositoryFileSystem](s, func(t *testcase.T) localfs.RepositoryFileSystem {
			return localfs.FileSystem{RootPath: root.Get(t)}
		})
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			return &localfs.Repository[testent.Foo, testent.FooID]{
				FS: fsys.Get(t), Dir: filepath.Join("nested", "entities"), FileExt: "json",
			}
		})
		ctx = let.Context(s)
		ent = let.Var(s, func(t *testcase.T) *testent.Foo {
			return &testent.Foo{ID: "setting", Foo: t.Random.StringN(40)}
		})
	)

	check := func(s *testcase.Spec) {
		confinement := func(s *testcase.Spec) {
			outside := let.Var(s, func(t *testcase.T) string {
				data, err := jsonkit.Codec{}.Marshal(*ent.Get(t))
				assert.NoError(t, err)
				return string(data)
			})
			s.Before(func(t *testcase.T) {
				assert.NoError(t, filesystem.MkdirAll(fsys.Get(t), subject.Get(t).Dir, 0o700))
				assert.NoError(t, filesystem.WriteFile(fsys.Get(t), "nested/outside.json", []byte(outside.Get(t)), 0o600))
				jailSymlink(t, "../outside.json", filepath.Join(root.Get(t), subject.Get(t).Dir, "setting.json"))
				data, err := filesystem.ReadFile(fsys.Get(t), filepath.Join(subject.Get(t).Dir, "setting.json"))
				assert.NoError(t, err, "the symlink is inside the FS root, but outside the repository Dir")
				assert.Equal(t, string(data), outside.Get(t))
			})

			assertConfined := func(t *testcase.T, err error) {
				t.Helper()
				assert.Error(t, err)
				data, err := filesystem.ReadFile(fsys.Get(t), "nested/outside.json")
				assert.NoError(t, err)
				assert.Equal(t, string(data), outside.Get(t))
			}

			s.Describe("#FindByID through an entity symlink", func(s *testcase.Spec) {
				act := func(t *testcase.T) (testent.Foo, bool, error) {
					return subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
				}
				s.Then("cannot read beyond Dir even within the broader FS jail", func(t *testcase.T) {
					got, found, err := act(t)
					assertConfined(t, err)
					assert.False(t, found)
					assert.Empty(t, got)
				})
			})

			s.Describe("#FindAll with an entity symlink", func(s *testcase.Spec) {
				act := func(t *testcase.T) ([]testent.Foo, error) {
					return iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
				}
				s.Then("skips the symlink rather than exposing an entity beyond Dir", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Empty(t, got)
				})
			})

			s.Describe("#Save through an entity symlink", func(s *testcase.Spec) {
				act := func(t *testcase.T) error { return subject.Get(t).Save(ctx.Get(t), ent.Get(t)) }
				s.Then("cannot replace an entity beyond Dir", func(t *testcase.T) {
					ent.Get(t).Foo = t.Random.StringN(40)
					assertConfined(t, act(t))
				})
			})

			s.Describe("#Update through an entity symlink", func(s *testcase.Spec) {
				act := func(t *testcase.T) error { return subject.Get(t).Update(ctx.Get(t), ent.Get(t)) }
				s.Then("cannot update an entity beyond Dir", func(t *testcase.T) {
					ent.Get(t).Foo = t.Random.StringN(40)
					assertConfined(t, act(t))
				})
			})
		}

		s.Context("without a transaction", confinement)
		s.When("using a transaction", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				tx, err := subject.Get(t).BeginTx(ctx.Super(t))
				assert.NoError(t, err)
				t.Cleanup(func() { _ = subject.Get(t).RollbackTx(tx) })
				return tx
			})
			confinement(s)
		})

		s.Describe("#CommitTx after Dir is renamed", func(s *testcase.Spec) {
			tx := let.Var(s, func(t *testcase.T) context.Context {
				tx, err := subject.Get(t).BeginTx(ctx.Get(t))
				assert.NoError(t, err)
				t.Cleanup(func() { _ = subject.Get(t).RollbackTx(tx) })
				return tx
			})
			act := func(t *testcase.T) error { return subject.Get(t).CommitTx(tx.Get(t)) }

			s.Then("keeps its opened repository root until the transaction ends", func(t *testcase.T) {
				if runtime.GOOS == "windows" {
					t.Skip("Windows may prevent renaming a directory while its lock file is open")
				}
				assert.NoError(t, subject.Get(t).Save(tx.Get(t), ent.Get(t)))
				assert.NoError(t, fsys.Get(t).Rename(subject.Get(t).Dir, "nested/moved"))
				assert.NoError(t, fsys.Get(t).Mkdir(subject.Get(t).Dir, 0o700))

				assert.NoError(t, act(t))
				data, err := filesystem.ReadFile(fsys.Get(t), "nested/moved/setting.json")
				assert.NoError(t, err)
				var got testent.Foo
				assert.NoError(t, jsonkit.Codec{}.Unmarshal(data, &got))
				assert.Equal(t, got, *ent.Get(t))
				entries, err := filesystem.ReadDir(fsys.Get(t), subject.Get(t).Dir)
				assert.NoError(t, err)
				assert.Empty(t, entries, "commit must not follow a replacement at the old Dir path")
			})
		})
	}

	s.Context("jailed localfs value", check)
	s.Context("jailed localfs pointer", func(s *testcase.Spec) {
		fsys.Let(s, func(t *testcase.T) localfs.RepositoryFileSystem {
			return &localfs.FileSystem{RootPath: root.Get(t)}
		})
		check(s)
	})
}

func TestRepository_CreateSymlink(t *testing.T) {
	s := testcase.NewSpec(t)
	root := let.Var(s, func(t *testcase.T) string { return t.TempDir() })
	subject := let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
		return &localfs.Repository[testent.Foo, testent.FooID]{Dir: root.Get(t)}
	})
	ctx := let.Context(s)
	ent := let.Var(s, func(t *testcase.T) *testent.Foo {
		return &testent.Foo{ID: "setting", Foo: t.Random.String()}
	})
	act := func(t *testcase.T) error { return subject.Get(t).Create(ctx.Get(t), ent.Get(t)) }

	s.Then("creates a new entity when its directory entry is absent", func(t *testcase.T) {
		assert.NoError(t, act(t))
	})

	s.When("the entity name is occupied by a dangling symlink", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			jailSymlink(t, "missing", filepath.Join(root.Get(t), "setting"))
		})
		check := func(t *testcase.T) {
			assert.ErrorIs(t, act(t), crud.ErrAlreadyExists)
			target, err := os.Readlink(filepath.Join(root.Get(t), "setting"))
			assert.NoError(t, err)
			assert.Equal(t, "missing", target)
			assert.Equal(t, []string{"setting"}, dirEntryNames(t, root.Get(t)))
		}
		s.Then("preserves the existing entry and reports a conflict", check)
		s.And("the create is transactional", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				tx, err := subject.Get(t).BeginTx(ctx.Super(t))
				assert.NoError(t, err)
				t.Cleanup(func() { _ = subject.Get(t).RollbackTx(tx) })
				return tx
			})
			s.Then("preserves the existing entry and reports a conflict", check)
		})
	})
}

// Decorate real backends so failure coverage still exercises actual file storage and cleanup.
type repositoryFaultFS struct {
	localfs.RepositoryFileSystem
	openErr, readErr, writeErr, closeErr, renameErr error
}

func (f *repositoryFaultFS) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	if f.openErr != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: f.openErr}
	}
	file, err := f.RepositoryFileSystem.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if f.readErr != nil {
		return repositoryReadErrorFile{File: file, err: f.readErr}, nil
	}
	if f.writeErr != nil || f.closeErr != nil {
		return repositoryWriteErrorFile{File: file, writeErr: f.writeErr, closeErr: f.closeErr}, nil
	}
	return file, nil
}

func (f *repositoryFaultFS) Rename(oldpath, newpath string) error {
	if f.renameErr != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: f.renameErr}
	}
	return f.RepositoryFileSystem.Rename(oldpath, newpath)
}

type repositoryReadErrorFile struct {
	filesystem.File
	err error
}

func (f repositoryReadErrorFile) Read([]byte) (int, error) { return 0, f.err }

type repositoryWriteErrorFile struct {
	filesystem.File
	writeErr, closeErr error
}

func (f repositoryWriteErrorFile) Write(p []byte) (int, error) {
	if f.writeErr == nil {
		return f.File.Write(p)
	}
	n, err := f.File.Write(p[:len(p)/2])
	return n, errors.Join(err, f.writeErr)
}

func (f repositoryWriteErrorFile) Close() error {
	return errors.Join(f.File.Close(), f.closeErr)
}
