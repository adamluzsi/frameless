package filesystemcontracts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/filesystem"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type FileSystem func(testing.TB) filesystem.FileSystem

func (c FileSystem) String() string {
	return reflects.SymbolicName(c)
}

func (c FileSystem) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c FileSystem) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c FileSystem) Spec(s *testcase.Spec) {
	s.Describe("#FileSystem", func(s *testcase.Spec) {
		s.Describe(".OpenFile", c.specOpenFile)
		s.Describe(".MkDir", c.specMkdir)
		s.Describe(".Remove", c.specRemove)
		s.Describe(".Stat", c.specStat)
	})
	s.Describe("#File", func(s *testcase.Spec) {
		s.Describe(".ReadDir", c.specFile_ReadDir)
		s.Describe(".Seek", c.specFile_Seek)
	})
}

func (c FileSystem) fileSystem() testcase.Var[filesystem.FileSystem] {
	return testcase.Var[filesystem.FileSystem]{
		ID:   "frameless.Local",
		Init: func(t *testcase.T) filesystem.FileSystem { return c(t) },
	}
}

func (c FileSystem) perm() testcase.Var[fs.FileMode] {
	return testcase.Var[fs.FileMode]{
		ID: "fs perm",
		Init: func(t *testcase.T) fs.FileMode {
			var mode fs.FileMode
			if t.Random.Bool() {
				mode |= filesystem.ModeUserRWX
			} else {
				mode |= filesystem.ModeUserRW
			}
			if t.Random.Bool() {
				mode |= filesystem.ModeOtherR
			}
			return mode
		},
	}
}

func (c FileSystem) name() testcase.Var[string] {
	return testcase.Var[string]{
		ID: "file name",
		Init: func(t *testcase.T) string {
			return t.Random.StringNWithCharset(5, "abcdefg")
		},
	}
}

func (c FileSystem) specOpenFile(s *testcase.Spec) {
	var flag = testcase.Var[int]{ID: "file open flag"}
	subject := func(t *testcase.T) (filesystem.File, error) {
		file, err := c.fileSystem().Get(t).OpenFile(c.name().Get(t), flag.Get(t), c.perm().Get(t))
		if err == nil {
			t.Defer(file.Close)
		}
		return file, err
	}

	s.When("name points to an unexistent file", func(s *testcase.Spec) {
		s.And("open flag is read only", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDONLY)

			s.Before(func(t *testcase.T) {
				_ = c.fileSystem().Get(t).Remove(c.name().Get(t))
			})

			c.andForAbsentFileOpening(s, subject, flag)

			s.Then("it yields error because the file is not existing", func(t *testcase.T) {
				_, err := subject(t)

				c.assertErrorIsNotExist(t, err, c.name().Get(t))
			})

			s.And("flag also has O_CREATE", func(s *testcase.Spec) {
				flag.LetValue(s, os.O_RDONLY|os.O_CREATE)

				s.Then("it creates an empty file", func(t *testcase.T) {
					file, err := subject(t)
					t.Must.Nil(err)

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
			t.Must.Nil(err)

			info, err := file.Stat()
			t.Must.Nil(err)
			t.Must.True(info.IsDir())
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
				t.Must.Nil(err)
				c.assertReaderEquals(t, []byte(content.Get(t)), file)
			})

			s.Then("returned file does not allow writing", func(t *testcase.T) {
				file, err := subject(t)
				t.Must.Nil(err)
				_, err = file.Write([]byte(t.Random.String()))
				c.assertWriteError(t, err, c.name().Get(t))
			})
		})

		s.And("we open with write only", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_WRONLY)

			c.andForTheExistingFileOpening(s, subject, c.name(), flag, content)

			s.Then("returned file can not be consumed for its content", func(t *testcase.T) {
				file, err := subject(t)
				t.Must.Nil(err)

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
				t.Must.Nil(err)

				c.assertReaderEquals(t, []byte(content.Get(t)), file)
			})

			c.thenCanBeWritten(s, subject, flag, content)
		})
	})
}

