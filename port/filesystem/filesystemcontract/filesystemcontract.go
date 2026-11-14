package filesystemcontract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// FileSystem is the behaviour expected from a file system.
//
// The subject must be a filesystem.FileOpener, because the contracts make and inspect files through OpenFile.
// FileSystem runs the FileOpener contract, and the contract of every other role the subject implements:
// FileInfoReader, DirectoryMaker, Remover, Renamer and fs.FS.
// Roles are detected on the subject's concrete type, even when passed through a narrower interface.
// Unsupported roles do not register tests.
func FileSystem[T filesystem.FileOpener](subject T, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)

	FileOpener(subject, opts...).Spec(s)

	if reflectkit.TypeImplements[any, filesystem.FileInfoReader](subject) {
		FileInfoReader(any(subject).(fileInfoReaderSubject), opts...).Spec(s)
	}
	if reflectkit.TypeImplements[any, filesystem.DirectoryMaker](subject) {
		DirectoryMaker(any(subject).(directoryMakerSubject), opts...).Spec(s)
	}
	if reflectkit.TypeImplements[any, filesystem.Remover](subject) {
		Remover(any(subject).(removerSubject), opts...).Spec(s)
	}
	if reflectkit.TypeImplements[any, filesystem.Renamer](subject) {
		Renamer(any(subject).(renamerSubject), opts...).Spec(s)
	}
	if reflectkit.TypeImplements[any, fs.FS](subject) {
		FS(any(subject).(fsSubject), opts...).Spec(s)
	}

	return s.AsSuite("FileSystem")
}

// FileOpener is the behaviour expected from every filesystem.FileOpener, and from the File its OpenFile returns.
func FileOpener(subject filesystem.FileOpener, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	spec := newSpec(subject, opts)

	s.Describe(".OpenFile", spec.specOpenFile)
	s.Describe("#File", func(s *testcase.Spec) {
		s.Describe(".ReadDir", spec.specFile_ReadDir)
		s.Describe(".Seek", spec.specFile_Seek)
	})

	return s.AsSuite("FileOpener")
}

// FileInfoReader is the behaviour expected from every filesystem.FileInfoReader.
func FileInfoReader(subject interface {
	filesystem.FileInfoReader
	filesystem.FileOpener
}, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe(".Stat", newSpec(subject, opts).specStat)
	return s.AsSuite("FileInfoReader")
}

// DirectoryMaker is the behaviour expected from every filesystem.DirectoryMaker.
func DirectoryMaker(subject interface {
	filesystem.DirectoryMaker
	filesystem.FileOpener
}, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe(".Mkdir", newSpec(subject, opts).specMkdir)
	return s.AsSuite("DirectoryMaker")
}

// Remover is the behaviour expected from every filesystem.Remover.
func Remover(subject interface {
	filesystem.Remover
	filesystem.FileOpener
}, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe(".Remove", newSpec(subject, opts).specRemove)
	return s.AsSuite("Remover")
}

// Renamer is the behaviour expected from every filesystem.Renamer.
func Renamer(subject interface {
	filesystem.Renamer
	filesystem.FileOpener
}, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe(".Rename", newSpec(subject, opts).specRename)
	return s.AsSuite("Renamer")
}

// FS is the behaviour expected from a file system which is also an fs.FS.
//
// It verifies that fs.FS sees the same files as the filesystem role interfaces do,
// and that it follows the io/fs conventions.
// This covers the optional io/fs interfaces it implements as well
// (fs.StatFS, fs.ReadFileFS, fs.ReadDirFS, fs.GlobFS, fs.SubFS, fs.ReadLinkFS),
// through the io/fs helper functions which use them, and through testing/fstest.
func FS(subject interface {
	fs.FS
	filesystem.FileOpener
}, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	spec := newSpec(subject, opts)

	s.Describe(".Open", spec.specFS_Open)
	s.Describe("fs.ReadFile", spec.specFS_ReadFile)
	s.Describe("fs.ReadDir", spec.specFS_ReadDir)
	s.Describe("fstest.TestFS", spec.specFS_TestFS)

	return s.AsSuite("fs.FS")
}

type (
	fileInfoReaderSubject = interface {
		filesystem.FileInfoReader
		filesystem.FileOpener
	}
	directoryMakerSubject = interface {
		filesystem.DirectoryMaker
		filesystem.FileOpener
	}
	removerSubject = interface {
		filesystem.Remover
		filesystem.FileOpener
	}
	renamerSubject = interface {
		filesystem.Renamer
		filesystem.FileOpener
	}
	fsSubject = interface {
		fs.FS
		filesystem.FileOpener
	}
)

func newSpec(subject filesystem.FileOpener, opts []Option) specFileSystem {
	return specFileSystem{
		FileSystem: subject,
		Config:     option.ToConfig[Config](opts),
	}
}

// specFileSystem holds the specs of every role interface.
// Scenarios needing additional roles are registered only when the subject implements them.
type specFileSystem struct {
	FileSystem filesystem.FileOpener
	Config     Config
}

func (c specFileSystem) stat(name string) (fs.FileInfo, error) {
	return c.FileSystem.(filesystem.FileInfoReader).Stat(name)
}

func (c specFileSystem) mkdir(name string, perm fs.FileMode) error {
	return c.FileSystem.(filesystem.DirectoryMaker).Mkdir(name, perm)
}

func (c specFileSystem) remove(name string) error {
	return c.FileSystem.(filesystem.Remover).Remove(name)
}

func (c specFileSystem) rename(oldpath, newpath string) error {
	return c.FileSystem.(filesystem.Renamer).Rename(oldpath, newpath)
}

// tryRemove removes name, when the subject is a Remover.
func (c specFileSystem) tryRemove(name string) {
	if remover, ok := c.FileSystem.(filesystem.Remover); ok {
		_ = remover.Remove(name)
	}
}

