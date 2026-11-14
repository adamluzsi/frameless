package fileio_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"testing"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestListFiles(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		backend = let.Var(s, func(t *testcase.T) *memory.FileSystem { return &memory.FileSystem{} })
		root    = let.Var(s, func(t *testcase.T) *faultFileSystem {
			return &faultFileSystem{FileSystem: backend.Get(t), file: &faultFile{}}
		})
		suffix = let.Var(s, func(t *testcase.T) string { return ".json" })
	)
	act := func(t *testcase.T) ([]string, error) {
		return fileio.ListFiles(struct{ filesystem.FileOpener }{root.Get(t)}, suffix.Get(t))
	}

	s.Then("returns only matching visible regular files in name order", func(t *testcase.T) {
		for _, name := range []string{"z.json", "a.json", ".hidden.json", "other.txt"} {
			assert.NoError(t, filesystem.WriteFile(backend.Get(t), name, []byte(name), 0o600))
		}
		assert.NoError(t, backend.Get(t).Mkdir("directory.json", 0o700))
		assert.NoError(t, filesystem.WriteFile(backend.Get(t), "directory.json/nested.json", nil, 0o600))

		names, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, names, []string{"a.json", "z.json"})
		assert.Equal(t, root.Get(t).openName, ".")
		assert.Equal(t, root.Get(t).file.closeCalls, 1)
	})

	s.Then("returns no names for an empty directory", func(t *testcase.T) {
		names, err := act(t)
		assert.NoError(t, err)
		assert.Empty(t, names)
	})

	s.When("the suffix is empty", func(s *testcase.Spec) {
		suffix.LetValue(s, "")

		s.Then("includes visible files regardless of extension", func(t *testcase.T) {
			for _, name := range []string{"file", "file.json", ".hidden"} {
				assert.NoError(t, filesystem.WriteFile(backend.Get(t), name, nil, 0o600))
			}
			names, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, names, []string{"file", "file.json"})
		})
	})

	s.When("opening the directory fails", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { root.Get(t).openErr = fs.ErrPermission })

		s.Then("returns the open error", func(t *testcase.T) {
			names, err := act(t)
			assert.ErrorIs(t, err, fs.ErrPermission)
			assert.Empty(t, names)
		})
	})

	s.When("reading the directory fails", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { root.Get(t).file.readDirErr = fs.ErrPermission })

		s.Then("returns the read error and closes the directory", func(t *testcase.T) {
			names, err := act(t)
			assert.ErrorIs(t, err, fs.ErrPermission)
			assert.Empty(t, names)
			assert.Equal(t, root.Get(t).file.closeCalls, 1)
		})
	})
}