func (c FileSystem) andForTheExistingFileOpening(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error),
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
				t.Must.NotNil(err)
				perr := c.isPathError(t, err)
				t.Must.Equal("open", perr.Op)
				t.Must.Contain(perr.Path, name.Get(t))
				t.Must.True(os.IsExist(err))
			})
		})

		s.Then("file opening succeeds with existing content since file already exists", func(t *testcase.T) {
			_, err := subject(t)
			t.Must.Nil(err)
			c.assertFileContent(t, name.Get(t), []byte(content.Get(t)))
		})
	})
}

func (c FileSystem) andForAbsentFileOpening(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error), flag testcase.Var[int]) {
	thenFileCanBeCreated := func(s *testcase.Spec) {
		s.Then("file opening succeeds with file is just created", func(t *testcase.T) {
			_, err := subject(t)
			t.Must.Nil(err)
			c.assertFileContent(t, c.name().Get(t), []byte{})
		})

		s.Then("permission matches the permission of the newly created file", func(t *testcase.T) {
			file, err := subject(t)
			t.Must.Nil(err)
			info, err := file.Stat()
			t.Must.Nil(err)

			t.Must.Equal(c.perm().Get(t).String(), info.Mode().String())
		})
	}

	s.And("O_CREATE flag is also given", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			flag.Set(t, flag.Get(t)|os.O_CREATE)
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

func (c FileSystem) thenCanBeWritten(s *testcase.Spec, subject func(t *testcase.T) (filesystem.File, error),
	flag testcase.Var[int],
	initialContent testcase.Var[string],
) {
	s.Then("returned file allows writing over the existing initialContent", func(t *testcase.T) {
		file, err := subject(t)
		t.Must.Nil(err)

		data := t.Random.String()
		c.writeToFile(t, file, []byte(data))
		t.Must.Nil(file.Close())

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
			t.Must.Nil(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))
			t.Must.Nil(file.Close())

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
			t.Must.Nil(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))

			if !(flag.Get(t)&os.O_WRONLY != 0) {
				t.Log("and reading after writing starts from the last position")
				_, err := file.Read([]byte{0})
				t.Must.ErrorIs(io.EOF, err)
			}

			t.Must.Nil(file.Close())
			expectedContent := append([]byte(initialContent.Get(t)), []byte(data)...)
			c.assertFileContent(t, c.name().Get(t), expectedContent)
		})

		s.Then(".Write always appends to the end of the file, regardless if seek were used", func(t *testcase.T) {
			file, err := subject(t)
			t.Must.Nil(err)

			_, err = file.Seek(0, io.SeekStart)
			t.Must.Nil(err)

			data := t.Random.String()
			c.writeToFile(t, file, []byte(data))
			t.Must.Nil(file.Close())

			expectedContent := append([]byte(initialContent.Get(t)), []byte(data)...)
			c.assertFileContent(t, c.name().Get(t), expectedContent)
		})
	})
}