// fileInfo describes name through Stat when the subject is a FileInfoReader,
// and through the File which OpenFile opens otherwise.
func (c specFileSystem) fileInfo(name string) (fs.FileInfo, error) {
	if _, ok := c.FileSystem.(filesystem.FileInfoReader); ok {
		return c.stat(name)
	}
	file, err := c.FileSystem.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (c specFileSystem) perm() testcase.Var[fs.FileMode] {
	return testcase.Var[fs.FileMode]{
		ID: "fs perm",
		Init: func(t *testcase.T) fs.FileMode {
			var mode fs.FileMode
			if t.Random.Bool() {
				mode |= filemode.UserRWX
			} else {
				mode |= filemode.UserRW
			}
			if t.Random.Bool() {
				mode |= filemode.OtherR
			}
			return mode
		},
	}
}

func (c specFileSystem) name() testcase.Var[string] {
	return testcase.Var[string]{
		ID: "file name",
		// Without a Remover, the made files stay, so the name must not collide with them.
		Init: c.randomName,
	}
}

func (c specFileSystem) specOpenFile(s *testcase.Spec) {
	var flag = testcase.Var[int]{ID: "file open flag"}
	subject := func(t *testcase.T) (filesystem.File, error) {
		file, err := c.FileSystem.OpenFile(c.name().Get(t), flag.Get(t), c.perm().Get(t))
		if err == nil {
			t.Defer(file.Close)
		}
		return file, err
	}

	s.When("name points to an unexistent file", func(s *testcase.Spec) {
		s.And("open flag is read only", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDONLY)

			s.Before(func(t *testcase.T) {
				c.tryRemove(c.name().Get(t))
			})

			c.andForAbsentFileOpening(s, subject, flag)

			s.Then("it yields error because the file is not existing", func(t *testcase.T) {
				_, err := subject(t)

				c.assertErrorIsNotExist(t, err, c.name().Get(t))
			})

			s.And("flag also has O_CREATE", func(s *testcase.Spec) {
				flag.LetValue(s, os.O_RDONLY|os.O_CREATE)
				s.Before(func(t *testcase.T) {
					t.Defer(c.tryRemove, c.name().Get(t))
				})

				s.Then("it creates an empty file", func(t *testcase.T) {
					file, err := subject(t)
					assert.Must(t).NoError(err)

					c.assertReaderEquals(t, []byte{}, file)
				})
			})
		})
	})

	s.When("name points to the current working directory", func(s *testcase.Spec) {
		c.name().LetValue(s, ".")
		flag.LetValue(s, os.O_RDONLY)

		s.Then("current working directory file is returned", func(t *testcase.T) {
			file, err := subject(t)
			assert.Must(t).NoError(err)

			info, err := file.Stat()
			assert.Must(t).NoError(err)
			assert.True(t, info.IsDir())
		})
	})

	s.When("name points to an existing file", func(s *testcase.Spec) {
		content := testcase.Let(s, func(t *testcase.T) string {
			str := t.Random.String()
			t.Log("initial file content:", str)
			return str
		})
		s.Before(func(t *testcase.T) {
			c.saveFile(t, c.name().Get(t), []byte(content.Get(t)))
		})

		s.And("we open with read only", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDONLY)

			c.andForTheExistingFileOpening(s, subject, c.name(), flag, content)

			s.Then("returned file can be consumed for its content", func(t *testcase.T) {
				file, err := subject(t)
				assert.Must(t).NoError(err)
				c.assertReaderEquals(t, []byte(content.Get(t)), file)
			})

			s.Then("returned file does not allow writing", func(t *testcase.T) {
				file, err := subject(t)
				assert.Must(t).NoError(err)
				_, err = file.Write([]byte(t.Random.String()))
				c.assertWriteError(t, err, c.name().Get(t))
			})
		})

		s.And("we open with write only", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_WRONLY)

			c.andForTheExistingFileOpening(s, subject, c.name(), flag, content)

			s.Then("returned file can not be consumed for its content", func(t *testcase.T) {
				file, err := subject(t)
				assert.Must(t).NoError(err)

				_, err = file.Read(make([]byte, 1))
				c.assertReadError(t, err, c.name().Get(t))
			})

			c.thenCanBeWritten(s, subject, flag, content)
		})

		s.And("we open the file in read-write mode", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDWR)

			c.andForTheExistingFileOpening(s, subject, c.name(), flag, content)

			s.Then("returned file's contents can read out", func(t *testcase.T) {
				file, err := subject(t)
				assert.Must(t).NoError(err)

				c.assertReaderEquals(t, []byte(content.Get(t)), file)
			})

			c.thenCanBeWritten(s, subject, flag, content)
		})
	})
}

func (c specFileSystem) andForTheExistingFileOpening(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error),
	name testcase.Var[string],
	flag testcase.Var[int],
	content testcase.Var[string]) {

	s.And("O_CREATE flag is also given", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			flag.Set(t, flag.Get(t)|os.O_CREATE)
		})

		s.And("O_EXCL is also provided", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				flag.Set(t, flag.Get(t)|os.O_EXCL)
			})

			s.Then("it will yield error as ECXL forbids opening on existing file", func(t *testcase.T) {
				_, err := subject(t)
				assert.Must(t).NotNil(err)
				perr := c.isPathError(t, err)
				assert.Must(t).Equal("open", perr.Op)
				assert.Contains(t, perr.Path, name.Get(t))
				assert.True(t, os.IsExist(err))
			})
		})

		s.Then("file opening succeeds with existing content since file already exists", func(t *testcase.T) {
			_, err := subject(t)
			assert.Must(t).NoError(err)
			c.assertFileContent(t, name.Get(t), []byte(content.Get(t)))
		})
	})
}

func (c specFileSystem) andForAbsentFileOpening(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error), flag testcase.Var[int]) {
	thenFileCanBeCreated := func(s *testcase.Spec) {
		s.Then("file opening succeeds with file is just created", func(t *testcase.T) {
			_, err := subject(t)
			assert.Must(t).NoError(err)
			c.assertFileContent(t, c.name().Get(t), []byte{})
		})

		s.Then("permission matches the permission of the newly created file", func(t *testcase.T) {
			file, err := subject(t)
			assert.Must(t).NoError(err)
			info, err := file.Stat()
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(c.perm().Get(t).String(), info.Mode().String())
		})
	}

	s.And("O_CREATE flag is also given", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			flag.Set(t, flag.Get(t)|os.O_CREATE)
			// clean up newly created files
			t.Defer(c.tryRemove, c.name().Get(t))
		})

		thenFileCanBeCreated(s)

		s.And("O_EXCL is also provided", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				flag.Set(t, flag.Get(t)|os.O_EXCL)
			})

			thenFileCanBeCreated(s)
		})
	})
}

func (c specFileSystem) thenCanBeWritten(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error),
	flag testcase.Var[int],
	initialContent testcase.Var[string],
) {
	s.Then("returned file allows writing over the existing initialContent", func(t *testcase.T) {
		file, err := subject(t)
		assert.Must(t).NoError(err)

		data := t.Random.String()
		c.writeToFile(t, file, []byte(data))
		assert.Must(t).NoError(file.Close())

		expectedContent := append([]byte{}, []byte(initialContent.Get(t))...)
		expectedContent = c.overwrite(expectedContent, []byte(data))
		c.assertFileContent(t, c.name().Get(t), expectedContent)
	})

	s.And("we also pass truncate file opening flag (os.O_TRUNC)", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			flag.Set(t, flag.Get(t)|os.O_TRUNC)
		})

		s.Then("returned file allows writing on the truncated file", func(t *testcase.T) {
			file, err := subject(t)
			assert.Must(t).NoError(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))
			assert.Must(t).NoError(file.Close())

			c.assertFileContent(t, c.name().Get(t), []byte(data))
		})
	})

	s.And("we also pass the append file opening flag (os.O_APPEND)", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Log("os.O_APPEND tells the underlying Local that all the write calls you do on that file handler should always append to the file so you don't need to set the offset to write on the correct part of the file.")
			flag.Set(t, flag.Get(t)|os.O_APPEND)
		})

		s.Then(".Write appends to the end of the file", func(t *testcase.T) {
			file, err := subject(t)
			assert.Must(t).NoError(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))

			if !(flag.Get(t)&os.O_WRONLY != 0) {
				t.Log("and reading after writing starts from the last position")
				_, err := file.Read([]byte{0})
				assert.Must(t).ErrorIs(io.EOF, err)
			}

			assert.Must(t).NoError(file.Close())
			expectedContent := append([]byte(initialContent.Get(t)), []byte(data)...)
			c.assertFileContent(t, c.name().Get(t), expectedContent)
		})

		s.Then(".Write always appends to the end of the file, regardless if seek were used", func(t *testcase.T) {
			file, err := subject(t)
			assert.Must(t).NoError(err)

			_, err = file.Seek(0, io.SeekStart)
			assert.Must(t).NoError(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))
			assert.Must(t).NoError(file.Close())

			expectedContent := append([]byte(initialContent.Get(t)), []byte(data)...)
			c.assertFileContent(t, c.name().Get(t), expectedContent)
		})
	})
}

