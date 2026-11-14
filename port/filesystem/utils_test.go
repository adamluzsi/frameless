package filesystem_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/testcase"

	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func makeFS(tb testing.TB) *memory.FileSystem {
	return &memory.FileSystem{}
}

func Test_createAndOpen(t *testing.T) {
	fsys := makeFS(t)
	name := "test.txt"
	file, err := filesystem.Create(fsys, name)
	assert.NoError(t, err)
	fileInfo, err := file.Stat()
	assert.NoError(t, err)
	assert.False(t, fileInfo.IsDir())
	assert.True(t, fileInfo.Mode()&filemode.UserRW != 0)

	data := "Hello, world!"
	n, err := file.Write([]byte(data))
	assert.NoError(t, err)
	assert.Equal(t, len([]byte(data)), n)
	assert.NoError(t, file.Close())

	file, err = filesystem.Open(fsys, name)
	assert.NoError(t, err)

	bs, err := io.ReadAll(file)
	assert.NoError(t, err)

	assert.Equal(t, data, string(bs))
}

func TestReadDir(t *testing.T) {
	fsys := makeFS(t)
	dirName := "test.d"

	t.Log("on missing dir, it yields error")
	_, err := filesystem.ReadDir(fsys, dirName)
	assert.ErrorIs(t, fs.ErrNotExist, err)

	t.Log("on empty dir, returns an empty list")
	assert.NoError(t, fsys.Mkdir(dirName, filemode.UserRWX))
	dirEntries, err := filesystem.ReadDir(fsys, dirName)
	assert.NoError(t, err)
	assert.Empty(t, dirEntries)

	t.Log("on dir with entries, return the list in sorted order")
	for _, fileName := range []string{"c", "a", "b"} {
		filePath := filepath.Join(dirName, fileName)
		f, err := filesystem.Create(fsys, filePath)
		assert.NoError(t, err)
		assert.NoError(t, f.Close())
		t.Cleanup(func() { fsys.Remove(filePath) })
	}
	dirEntries, err = filesystem.ReadDir(fsys, dirName)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(dirEntries))
	assert.Equal(t, "a", dirEntries[0].Name())
	assert.Equal(t, "b", dirEntries[1].Name())
	assert.Equal(t, "c", dirEntries[2].Name())
}