func (c FileSystem) specMkdir(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		err := c.fileSystem().Get(t).Mkdir(c.name().Get(t), c.perm().Get(t))
		if err == nil {
			t.Defer(c.fileSystem().Get(t).Remove, c.name().Get(t))
		}
		return err
	}

	s.When("when name points to a non-existing file path", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, err := c.fileSystem().Get(t).Stat(c.name().Get(t))
			t.Must.True(os.IsNotExist(err))
		})

		s.Then("directory can be made without an issue", func(t *testcase.T) {
			t.Must.Nil(subject(t))
		})

		s.And("on successful directory making", func(s *testcase.Spec) {
			cTime := testcase.Let[time.Time](s, nil)
			s.Before(func(t *testcase.T) {
				cTime.Set(t, time.Now().UTC())
				t.Must.Nil(subject(t))
			})

			assertFileInfo := func(t *testcase.T, info fs.FileInfo) {
				t.Must.True(info.IsDir())
				t.Must.Equal((c.perm().Get(t) | fs.ModeDir).String(), info.Mode().String())
				t.Must.True(cTime.Get(t).Before(info.ModTime()) || cTime.Get(t).Equal(info.ModTime()))
			}

			s.Then("FileSystem.Stat returns the directory details", func(t *testcase.T) {
				info, err := c.fileSystem().Get(t).Stat(c.name().Get(t))
				t.Must.Nil(err)
				assertFileInfo(t, info)
			})

			s.Then("FileSystem.OpenFile returns the directory details", func(t *testcase.T) {
				file, err := c.fileSystem().Get(t).OpenFile(c.name().Get(t), os.O_RDONLY, 0)
				t.Must.Nil(err)
				t.Defer(file.Close)

				info, err := file.Stat()
				t.Must.Nil(err)
				assertFileInfo(t, info)

				entries, err := file.ReadDir(-1)
				t.Must.Nil(err)
				t.Must.Empty(entries)
			})
		})
	})

	s.When("when name points to an existing file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), filesystem.ModeUserRWX)
		})

		s.Then("directory making fails", func(t *testcase.T) {
			err := subject(t)
			t.Must.NotNil(err)
			t.Must.True(os.IsExist(err))

			perr := c.isPathError(t, err)
			t.Must.Equal("mkdir", perr.Op)
			t.Must.Contain(perr.Path, c.name().Get(t))
		})
	})

	s.When("when name points to an existing directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.Nil(c.fileSystem().Get(t).Mkdir(c.name().Get(t), 0700))
			t.Defer(c.fileSystem().Get(t).Remove, c.name().Get(t))
		})

		s.Then("directory making fails", func(t *testcase.T) {
			err := subject(t)
			t.Must.NotNil(err)
			t.Must.True(os.IsExist(err))

			perr := c.isPathError(t, err)
			t.Must.Equal("mkdir", perr.Op)
		})
	})
}

func (c FileSystem) specRemove(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		return c.fileSystem().Get(t).Remove(c.name().Get(t))
	}

	s.When("name points to nothing", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, err := c.fileSystem().Get(t).Stat(c.name().Get(t))
			t.Must.True(os.IsNotExist(err))
		})

		s.Then("it yields error", func(t *testcase.T) {
			err := subject(t)
			t.Must.NotNil(err)
			t.Must.True(os.IsNotExist(err))

			perr := c.isPathError(t, err)
			t.Must.Equal("remove", perr.Op)
			t.Must.Contain(perr.Path, c.name().Get(t))
		})
	})

	s.When("name points to a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), c.perm().Get(t))
		})

		s.Then("it will remove the file", func(t *testcase.T) {
			t.Must.Nil(subject(t))
			_, err := c.fileSystem().Get(t).Stat(c.name().Get(t))
			os.IsNotExist(err)
		})
	})

	s.When("name points to a directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.Nil(c.fileSystem().Get(t).Mkdir(c.name().Get(t), 0700))
			t.Defer(c.fileSystem().Get(t).Remove, c.name().Get(t))
		})

		s.Then("it will remove the directory", func(t *testcase.T) {
			t.Must.Nil(subject(t))
			_, err := c.fileSystem().Get(t).Stat(c.name().Get(t))
			os.IsNotExist(err)
		})

		s.And("the directory has a file", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				fPath := filepath.Join(c.name().Get(t), "file.tdd")
				c.touchFile(t, fPath, 0700)
			})

			s.Then("it will yield error because the directory is not empty", func(t *testcase.T) {
				err := subject(t)
				t.Must.NotNil(err)

				perr := c.isPathError(t, err)
				t.Must.Equal("remove", perr.Op)
				t.Must.Contain(perr.Path, c.name().Get(t))
				t.Must.Equal(syscall.ENOTEMPTY, perr.Err)
			})
		})
	})
}