func (c specFileSystem) specMkdir(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		err := c.mkdir(c.name().Get(t), c.perm().Get(t))
		if err == nil {
			t.Defer(c.tryRemove, c.name().Get(t))
		}
		return err
	}

	s.When("when name points to a non-existing file path", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, err := c.fileInfo(c.name().Get(t))
			assert.True(t, os.IsNotExist(err))
		})

		s.Then("directory can be made without an issue", func(t *testcase.T) {
			assert.Must(t).NoError(subject(t))
		})

		s.And("on successful directory making", func(s *testcase.Spec) {
			cTime := testcase.Let[time.Time](s, nil)
			s.Before(func(t *testcase.T) {
				cTime.Set(t, time.Now().UTC())
				assert.Must(t).NoError(subject(t))
			})

			assertFileInfo := func(t *testcase.T, info fs.FileInfo) {
				t.Helper()
				assert.True(t, info.IsDir())
				assert.Must(t).Equal((c.perm().Get(t) | fs.ModeDir).String(), info.Mode().String())
				c.assertFileTime(t, cTime.Get(t), info.ModTime())
			}

			if reflectkit.TypeImplements[any, filesystem.FileInfoReader](c.FileSystem) {
				s.Then("FileSystem.Stat returns the directory details", func(t *testcase.T) {
					info, err := c.stat(c.name().Get(t))
					assert.Must(t).NoError(err)
					assertFileInfo(t, info)
				})
			}

			s.Then("FileSystem.OpenFile returns the directory details", func(t *testcase.T) {
				file, err := c.FileSystem.OpenFile(c.name().Get(t), os.O_RDONLY, 0)
				assert.Must(t).NoError(err)
				t.Defer(file.Close)

				info, err := file.Stat()
				assert.Must(t).NoError(err)
				assertFileInfo(t, info)

				entries, err := file.ReadDir(-1)
				assert.Must(t).NoError(err)
				assert.Must(t).Empty(entries)
			})

			s.Then("reading the directory as a file fails, rather than reading as empty", func(t *testcase.T) {
				file, err := c.FileSystem.OpenFile(c.name().Get(t), os.O_RDONLY, 0)
				assert.Must(t).NoError(err)
				t.Defer(file.Close)

				_, err = file.Read(make([]byte, 1))
				c.assertReadError(t, err, c.name().Get(t))
			})
		})
	})

	s.When("when name points to an existing file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), filemode.UserRWX)
		})

		s.Then("directory making fails", func(t *testcase.T) {
			err := subject(t)
			assert.Must(t).NotNil(err)
			assert.True(t, os.IsExist(err))

			perr := c.isPathError(t, err)
			assert.Must(t).Equal("mkdir", perr.Op)
			assert.Contains(t, perr.Path, c.name().Get(t))
		})
	})

	s.When("when name points to an existing directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.mkdir(c.name().Get(t), 0700))
			t.Defer(c.tryRemove, c.name().Get(t))
		})

		s.Then("directory making fails", func(t *testcase.T) {
			err := subject(t)
			assert.Must(t).NotNil(err)
			assert.True(t, os.IsExist(err))

			perr := c.isPathError(t, err)
			assert.Must(t).Equal("mkdir", perr.Op)
		})
	})
}

func (c specFileSystem) assertFileTime(t *testcase.T, cTime, modTime time.Time) {
	// In certain file systems, the modification timestamp might have lower precision
	// than our creation timestamp, offering only second-level accuracy.
	const rounding = time.Second
	cTime = cTime.UTC().Round(rounding)
	modTime = modTime.UTC().Round(rounding)
	assert.True(t, cTime.Before(modTime) || cTime.Equal(modTime))
}

func (c specFileSystem) specRemove(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		return c.remove(c.name().Get(t))
	}

	s.When("name points to nothing", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, err := c.fileInfo(c.name().Get(t))
			assert.True(t, os.IsNotExist(err))
		})

		s.Then("it yields error", func(t *testcase.T) {
			err := subject(t)
			assert.Must(t).NotNil(err)
			assert.True(t, os.IsNotExist(err))

			perr := c.isPathError(t, err)
			assert.Must(t).Equal("remove", perr.Op)
			assert.Contains(t, perr.Path, c.name().Get(t))
		})
	})

	s.When("name points to a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), c.perm().Get(t))
		})

		s.Then("it will remove the file", func(t *testcase.T) {
			assert.Must(t).NoError(subject(t))
			_, err := c.fileInfo(c.name().Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("name points to a directory", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.mkdir(c.name().Get(t), 0700))
			t.Defer(c.tryRemove, c.name().Get(t))
		})

		s.Then("it will remove the directory", func(t *testcase.T) {
			assert.Must(t).NoError(subject(t))
			_, err := c.fileInfo(c.name().Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.And("the directory has a file", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				fPath := filepath.Join(c.name().Get(t), "file.tdd")
				c.touchFile(t, fPath, 0700)
			})

			s.Then("it will yield error because the directory is not empty", func(t *testcase.T) {
				err := subject(t)
				assert.Must(t).NotNil(err)

				perr := c.isPathError(t, err)
				assert.Must(t).Equal("remove", perr.Op)
				assert.Contains(t, perr.Path, c.name().Get(t))
				assert.Must(t).Equal(syscall.ENOTEMPTY, perr.Err)
			})
		})
	})
}