func TestCreateFile(t *testing.T) {
	s := testcase.NewSpec(t)

	v := letFileWrite(s)
	act := func(t *testcase.T) error {
		root := v.root.Get(t)
		fsys := struct {
			filesystem.FileOpener
			filesystem.FileInfoReader
			filesystem.Remover
			filesystem.Renamer
		}{root, root, root, root}
		return fileio.CreateFile(fsys, v.name.Get(t), v.data.Get(t), v.perm.Get(t))
	}

	s.Then("creates a complete file with the requested mode and no temporary files", func(t *testcase.T) {
		assert.NoError(t, act(t))
		v.assertWritten(t)
	})

	s.Then("keeps the new file hidden until its contents are closed and renamed", func(t *testcase.T) {
		v.root.Get(t).beforeRename = func(oldName, newName string) {
			_, err := v.backend.Get(t).Stat(v.name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
			v.assertReadyToRename(t, oldName, newName)
			names, err := fileio.ListFiles(v.backend.Get(t), "")
			assert.NoError(t, err)
			assert.Empty(t, names)
		}
		assert.NoError(t, act(t))
		assert.Equal(t, v.root.Get(t).renameCalls, 1)
		v.assertWritten(t)
	})

	s.When("the name already belongs to a file", func(s *testcase.Spec) {
		original := let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(128)) })
		s.Before(func(t *testcase.T) {
			assert.NoError(t, filesystem.WriteFile(v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400))
		})

		s.Then("returns fs.ErrExist without opening a temporary file or changing the original", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrExist)
			assertStoredFile(t, v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400)
			assert.Empty(t, v.root.Get(t).openName)
			assert.Empty(t, v.root.Get(t).removed)
			assert.Equal(t, directoryNames(t, v.backend.Get(t)), []string{v.name.Get(t)})
		})
	})

	s.When("the name already belongs to a directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { assert.NoError(t, v.backend.Get(t).Mkdir(v.name.Get(t), 0o700)) })

		s.Then("returns fs.ErrExist without disturbing the directory", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrExist)
			info, err := v.backend.Get(t).Stat(v.name.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Empty(t, v.root.Get(t).openName)
			assert.Equal(t, directoryNames(t, v.backend.Get(t)), []string{v.name.Get(t)})
		})
	})

	s.When("checking for an existing entry fails", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { v.root.Get(t).statErr = fs.ErrPermission })

		s.Then("returns the stat error without writing anything", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrPermission)
			assert.Empty(t, v.root.Get(t).openName)
			assert.Empty(t, directoryNames(t, v.backend.Get(t)))
		})
	})

	s.When("the file supports chmod but creation does not require exact permissions", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			v.root.Get(t).wrap = func(f filesystem.File) filesystem.File {
				return chmodFile{File: f, chmod: func(fs.FileMode) error { return fs.ErrPermission }}
			}
		})

		s.Then("does not require chmod to publish the file", func(t *testcase.T) {
			assert.NoError(t, act(t))
			v.assertWritten(t)
		})
	})

	s.Describe("with Lstat support", func(s *testcase.Spec) {
		root := let.Var(s, func(t *testcase.T) *nativeFileSystem {
			r, err := os.OpenRoot(t.TempDir())
			assert.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, r.Close()) })
			return &nativeFileSystem{Root: r}
		})
		act := func(t *testcase.T) error {
			return fileio.CreateFile(root.Get(t), v.name.Get(t), v.data.Get(t), v.perm.Get(t))
		}

		s.Then("creates an absent destination without leaving temporary files", func(t *testcase.T) {
			assert.NoError(t, act(t))
			data, err := filesystem.ReadFile(root.Get(t), v.name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, data, v.data.Get(t))
			assert.Equal(t, directoryNames(t, root.Get(t)), []string{v.name.Get(t)})
		})

		for _, tc := range []struct {
			desc         string
			targetExists bool
		}{
			{desc: "the destination is an ordinary symlink", targetExists: true},
			{desc: "the destination is a dangling symlink", targetExists: false},
		} {
			s.When(tc.desc, func(s *testcase.Spec) {
				target := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() + ".target" })
				original := let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(128)) })
				s.Before(func(t *testcase.T) {
					if tc.targetExists {
						assert.NoError(t, filesystem.WriteFile(root.Get(t), target.Get(t), original.Get(t), 0o600))
					}
					err := root.Get(t).Symlink(target.Get(t), v.name.Get(t))
					if runtime.GOOS == "windows" && os.IsPermission(err) {
						t.Skipf("symlinks require Windows Developer Mode or elevated privileges: %v", err)
					}
					assert.NoError(t, err)
				})

				s.Then("returns fs.ErrExist without changing the symlink or its target", func(t *testcase.T) {
					before, err := root.Get(t).Lstat(v.name.Get(t))
					assert.NoError(t, err)

					assert.ErrorIs(t, act(t), fs.ErrExist)

					after, err := root.Get(t).Lstat(v.name.Get(t))
					assert.NoError(t, err)
					assert.Equal(t, after.Mode().Type(), fs.ModeSymlink)
					assert.True(t, os.SameFile(before, after))
					link, err := root.Get(t).Readlink(v.name.Get(t))
					assert.NoError(t, err)
					assert.Equal(t, link, target.Get(t))
					names := []string{v.name.Get(t)}
					if tc.targetExists {
						data, err := filesystem.ReadFile(root.Get(t), target.Get(t))
						assert.NoError(t, err)
						assert.Equal(t, data, original.Get(t))
						names = append(names, target.Get(t))
					} else {
						_, err := root.Get(t).Stat(target.Get(t))
						assert.ErrorIs(t, err, fs.ErrNotExist)
					}
					assert.ContainsExactly(t, names, directoryNames(t, root.Get(t)))
				})
			})
		}

		s.When("Lstat fails", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				root.Get(t).lstatErr = &fs.PathError{Op: "lstat", Path: v.name.Get(t), Err: fs.ErrPermission}
			})

			s.Then("returns the error without falling back to Stat or writing a file", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), root.Get(t).lstatErr)
				assert.Empty(t, directoryNames(t, root.Get(t)))
			})
		})
	})

	specWriteFailures(s, v, act, false, true, func(t *testcase.T) {
		assert.Empty(t, directoryNames(t, v.backend.Get(t)))
	})
}

