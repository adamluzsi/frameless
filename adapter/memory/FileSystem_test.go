package memory_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
	filesystemcontracts "go.llib.dev/frameless/port/filesystem/filesystemcontract"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleFileSystem() {
	fsys := &memory.FileSystem{}

	file, err := fsys.OpenFile("test", os.O_RDWR|os.O_CREATE|os.O_EXCL, filemode.UserRWX)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	file.Write([]byte("Hello world!"))
	file.Seek(0, io.SeekStart)

	bs, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(bs)) // "Hello world!"

	file.Close()
	fsys.Remove("test")

	fsys.Mkdir("a", filemode.UserRWX)

	file2Name := filepath.Join("a", "test.txt")
	file2, err := filesystem.Create(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	file2, err = filesystem.Open(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		return fs.SkipDir
	})
}

func TestFileSystem_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(&memory.FileSystem{}).Test(t)
}

func TestFileSystem_smoke(t *testing.T) {
	mfs := &memory.FileSystem{}

	name := "test"
	file, err := mfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_APPEND, filemode.UserRWX)
	assert.NoError(t, err)
	defer func() { assert.Should(t).NoError(mfs.Remove(name)) }()

	_, err = file.Write([]byte("/foo"))
	assert.NoError(t, err)
	_, err = file.Write([]byte("/bar"))
	assert.NoError(t, err)
	file.Seek(0, io.SeekStart)
	_, err = file.Write([]byte("/baz"))
	assert.NoError(t, err)

	assert.NoError(t, file.Close())

	file, err = mfs.OpenFile(name, os.O_RDONLY, 0)
	assert.NoError(t, err)

	bs, err := io.ReadAll(file)
	assert.NoError(t, err)
	assert.Equal(t, "/foo/bar/baz", string(bs))
}