func (c specFileSystem) specRename(s *testcase.Spec) {
	var (
		oldName = testcase.Let(s, c.randomName)
		newName = testcase.Let(s, c.randomName)
		content = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
	)
	act := func(t *testcase.T) error {
		return c.rename(oldName.Get(t), newName.Get(t))
	}
	s.Before(func(t *testcase.T) {
		t.Defer(c.tryRemove, newName.Get(t))
		c.writeNewFile(t, oldName.Get(t), c.perm().Get(t), []byte(content.Get(t)))
	})

	s.Then("the file is moved to the new name", func(t *testcase.T) {
		assert.NoError(t, act(t))

		c.assertFileContent(t, newName.Get(t), []byte(content.Get(t)))
		_, err := c.fileInfo(oldName.Get(t))
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	s.Then("the file keeps its mode", func(t *testcase.T) {
		before, err := c.fileInfo(oldName.Get(t))
		assert.Must(t).NoError(err)

		assert.NoError(t, act(t))

		after, err := c.fileInfo(newName.Get(t))
		assert.NoError(t, err)
		assert.Equal(t, before.Mode().String(), after.Mode().String())
	})

	s.When("new name points to an existing file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.saveFile(t, newName.Get(t), []byte(t.Random.String()))
		})

		s.Then("the existing file is replaced", func(t *testcase.T) {
			assert.NoError(t, act(t))

			c.assertFileContent(t, newName.Get(t), []byte(content.Get(t)))
			_, err := c.fileInfo(oldName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("new name points to an existing directory", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			c.touchDir(t, newName.Get(t), filemode.UserRWX)
		})

		s.Then("it yields an error, as a directory is not replaced", func(t *testcase.T) {
			c.assertLinkError(t, act(t), oldName.Get(t), newName.Get(t))
		})

		s.Then("both the file and the directory stay as they were", func(t *testcase.T) {
			_ = act(t)

			c.assertFileContent(t, oldName.Get(t), []byte(content.Get(t)))
			info, err := c.fileInfo(newName.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
		})
	})

	s.When("new name is in a directory which doesn't exist", func(s *testcase.Spec) {
		newName.Let(s, func(t *testcase.T) string {
			return filepath.Join(c.randomName(t), c.randomName(t))
		})

		s.Then("it yields an error, as the directory is not created", func(t *testcase.T) {
			err := act(t)
			c.assertLinkError(t, err, oldName.Get(t), newName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.Then("the file stays at the old name", func(t *testcase.T) {
			_ = act(t)

			c.assertFileContent(t, oldName.Get(t), []byte(content.Get(t)))
		})
	})

	s.When("new name is the same as the old name", func(s *testcase.Spec) {
		newName.Let(s, func(t *testcase.T) string { return oldName.Get(t) })

		s.Then("the file stays as it was", func(t *testcase.T) {
			assert.NoError(t, act(t))

			c.assertFileContent(t, oldName.Get(t), []byte(content.Get(t)))
		})
	})

	s.When("old name points to nothing", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.remove(oldName.Get(t)))
		})

		s.Then("it yields an error, as there is nothing to rename", func(t *testcase.T) {
			err := act(t)
			c.assertLinkError(t, err, oldName.Get(t), newName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.Then("nothing is made at the new name", func(t *testcase.T) {
			_ = act(t)

			_, err := c.fileInfo(newName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})

	s.When("old name points to a directory instead of a file", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) || !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		entryName := testcase.Let(s, c.randomName)
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.remove(oldName.Get(t)))
			c.touchDir(t, oldName.Get(t), filemode.UserRWX)
			t.Defer(c.tryRemove, filepath.Join(newName.Get(t), entryName.Get(t)))
			c.saveFile(t, filepath.Join(oldName.Get(t), entryName.Get(t)), []byte(content.Get(t)))
		})

		s.Then("the directory is moved to the new name, along with its entries", func(t *testcase.T) {
			assert.NoError(t, act(t))

			info, err := c.fileInfo(newName.Get(t))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
			c.assertFileContent(t, filepath.Join(newName.Get(t), entryName.Get(t)), []byte(content.Get(t)))
			_, err = c.fileInfo(oldName.Get(t))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})

		s.And("new name is inside the directory", func(s *testcase.Spec) {
			newName.Let(s, func(t *testcase.T) string {
				return filepath.Join(oldName.Get(t), c.randomName(t))
			})

			s.Then("it yields an error, as a directory can't be moved into itself", func(t *testcase.T) {
				c.assertLinkError(t, act(t), oldName.Get(t), newName.Get(t))
			})

			s.Then("the directory stays as it was", func(t *testcase.T) {
				_ = act(t)

				c.assertFileContent(t, filepath.Join(oldName.Get(t), entryName.Get(t)), []byte(content.Get(t)))
				_, err := c.fileInfo(newName.Get(t))
				assert.ErrorIs(t, err, fs.ErrNotExist)
			})
		})
	})
}

func (c specFileSystem) specStat(s *testcase.Spec) {
	subject := func(t *testcase.T) (fs.FileInfo, error) {
		return c.stat(c.name().Get(t))
	}

	s.When("name points to nothing", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.tryRemove(c.name().Get(t))
		})

		s.Then("it yields error", func(t *testcase.T) {
			_, err := subject(t)
			assert.Must(t).NotNil(err)
			assert.True(t, os.IsNotExist(err))

			perr := c.isPathError(t, err)
			assert.Must(t).Equal("stat", perr.Op)
			assert.Contains(t, perr.Path, c.name().Get(t))
		})
	})

	s.When("name points to a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), c.perm().Get(t))
		})

		s.Then("it will return file stat", func(t *testcase.T) {
			info, err := subject(t)
			assert.Must(t).NoError(err)
			assert.Contains(t, info.Name(), c.name().Get(t))
			assert.Must(t).Equal(c.perm().Get(t).String(), info.Mode().String())
			assert.Must(t).False(info.IsDir())
		})
	})

	s.When("name points to a directory", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			c.perm().Set(t, c.perm().Get(t)|fs.ModeDir)
			assert.Must(t).NoError(c.mkdir(c.name().Get(t), c.perm().Get(t)))
			t.Defer(c.tryRemove, c.name().Get(t))
		})

		s.Then("it will return directory stat", func(t *testcase.T) {
			info, err := subject(t)
			assert.Must(t).NoError(err)
			assert.Contains(t, info.Name(), c.name().Get(t))
			assert.Must(t).Equal(c.perm().Get(t).String(), info.Mode().String())
			assert.True(t, info.IsDir())
		})
	})

	s.When("name points to the current working directory", func(s *testcase.Spec) {
		c.name().LetValue(s, ".")

		s.Then("it will return directory stat", func(t *testcase.T) {
			info, err := subject(t)
			assert.Must(t).NoError(err)
			assert.True(t, info.IsDir())
		})
	})
}

