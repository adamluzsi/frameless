package localfs_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/testcase/assert"

	filesystemcontracts "go.llib.dev/frameless/port/filesystem/filesystemcontract"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/testcase"
)

func ExampleFileSystem() {
	fsys := localfs.FileSystem{}

	file, err := fsys.OpenFile("test", os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystem.ModeUserRWX)
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

	fsys.Mkdir("a", filesystem.ModeUserRWX)

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

func TestLocal_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(localfs.FileSystem{RootPath: t.TempDir()}).Test(t)
}

func TestFileSystem_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	mfs := &localfs.FileSystem{}

	name := "test"
	file, err := mfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_APPEND, filesystem.ModeUserRWX)
	it.Must.NoError(err)
	defer func() { it.Should.NoError(mfs.Remove(name)) }()

	_, err = file.Write([]byte("/foo"))
	it.Must.NoError(err)
	_, err = file.Write([]byte("/bar"))
	it.Must.NoError(err)
	file.Seek(0, io.SeekStart)
	_, err = file.Write([]byte("/baz"))
	it.Must.NoError(err)

	it.Must.NoError(file.Close())

	file, err = mfs.OpenFile(name, os.O_RDONLY, 0)
	it.Must.NoError(err)

	bs, err := io.ReadAll(file)
	it.Must.NoError(err)
	it.Must.Equal("/foo/bar/baz", string(bs))
}

func TestLocal_rootPath(t *testing.T) {
	s := testcase.NewSpec(t)

	getSysTmpDir := func(t *testcase.T) string {
		t.Helper()
		tmpDir := os.TempDir()
		stat, err := os.Stat(tmpDir)
		t.Must.NoError(err)
		t.Must.True(stat.IsDir())
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
		file, err := fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystem.ModeUserRWX)
		if err == nil {
			t.Must.NoError(file.Close())
			t.Cleanup(func() { _ = fs.Remove(name) })
		}
		return err
	}

	s.Test("without .RootPath set, fs is not jailed", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := localfs.FileSystem{}

		name := tmpFile(t, tmpDir)
		t.Must.NoError(touchFile(t, fs, name))
		_, err := os.Stat(name)
		t.Must.NoError(err)

		name = tmpFile(t, getSysTmpDir(t))
		t.Must.NoError(touchFile(t, fs, name))
		_, err = os.Stat(name)
		t.Must.NoError(err)
	})

	s.Test("with .RootPath set, fs is jailed and path used as relative path", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := localfs.FileSystem{RootPath: tmpDir}

		name := makeName(t)
		t.Must.NoError(touchFile(t, fs, name))
		_, err := os.Stat(filepath.Join(tmpDir, name))
		t.Must.NoError(err)
		_, err = fs.Stat(name)
		t.Must.NoError(err)
		t.Must.NoError(fs.Mkdir(makeName(t), filesystem.ModeUserRWX))
		t.Must.NoError(fs.Remove(name))

		path := filepath.Join("..", name)
		t.Must.ErrorIs(syscall.EACCES, touchFile(t, fs, path))
		t.Must.ErrorIs(syscall.EACCES, fs.Mkdir(path, 0700))
		t.Must.ErrorIs(syscall.EACCES, func() error { _, err := fs.Stat(path); return err }())
		t.Must.ErrorIs(syscall.EACCES, fs.Remove(path))
	})
}