func (c FileSystem) specStat(s *testcase.Spec) {
	subject := func(t *testcase.T) (fs.FileInfo, error) {
		return c.fileSystem().Get(t).Stat(c.name().Get(t))
	}

	s.When("name points to nothing", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_ = c.fileSystem().Get(t).Remove(c.name().Get(t))
		})

		s.Then("it yields error", func(t *testcase.T) {
			_, err := subject(t)
			t.Must.NotNil(err)
			t.Must.True(os.IsNotExist(err))

			perr := c.isPathError(t, err)
			t.Must.Equal("stat", perr.Op)
			t.Must.Contain(perr.Path, c.name().Get(t))
		})
	})

	s.When("name points to a file", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchFile(t, c.name().Get(t), c.perm().Get(t))
		})

		s.Then("it will return file stat", func(t *testcase.T) {
			info, err := subject(t)
			t.Must.Nil(err)
			t.Must.Contain(info.Name(), c.name().Get(t))
			t.Must.Equal(c.perm().Get(t).String(), info.Mode().String())
			t.Must.False(info.IsDir())
		})
	})

	s.When("name points to a directory", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.perm().Set(t, c.perm().Get(t)|fs.ModeDir)
			t.Must.Nil(c.fileSystem().Get(t).Mkdir(c.name().Get(t), c.perm().Get(t)))
			t.Defer(c.fileSystem().Get(t).Remove, c.name().Get(t))
		})

		s.Then("it will return directory stat", func(t *testcase.T) {
			info, err := subject(t)
			t.Must.Nil(err)
			t.Must.Contain(info.Name(), c.name().Get(t))
			t.Must.Equal(c.perm().Get(t).String(), info.Mode().String())
			t.Must.True(info.IsDir())
		})
	})

	s.When("name points to the current working directory", func(s *testcase.Spec) {
		c.name().LetValue(s, ".")

		s.Then("it will return directory stat", func(t *testcase.T) {
			info, err := subject(t)
			t.Must.Nil(err)
			t.Must.True(info.IsDir())
		})
	})
}

func (c FileSystem) specFile_ReadDir(s *testcase.Spec) {
	var (
		n    = testcase.Let[int](s, nil)
		file = testcase.Let(s, func(t *testcase.T) filesystem.File {
			file, err := c.fileSystem().Get(t).OpenFile(c.name().Get(t), os.O_RDONLY, 0)
			if err != nil {
				t.Log(err.Error())
			}
			t.Must.Nil(err)
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
			t.Must.NotNil(err)

			perr := c.isPathError(t, err)
			t.Must.Equal("fdopendir", perr.Op)
			t.Must.Contain(perr.Path, c.name().Get(t))
			t.Must.ErrorIs(syscall.ENOTDIR, perr.Err)
		})
	})

	s.When("name points to the working directory as .", func(s *testcase.Spec) {
		n.LetValue(s, -1)
		c.name().LetValue(s, ".")

		s.And("it contains files", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c.touchFile(t, filepath.Join(".", "a"), filesystem.ModeUserRWX)
				c.touchFile(t, filepath.Join(".", "b"), filesystem.ModeUserRWX)
				c.touchFile(t, filepath.Join(".", "c"), filesystem.ModeUserRWX)
			})

			s.Then("directory entries are returned", func(t *testcase.T) {
				entries, err := subject(t)
				t.Must.Nil(err)
				t.Must.NotEmpty(entries)
				t.Must.Equal(3, len(entries))

				names := []string{"a", "b", "c"}
				for _, entry := range entries {
					t.Must.Contain(names, entry.Name())
				}
			})
		})
	})

	s.When("a directory exists where the file name points", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			c.touchDir(t, c.name().Get(t), c.perm().Get(t)|filesystem.ModeUserRWX)
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
				t.Must.Nil(err)
				t.Must.Empty(dirEntries)
			})
		})

		s.And("n is less than zero which means give me back everything", func(s *testcase.Spec) {
			n.LetValue(s, -1)

			s.Then("it will return that the directory is empty", func(t *testcase.T) {
				entries, err := subject(t)
				t.Must.Nil(err)
				t.Must.Empty(entries)
			})

			s.And("directory has file(s)", func(s *testcase.Spec) {
				dirFileNames.Bind(s)
				const expectedEntryPerm = filesystem.ModeUserRWX

				cTime := testcase.Let[time.Time](s, nil)
				s.Before(func(t *testcase.T) {
					cTime.Set(t, time.Now().UTC())
					for _, bname := range dirFileNames.Get(t) {
						c.touchFile(t, filepath.Join(c.name().Get(t), bname), expectedEntryPerm)
					}
				})

				s.Then("it lists all the directory entry", func(t *testcase.T) {
					entries, err := subject(t)
					t.Must.Nil(err)
					t.Must.NotEmpty(entries)

					dirFileNames := dirFileNames.Get(t)
					t.Must.Equal(len(dirFileNames), len(entries))
					for _, ent := range entries {
						t.Must.Contain(dirFileNames, ent.Name())
						t.Must.False(ent.IsDir())

						info, err := ent.Info()
						t.Must.Nil(err)
						_ = info.Sys()
						t.Must.False(info.IsDir())
						t.Must.True(cTime.Get(t).Before(info.ModTime()) || cTime.Get(t).Equal(info.ModTime()))
						t.Must.Equal(info.Mode().Type(), ent.Type())
						t.Must.False(ent.Type()&fs.ModeDir != 0, "no ModeDir flag is expected")
					}
				})

				s.Then("it lists entries, but only on the first call", func(t *testcase.T) {
					entries, err := subject(t)
					t.Must.Nil(err)
					t.Must.NotEmpty(entries)

					entries, err = subject(t)
					t.Must.Nil(err)
					t.Must.Empty(entries)
				})
			})
		})

		s.And("n is a bigger than zero which means give me back the next N amount of entry", func(s *testcase.Spec) {
			n.LetValue(s, 1)

			s.Then("it will return that the directory is empty by stating io.EOF", func(t *testcase.T) {
				_, err := subject(t)
				t.Must.ErrorIs(io.EOF, err)
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
					t.Must.Nil(err)
					t.Must.NotEmpty(entries)
					t.Must.Equal(n.Get(t), len(entries))

				consuming:
					for {
						entrs, err := subject(t)
						if err == io.EOF {
							break consuming
						}
						t.Must.Nil(err)
						entries = append(entries, entrs...)
					}

					dirFileNames := dirFileNames.Get(t)
					t.Must.Equal(len(dirFileNames), len(entries))

					for _, ent := range entries {
						t.Must.Contain(dirFileNames, ent.Name())
					}
				})
			})
		})
	})
}