func (c specFileSystem) specFile_ReadDir(s *testcase.Spec) {
	var (
		n    = testcase.Let[int](s, nil)
		file = testcase.Let(s, func(t *testcase.T) filesystem.File {
			file, err := c.FileSystem.OpenFile(c.name().Get(t), os.O_RDONLY, 0)
			if err != nil {
				t.Log(err.Error())
			}
			assert.Must(t).NoError(err)
			t.Defer(file.Close)
			return file
		})
	)
	subject := func(t *testcase.T) ([]fs.DirEntry, error) {
		return file.Get(t).ReadDir(n.Get(t))
	}

	s.When("name points to a file", func(s *testcase.Spec) {
		n.LetValue(s, -1)

		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), c.perm().Get(t))
		})

		s.Then("it will return an empty list", func(t *testcase.T) {
			_, err := subject(t)
			assert.Must(t).NotNil(err)

			perr := c.isPathError(t, err)
			assert.Contains(t, []string{"fdopendir", "readdirent"}, perr.Op)
			assert.Contains(t, perr.Path, c.name().Get(t))
			assert.Must(t).ErrorIs(syscall.ENOTDIR, perr.Err)
		})
	})

	s.When("name points to the working directory as .", func(s *testcase.Spec) {
		// Listing exactly the files below needs an otherwise empty root,
		// which only a Remover can restore after the other specs.
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		n.LetValue(s, -1)
		c.name().LetValue(s, ".")

		s.And("it contains files", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c.touchFile(t, filepath.Join(".", "a"), filemode.UserRWX)
				c.touchFile(t, filepath.Join(".", "b"), filemode.UserRWX)
				c.touchFile(t, filepath.Join(".", "c"), filemode.UserRWX)
			})

			s.Then("directory entries are returned", func(t *testcase.T) {
				entries, err := subject(t)
				assert.Must(t).NoError(err)
				assert.Must(t).NotEmpty(entries)
				assert.Must(t).Equal(3, len(entries))

				names := []string{"a", "b", "c"}
				for _, entry := range entries {
					assert.Contains(t, names, entry.Name())
				}
			})
		})
	})

	s.When("a directory exists where the file name points", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			c.touchDir(t, c.name().Get(t), c.perm().Get(t)|filemode.UserRWX)
		})

		dirFileNames := testcase.Var[[]string]{ID: "dirFileNames", Init: func(t *testcase.T) []string {
			var names []string
			l := t.Random.IntB(3, 10)
			for i := 0; i < l; i++ {
				bname := t.Random.StringNWithCharset(3, "qwerty")
				names = append(names, fmt.Sprintf("%d-%s", i, bname))
			}
			return names
		}}

		s.And("n is zero", func(s *testcase.Spec) {
			n.LetValue(s, 0)

			s.Then("it will return back an empty list successfully", func(t *testcase.T) {
				dirEntries, err := subject(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Empty(dirEntries)
			})
		})

		s.And("n is less than zero which means give me back everything", func(s *testcase.Spec) {
			n.LetValue(s, -1)

			s.Then("it will return that the directory is empty", func(t *testcase.T) {
				entries, err := subject(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Empty(entries)
			})

			s.And("directory has file(s)", func(s *testcase.Spec) {
				dirFileNames.Bind(s)
				const expectedEntryPerm = filemode.UserRWX

				cTime := testcase.Let[time.Time](s, nil)
				s.Before(func(t *testcase.T) {
					cTime.Set(t, time.Now().UTC())
					for _, bname := range dirFileNames.Get(t) {
						c.touchFile(t, filepath.Join(c.name().Get(t), bname), expectedEntryPerm)
					}
				})

				s.Then("it lists all the directory entry", func(t *testcase.T) {
					entries, err := subject(t)
					assert.Must(t).NoError(err)
					assert.Must(t).NotEmpty(entries)

					dirFileNames := dirFileNames.Get(t)
					assert.Must(t).Equal(len(dirFileNames), len(entries))
					for _, ent := range entries {
						assert.Contains(t, dirFileNames, ent.Name())
						assert.Must(t).False(ent.IsDir())

						info, err := ent.Info()
						assert.Must(t).NoError(err)
						_ = info.Sys()
						assert.Must(t).False(info.IsDir())
						c.assertFileTime(t, cTime.Get(t), info.ModTime())
						assert.Must(t).Equal(info.Mode().Type(), ent.Type())
						assert.Must(t).False(ent.Type()&fs.ModeDir != 0, "no ModeDir flag is expected")
					}
				})

				s.Then("it lists entries, but only on the first call", func(t *testcase.T) {
					entries, err := subject(t)
					assert.Must(t).NoError(err)
					assert.Must(t).NotEmpty(entries)

					entries, err = subject(t)
					assert.Must(t).NoError(err)
					assert.Must(t).Empty(entries)
				})
			})
		})

		s.And("n is a bigger than zero which means give me back the next N amount of entry", func(s *testcase.Spec) {
			n.LetValue(s, 1)

			s.Then("it will return that the directory is empty by stating io.EOF", func(t *testcase.T) {
				_, err := subject(t)
				assert.Must(t).ErrorIs(io.EOF, err)
			})

			s.And("directory has file(s)", func(s *testcase.Spec) {
				dirFileNames.Bind(s)

				s.Before(func(t *testcase.T) {
					for _, bname := range dirFileNames.Get(t) {
						c.touchFile(t, filepath.Join(c.name().Get(t), bname), 0700)
					}
				})

				s.Then("it iterates over the entries in chunks of N", func(t *testcase.T) {
					entries, err := subject(t)
					assert.Must(t).NoError(err)
					assert.Must(t).NotEmpty(entries)
					assert.Must(t).Equal(n.Get(t), len(entries))

				consuming:
					for {
						entrs, err := subject(t)
						if err == io.EOF {
							break consuming
						}
						assert.Must(t).NoError(err)
						entries = append(entries, entrs...)
					}

					dirFileNames := dirFileNames.Get(t)
					assert.Must(t).Equal(len(dirFileNames), len(entries))

					for _, ent := range entries {
						assert.Contains(t, dirFileNames, ent.Name())
					}
				})
			})
		})
	})
}

// fsys is the subject as an fs.FS. Only the specs of the FS contract use it.
func (c specFileSystem) fsys() fs.FS {
	return c.FileSystem.(fs.FS)
}

