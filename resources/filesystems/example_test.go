package filesystems_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/adamluzsi/frameless/resources/filesystems"
)

func ExampleLFS() {
	fsys := filesystems.LFS{}

	file, err := fsys.OpenFile("test", os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystems.ModeUserRWX)
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

	fsys.Mkdir("a", filesystems.ModeUserRWX)

	file2Name := filepath.Join("a", "test.txt")
	file2, err := filesystems.Create(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	file2, err = filesystems.Open(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	filesystems.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		return fs.SkipDir
	})
}

func ExampleMemory() {
	fsys := &filesystems.Memory{}

	file, err := fsys.OpenFile("test", os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystems.ModeUserRWX)
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

	fsys.Mkdir("a", filesystems.ModeUserRWX)

	file2Name := filepath.Join("a", "test.txt")
	file2, err := filesystems.Create(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	file2, err = filesystems.Open(fsys, file2Name)
	if err != nil {
		panic(err)
	}
	file2.Close()

	filesystems.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		return fs.SkipDir
	})
}
