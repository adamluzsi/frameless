package localfs_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/port/filesystem/filemode"
	filesystemcontracts "go.llib.dev/frameless/port/filesystem/filesystemcontract"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/testcase"
)

func ExampleFileSystem() {
	fsys := localfs.FileSystem{}

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

func TestFileSystem(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("implements fs.FS", func(t *testcase.T) {
		fsys := localfs.FileSystem{RootPath: t.TempDir()}

		dir := filepath.Join("foo")
		name := filepath.Join(dir, t.Random.UUID())
		assert.NoError(t, fsys.Mkdir(dir, filemode.UserRWX))
		exp := []byte(t.Random.String())

		{ // mk file
			infile, err := fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, filemode.UserRWX)
			assert.NoError(t, err)
			assert.NotNil(t, infile)
			t.Cleanup(func() { _ = fsys.Remove(name) })
			_, err = iokit.WriteAll(infile, exp)
			assert.NoError(t, err)
			assert.NoError(t, infile.Close())
		}

		dirFS := os.DirFS(fsys.RootPath)
		file1, err := fsys.Open(name)
		assert.NoError(t, err)
		assert.NotNil(t, file1)
		file2, err := dirFS.Open(name)
		assert.NoError(t, err)
		assert.NotNil(t, file2)

		got, err := io.ReadAll(file1)
		assert.NoError(t, file1.Close())
		assert.NoError(t, err)
		assert.Equal(t, exp, got)

		got, err = io.ReadAll(file2)
		assert.NoError(t, file2.Close())
		assert.NoError(t, err)
		assert.Equal(t, exp, got)

		file1, err = fsys.Open("unknown-file-name")
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Nil(t, file1)
		pathErr1, ok := errorkit.As[*fs.PathError](err)
		assert.True(t, ok)
		assert.NotEmpty(t, pathErr1.Err)
		assert.NotEmpty(t, pathErr1.Op)
		assert.NotEmpty(t, pathErr1.Path)

		file2, err = dirFS.Open("unknown-file-name")
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Nil(t, file2)
		pathErr2, ok := errorkit.As[*fs.PathError](err)
		assert.True(t, ok)
		assert.NotEmpty(t, pathErr2.Err)
		assert.NotEmpty(t, pathErr2.Op)
		assert.NotEmpty(t, pathErr2.Path)

		assert.Equal(t, pathErr1.Op, pathErr2.Op)
		assert.ErrorIs(t, pathErr1.Err, pathErr2.Err)
	})
}

func TestFileSystem_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(localfs.FileSystem{RootPath: t.TempDir()}).Test(t)
}

func TestFileSystem_smoke(t *testing.T) {
	mfs := &localfs.FileSystem{}

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

func TestFileSystem_rootPath(t *testing.T) {
	s := testcase.NewSpec(t)

	getSysTmpDir := func(t *testcase.T) string {
		t.Helper()
		tmpDir := os.TempDir()
		stat, err := os.Stat(tmpDir)
		assert.Must(t).NoError(err)
		assert.True(t, stat.IsDir())
		return tmpDir
	}

	makeName := func(t *testcase.T) string {
		t.Helper()
		return fmt.Sprintf("%d-%s",
			t.Random.Int(),
			t.Random.StringNWithCharset(5, "qwerty"))
	}

	tmpFile := func(t *testcase.T, dir string) string {
		t.Helper()
		return filepath.Join(dir, makeName(t))
	}

	touchFile := func(t *testcase.T, fs filesystem.FileSystem, name string) error {
		t.Helper()
		file, err := fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, filemode.UserRWX)
		if err == nil {
			assert.NoError(t, file.Close())
			t.Cleanup(func() { _ = fs.Remove(name) })
		}
		return err
	}

	s.Test("without .RootPath set, fs is not jailed", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := localfs.FileSystem{}

		name := tmpFile(t, tmpDir)
		assert.Must(t).NoError(touchFile(t, fs, name))
		_, err := os.Stat(name)
		assert.Must(t).NoError(err)

		name = tmpFile(t, getSysTmpDir(t))
		assert.Must(t).NoError(touchFile(t, fs, name))
		_, err = os.Stat(name)
		assert.Must(t).NoError(err)
	})

	s.Test("with .RootPath set, fs is jailed and path used as relative path", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := localfs.FileSystem{RootPath: tmpDir}

		name := makeName(t)
		assert.Must(t).NoError(touchFile(t, fs, name))
		_, err := os.Stat(filepath.Join(tmpDir, name))
		assert.Must(t).NoError(err)
		_, err = fs.Stat(name)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(fs.Mkdir(makeName(t), filemode.UserRWX))
		assert.Must(t).NoError(fs.Remove(name))

		path := filepath.Join("..", name)
		assert.Must(t).ErrorIs(syscall.EACCES, touchFile(t, fs, path))
		assert.Must(t).ErrorIs(syscall.EACCES, fs.Mkdir(path, 0700))
		assert.Must(t).ErrorIs(syscall.EACCES, func() error { _, err := fs.Stat(path); return err }())
		assert.Must(t).ErrorIs(syscall.EACCES, fs.Remove(path))
	})
}