func TestReplaceFile(t *testing.T) {
	s := testcase.NewSpec(t)

	v := letFileWrite(s)
	original := let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(128)) })
	exists := let.Var(s, func(t *testcase.T) bool { return true })
	act := func(t *testcase.T) error {
		root := v.root.Get(t)
		fsys := struct {
			filesystem.FileOpener
			filesystem.Remover
			filesystem.Renamer
		}{root, root, root}
		return fileio.ReplaceFile(fsys, v.name.Get(t), v.data.Get(t), v.perm.Get(t))
	}

	s.Before(func(t *testcase.T) {
		if exists.Get(t) {
			assert.NoError(t, filesystem.WriteFile(v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400))
		}
	})

	s.Then("replaces the contents and mode without leaving temporary files", func(t *testcase.T) {
		assert.NoError(t, act(t))
		v.assertWritten(t)
	})

	s.Then("leaves the original intact until the complete replacement is closed and renamed", func(t *testcase.T) {
		v.root.Get(t).beforeRename = func(oldName, newName string) {
			assertStoredFile(t, v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400)
			v.assertReadyToRename(t, oldName, newName)
			names, err := fileio.ListFiles(v.backend.Get(t), "")
			assert.NoError(t, err)
			assert.Equal(t, names, []string{v.name.Get(t)})
		}
		assert.NoError(t, act(t))
		assert.Equal(t, v.root.Get(t).renameCalls, 1)
		v.assertWritten(t)
	})

	s.When("the destination does not exist", func(s *testcase.Spec) {
		exists.LetValue(s, false)

		s.Then("creates the file", func(t *testcase.T) {
			assert.NoError(t, act(t))
			v.assertWritten(t)
		})
	})

	specExactPerm(s, v, act)
	specWriteFailures(s, v, act, true, true, func(t *testcase.T) {
		assertStoredFile(t, v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400)
		assert.Equal(t, directoryNames(t, v.backend.Get(t)), []string{v.name.Get(t)})
	})
}