func (c FileSystem) specFile_Seek(s *testcase.Spec) {
	var (
		data = testcase.Let(s, func(t *testcase.T) []byte {
			return []byte(t.Random.String())
		})
		file = testcase.Let(s, func(t *testcase.T) filesystem.File {
			c.saveFile(t, c.name().Get(t), data.Get(t))
			file, err := c.fileSystem().Get(t).OpenFile(c.name().Get(t), os.O_RDWR, 0)
			t.Must.Nil(err)
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
			t.Must.Nil(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			t.Must.Nil(err)

			t.Must.Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			t.Must.Nil(err)

			expectedContent, err := io.ReadAll(dataReader)
			t.Must.Nil(err)

			t.Must.Equal(string(expectedContent), string(actualContent))
		})
	})

	s.When("whence from the end", func(s *testcase.Spec) {
		whence.LetValue(s, io.SeekEnd)

		s.Then("it will seek starting from the end", func(t *testcase.T) {
			actualAbs, err := subject(t)
			t.Must.Nil(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			t.Must.Nil(err)

			t.Must.Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			t.Must.Nil(err)

			expectedContent, err := io.ReadAll(dataReader)
			t.Must.Nil(err)

			t.Must.Equal(string(expectedContent), string(actualContent))
		})
	})

	s.When("whence from the current position", func(s *testcase.Spec) {
		whence.LetValue(s, io.SeekCurrent)

		s.Then("it will seek starting from the start by default", func(t *testcase.T) {
			actualAbs, err := subject(t)
			t.Must.Nil(err)

			dataReader := bytes.NewReader(data.Get(t))
			expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
			t.Must.Nil(err)

			t.Must.Equal(expectedAbs, actualAbs)

			actualContent, err := io.ReadAll(file.Get(t))
			t.Must.Nil(err)

			expectedContent, err := io.ReadAll(dataReader)
			t.Must.Nil(err)

			t.Must.Equal(string(expectedContent), string(actualContent))
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
				t.Must.Nil(err)

				dataReader := bytes.NewReader(data.Get(t))
				someInitialReading(t, dataReader)

				expectedAbs, err := dataReader.Seek(offset.Get(t), whence.Get(t))
				t.Must.Nil(err)
				t.Must.Equal(expectedAbs, actualAbs)

				actualContent, err := io.ReadAll(file.Get(t))
				t.Must.Nil(err)

				t.Log(string(actualContent))

				expectedContent, err := io.ReadAll(dataReader)
				t.Must.Nil(err)
				t.Must.Equal(string(expectedContent), string(actualContent))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (c FileSystem) touchDir(t *testcase.T, name string, perm fs.FileMode) {
	t.Helper()
	t.Must.Nil(c.fileSystem().Get(t).Mkdir(name, perm|fs.ModeDir))
	t.Defer(c.fileSystem().Get(t).Remove, name)
}

func (c FileSystem) touchFile(t *testcase.T, name string, perm fs.FileMode) {
	t.Helper()
	file, err := c.fileSystem().Get(t).OpenFile(name, os.O_RDONLY|os.O_CREATE|os.O_EXCL, perm)
	t.Must.Nil(err)
	t.Must.Nil(file.Close())
	t.Defer(c.fileSystem().Get(t).Remove, name)
}

func (c FileSystem) saveFile(t *testcase.T, name string, data []byte) {
	file, err := c.fileSystem().Get(t).OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filesystem.ModeUserRWX)
	t.Must.Nil(err)
	defer func() { t.Should.Nil(file.Close()) }()
	t.Defer(c.fileSystem().Get(t).Remove, name)
	c.writeToFile(t, file, data)
}

func (c FileSystem) overwrite(dst []byte, src []byte) []byte {
	l := len(dst)
	if l < len(src) {
		l = len(src)
	}
	out := make([]byte, l)
	for i, b := range dst {
		out[i] = b
	}
	for i, b := range src {
		out[i] = b
	}
	return out
}

func (c FileSystem) writeToFile(t *testcase.T, file filesystem.File, data []byte) {
	t.Helper()
	n, err := file.Write(data)
	t.Must.Nil(err)
	t.Must.Equal(len(data), n)
}

func (c FileSystem) isPathError(t *testcase.T, err error) *fs.PathError {
	t.Helper()
	var pathError *fs.PathError
	t.Must.True(errors.As(err, &pathError), "*fs.PathError was expected")
	return pathError
}

func (c FileSystem) assertErrorIsNotExist(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	t.Must.Contain(pathError.Path, name)
	t.Must.Equal("open", pathError.Op)
	t.Must.True(os.IsNotExist(pathError))
}

func (c FileSystem) assertReadError(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	t.Must.Contain(pathError.Path, name)
	t.Must.Equal("read", pathError.Op)
	t.Must.NotNil(pathError.Err)
}

func (c FileSystem) assertWriteError(t *testcase.T, err error, name string) {
	t.Helper()
	pathError := c.isPathError(t, err)
	t.Must.Contain(pathError.Path, name)
	t.Must.Equal("write", pathError.Op)
	t.Must.NotNil(pathError.Err)
}

func (c FileSystem) assertReaderEquals(tb testing.TB, expected []byte, actual io.ReadCloser) {
	tb.Helper()
	defer actual.Close()
	bytes, err := io.ReadAll(actual)
	assert.Must(tb).Nil(err)
	assert.Must(tb).Equal(string(expected), string(bytes))
}

func (c FileSystem) assertFileContent(t *testcase.T, name string, expected []byte) {
	t.Helper()
	t.Eventually(func(it assert.It) {
		file, err := c.fileSystem().Get(t).OpenFile(name, os.O_RDONLY, 0)
		it.Must.Nil(err)
		defer file.Close()
		info, err := file.Stat()
		it.Must.Nil(err)
		it.Should.Equal(int64(len(expected)), info.Size())
		c.assertReaderEquals(it, expected, file)
	})
}
