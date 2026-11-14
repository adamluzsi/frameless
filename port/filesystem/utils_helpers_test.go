package filesystem_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestReadFile(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		backend = let.Var(s, func(t *testcase.T) *memory.FileSystem { return makeFS(t) })
		file    = let.Var(s, func(t *testcase.T) *helperFile { return &helperFile{} })
		subject = let.Var(s, func(t *testcase.T) *helperFileSystem {
			return &helperFileSystem{FileSystem: backend.Get(t), file: file.Get(t)}
		})
		name    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		content = let.Var(s, func(t *testcase.T) []byte {
			return []byte(t.Random.StringN(64))
		})
		exists = let.Var(s, func(t *testcase.T) bool { return true })
	)
	act := func(t *testcase.T) ([]byte, error) {
		return filesystem.ReadFile(subject.Get(t), name.Get(t))
	}

	s.Before(func(t *testcase.T) {
		if exists.Get(t) {
			helperWriteFile(t, backend.Get(t), name.Get(t), content.Get(t), filemode.UserRW)
		}
	})

	s.Then("returns the whole content without reporting EOF", func(t *testcase.T) {
		got, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, got, content.Get(t))
	})

	s.Then("closes the opened file before returning", func(t *testcase.T) {
		_, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, file.Get(t).closeCalls, 1)
		_, err = file.Get(t).File.Read(make([]byte, 1))
		assert.ErrorIs(t, err, fs.ErrClosed)
	})

	s.When("the file is empty", func(s *testcase.Spec) {
		content.LetValue(s, nil)

		s.Then("returns empty content without an error", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Empty(t, got)
			assert.NotNil(t, got)
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})

	s.When("the content exceeds the initial read buffer", func(s *testcase.Spec) {
		content.Let(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(8192)) })

		s.Then("reads all of it", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, content.Get(t))
		})
	})

	s.When("the file does not exist", func(s *testcase.Spec) {
		exists.LetValue(s, false)

		s.Then("returns fs.ErrNotExist and no content", func(t *testcase.T) {
			got, err := act(t)
			assert.ErrorIs(t, err, fs.ErrNotExist)
			assert.Nil(t, got)
			assert.Equal(t, file.Get(t).closeCalls, 0)
		})
	})

	s.When("opening the file fails", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			fsys := subject.Super(t)
			fsys.openErr = &fs.PathError{Op: "open", Path: name.Get(t), Err: fs.ErrPermission}
			return fsys
		})

		s.Then("returns the open error without reading or closing a file", func(t *testcase.T) {
			got, err := act(t)
			assert.ErrorIs(t, err, subject.Get(t).openErr)
			assert.Nil(t, got)
			assert.Equal(t, file.Get(t).closeCalls, 0)
		})
	})

	s.When("the name points to a directory", func(s *testcase.Spec) {
		exists.LetValue(s, false)
		s.Before(func(t *testcase.T) {
			assert.NoError(t, backend.Get(t).Mkdir(name.Get(t), filemode.UserRWX))
		})

		s.Then("returns a read error and closes the directory", func(t *testcase.T) {
			_, err := act(t)
			assert.Error(t, err)
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})

	s.When("reading fails after returning some bytes", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.read = func(p []byte) (int, error) {
				return copy(p, content.Get(t)[:8]), io.ErrUnexpectedEOF
			}
			return f
		})

		s.Then("returns the partial content and read error, and closes the file", func(t *testcase.T) {
			got, err := act(t)
			assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
			assert.Equal(t, got, content.Get(t)[:8])
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})

		s.And("closing also fails", func(s *testcase.Spec) {
			file.Let(s, func(t *testcase.T) *helperFile {
				f := file.Super(t)
				f.closeErr = errors.New("close failed")
				return f
			})

			s.Then("keeps the read error and partial content", func(t *testcase.T) {
				got, err := act(t)
				assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
				assert.False(t, errors.Is(err, file.Get(t).closeErr))
				assert.Equal(t, got, content.Get(t)[:8])
				assert.Equal(t, file.Get(t).closeCalls, 1)
			})
		})
	})

	s.When("the final read returns data together with EOF", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.read = func(p []byte) (int, error) { return copy(p, content.Get(t)), io.EOF }
			return f
		})

		s.Then("includes the final bytes without reporting EOF", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, content.Get(t))
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})

	s.When("only closing fails", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.closeErr = errors.New("close failed")
			return f
		})

		s.Then("ignores the close error like os.ReadFile", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, content.Get(t))
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})
}