func TestWriteNewFile(t *testing.T) {
	s := testcase.NewSpec(t)

	v := letFileWrite(s)
	exactPerm := let.Var(s, func(t *testcase.T) bool { return true })
	act := func(t *testcase.T) error {
		root := v.root.Get(t)
		fsys := struct {
			filesystem.FileOpener
			filesystem.Remover
		}{root, root}
		return fileio.WriteNewFile(fsys, v.name.Get(t), v.data.Get(t), v.perm.Get(t), exactPerm.Get(t))
	}

	s.Then("exclusively creates, writes and closes a file without optional capabilities", func(t *testcase.T) {
		assert.NoError(t, act(t))
		v.assertWritten(t)
		assert.Equal(t, v.root.Get(t).openName, v.name.Get(t))
		assert.Equal(t, v.root.Get(t).openFlag, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		assert.Equal(t, v.root.Get(t).openPerm, v.perm.Get(t))
		assert.Equal(t, v.root.Get(t).renameCalls, 0)
		assert.Empty(t, v.root.Get(t).removed)
	})

	s.When("the data is empty", func(s *testcase.Spec) {
		v.data.LetValue(s, nil)

		s.Then("creates an empty file", func(t *testcase.T) {
			assert.NoError(t, act(t))
			data, err := filesystem.ReadFile(v.backend.Get(t), v.name.Get(t))
			assert.NoError(t, err)
			assert.Empty(t, data)
			assert.Equal(t, v.root.Get(t).file.closeCalls, 1)
		})
	})

	s.When("the file already exists", func(s *testcase.Spec) {
		original := let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(128)) })
		s.Before(func(t *testcase.T) {
			assert.NoError(t, filesystem.WriteFile(v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400))
		})

		s.Then("returns fs.ErrExist without replacing or removing it", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrExist)
			assertStoredFile(t, v.backend.Get(t), v.name.Get(t), original.Get(t), 0o400)
			assert.Empty(t, v.root.Get(t).removed)
			assert.Equal(t, v.root.Get(t).file.closeCalls, 0)
		})
	})

	s.When("only sync is supported", func(s *testcase.Spec) {
		calls := let.Var(s, func(t *testcase.T) int { return 0 })
		s.Before(func(t *testcase.T) {
			v.root.Get(t).wrap = func(f filesystem.File) filesystem.File {
				return syncFile{File: f, sync: func() error {
					calls.Set(t, calls.Get(t)+1)
					_, err := f.Stat()
					return err
				}}
			}
		})

		s.Then("syncs while the file is still open", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), 1)
			v.assertWritten(t)
		})
	})

	s.When("exact permissions are not requested", func(s *testcase.Spec) {
		exactPerm.LetValue(s, false)
		s.Before(func(t *testcase.T) {
			v.root.Get(t).wrap = func(f filesystem.File) filesystem.File {
				return chmodFile{File: f, chmod: func(fs.FileMode) error { return fs.ErrPermission }}
			}
		})

		s.Then("does not call chmod", func(t *testcase.T) {
			assert.NoError(t, act(t))
			v.assertWritten(t)
		})
	})

	specExactPerm(s, v, act)
	specWriteFailures(s, v, act, true, false, func(t *testcase.T) {
		assert.Empty(t, directoryNames(t, v.backend.Get(t)))
	})
}

type fileWriteVars struct {
	backend testcase.Var[*memory.FileSystem]
	root    testcase.Var[*faultFileSystem]
	name    testcase.Var[string]
	data    testcase.Var[[]byte]
	perm    testcase.Var[fs.FileMode]
}

func letFileWrite(s *testcase.Spec) fileWriteVars {
	backend := let.Var(s, func(t *testcase.T) *memory.FileSystem { return &memory.FileSystem{} })
	return fileWriteVars{
		backend: backend,
		root: let.Var(s, func(t *testcase.T) *faultFileSystem {
			return &faultFileSystem{FileSystem: backend.Get(t), file: &faultFile{}}
		}),
		name: let.Var(s, func(t *testcase.T) string { return t.Random.UUID() + ".json" }),
		data: let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(64)) }),
		perm: let.Var(s, func(t *testcase.T) fs.FileMode { return 0o640 }),
	}
}

func (v fileWriteVars) assertWritten(t *testcase.T) {
	t.Helper()
	assertStoredFile(t, v.backend.Get(t), v.name.Get(t), v.data.Get(t), v.perm.Get(t))
	assert.Equal(t, directoryNames(t, v.backend.Get(t)), []string{v.name.Get(t)})
	assert.Equal(t, v.root.Get(t).file.closeCalls, 1)
	_, err := v.root.Get(t).file.File.Stat()
	assert.ErrorIs(t, err, fs.ErrClosed)
}