func (c specFileSystem) specFS_Open(s *testcase.Spec) {
	var (
		content  = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
		fileName = testcase.Let(s, c.randomName)
		// filePath is the name of the file for the filesystem.FileSystem methods.
		filePath = testcase.Let(s, func(t *testcase.T) string { return fileName.Get(t) })
		// name is the name given to fs.FS, which is always slash separated.
		name = testcase.Let(s, func(t *testcase.T) string { return fileName.Get(t) })
	)
	act := func(t *testcase.T) (fs.File, error) {
		file, err := c.fsys().Open(name.Get(t))
		if err == nil {
			t.Defer(file.Close)
		}
		return file, err
	}
	s.Before(func(t *testcase.T) {
		c.writeNewFile(t, filePath.Get(t), c.perm().Get(t), []byte(content.Get(t)))
	})

	s.Then("it returns what was written into the file through FileSystem.OpenFile", func(t *testcase.T) {
		c.assertOpenedContent(t, act, []byte(content.Get(t)))
	})

	if reflectkit.TypeImplements[any, filesystem.FileInfoReader](c.FileSystem) {
		s.Then("the opened file's stat agrees with FileSystem.Stat", func(t *testcase.T) {
			file, err := act(t)
			assert.Must(t).NoError(err)
			got, err := file.Stat()
			assert.Must(t).NoError(err)

			exp, err := c.stat(filePath.Get(t))
			assert.Must(t).NoError(err)
			assert.Equal(t, exp.Name(), got.Name())
			assert.Equal(t, exp.Size(), got.Size())
			assert.Equal(t, exp.Mode().String(), got.Mode().String())
			assert.Equal(t, exp.IsDir(), got.IsDir())
			assert.True(t, exp.ModTime().Equal(got.ModTime()),
				assert.Message(fmt.Sprintf("ModTime: FileSystem.Stat=%s fs.File.Stat=%s", exp.ModTime(), got.ModTime())))
		})
	}

	s.Then("the file is opened for reading only", func(t *testcase.T) {
		file, err := act(t)
		assert.Must(t).NoError(err)

		if w, ok := file.(io.Writer); ok {
			_, err = w.Write([]byte(t.Random.StringN(t.Random.IntBetween(1, 42))))
			assert.Error(t, err)
		}
		c.assertFileContent(t, filePath.Get(t), []byte(content.Get(t)))
	})

	s.When("name points to nothing", func(s *testcase.Spec) {
		name.Let(s, c.randomName)

		s.Then("it yields fs.ErrNotExist", func(t *testcase.T) {
			_, err := act(t)
			c.assertFSOpenError(t, err, name.Get(t), fs.ErrNotExist)
		})
	})

	s.When("name points to the root directory", func(s *testcase.Spec) {
		name.LetValue(s, ".")

		s.Then("it opens the directory, which lists the file made through FileSystem.OpenFile", func(t *testcase.T) {
			file, err := act(t)
			assert.Must(t).NoError(err)
			assert.Contains(t, c.readDirNames(t, file), fileName.Get(t))
		})
	})

	s.When("the file is rewritten through FileSystem.OpenFile", func(s *testcase.Spec) {
		newContent := testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
		s.Before(func(t *testcase.T) {
			file, err := c.FileSystem.OpenFile(filePath.Get(t), os.O_WRONLY|os.O_TRUNC, 0)
			assert.Must(t).NoError(err)
			c.writeToFile(t, file, []byte(newContent.Get(t)))
			assert.Must(t).NoError(file.Close())
		})

		s.Then("it returns the new content", func(t *testcase.T) {
			c.assertOpenedContent(t, act, []byte(newContent.Get(t)))
		})
	})

	s.When("the file is removed through FileSystem.Remove", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.remove(filePath.Get(t)))
		})

		s.Then("it yields fs.ErrNotExist", func(t *testcase.T) {
			_, err := act(t)
			c.assertFSOpenError(t, err, name.Get(t), fs.ErrNotExist)
		})
	})

	s.When("the file is renamed through FileSystem.Rename", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Renamer](c.FileSystem) {
			return
		}
		newName := testcase.Let(s, c.randomName)
		s.Before(func(t *testcase.T) {
			t.Defer(c.tryRemove, newName.Get(t))
			assert.Must(t).NoError(c.rename(filePath.Get(t), newName.Get(t)))
		})

		s.Then("the old name yields fs.ErrNotExist", func(t *testcase.T) {
			_, err := act(t)
			c.assertFSOpenError(t, err, name.Get(t), fs.ErrNotExist)
		})

		s.And("name is the new name", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string { return newName.Get(t) })

			s.Then("it returns the content written before the rename", func(t *testcase.T) {
				c.assertOpenedContent(t, act, []byte(content.Get(t)))
			})
		})
	})

	s.When("the file is in a directory made with FileSystem.Mkdir", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
			return
		}
		dirName := testcase.Let(s, func(t *testcase.T) string {
			name := c.randomName(t)
			c.touchDir(t, name, filemode.UserRWX)
			return name
		})
		filePath.Let(s, func(t *testcase.T) string {
			return filepath.Join(dirName.Get(t), fileName.Get(t))
		})
		name.Let(s, func(t *testcase.T) string {
			return path.Join(dirName.Get(t), fileName.Get(t))
		})

		s.Then("it returns the file's content for its slash separated name", func(t *testcase.T) {
			c.assertOpenedContent(t, act, []byte(content.Get(t)))
		})

		s.And("name points to the directory", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string { return dirName.Get(t) })

			s.Then("it opens the directory, which lists the file made in it", func(t *testcase.T) {
				file, err := act(t)
				assert.Must(t).NoError(err)

				info, err := file.Stat()
				assert.Must(t).NoError(err)
				assert.True(t, info.IsDir())
				assert.Equal(t, []string{fileName.Get(t)}, c.readDirNames(t, file))
			})
		})

		s.And("name points to nothing in the directory", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string {
				return path.Join(dirName.Get(t), c.randomName(t))
			})

			s.Then("it yields fs.ErrNotExist for the slash separated name it was given", func(t *testcase.T) {
				_, err := act(t)
				c.assertFSOpenError(t, err, name.Get(t), fs.ErrNotExist)
			})
		})

		// Each of these names would be resolved by the os package, many of them to the file or the directory,
		// but fs.FS only accepts names which fs.ValidPath allows.
		// Accepting ".." or absolute names would let fs.FS consumers reach outside of the file system.
		for _, invalid := range []struct {
			desc string
			name func(dir, file string) string
		}{
			{"is absolute", func(dir, file string) string { return "/" + dir + "/" + file }},
			{"starts with a dot segment", func(dir, file string) string { return "./" + dir + "/" + file }},
			{"has an empty segment", func(dir, file string) string { return dir + "//" + file }},
			{"has a dot segment", func(dir, file string) string { return dir + "/./" + file }},
			{"has a dot-dot segment", func(dir, file string) string { return dir + "/../" + dir + "/" + file }},
			{"ends with a dot segment", func(dir, file string) string { return dir + "/." }},
			{"ends with a slash", func(dir, file string) string { return dir + "/" }},
			{"is dot-dot, the parent of the root", func(dir, file string) string { return ".." }},
			{"is empty", func(dir, file string) string { return "" }},
		} {
			s.And("name "+invalid.desc+", which fs.ValidPath doesn't allow", func(s *testcase.Spec) {
				name.Let(s, func(t *testcase.T) string {
					return invalid.name(dirName.Get(t), fileName.Get(t))
				})

				s.Then("it is rejected with fs.ErrInvalid or fs.ErrNotExist", func(t *testcase.T) {
					_, err := act(t)
					c.assertFSOpenError(t, err, name.Get(t), fs.ErrInvalid, fs.ErrNotExist)
				})
			})
		}
	})
}

func (c specFileSystem) specFS_ReadFile(s *testcase.Spec) {
	var (
		content = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
		// name is at the root, so it is the same for fs.FS and for the filesystem.FileSystem methods.
		name = testcase.Let(s, c.randomName)
	)
	act := func(t *testcase.T) ([]byte, error) {
		return fs.ReadFile(c.fsys(), name.Get(t))
	}
	s.Before(func(t *testcase.T) {
		c.writeNewFile(t, name.Get(t), c.perm().Get(t), []byte(content.Get(t)))
	})

	s.Then("it returns what was written into the file through FileSystem.OpenFile", func(t *testcase.T) {
		t.Eventually(func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, content.Get(t), string(data))
		})
	})

	s.When("more is appended to the file through FileSystem.OpenFile", func(s *testcase.Spec) {
		more := testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
		s.Before(func(t *testcase.T) {
			file, err := c.FileSystem.OpenFile(name.Get(t), os.O_WRONLY|os.O_APPEND, 0)
			assert.Must(t).NoError(err)
			c.writeToFile(t, file, []byte(more.Get(t)))
			assert.Must(t).NoError(file.Close())
		})

		s.Then("it returns the whole content", func(t *testcase.T) {
			t.Eventually(func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, content.Get(t)+more.Get(t), string(data))
			})
		})
	})

	s.When("the file is removed through FileSystem.Remove", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.remove(name.Get(t)))
		})

		s.Then("it yields fs.ErrNotExist", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})
}