func TestWriteFile(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		backend = let.Var(s, func(t *testcase.T) *memory.FileSystem { return makeFS(t) })
		file    = let.Var(s, func(t *testcase.T) *helperFile { return &helperFile{} })
		subject = let.Var(s, func(t *testcase.T) *helperFileSystem {
			return &helperFileSystem{FileSystem: backend.Get(t), file: file.Get(t)}
		})
		name    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		content = let.Var(s, func(t *testcase.T) []byte {
			return []byte(t.Random.StringN(64))
		})
		perm = let.Var(s, func(t *testcase.T) fs.FileMode { return filemode.UserRW })
	)
	act := func(t *testcase.T) error {
		return filesystem.WriteFile(subject.Get(t), name.Get(t), content.Get(t), perm.Get(t))
	}

	s.Then("creates the file with the content and requested permissions", func(t *testcase.T) {
		assert.NoError(t, act(t))
		assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
		info, err := backend.Get(t).Stat(name.Get(t))
		assert.NoError(t, err)
		assert.Equal(t, info.Mode().Perm(), perm.Get(t))
	})

	s.Then("closes the opened file before returning", func(t *testcase.T) {
		assert.NoError(t, act(t))
		assert.Equal(t, file.Get(t).closeCalls, 1)
		_, err := file.Get(t).File.Write([]byte("more"))
		assert.ErrorIs(t, err, fs.ErrClosed)
	})

	s.When("the content is empty", func(s *testcase.Spec) {
		content.LetValue(s, nil)

		s.Then("creates an empty file and closes it", func(t *testcase.T) {
			assert.NoError(t, act(t))
			info, err := backend.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, info.Size(), int64(0))
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})

	s.When("the file already exists", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			helperWriteFile(t, backend.Get(t), name.Get(t), []byte(t.Random.StringN(128)), filemode.UserRWX)
		})

		s.Then("replaces its content without leaving a tail", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
		})

		s.Then("keeps the existing permissions", func(t *testcase.T) {
			assert.NoError(t, act(t))
			info, err := backend.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, info.Mode().Perm(), filemode.UserRWX)
		})

		s.And("the replacement content is empty", func(s *testcase.Spec) {
			content.LetValue(s, nil)

			s.Then("truncates the existing file to zero bytes", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Empty(t, helperReadFile(t, backend.Get(t), name.Get(t)))
			})
		})
	})

	s.When("the parent directory does not exist", func(s *testcase.Spec) {
		name.Let(s, func(t *testcase.T) string { return filepath.Join("missing", name.Super(t)) })

		s.Then("returns fs.ErrNotExist without creating the parent", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrNotExist)
			_, err := backend.Get(t).Stat(filepath.Dir(name.Get(t)))
			assert.ErrorIs(t, err, fs.ErrNotExist)
			assert.Equal(t, file.Get(t).closeCalls, 0)
		})
	})

	s.When("the name points to a directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.NoError(t, backend.Get(t).Mkdir(name.Get(t), filemode.UserRWX))
		})

		s.Then("fails without replacing the directory", func(t *testcase.T) {
			assert.Error(t, act(t))
			info, err := backend.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Equal(t, file.Get(t).closeCalls, 0)
		})
	})

	s.When("opening the file fails", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			fsys := subject.Super(t)
			fsys.openErr = &fs.PathError{Op: "open", Path: name.Get(t), Err: fs.ErrPermission}
			return fsys
		})

		s.Then("returns the open error without writing or closing a file", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), subject.Get(t).openErr)
			assert.Equal(t, file.Get(t).writeCalls, 0)
			assert.Equal(t, file.Get(t).closeCalls, 0)
			_, err := backend.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("writing fails after writing some bytes", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.write = func(p []byte) (int, error) {
				n, err := f.File.Write(p[:8])
				assert.NoError(t, err)
				return n, io.ErrClosedPipe
			}
			return f
		})

		s.Then("returns the write error, leaves the partial content and closes the file", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), io.ErrClosedPipe)
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t)[:8])
			assert.Equal(t, file.Get(t).writeCalls, 1)
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})

		s.And("closing also fails", func(s *testcase.Spec) {
			file.Let(s, func(t *testcase.T) *helperFile {
				f := file.Super(t)
				f.closeErr = errors.New("close failed")
				return f
			})

			s.Then("returns the write error rather than the close error", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, io.ErrClosedPipe)
				assert.False(t, errors.Is(err, file.Get(t).closeErr))
				assert.Equal(t, file.Get(t).closeCalls, 1)
			})
		})
	})

	s.When("the writer reports a short write without an error", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.write = func(p []byte) (int, error) { return f.File.Write(p[:8]) }
			return f
		})

		s.Then("returns io.ErrShortWrite without retrying and closes the file", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), io.ErrShortWrite)
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t)[:8])
			assert.Equal(t, file.Get(t).writeCalls, 1)
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})

		s.And("closing also fails", func(s *testcase.Spec) {
			file.Let(s, func(t *testcase.T) *helperFile {
				f := file.Super(t)
				f.closeErr = errors.New("close failed")
				return f
			})

			s.Then("keeps the short-write error", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, io.ErrShortWrite)
				assert.False(t, errors.Is(err, file.Get(t).closeErr))
				assert.Equal(t, file.Get(t).closeCalls, 1)
			})
		})
	})

	s.When("only closing fails", func(s *testcase.Spec) {
		file.Let(s, func(t *testcase.T) *helperFile {
			f := file.Super(t)
			f.closeErr = errors.New("close failed")
			return f
		})

		s.Then("returns the close error after writing the content", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), file.Get(t).closeErr)
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
			assert.Equal(t, file.Get(t).closeCalls, 1)
		})
	})
}