// TestWalkDir
//
// TODO: make stub tests to cover all rainy-Path
func TestWalkDir(t *testing.T) {
	fsys := makeFS(t)

	touchFile := func(tb testing.TB, name string) {
		file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filemode.UserRWX)
		assert.NoError(tb, err)
		assert.NoError(tb, file.Close())
	}

	assert.NoError(t, fsys.Mkdir("a", filemode.UserRWX))
	touchFile(t, "a/1")
	touchFile(t, "a/2")
	touchFile(t, "a/3")
	assert.NoError(t, fsys.Mkdir("b", filemode.UserRWX))
	touchFile(t, "b/4")
	touchFile(t, "b/5")
	touchFile(t, "b/6")
	assert.NoError(t, fsys.Mkdir("a/c", filemode.UserRWX))
	touchFile(t, "a/c/7")
	touchFile(t, "a/c/8")
	touchFile(t, "a/c/9")

	var names []string
	assert.NoError(t, filesystem.WalkDir(fsys, "a", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(t, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(t, []string{"a", "1", "2", "3", "c", "7", "8", "9"}, names)

	names = nil
	assert.NoError(t, filesystem.WalkDir(fsys, "a/c", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(t, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(t, []string{"c", "7", "8", "9"}, names)

	names = nil
	assert.NoError(t, filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Log(err.Error())
		}
		assert.NoError(t, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(t, []string{".", "a", "1", "2", "3", "c", "7", "8", "9", "b", "4", "5", "6"}, names)

	names = nil
	assert.NoError(t, filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(t, err)
		names = append(names, d.Name())

		// intentionally skipping after name is already added,
		// to confirm that the restapi of the directory is skipped
		if err == nil && d.IsDir() && d.Name() == "a" {
			return fs.SkipDir
		}
		return nil
	}))
	assert.Equal(t, []string{".", "a", "b", "4", "5", "6"}, names)
}

func TestOpen(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		backend = let.Var(s, func(t *testcase.T) *memory.FileSystem { return &memory.FileSystem{} })
		subject = let.Var(s, func(t *testcase.T) filesystem.FileSystem {
			return struct{ filesystem.FileSystem }{backend.Get(t)}
		})
		name    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		content = let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(64)) })
		exists  = let.Var(s, func(t *testcase.T) bool { return true })
	)
	act := func(t *testcase.T) (filesystem.File, error) {
		return filesystem.Open(subject.Get(t), name.Get(t))
	}

	s.Before(func(t *testcase.T) {
		assert.NoError(t, filesystem.MkdirAll(backend.Get(t), filepath.Dir(name.Get(t)), filemode.UserRWX))
		if exists.Get(t) {
			helperWriteFile(t, backend.Get(t), name.Get(t), content.Get(t), filemode.UserRW)
		}
	})

	behavesLikeOpen := func(s *testcase.Spec) {
		s.Then("opens the existing content without changing it", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
		})

		s.Then("returns a handle that supports seeking and reading metadata", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			info, err := file.Stat()
			assert.NoError(t, err)
			assert.Equal(t, info.Name(), filepath.Base(name.Get(t)))
			assert.Equal(t, info.Size(), int64(len(content.Get(t))))
			assert.False(t, info.IsDir())
			offset := int64(t.Random.IntBetween(1, len(content.Get(t))-1))
			pos, err := file.Seek(offset, io.SeekStart)
			assert.NoError(t, err)
			assert.Equal(t, pos, offset)
			assert.Equal(t, assert.ReadAll(t, file), content.Get(t)[offset:])
		})

		s.Then("rejects writes without changing the stored content", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			n, err := file.Write([]byte("must not be written"))
			assert.Error(t, err)
			assert.Empty(t, n)
			assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
		})

		s.When("the file does not exist", func(s *testcase.Spec) {
			exists.LetValue(s, false)

			s.Then("returns a PathError without creating the file", func(t *testcase.T) {
				file, err := act(t)
				assert.Nil(t, file)
				assert.ErrorIs(t, err, fs.ErrNotExist)
				var pathError *fs.PathError
				assert.True(t, errors.As(err, &pathError))
				assert.Equal(t, pathError.Op, "open")
				_, err = backend.Get(t).Stat(name.Get(t))
				assert.ErrorIs(t, err, fs.ErrNotExist)
			})
		})

		s.When("the name points to a directory", func(s *testcase.Spec) {
			exists.LetValue(s, false)
			child := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			s.Before(func(t *testcase.T) {
				assert.NoError(t, backend.Get(t).Mkdir(name.Get(t), filemode.UserRWX))
				helperWriteFile(t, backend.Get(t), filepath.Join(name.Get(t), child.Get(t)), content.Get(t), filemode.UserRW)
			})

			s.Then("returns a handle that can list its entries", func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = file.Close() })
				entries, err := file.ReadDir(-1)
				assert.NoError(t, err)
				assert.Equal(t, len(entries), 1)
				assert.Equal(t, entries[0].Name(), child.Get(t))
			})
		})
	}

	behavesLikeOpen(s)

	s.When("the filesystem also implements fs.FS", func(s *testcase.Spec) {
		standard := let.Var(s, func(t *testcase.T) *helperOpenFS {
			fsys := &helperOpenFS{FS: backend.Get(t), file: &helperFile{}}
			t.Cleanup(func() {
				if fsys.file.File != nil {
					_ = fsys.file.File.Close()
				}
			})
			return fsys
		})
		subject.Let(s, func(t *testcase.T) filesystem.FileSystem {
			return struct {
				filesystem.FileSystem
				fs.FS
			}{backend.Get(t), standard.Get(t)}
		})

		behavesLikeOpen(s)

		s.Then("hands the fs.FS handle to the caller without closing it", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			assert.NotNil(t, standard.Get(t).file.File)
			assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
			assert.NoError(t, file.Close())
			_, err = standard.Get(t).file.Read(make([]byte, 1))
			assert.ErrorIs(t, err, fs.ErrClosed)
		})

		s.And("fs.FS cannot access a file that OpenFile could open", func(s *testcase.Spec) {
			standard.Let(s, func(t *testcase.T) *helperOpenFS {
				fsys := standard.Super(t)
				fsys.FS = fstest.MapFS{}
				return fsys
			})

			s.Then("preserves the fs.FS error rather than falling back", func(t *testcase.T) {
				file, err := act(t)
				assert.Nil(t, file)
				assert.ErrorIs(t, err, &fs.PathError{Op: "open", Path: name.Get(t), Err: fs.ErrNotExist})
				assert.Equal(t, helperReadFile(t, backend.Get(t), name.Get(t)), content.Get(t))
			})
		})

		s.And("fs.FS exposes only the basic fs.File capabilities", func(s *testcase.Spec) {
			standard.Let(s, func(t *testcase.T) *helperOpenFS {
				fsys := standard.Super(t)
				fsys.basic = true
				return fsys
			})

			behavesLikeOpen(s)

			s.Then("closes the basic handle and returns a usable full handle", func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = file.Close() })
				assert.NotNil(t, standard.Get(t).file.File)
				_, err = standard.Get(t).file.Read(make([]byte, 1))
				assert.ErrorIs(t, err, fs.ErrClosed)
				assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
			})

			s.And("closing the basic handle fails", func(s *testcase.Spec) {
				standard.Let(s, func(t *testcase.T) *helperOpenFS {
					fsys := standard.Super(t)
					fsys.file.closeErr = &fs.PathError{Op: "close", Path: name.Get(t), Err: fs.ErrPermission}
					return fsys
				})

				s.Then("returns the close error instead of hiding it with a successful reopen", func(t *testcase.T) {
					file, err := act(t)
					if file != nil {
						t.Cleanup(func() { _ = file.Close() })
					}
					assert.Nil(t, file)
					assert.ErrorIs(t, err, standard.Get(t).file.closeErr)
				})
			})

			s.And("the fallback cannot open the file", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) filesystem.FileSystem {
					return struct {
						filesystem.FileSystem
						fs.FS
					}{&memory.FileSystem{}, standard.Get(t)}
				})

				s.Then("returns the fallback error after closing the basic handle", func(t *testcase.T) {
					file, err := act(t)
					assert.Nil(t, file)
					assert.ErrorIs(t, err, fs.ErrNotExist)
					_, err = standard.Get(t).file.Read(make([]byte, 1))
					assert.ErrorIs(t, err, fs.ErrClosed)
				})
			})
		})

		for _, spelling := range []string{"nested/file", "./file", "nested/../file", "nested//file"} {
			s.And("the native path is spelled "+spelling, func(s *testcase.Spec) {
				name.LetValue(s, filepath.FromSlash(spelling))

				s.Then("opens it using the backend's path conventions", func(t *testcase.T) {
					file, err := act(t)
					assert.NoError(t, err)
					t.Cleanup(func() { _ = file.Close() })
					assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
				})
			})
		}

		s.And("the name is an absolute native path", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string {
				path, err := filepath.Abs(name.Super(t))
				assert.NoError(t, err)
				return path
			})

			s.Then("opens it without imposing fs.FS path restrictions", func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = file.Close() })
				assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
			})
		})
	})
}

// Keep real file behaviour while observing ownership and optionally narrowing
// the handle's method set to the minimum promised by fs.FS.
type helperOpenFS struct {
	fs.FS
	file  *helperFile
	basic bool
}

func (fsys *helperOpenFS) Open(name string) (fs.File, error) {
	file, err := fsys.FS.Open(name)
	if err != nil {
		return nil, err
	}
	fsys.file.File = file.(filesystem.File)
	if fsys.basic {
		return struct{ fs.File }{fsys.file}, nil
	}
	return fsys.file, nil
}