func (c specFileSystem) specFS_ReadDir(s *testcase.Spec) {
	if !reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem) {
		return
	}
	var (
		// dirName is at the root, so it is the same for fs.FS and for the filesystem.FileSystem methods.
		dirName   = testcase.Let(s, c.randomName)
		fileNames = testcase.Let(s, func(t *testcase.T) []string {
			var names []string
			for i := range t.Random.IntBetween(2, 5) {
				names = append(names, fmt.Sprintf("%d-%s", i, c.randomName(t)))
			}
			return names
		})
	)
	act := func(t *testcase.T) ([]fs.DirEntry, error) {
		return fs.ReadDir(c.fsys(), dirName.Get(t))
	}
	s.Before(func(t *testcase.T) {
		c.touchDir(t, dirName.Get(t), filemode.UserRWX)
		for _, name := range fileNames.Get(t) {
			c.touchFile(t, filepath.Join(dirName.Get(t), name), c.perm().Get(t))
		}
	})

	s.Then("it lists the files made through FileSystem.OpenFile, sorted by name", func(t *testcase.T) {
		entries, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, slices.Sorted(slices.Values(fileNames.Get(t))), c.entryNames(entries))
	})

	if reflectkit.TypeImplements[any, filesystem.FileInfoReader](c.FileSystem) {
		s.Then("the entries agree with FileSystem.Stat", func(t *testcase.T) {
			entries, err := act(t)
			assert.NoError(t, err)
			assert.NotEmpty(t, entries)

			for _, entry := range entries {
				exp, err := c.stat(filepath.Join(dirName.Get(t), entry.Name()))
				assert.Must(t).NoError(err)
				assert.Equal(t, exp.IsDir(), entry.IsDir())
				assert.Equal(t, exp.Mode().Type(), entry.Type())

				got, err := entry.Info()
				assert.Must(t).NoError(err)
				assert.Equal(t, exp.Name(), got.Name())
				assert.Equal(t, exp.Size(), got.Size())
				assert.Equal(t, exp.Mode().String(), got.Mode().String())
			}
		})
	}

	s.When("the directory has a directory made with FileSystem.Mkdir", func(s *testcase.Spec) {
		subDirName := testcase.Let(s, c.randomName)
		s.Before(func(t *testcase.T) {
			c.touchDir(t, filepath.Join(dirName.Get(t), subDirName.Get(t)), filemode.UserRWX)
		})

		s.Then("it is listed as a directory", func(t *testcase.T) {
			entries, err := act(t)
			assert.NoError(t, err)

			entry, ok := c.findEntry(entries, subDirName.Get(t))
			assert.Must(t).True(ok, assert.Message("directory entry not found: "+subDirName.Get(t)))
			assert.True(t, entry.IsDir())
			assert.True(t, entry.Type().IsDir())
		})
	})

	s.When("a file is removed through FileSystem.Remove", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(c.remove(filepath.Join(dirName.Get(t), fileNames.Get(t)[0])))
		})

		s.Then("it is no longer listed", func(t *testcase.T) {
			entries, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, slices.Sorted(slices.Values(fileNames.Get(t)[1:])), c.entryNames(entries))
		})
	})

	s.When("the directory is removed through FileSystem.Remove", func(s *testcase.Spec) {
		if !reflectkit.TypeImplements[any, filesystem.Remover](c.FileSystem) {
			return
		}
		s.Before(func(t *testcase.T) {
			for _, name := range fileNames.Get(t) {
				assert.Must(t).NoError(c.remove(filepath.Join(dirName.Get(t), name)))
			}
			assert.Must(t).NoError(c.remove(dirName.Get(t)))
		})

		s.Then("it yields fs.ErrNotExist", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, fs.ErrNotExist)
		})
	})
}

// specFS_TestFS runs the conformance checks of testing/fstest on files made through the filesystem roles.
// They verify that every optional io/fs interface the subject implements agrees with Open,
// and that names which fs.ValidPath doesn't allow are rejected.
//
// fstest.TestFS walks the whole subject, whose contents must not change concurrently.
// Without DirectoryMaker, the fixture is a file at the root rather than a nested tree.
func (c specFileSystem) specFS_TestFS(s *testcase.Spec) {
	hasDirectories := reflectkit.TypeImplements[any, filesystem.DirectoryMaker](c.FileSystem)
	var (
		dirName     = testcase.Let(s, c.randomName)
		fileName    = testcase.Let(s, c.randomName)
		subDirName  = testcase.Let(s, c.randomName)
		subFileName = testcase.Let(s, c.randomName)
	)
	act := func(t *testcase.T) error {
		if !hasDirectories {
			return fstest.TestFS(c.fsys(), fileName.Get(t))
		}
		return fstest.TestFS(c.fsys(),
			dirName.Get(t),
			path.Join(dirName.Get(t), fileName.Get(t)),
			path.Join(dirName.Get(t), subDirName.Get(t)),
			path.Join(dirName.Get(t), subDirName.Get(t), subFileName.Get(t)),
		)
	}
	s.Before(func(t *testcase.T) {
		if !hasDirectories {
			c.writeNewFile(t, fileName.Get(t), c.perm().Get(t), []byte(t.Random.String()))
			return
		}
		dir := dirName.Get(t)
		c.touchDir(t, dir, filemode.UserRWX)
		c.writeNewFile(t, filepath.Join(dir, fileName.Get(t)), c.perm().Get(t), []byte(t.Random.String()))
		subDir := filepath.Join(dir, subDirName.Get(t))
		c.touchDir(t, subDir, filemode.UserRWX)
		c.writeNewFile(t, filepath.Join(subDir, subFileName.Get(t)), c.perm().Get(t), []byte(t.Random.String()))
	})

	s.Then("the files made through the FileSystem pass the checks of every io/fs interface it implements", func(t *testcase.T) {
		assert.NoError(t, act(t))
	})
}