func TestMkdirAll(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		subject = let.Var(s, func(t *testcase.T) *helperFileSystem {
			return &helperFileSystem{FileSystem: makeFS(t)}
		})
		top  = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		mid  = let.Var(s, func(t *testcase.T) string { return filepath.Join(top.Get(t), t.Random.UUID()) })
		name = let.Var(s, func(t *testcase.T) string { return filepath.Join(mid.Get(t), t.Random.UUID()) })
		perm = let.Var(s, func(t *testcase.T) fs.FileMode { return filemode.UserRWX })
	)
	act := func(t *testcase.T) error {
		return filesystem.MkdirAll(subject.Get(t), name.Get(t), perm.Get(t))
	}
	assertDir := func(t *testcase.T, path string, mode fs.FileMode) {
		t.Helper()
		info, err := subject.Get(t).Stat(path)
		assert.NoError(t, err, assert.MessageF("%q", path))
		assert.True(t, info.IsDir(), assert.MessageF("%q is a directory", path))
		assert.Equal(t, info.Mode().Perm(), mode, assert.MessageF("%q", path))
	}

	s.Then("creates the directory and all missing parents with the requested permissions", func(t *testcase.T) {
		assert.NoError(t, act(t))
		for _, path := range []string{top.Get(t), mid.Get(t), name.Get(t)} {
			assertDir(t, path, perm.Get(t))
		}
	})

	s.Test("creates relative paths inside a filesystem jail", func(t *testcase.T) {
		subject.Set(t, &helperFileSystem{FileSystem: helperJailedFileSystem(t, t.TempDir())})

		assert.NoError(t, act(t))
		for _, path := range []string{top.Get(t), mid.Get(t), name.Get(t)} {
			assertDir(t, path, perm.Get(t))
		}
	})

	s.Test("creates absolute paths when the backend allows them", func(t *testcase.T) {
		subject.Set(t, &helperFileSystem{FileSystem: helperOSFileSystem{}})
		top.Set(t, filepath.Join(t.TempDir(), t.Random.UUID()))

		assert.NoError(t, act(t))
		for _, path := range []string{top.Get(t), mid.Get(t), name.Get(t)} {
			assertDir(t, path, perm.Get(t))
		}
	})

	s.When("a parent already exists with different permissions", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.NoError(t, subject.Get(t).Mkdir(top.Get(t), 0750))
		})

		s.Then("creates the missing directories without changing the existing parent", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assertDir(t, top.Get(t), 0750)
			assertDir(t, mid.Get(t), perm.Get(t))
			assertDir(t, name.Get(t), perm.Get(t))
		})
	})

	s.When("the directory already exists", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			for _, path := range []string{top.Get(t), mid.Get(t), name.Get(t)} {
				assert.NoError(t, subject.Get(t).Mkdir(path, 0750))
			}
			helperWriteFile(t, subject.Get(t), filepath.Join(name.Get(t), "file"), []byte("kept"), filemode.UserRW)
		})

		s.Then("leaves its content and permissions unchanged", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assertDir(t, name.Get(t), 0750)
			assert.Equal(t, helperReadFile(t, subject.Get(t), filepath.Join(name.Get(t), "file")), []byte("kept"))
			entries, err := filesystem.ReadDir(subject.Get(t), name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, len(entries), 1)
		})
	})

	s.When("the name is dot", func(s *testcase.Spec) {
		name.LetValue(s, ".")

		s.Then("succeeds without changing the existing root", func(t *testcase.T) {
			before, err := subject.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.NoError(t, act(t))
			assertDir(t, name.Get(t), before.Mode().Perm())
		})
	})

	s.When("the name is an absolute filesystem root", func(s *testcase.Spec) {
		name.Let(s, func(t *testcase.T) string {
			cwd, err := os.Getwd()
			assert.NoError(t, err)
			return filepath.VolumeName(cwd) + string(filepath.Separator)
		})

		s.Then("succeeds when that root is allowed by the backend", func(t *testcase.T) {
			assert.NoError(t, act(t))
			info, err := subject.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
		})
	})

	s.When("the name is empty", func(s *testcase.Spec) {
		name.LetValue(s, "")

		s.Then("returns a mkdir PathError instead of treating it as dot", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, fs.ErrNotExist)
			var pathError *fs.PathError
			assert.True(t, errors.As(err, &pathError))
			assert.Equal(t, pathError.Op, "mkdir")
			assert.Equal(t, pathError.Path, name.Get(t))
		})
	})

	s.When("the name is taken by a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.NoError(t, subject.Get(t).Mkdir(top.Get(t), filemode.UserRWX))
			assert.NoError(t, subject.Get(t).Mkdir(mid.Get(t), filemode.UserRWX))
			helperWriteFile(t, subject.Get(t), name.Get(t), []byte("kept"), filemode.UserRW)
		})

		s.Then("returns a mkdir ENOTDIR PathError and preserves the file", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, syscall.ENOTDIR)
			var pathError *fs.PathError
			assert.True(t, errors.As(err, &pathError))
			assert.Equal(t, pathError.Op, "mkdir")
			assert.Equal(t, pathError.Path, name.Get(t))
			assert.Equal(t, helperReadFile(t, subject.Get(t), name.Get(t)), []byte("kept"))
		})
	})

	s.When("an ancestor is a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			helperWriteFile(t, subject.Get(t), top.Get(t), []byte("kept"), filemode.UserRW)
		})

		s.Then("returns ENOTDIR for the ancestor and preserves the file", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, syscall.ENOTDIR)
			var pathError *fs.PathError
			assert.True(t, errors.As(err, &pathError))
			assert.Equal(t, pathError.Path, top.Get(t))
			assert.Equal(t, helperReadFile(t, subject.Get(t), top.Get(t)), []byte("kept"))
		})
	})

	s.When("creating a parent fails", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			fsys := subject.Super(t)
			fsys.mkdir = func(path string, mode fs.FileMode) error {
				if path == mid.Get(t) {
					return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrPermission}
				}
				return fsys.FileSystem.Mkdir(path, mode)
			}
			return fsys
		})

		s.Then("returns the parent error without creating descendants or rolling back ancestors", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, &fs.PathError{Op: "mkdir", Path: mid.Get(t), Err: fs.ErrPermission})
			assertDir(t, top.Get(t), perm.Get(t))
			_, err = subject.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("another caller creates each directory between Stat and Mkdir", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			fsys := subject.Super(t)
			fsys.mkdir = func(path string, mode fs.FileMode) error {
				assert.NoError(t, fsys.FileSystem.Mkdir(path, mode))
				return fsys.FileSystem.Mkdir(path, mode)
			}
			return fsys
		})

		s.Then("accepts the directories that appeared in the meantime", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assertDir(t, name.Get(t), perm.Get(t))
		})
	})

	s.When("a file appears at the destination between Stat and Mkdir", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			fsys := subject.Super(t)
			fsys.mkdir = func(path string, mode fs.FileMode) error {
				if path == name.Get(t) {
					helperWriteFile(t, fsys.FileSystem, path, []byte("kept"), filemode.UserRW)
				}
				return fsys.FileSystem.Mkdir(path, mode)
			}
			return fsys
		})

		s.Then("keeps the mkdir error rather than accepting a non-directory", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrExist)
			assert.Equal(t, helperReadFile(t, subject.Get(t), name.Get(t)), []byte("kept"))
		})
	})

	// Keep these spellings uncleaned: resolving them through the OS may create
	// ancestors or encounter files which filepath.Clean would erase.
	for _, spelling := range []string{
		"missing/child/grandchild", "missing/.", "missing/..", "missing/../created/child",
		"missing/./child", "missing//child///", "file", "file/child", "file/.", "file/../created", "file/",
	} {
		s.Test("matches os.MkdirAll for "+spelling, func(t *testcase.T) {
			actualRoot, expectedRoot := t.TempDir(), t.TempDir()
			for _, root := range []string{actualRoot, expectedRoot} {
				assert.NoError(t, os.WriteFile(filepath.Join(root, "file"), []byte("kept"), filemode.UserRW))
			}
			rawPath := func(root string) string {
				return root + string(filepath.Separator) + filepath.FromSlash(spelling)
			}
			subject.Set(t, &helperFileSystem{FileSystem: helperOSFileSystem{}})
			name.Set(t, rawPath(actualRoot))
			expectedErr := os.MkdirAll(rawPath(expectedRoot), perm.Get(t))

			err := act(t)

			if expectedErr == nil {
				assert.NoError(t, err)
			} else {
				var expectedPathError, actualPathError *fs.PathError
				assert.True(t, errors.As(expectedErr, &expectedPathError))
				assert.True(t, errors.As(err, &actualPathError))
				assert.ErrorIs(t, actualPathError.Err, expectedPathError.Err)
				assert.Equal(t, actualPathError.Op, expectedPathError.Op)
				assert.Equal(t, strings.TrimPrefix(actualPathError.Path, actualRoot), strings.TrimPrefix(expectedPathError.Path, expectedRoot))
			}
			assert.Equal(t, helperTree(t, actualRoot), helperTree(t, expectedRoot))
		})
	}

	s.When("an absolute path is supplied to a jailed backend", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			return &helperFileSystem{FileSystem: helperJailedFileSystem(t, t.TempDir())}
		})
		name.Let(s, func(t *testcase.T) string { return filepath.Join(t.TempDir(), "forbidden") })

		s.Then("preserves the backend's refusal without creating the destination", func(t *testcase.T) {
			_, expectedErr := subject.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, act(t), helperPathErrorCause(t, expectedErr))
			_, err := os.Stat(name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("dot-dot would leave the filesystem jail", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			return &helperFileSystem{FileSystem: helperJailedFileSystem(t, t.TempDir())}
		})
		name.LetValue(s, filepath.FromSlash("../forbidden"))

		s.Then("preserves the backend's rejection", func(t *testcase.T) {
			_, expectedErr := subject.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, act(t), helperPathErrorCause(t, expectedErr))
		})
	})

	s.When("a symlink escapes the jail before a dot-dot segment", func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *helperFileSystem {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
				t.Skipf("symlinks are unavailable: %v", err)
			}
			return &helperFileSystem{FileSystem: helperJailedFileSystem(t, root)}
		})
		name.LetValue(s, filepath.FromSlash("escape/../created"))

		s.Then("does not clean away the escape and create a directory inside the jail", func(t *testcase.T) {
			// The first unresolvable ancestor is the existing symlink itself.
			expectedErr := subject.Get(t).Mkdir("escape", perm.Get(t))
			assert.ErrorIs(t, act(t), helperPathErrorCause(t, expectedErr))
			_, err := subject.Get(t).Stat("created")
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.And("cleaning the path would yield the existing jail root", func(s *testcase.Spec) {
			name.LetValue(s, filepath.FromSlash("escape/.."))

			s.Then("still refuses the unsafe path rather than reporting success", func(t *testcase.T) {
				expectedErr := subject.Get(t).Mkdir("escape", perm.Get(t))
				assert.ErrorIs(t, act(t), helperPathErrorCause(t, expectedErr))
			})
		})
	})
}