func (v fileWriteVars) assertReadyToRename(t *testcase.T, oldName, newName string) {
	t.Helper()
	assert.Equal(t, newName, v.name.Get(t))
	assert.MatchRegexp(t, oldName, `^\..+\.tmp-.+$`)
	assertStoredFile(t, v.backend.Get(t), oldName, v.data.Get(t), v.perm.Get(t))
	assert.Equal(t, v.root.Get(t).file.closeCalls, 1)
	_, err := v.root.Get(t).file.File.Stat()
	assert.ErrorIs(t, err, fs.ErrClosed)
}

func specExactPerm(s *testcase.Spec, v fileWriteVars, act func(*testcase.T) error) {
	s.When("only chmod is supported", func(s *testcase.Spec) {
		modes := let.Var(s, func(t *testcase.T) []fs.FileMode { return nil })
		s.Before(func(t *testcase.T) {
			v.root.Get(t).wrap = func(f filesystem.File) filesystem.File {
				return chmodFile{File: f, chmod: func(mode fs.FileMode) error {
					modes.Set(t, append(modes.Get(t), mode))
					_, err := f.Stat()
					return err
				}}
			}
		})

		s.Then("applies the requested permission before closing the file", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, modes.Get(t), []fs.FileMode{v.perm.Get(t)})
			v.assertWritten(t)
		})
	})
}

func specWriteFailures(s *testcase.Spec, v fileWriteVars, act func(*testcase.T) error, exactPerm, rename bool, unchanged func(*testcase.T)) {
	failures := []string{"open", "short write", "write", "sync", "close"}
	if exactPerm {
		failures = append(failures, "chmod")
	}
	if rename {
		failures = append(failures, "rename")
	}
	for _, failure := range failures {
		s.When(failure+" fails", func(s *testcase.Spec) {
			expected := let.Var(s, func(t *testcase.T) error {
				if failure == "short write" {
					return io.ErrShortWrite
				}
				return errors.New("injected " + failure + " error")
			})
			s.Before(func(t *testcase.T) {
				root := v.root.Get(t)
				switch failure {
				case "open":
					root.openErr = expected.Get(t)
				case "short write":
					root.file.shortWrite = true
				case "write":
					root.file.writeErr = expected.Get(t)
				case "chmod":
					root.wrap = func(f filesystem.File) filesystem.File {
						return chmodFile{File: f, chmod: func(fs.FileMode) error { return expected.Get(t) }}
					}
				case "sync":
					root.wrap = func(f filesystem.File) filesystem.File {
						return syncFile{File: f, sync: func() error { return expected.Get(t) }}
					}
				case "close":
					root.file.closeErr = expected.Get(t)
				case "rename":
					root.renameErr = expected.Get(t)
				}
			})

			s.Then("returns the error, closes before cleanup and leaves the destination unchanged", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expected.Get(t))
				unchanged(t)
				root := v.root.Get(t)
				if failure == "open" {
					assert.Equal(t, root.file.closeCalls, 0)
					assert.Empty(t, root.removed)
				} else {
					assert.Equal(t, root.file.closeCalls, 1)
					_, err := root.file.File.Stat()
					assert.ErrorIs(t, err, fs.ErrClosed)
					assert.Equal(t, root.removed, []string{root.openName})
				}
				if failure == "rename" {
					assert.Equal(t, root.renameCalls, 1)
				} else {
					assert.Equal(t, root.renameCalls, 0)
				}
			})
		})
	}

	s.When("both writing and closing fail", func(s *testcase.Spec) {
		writeErr := let.Error(s)
		closeErr := let.Error(s)
		s.Before(func(t *testcase.T) {
			v.root.Get(t).file.writeErr = writeErr.Get(t)
			v.root.Get(t).file.closeErr = closeErr.Get(t)
		})

		s.Then("preserves both errors and still cleans up the closed file", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, writeErr.Get(t))
			assert.ErrorIs(t, err, closeErr.Get(t))
			assert.Equal(t, v.root.Get(t).file.closeCalls, 1)
			unchanged(t)
		})
	})
}

