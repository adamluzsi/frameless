package memory_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/port/filesystem"
	filesystemcontracts "go.llib.dev/frameless/port/filesystem/filesystemcontracts"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/testcase/assert"
)

func ExampleFileSystem() {
	fsys := &memory.FileSystem{}

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

func TestFileSystem_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(&memory.FileSystem{}).Test(t)
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