// The wrappers inject I/O failures while retaining a real backend and file handle.
// Arrangement and inspection use the backend directly, so only the helper's
// handle is observed and closed by helperFile.
type helperFileSystem struct {
	FileSystem interface {
		filesystem.FileOpener
		filesystem.FileInfoReader
		filesystem.DirectoryMaker
	}
	file    *helperFile
	openErr error
	mkdir   func(string, fs.FileMode) error
}

func (fsys *helperFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	if fsys.openErr != nil {
		return nil, fsys.openErr
	}
	file, err := fsys.FileSystem.OpenFile(name, flag, perm)
	if err != nil || fsys.file == nil {
		return file, err
	}
	fsys.file.File = file
	return fsys.file, nil
}

func (fsys *helperFileSystem) Stat(name string) (fs.FileInfo, error) {
	return fsys.FileSystem.Stat(name)
}

func (fsys *helperFileSystem) Mkdir(name string, perm fs.FileMode) error {
	if fsys.mkdir != nil {
		return fsys.mkdir(name, perm)
	}
	return fsys.FileSystem.Mkdir(name, perm)
}

type helperFile struct {
	filesystem.File
	read       func([]byte) (int, error)
	write      func([]byte) (int, error)
	closeErr   error
	writeCalls int
	closeCalls int
}