func (c specFileSystem) specFile_Seek(s *testcase.Spec) {
	var (
		data = testcase.Let(s, func(t *testcase.T) []byte {
			return []byte(t.Random.String())
		})
		file = testcase.Let(s, func(t *testcase.T) filesystem.File {
			c.saveFile(t, c.name().Get(t), data.Get(t))
			file, err := c.FileSystem.OpenFile(c.name().Get(t), os.O_RDWR, 0)
			assert.Must(t).NoError(err)
			return file
		})
		offset = testcase.Let[int64](s, func(t *testcase.T) int64 {
			return int64(t.Random.IntN(len(data.Get(t))))
		})
		whence = testcase.Let[int](s, nil)
	)
	subject := func(t *testcase.T) (int64, error) {
		return file.Get(t).Seek(offset.Get(t), whence.Get(t))
	}

	s.When("whence from beginning", func(s *testcase.Spec) {
		whence.LetValue(s, io.SeekStart)

		s.Then("it will seek from the start", func(t *testcase.T) {
			actualAbs, err := subject(t)
			assert.Must(t).NoError(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			assert.Must(t).NoError(err)

			expectedContent, err := io.ReadAll(dataReader)
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(string(expectedContent), string(actualContent))
		})
	})

	s.When("whence from the end", func(s *testcase.Spec) {
		whence.LetValue(s, io.SeekEnd)

		s.Then("it will seek starting from the end", func(t *testcase.T) {
			actualAbs, err := subject(t)
			assert.Must(t).NoError(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			assert.Must(t).NoError(err)

			expectedContent, err := io.ReadAll(dataReader)
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(string(expectedContent), string(actualContent))
		})
	})

	s.When("whence from the current position", func(s *testcase.Spec) {
		whence.LetValue(s, io.SeekCurrent)

		s.Then("it will seek starting from the start by default", func(t *testcase.T) {
			actualAbs, err := subject(t)
			assert.Must(t).NoError(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			assert.Must(t).NoError(err)

			expectedContent, err := io.ReadAll(dataReader)
			assert.Must(t).NoError(err)

			assert.Must(t).Equal(string(expectedContent), string(actualContent))
		})

		s.And("if we make some reading on the file", func(s *testcase.Spec) {
			readLen := testcase.Let(s, func(t *testcase.T) int {
				halfDataLength := len(data.Get(t)) / 2
				if halfDataLength == 0 {
					return 0
				}
				return t.Random.IntN(halfDataLength)
			})
			someInitialReading := func(t *testcase.T, r io.Reader) {
				_, _ = r.Read(make([]byte, readLen.Get(t)))
			}

			s.Before(func(t *testcase.T) {
				someInitialReading(t, file.Get(t))
			})

			s.Then("it starts seeking relative to the previous reading", func(t *testcase.T) {
				actualAbs, err := subject(t)
				assert.Must(t).NoError(err)

				dataReader := bytes.NewReader(data.Get(t))
				someInitialReading(t, dataReader)

				expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(expectedAbs, actualAbs)

				actualContent, err := io.ReadAll(file.Get(t))
				assert.Must(t).NoError(err)

				t.Log(string(actualContent))

				expectedContent, err := io.ReadAll(dataReader)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(string(expectedContent), string(actualContent))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (c specFileSystem) touchDir(t *testcase.T, name string, perm fs.FileMode) {
	t.Helper()
	assert.Must(t).NoError(c.mkdir(name, perm|fs.ModeDir))
	t.Defer(c.tryRemove, name)
}

func (c specFileSystem) touchFile(t *testcase.T, name string, perm fs.FileMode) {
	t.Helper()
	file, err := c.FileSystem.OpenFile(name, os.O_RDONLY|os.O_CREATE|os.O_EXCL, perm)
	assert.Must(t).NoError(err)
	assert.Must(t).NoError(file.Close())
	t.Defer(c.tryRemove, name)
}

func (c specFileSystem) saveFile(t *testcase.T, name string, data []byte) {
	file, err := c.FileSystem.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filemode.UserRWX)
	assert.Must(t).NoError(err)
	defer func() { assert.Should(t).NoError(file.Close()) }()
	t.Defer(c.tryRemove, name)
	c.writeToFile(t, file, data)
}

func (c specFileSystem) writeNewFile(t *testcase.T, name string, perm fs.FileMode, data []byte) {
	t.Helper()
	file, err := c.FileSystem.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	assert.Must(t).NoError(err)
	t.Defer(c.tryRemove, name)
	c.writeToFile(t, file, data)
	assert.Must(t).NoError(file.Close())
}

// randomName makes a name which is unlikely to collide with another one,
// even on a case-insensitive file system.
func (c specFileSystem) randomName(t *testcase.T) string {
	return t.Random.StringNWithCharset(8, "abcdefghijklmnopqrstuvwxyz")
}

func (c specFileSystem) overwrite(dst []byte, src []byte) []byte {
	l := len(dst)
	if l < len(src) {
		l = len(src)
	}
	out := make([]byte, l)
	copy(out, dst)
	copy(out, src)
	return out
}

func (c specFileSystem) writeToFile(t *testcase.T, file filesystem.File, data []byte) {
	t.Helper()
	n, err := file.Write(data)
	assert.Must(t).NoError(err)
	assert.Must(t).Equal(len(data), n)
}

func (c specFileSystem) isPathError(t *testcase.T, err error) *fs.PathError {
	t.Helper()
	var pathError *fs.PathError
	assert.True(t, errors.As(err, &pathError), "*fs.PathError was expected")
	return pathError
}

// assertLinkError asserts that err is the *os.LinkError of a failed rename, as Renamer.Rename promises.
func (c specFileSystem) assertLinkError(t *testcase.T, err error, oldName, newName string) {
	t.Helper()
	var linkError *os.LinkError
	assert.Must(t).True(errors.As(err, &linkError), "*os.LinkError was expected")
	assert.Equal(t, "rename", linkError.Op)
	assert.Contains(t, linkError.Old, oldName)
	assert.Contains(t, linkError.New, newName)
	assert.NotNil(t, linkError.Err)
}

func (c specFileSystem) assertErrorIsNotExist(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	assert.Contains(t, pathError.Path, name)
	assert.Must(t).Equal("open", pathError.Op)
	assert.True(t, os.IsNotExist(pathError))
}

func (c specFileSystem) assertReadError(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	assert.Contains(t, pathError.Path, name)
	assert.Must(t).Equal("read", pathError.Op)
	assert.Must(t).NotNil(pathError.Err)
}

func (c specFileSystem) assertWriteError(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	assert.Contains(t, pathError.Path, name)
	assert.Must(t).Equal("write", pathError.Op)
	assert.Must(t).NotNil(pathError.Err)
}

func (c specFileSystem) assertReaderEquals(tb testing.TB, expected []byte, actual io.ReadCloser) {
	tb.Helper()
	defer actual.Close()
	bytes, err := io.ReadAll(actual)
	assert.NoError(tb, err)
	assert.Must(tb).Equal(string(expected), string(bytes))
}

// assertFSOpenError asserts that err is what the fs.FS.Open documentation asks for when Open fails:
// a *fs.PathError with "open" as its Op, the name given to Open as its Path, and one of the targets as its Err.
func (c specFileSystem) assertFSOpenError(t *testcase.T, err error, name string, targets ...error) {
	t.Helper()
	assert.Error(t, err, "fs.FS.Open was expected to fail")
	pathError := c.isPathError(t, err)
	assert.Equal(t, "open", pathError.Op)
	assert.Equal(t, name, pathError.Path, "the Path of the error is expected to be the name given to fs.FS.Open")
	assert.AnyOf(t, func(a *assert.A) {
		for _, target := range targets {
			a.Case(func(tb testing.TB) { assert.ErrorIs(tb, err, target) })
		}
	})
}

// assertOpenedContent asserts that the file which open opens through fs.FS has the expected content.
func (c specFileSystem) assertOpenedContent(t *testcase.T, open func(t *testcase.T) (fs.File, error), expected []byte) {
	t.Helper()
	t.Eventually(func(t *testcase.T) {
		file, err := open(t)
		assert.NoError(t, err)
		c.assertReaderEquals(t, expected, file)
	})
}

// readDirNames lists the names in an opened directory, which must be an fs.ReadDirFile.
func (c specFileSystem) readDirNames(t *testcase.T, file fs.File) []string {
	t.Helper()
	dir, ok := file.(fs.ReadDirFile)
	assert.Must(t).True(ok, "fs.ReadDirFile was expected for a directory")
	entries, err := dir.ReadDir(-1)
	assert.Must(t).NoError(err)
	return c.entryNames(entries)
}

func (c specFileSystem) entryNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func (c specFileSystem) findEntry(entries []fs.DirEntry, name string) (fs.DirEntry, bool) {
	for _, entry := range entries {
		if entry.Name() == name {
			return entry, true
		}
	}
	return nil, false
}

func (c specFileSystem) assertFileContent(t *testcase.T, name string, expected []byte) {
	t.Helper()
	t.Eventually(func(it *testcase.T) {
		file, err := c.FileSystem.OpenFile(name, os.O_RDONLY, 0)
		assert.NoError(it, err)
		defer file.Close()
		info, err := file.Stat()
		assert.NoError(it, err)
		assert.Should(it).Equal(int64(len(expected)), info.Size())
		c.assertReaderEquals(it, expected, file)
	})
}

type Option interface {
	option.Option[Config]
}

type Config struct{}