func TestFileSystem(t *testing.T) {
	s := testcase.NewSpec(t)
	s.HasSideEffect() // timecop moves the process-wide clock

	var (
		now     = let.Time(s)
		subject = let.Var(s, func(t *testcase.T) *memory.FileSystem {
			return &memory.FileSystem{}
		})
	)
	s.Before(func(t *testcase.T) {
		timecop.Travel(t, now.Get(t), timecop.Freeze)
	})

	s.Describe("#Stat", func(s *testcase.Spec) {
		var (
			fileName = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			content  = let.Var(s, randomContent)
			name     = let.Var(s, func(t *testcase.T) string { return fileName.Get(t) })
		)
		act := func(t *testcase.T) (fs.FileInfo, error) {
			return subject.Get(t).Stat(name.Get(t))
		}
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), fileName.Get(t), content.Get(t))
		})

		s.Then("it describes the file", func(t *testcase.T) {
			info, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, info.Name(), fileName.Get(t))
			assert.False(t, info.IsDir())
			assert.Equal(t, info.Size(), int64(len(content.Get(t))))
			assert.Equal(t, info.ModTime(), now.Get(t))
		})

		s.When("name points to the root directory", func(s *testcase.Spec) {
			name.LetValue(s, ".")

			s.Then("it describes a directory, in its file mode as well", func(t *testcase.T) {
				info, err := act(t)
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
				assert.True(t, info.Mode().IsDir())
			})

			s.Then("its modification time is when the file system was first used", func(t *testcase.T) {
				info, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, info.ModTime(), now.Get(t))
			})

			s.And("time passes", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					timecop.Travel(t, time.Hour)
				})

				s.Then("its modification time stays the same", func(t *testcase.T) {
					info, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, info.ModTime(), now.Get(t))
				})
			})
		})

		s.When("name points to a directory above the root", func(s *testcase.Spec) {
			name.LetValue(s, "..")

			s.Then("it describes a directory, as the directories above the working directory exist on disk too", func(t *testcase.T) {
				info, err := act(t)
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			})
		})

		specNameThroughFile(s, subject, name, func(t *testcase.T) error {
			_, err := act(t)
			return err
		})
	})

	s.Describe("#Open", func(s *testcase.Spec) {
		var (
			fileName = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			content  = let.Var(s, randomContent)
			name     = let.Var(s, func(t *testcase.T) string { return fileName.Get(t) })
		)
		act := func(t *testcase.T) (fs.File, error) {
			file, err := subject.Get(t).Open(name.Get(t))
			if err == nil {
				t.Defer(file.Close)
			}
			return file, err
		}
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), fileName.Get(t), content.Get(t))
		})

		s.Then("the file's content can be read", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			data, err := io.ReadAll(file)
			assert.NoError(t, err)
			assert.Equal(t, string(data), content.Get(t))
		})

		s.Then("the file is opened read-only", func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			w, ok := file.(io.Writer)
			assert.True(t, ok)
			_, err = w.Write([]byte(randomContent(t)))
			assert.ErrorIs(t, err, fs.ErrPermission)
			assert.Equal(t, readFile(t, subject.Get(t), fileName.Get(t)), content.Get(t))
		})

		s.When("name points to nothing", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string { return t.Random.UUID() })

			s.Then("it reports fs.ErrNotExist", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, fs.ErrNotExist)
			})
		})

		s.When("name is not a valid fs.FS path, even though it would lead to the file", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string {
				return random.Pick(t.Random,
					"./"+fileName.Get(t),
					fileName.Get(t)+"/.",
					"x/../"+fileName.Get(t),
					filepath.ToSlash(absPath(t, fileName.Get(t))),
				)
			})

			s.Then("it is rejected with fs.ErrInvalid", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, fs.ErrInvalid)

				var pathErr *fs.PathError
				assert.True(t, errors.As(err, &pathErr))
				assert.Equal(t, pathErr.Op, "open")
				assert.Equal(t, pathErr.Path, name.Get(t))
			})
		})

		s.When("name points to the root directory", func(s *testcase.Spec) {
			name.LetValue(s, ".")

			s.Then("it opens a directory that lists its entries", func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)

				info, err := file.Stat()
				assert.NoError(t, err)
				assert.True(t, info.IsDir())

				dir, ok := file.(fs.ReadDirFile)
				assert.True(t, ok)
				entries, err := dir.ReadDir(-1)
				assert.NoError(t, err)
				assert.Equal(t, len(entries), 1)
				assert.Equal(t, entries[0].Name(), fileName.Get(t))
			})
		})

		specNameThroughFile(s, subject, name, func(t *testcase.T) error {
			_, err := act(t)
			return err
		})
	})

	s.Describe("as fs.FS", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return fstest.TestFS(subject.Get(t), "a", "a/b.txt", "a/c", "a/c/d.txt", "e.txt")
		}
		s.Before(func(t *testcase.T) {
			fsys := subject.Get(t)
			assert.Must(t).NoError(fsys.Mkdir("a", filemode.UserRWX))
			writeFile(t, fsys, filepath.Join("a", "b.txt"), randomContent(t))
			assert.Must(t).NoError(fsys.Mkdir(filepath.Join("a", "c"), filemode.UserRWX))
			writeFile(t, fsys, filepath.Join("a", "c", "d.txt"), randomContent(t))
			writeFile(t, fsys, "e.txt", randomContent(t))
		})

		s.Then("it passes the testing/fstest conformance checks", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})
	})

	s.Describe("#OpenFile", func(s *testcase.Spec) {
		var (
			name = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			flag = let.VarOf(s, os.O_RDWR|os.O_CREATE)
		)
		act := func(t *testcase.T) (filesystem.File, error) {
			file, err := subject.Get(t).OpenFile(name.Get(t), flag.Get(t), filemode.UserRW)
			if err == nil {
				t.Defer(file.Close)
			}
			return file, err
		}

		s.Then("it creates the file, modified at the clock's current time", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			info, err := subject.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, info.ModTime(), now.Get(t))
		})

		specCreatingInParentDirectory(s, subject, name, func(t *testcase.T) error {
			_, err := act(t)
			return err
		})

		specNameThroughFile(s, subject, name, func(t *testcase.T) error {
			_, err := act(t)
			return err
		})

		s.When("the file is opened without O_CREATE", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDONLY)

			s.Then("a missing file is reported with fs.ErrNotExist, and it is not created", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, fs.ErrNotExist)

				_, err = subject.Get(t).Stat(name.Get(t))
				assert.ErrorIs(t, err, fs.ErrNotExist)
			})

			specNameThroughFile(s, subject, name, func(t *testcase.T) error {
				_, err := act(t)
				return err
			})
		})

		s.When("an existing file is opened with O_TRUNC", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_WRONLY|os.O_TRUNC)

			s.Before(func(t *testcase.T) {
				writeFile(t, subject.Get(t), name.Get(t), randomContent(t))
				timecop.Travel(t, time.Hour)
			})

			s.Then("the file is emptied and modified at the clock's current time", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)

				info, err := subject.Get(t).Stat(name.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, info.Size(), int64(0))
				assert.Equal(t, info.ModTime(), now.Get(t).Add(time.Hour))
			})
		})

		s.When("name points to a directory", func(s *testcase.Spec) {
			dir := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			name.Let(s, func(t *testcase.T) string {
				return random.Pick(t.Random, ".", dir.Get(t))
			})
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(subject.Get(t).Mkdir(dir.Get(t), filemode.UserRWX))
			})

			s.And("it is opened for writing", func(s *testcase.Spec) {
				flag.Let(s, func(t *testcase.T) int {
					return random.Pick(t.Random,
						os.O_WRONLY,
						os.O_RDWR,
						os.O_RDWR|os.O_CREATE,
						os.O_WRONLY|os.O_TRUNC,
					)
				})

				s.Then("it fails with EISDIR, like an OS file system", func(t *testcase.T) {
					_, err := act(t)
					assert.ErrorIs(t, err, syscall.EISDIR)
				})

				s.Then("the directory stays a directory", func(t *testcase.T) {
					_, _ = act(t)

					info, err := subject.Get(t).Stat(name.Get(t))
					assert.NoError(t, err)
					assert.True(t, info.IsDir())
				})
			})
		})

		s.When("name points to the directory above the root", func(s *testcase.Spec) {
			name.LetValue(s, "..")
			flag.LetValue(s, os.O_RDONLY)

			s.Then("it opens a directory that lists the root directory, as on disk", func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)

				entries, err := file.ReadDir(-1)
				assert.NoError(t, err)
				assert.Contains(t, entryNames(entries), filepath.Base(absPath(t, ".")))
			})
		})
	})

	s.Describe("#Mkdir", func(s *testcase.Spec) {
		name := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		act := func(t *testcase.T) error {
			return subject.Get(t).Mkdir(name.Get(t), filemode.UserRWX)
		}

		s.Then("it creates a directory, modified at the clock's current time", func(t *testcase.T) {
			assert.NoError(t, act(t))

			info, err := subject.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.Mode().IsDir())
			assert.Equal(t, info.ModTime(), now.Get(t))
		})

		specCreatingInParentDirectory(s, subject, name, act)
		specNameThroughFile(s, subject, name, act)

		s.When("name points to the root directory", func(s *testcase.Spec) {
			name.LetValue(s, ".")

			s.Then("it reports fs.ErrExist, as the root always exists", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), fs.ErrExist)
			})
		})
	})

	s.Describe("#Remove", func(s *testcase.Spec) {
		var (
			file = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			name = let.Var(s, func(t *testcase.T) string { return file.Get(t) })
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Remove(name.Get(t))
		}
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), file.Get(t), randomContent(t))
		})

		s.Then("the file no longer exists", func(t *testcase.T) {
			assert.NoError(t, act(t))

			_, err := subject.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.When("name points to the root directory, or to a directory above it", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string {
				return random.Pick(t.Random, ".", "..", filepath.Join("..", ".."))
			})

			s.Then("it fails with fs.ErrPermission, as these always exist", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), fs.ErrPermission)
			})

			s.Then("the directory still exists", func(t *testcase.T) {
				_ = act(t)

				info, err := subject.Get(t).Stat(name.Get(t))
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			})
		})

		specNameThroughFile(s, subject, name, act)
	})

	s.Describe("#Rename", func(s *testcase.Spec) {
		var (
			file    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			content = let.Var(s, randomContent)
			oldName = let.Var(s, func(t *testcase.T) string { return file.Get(t) })
			newName = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Rename(oldName.Get(t), newName.Get(t))
		}
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), file.Get(t), content.Get(t))
			timecop.Travel(t, time.Hour)
		})

		s.Then("the file is moved to the new name, and keeps its modification time", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, readFile(t, subject.Get(t), newName.Get(t)), content.Get(t))
			info, err := subject.Get(t).Stat(newName.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, info.ModTime(), now.Get(t))
			_, err = subject.Get(t).Stat(oldName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		specCreatingInParentDirectory(s, subject, newName, act)
		specNameThroughFile(s, subject, newName, act)
		specNameThroughFile(s, subject, oldName, act)

		s.When("new name points to an existing directory", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(subject.Get(t).Mkdir(newName.Get(t), filemode.UserRWX))
			})

			s.Then("it fails with EEXIST, as os.Rename does on Unix", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), syscall.EEXIST)
			})
		})

		s.When("old name points to a directory, and new name to an existing file", func(s *testcase.Spec) {
			oldName.Let(s, func(t *testcase.T) string { return t.Random.UUID() })
			newName.Let(s, func(t *testcase.T) string { return file.Get(t) })
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(subject.Get(t).Mkdir(oldName.Get(t), filemode.UserRWX))
			})

			s.Then("it fails with ENOTDIR, as os.Rename does on Unix", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), syscall.ENOTDIR)
			})

			s.Then("the file stays as it was", func(t *testcase.T) {
				_ = act(t)

				assert.Equal(t, readFile(t, subject.Get(t), file.Get(t)), content.Get(t))
			})
		})

		s.When("old name points to a directory, and new name is inside it", func(s *testcase.Spec) {
			oldName.Let(s, func(t *testcase.T) string { return t.Random.UUID() })
			newName.Let(s, func(t *testcase.T) string {
				return filepath.Join(oldName.Get(t), t.Random.UUID())
			})
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(subject.Get(t).Mkdir(oldName.Get(t), filemode.UserRWX))
			})

			s.Then("it fails with EINVAL, as os.Rename does on Unix", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), syscall.EINVAL)
			})

			s.Then("the directory stays where it was, with nothing in it", func(t *testcase.T) {
				_ = act(t)

				entries, err := filesystem.ReadDir(subject.Get(t), oldName.Get(t))
				assert.NoError(t, err)
				assert.Empty(t, entries)
			})
		})

		s.When("old name points to the root directory, or to a directory above it", func(s *testcase.Spec) {
			oldName.Let(s, func(t *testcase.T) string {
				return random.Pick(t.Random, ".", "..", filepath.Join("..", ".."))
			})

			s.Then("it fails with fs.ErrPermission, as these always exist", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), fs.ErrPermission)
			})

			s.Then("the directory still exists", func(t *testcase.T) {
				_ = act(t)

				info, err := subject.Get(t).Stat(oldName.Get(t))
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			})
		})

		s.When("the file is open", func(s *testcase.Spec) {
			handle := let.Var(s, func(t *testcase.T) filesystem.File {
				handle, err := subject.Get(t).OpenFile(file.Get(t), os.O_RDWR, 0)
				assert.Must(t).NoError(err)
				t.Defer(handle.Close)
				return handle
			})
			s.Before(func(t *testcase.T) {
				handle.Get(t) // open the handle before the rename
			})

			s.Then("the handle follows the file, so its writes land in the renamed file", func(t *testcase.T) {
				assert.NoError(t, act(t))

				data := randomContent(t)
				_, err := handle.Get(t).Write([]byte(data))
				assert.NoError(t, err)
				assert.Equal(t, readFile(t, subject.Get(t), newName.Get(t)), overwrite(content.Get(t), data))
			})

			s.Then("the handle still names the file as it was opened, as an OS file does", func(t *testcase.T) {
				assert.NoError(t, act(t))

				info, err := handle.Get(t).Stat()
				assert.NoError(t, err)
				assert.Equal(t, info.Name(), filepath.Base(file.Get(t)))
			})
		})
	})

	s.Describe("FileSystemFile", func(s *testcase.Spec) {
		var (
			name    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			content = let.Var(s, randomContent)
			flag    = let.VarOf(s, os.O_RDWR)
			file    = let.Var(s, func(t *testcase.T) filesystem.File {
				file, err := subject.Get(t).OpenFile(name.Get(t), flag.Get(t), 0)
				assert.Must(t).NoError(err)
				t.Defer(file.Close)
				return file
			})
		)
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), name.Get(t), content.Get(t))
		})

		s.Describe("#Write", func(s *testcase.Spec) {
			data := let.Var(s, randomContent)
			act := func(t *testcase.T) (int, error) {
				return file.Get(t).Write([]byte(data.Get(t)))
			}

			s.Then("it writes over the content from the handle's offset", func(t *testcase.T) {
				n, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, n, len(data.Get(t)))
				assert.Equal(t, readFile(t, subject.Get(t), name.Get(t)), overwrite(content.Get(t), data.Get(t)))
			})

			s.Then("handles opened earlier read the written data too", func(t *testcase.T) {
				reader, err := subject.Get(t).OpenFile(name.Get(t), os.O_RDONLY, 0)
				assert.Must(t).NoError(err)
				t.Defer(reader.Close)

				_, err = act(t)
				assert.NoError(t, err)

				got, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, string(got), overwrite(content.Get(t), data.Get(t)))
			})

			s.When("time has passed since the file was last modified", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					timecop.Travel(t, time.Hour)
				})

				s.Then("the file is modified at the clock's current time", func(t *testcase.T) {
					_, err := act(t)
					assert.NoError(t, err)

					info, err := subject.Get(t).Stat(name.Get(t))
					assert.NoError(t, err)
					assert.Equal(t, info.ModTime(), now.Get(t).Add(time.Hour))
				})
			})
		})

		s.Describe("#Close", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return file.Get(t).Close()
			}

			s.Then("it closes the handle", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, err := file.Get(t).Read(make([]byte, 1))
				assert.ErrorIs(t, err, fs.ErrClosed)
			})

			s.When("another handle changed the file while this one was open", func(s *testcase.Spec) {
				flag.Let(s, func(t *testcase.T) int {
					return random.Pick(t.Random, os.O_RDONLY, os.O_WRONLY, os.O_RDWR)
				})
				appended := let.Var(s, randomContent)

				s.Before(func(t *testcase.T) {
					file.Get(t) // open this handle before the other one writes
					timecop.Travel(t, time.Hour)

					other, err := subject.Get(t).OpenFile(name.Get(t), os.O_WRONLY|os.O_APPEND, 0)
					assert.Must(t).NoError(err)
					_, err = other.Write([]byte(appended.Get(t)))
					assert.Must(t).NoError(err)
					assert.Must(t).NoError(other.Close())

					timecop.Travel(t, time.Hour)
				})

				s.Then("closing keeps the other handle's changes", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, readFile(t, subject.Get(t), name.Get(t)), content.Get(t)+appended.Get(t))
				})

				s.Then("closing leaves the modification time alone", func(t *testcase.T) {
					assert.NoError(t, act(t))

					info, err := subject.Get(t).Stat(name.Get(t))
					assert.NoError(t, err)
					assert.Equal(t, info.ModTime(), now.Get(t).Add(time.Hour))
				})
			})

			s.When("the handle is already closed", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					assert.Must(t).NoError(file.Get(t).Close())
				})

				s.Then("it reports fs.ErrClosed", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), fs.ErrClosed)
				})
			})
		})

		s.Describe("#ReadDir", func(s *testcase.Spec) {
			var (
				dir   = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
				names = let.Var(s, func(t *testcase.T) []string {
					return random.Slice(t.Random.IntBetween(1, 5), t.Random.UUID, random.UniqueValues)
				})
				n = let.VarOf(s, -1)
			)
			file.Let(s, func(t *testcase.T) filesystem.File {
				file, err := subject.Get(t).OpenFile(dir.Get(t), os.O_RDONLY, 0)
				assert.Must(t).NoError(err)
				t.Defer(file.Close)
				return file
			})
			act := func(t *testcase.T) ([]fs.DirEntry, error) {
				return file.Get(t).ReadDir(n.Get(t))
			}
			s.Before(func(t *testcase.T) {
				assert.Must(t).NoError(subject.Get(t).Mkdir(dir.Get(t), filemode.UserRWX))
				for _, name := range names.Get(t) {
					writeFile(t, subject.Get(t), filepath.Join(dir.Get(t), name), randomContent(t))
				}
			})

			s.Then("it lists every entry of the directory", func(t *testcase.T) {
				entries, err := act(t)
				assert.NoError(t, err)
				assert.ContainsExactly(t, entryNames(entries), names.Get(t))
			})

			s.When("n is zero", func(s *testcase.Spec) {
				n.LetValue(s, 0)

				s.Then("it lists every entry, as fs.ReadDirFile defines for n <= 0", func(t *testcase.T) {
					entries, err := act(t)
					assert.NoError(t, err)
					assert.ContainsExactly(t, entryNames(entries), names.Get(t))
				})
			})
		})
	})

	s.Describe("concurrent use", func(s *testcase.Spec) {
		var (
			existing  = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			removable = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			created   = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			dir       = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			renamable = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			renamed   = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			// renamableHandle is open on the renamable file, while it is renamed.
			renamableHandle = let.Var(s, func(t *testcase.T) filesystem.File {
				file, err := subject.Get(t).OpenFile(renamable.Get(t), os.O_RDONLY, 0)
				assert.Must(t).NoError(err)
				t.Defer(file.Close)
				return file
			})
		)
		act := func(t *testcase.T) []error {
			var (
				fsys      = subject.Get(t)
				existing  = existing.Get(t)
				removable = removable.Get(t)
				created   = created.Get(t)
				dir       = dir.Get(t)
				renamable = renamable.Get(t)
				renamed   = renamed.Get(t)
				handle    = renamableHandle.Get(t)
				data      = []byte(randomContent(t))
				errs      = make([]error, 9)
			)
			testcase.Race(
				func() { errs[0] = fsys.Mkdir(dir, filemode.UserRWX) },
				func() { errs[1] = fsys.Remove(removable) },
				func() { _, errs[2] = fsys.Stat(existing) },
				func() { errs[3] = writeTo(fsys, created, os.O_WRONLY|os.O_CREATE|os.O_EXCL, data) },
				func() { errs[4] = writeTo(fsys, existing, os.O_WRONLY|os.O_APPEND, data) },
				func() { _, errs[5] = fs.ReadFile(fsys, existing) },
				func() { _, errs[6] = fs.ReadDir(fsys, ".") },
				func() { errs[7] = fsys.Rename(renamable, renamed) },
				func() { _, errs[8] = handle.Stat() },
			)
			return errs
		}
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), existing.Get(t), randomContent(t))
			writeFile(t, subject.Get(t), removable.Get(t), randomContent(t))
			writeFile(t, subject.Get(t), renamable.Get(t), randomContent(t))
		})

		s.Then("every operation succeeds, without data races", func(t *testcase.T) {
			for _, err := range act(t) {
				assert.NoError(t, err)
			}

			fsys := subject.Get(t)
			_, err := fsys.Stat(dir.Get(t))
			assert.NoError(t, err)
			_, err = fsys.Stat(created.Get(t))
			assert.NoError(t, err)
			_, err = fsys.Stat(removable.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
			_, err = fsys.Stat(renamed.Get(t))
			assert.NoError(t, err)
			_, err = fsys.Stat(renamable.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})
}

// specCreatingInParentDirectory describes how creating name, by act, depends on the directory name would be created in.
func specCreatingInParentDirectory(s *testcase.Spec, subject testcase.Var[*memory.FileSystem], name testcase.Var[string], act func(t *testcase.T) error) {
	s.When("name is inside a directory", func(s *testcase.Spec) {
		dir := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		name.Let(s, func(t *testcase.T) string {
			return filepath.Join(dir.Get(t), t.Random.UUID())
		})
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(subject.Get(t).Mkdir(dir.Get(t), filemode.UserRWX))
		})

		s.Then("it is created in that directory", func(t *testcase.T) {
			assert.NoError(t, act(t))

			entries, err := filesystem.ReadDir(subject.Get(t), dir.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, entryNames(entries), []string{filepath.Base(name.Get(t))})
		})
	})

	s.When("name's parent directory does not exist", func(s *testcase.Spec) {
		name.Let(s, func(t *testcase.T) string {
			return filepath.Join(t.Random.UUID(), t.Random.UUID())
		})

		s.Then("it fails with fs.ErrNotExist, like an OS file system", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), fs.ErrNotExist)
		})

		s.Then("neither name nor its parent directory is created", func(t *testcase.T) {
			_ = act(t)

			_, err := subject.Get(t).Stat(name.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
			_, err = subject.Get(t).Stat(filepath.Dir(name.Get(t)))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("name is in a directory above the root", func(s *testcase.Spec) {
		name.Let(s, func(t *testcase.T) string {
			return filepath.Join("..", t.Random.UUID())
		})

		s.Then("it is created, as the directories above the working directory exist on disk too", func(t *testcase.T) {
			assert.NoError(t, act(t))

			_, err := subject.Get(t).Stat(name.Get(t))
			assert.NoError(t, err)
		})
	})
}

// specNameThroughFile describes how act, which uses name, fails when a directory on name's path is actually a file.
// Like the path resolution of an OS file system, it fails with ENOTDIR, and not with fs.ErrNotExist,
// however deep name is below the file.
func specNameThroughFile(s *testcase.Spec, subject testcase.Var[*memory.FileSystem], name testcase.Var[string], act func(t *testcase.T) error) {
	s.When("a directory on name's path is a file", func(s *testcase.Spec) {
		var (
			file    = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
			content = let.Var(s, randomContent)
		)
		name.Let(s, func(t *testcase.T) string {
			below := random.Slice(t.Random.IntBetween(1, 2), t.Random.UUID)
			// slash separated, so it is a valid name for fs.FS and the os package alike
			return path.Join(append([]string{file.Get(t)}, below...)...)
		})
		s.Before(func(t *testcase.T) {
			writeFile(t, subject.Get(t), file.Get(t), content.Get(t))
		})

		s.Then("it fails with ENOTDIR, like an OS file system", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, syscall.ENOTDIR)
			assert.False(t, errors.Is(err, fs.ErrNotExist), assert.Message("a missing name and a file in the way are told apart"))
		})

		s.Then("the file is left as it was", func(t *testcase.T) {
			_ = act(t)

			info, err := subject.Get(t).Stat(file.Get(t))
			assert.NoError(t, err)
			assert.False(t, info.IsDir())
			assert.Equal(t, readFile(t, subject.Get(t), file.Get(t)), content.Get(t))
		})
	})
}

func randomContent(t *testcase.T) string {
	return t.Random.StringN(t.Random.IntBetween(1, 64))
}

func writeFile(t *testcase.T, fsys filesystem.FileSystem, name, content string) {
	t.Helper()
	assert.Must(t).NoError(writeTo(fsys, name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, []byte(content)))
}

func writeTo(fsys filesystem.FileSystem, name string, flag int, data []byte) error {
	file, err := fsys.OpenFile(name, flag, filemode.UserRW)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}

func readFile(t *testcase.T, fsys filesystem.FileSystem, name string) string {
	t.Helper()
	file, err := filesystem.Open(fsys, name)
	assert.Must(t).NoError(err)
	defer file.Close()
	data, err := io.ReadAll(file)
	assert.Must(t).NoError(err)
	return string(data)
}

// overwrite returns what a file holding dst contains after src is written at its start.
func overwrite(dst, src string) string {
	if len(dst) <= len(src) {
		return src
	}
	return src + dst[len(src):]
}

func entryNames(entries []fs.DirEntry) []string {
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func absPath(t *testcase.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(name)
	assert.Must(t).NoError(err)
	return path
}