func (f *helperFile) Read(p []byte) (int, error) {
	if f.read != nil {
		return f.read(p)
	}
	return f.File.Read(p)
}

func (f *helperFile) Write(p []byte) (int, error) {
	f.writeCalls++
	if f.write != nil {
		return f.write(p)
	}
	return f.File.Write(p)
}

func (f *helperFile) Close() error {
	f.closeCalls++
	if err := f.File.Close(); err != nil {
		return err
	}
	return f.closeErr
}

func helperWriteFile(t testing.TB, fsys filesystem.FileOpener, name string, content []byte, perm fs.FileMode) {
	t.Helper()
	file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	n, err := file.Write(content)
	assert.NoError(t, err)
	assert.Equal(t, n, len(content))
	assert.NoError(t, file.Close())
}

func helperReadFile(t testing.TB, fsys filesystem.FileOpener, name string) []byte {
	t.Helper()
	file, err := fsys.OpenFile(name, os.O_RDONLY, 0)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	return assert.ReadAll(t, file)
}

type helperOSFileSystem struct{}

func (helperOSFileSystem) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
func (helperOSFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	return os.OpenFile(name, flag, perm)
}
func (helperOSFileSystem) Mkdir(name string, perm fs.FileMode) error { return os.Mkdir(name, perm) }
func (helperOSFileSystem) Remove(name string) error                  { return os.Remove(name) }
func (helperOSFileSystem) Rename(oldpath, newpath string) error      { return os.Rename(oldpath, newpath) }

type helperRootFileSystem struct{ *os.Root }

func (fsys helperRootFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	return fsys.Root.OpenFile(name, flag, perm)
}

func helperJailedFileSystem(t testing.TB, path string) helperRootFileSystem {
	t.Helper()
	root, err := os.OpenRoot(path)
	assert.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, root.Close()) })
	return helperRootFileSystem{Root: root}
}

func helperPathErrorCause(t testing.TB, err error) error {
	t.Helper()
	var pathError *fs.PathError
	assert.True(t, errors.As(err, &pathError))
	return pathError.Err
}

func helperTree(t testing.TB, root string) map[string]fs.FileMode {
	t.Helper()
	entries := make(map[string]fs.FileMode)
	assert.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		assert.NoError(t, err)
		info, err := entry.Info()
		assert.NoError(t, err)
		entries[name] = info.Mode()
		return nil
	}))
	return entries
}