func assertStoredFile(t testing.TB, root interface {
	filesystem.FileOpener
	filesystem.FileInfoReader
}, name string, data []byte, perm fs.FileMode) {
	t.Helper()
	got, err := filesystem.ReadFile(root, name)
	assert.NoError(t, err)
	assert.Equal(t, got, data)
	info, err := root.Stat(name)
	assert.NoError(t, err)
	assert.Equal(t, info.Mode().Perm(), perm.Perm())
}

func directoryNames(t testing.TB, root filesystem.FileOpener) []string {
	t.Helper()
	entries, err := filesystem.ReadDir(root, ".")
	assert.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

type faultFileSystem struct {
	fileio.FileSystem
	file         *faultFile
	wrap         func(filesystem.File) filesystem.File
	openErr      error
	statErr      error
	renameErr    error
	beforeRename func(string, string)
	openName     string
	openFlag     int
	openPerm     fs.FileMode
	renameCalls  int
	removed      []string
}

func (root *faultFileSystem) Stat(name string) (fs.FileInfo, error) {
	if root.statErr != nil {
		return nil, root.statErr
	}
	return root.FileSystem.Stat(name)
}

func (root *faultFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	root.openName, root.openFlag, root.openPerm = name, flag, perm
	if root.openErr != nil {
		return nil, root.openErr
	}
	f, err := root.FileSystem.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	root.file.File = f
	if root.wrap != nil {
		return root.wrap(root.file), nil
	}
	return root.file, nil
}

func (root *faultFileSystem) Rename(oldName, newName string) error {
	root.renameCalls++
	if root.beforeRename != nil {
		root.beforeRename(oldName, newName)
	}
	if root.renameErr != nil {
		return root.renameErr
	}
	return root.FileSystem.Rename(oldName, newName)
}

func (root *faultFileSystem) Remove(name string) error {
	root.removed = append(root.removed, name)
	// Model filesystems that cannot remove an open file, even when Close reports an error.
	if root.file.File != nil {
		if _, err := root.file.File.Stat(); !errors.Is(err, fs.ErrClosed) {
			return fs.ErrPermission
		}
	}
	return root.FileSystem.Remove(name)
}

type faultFile struct {
	filesystem.File
	shortWrite bool
	writeErr   error
	closeErr   error
	readDirErr error
	closeCalls int
}

func (f *faultFile) Write(p []byte) (int, error) {
	if f.shortWrite || f.writeErr != nil {
		n, err := f.File.Write(p[:len(p)/2])
		return n, errors.Join(err, f.writeErr)
	}
	return f.File.Write(p)
}

func (f *faultFile) Close() error {
	f.closeCalls++
	return errors.Join(f.File.Close(), f.closeErr)
}

func (f *faultFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.readDirErr != nil {
		return nil, f.readDirErr
	}
	return f.File.ReadDir(n)
}

type chmodFile struct {
	filesystem.File
	chmod func(fs.FileMode) error
}

func (f chmodFile) Chmod(mode fs.FileMode) error { return f.chmod(mode) }

type syncFile struct {
	filesystem.File
	sync func() error
}

func (f syncFile) Sync() error { return f.sync() }

type nativeFileSystem struct {
	*os.Root
	lstatErr error
}

func (root *nativeFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	return root.Root.OpenFile(name, flag, perm)
}

func (root *nativeFileSystem) Lstat(name string) (fs.FileInfo, error) {
	if root.lstatErr != nil {
		return nil, root.lstatErr
	}
	return root.Root.Lstat(name)
}
