package filesystem_test

import (
	"io/fs"

	"github.com/adamluzsi/frameless/adapters/filesystems"
	"github.com/adamluzsi/frameless/ports/filesystem"
)

func ExampleOpen() {
	var fsys filesystem.FileSystem = &filesystems.Memory{}

	file, err := filesystem.Open(fsys, "testfile")
	if err != nil {
		panic(err)
	}
	_ = file
}

func ExampleCreate() {
	var fsys filesystem.FileSystem = &filesystems.Memory{}

	file, err := filesystem.Create(fsys, "testfile")
	if err != nil {
		panic(err)
	}
	_ = file
}

func ExampleReadDir() {
	var fsys filesystem.FileSystem = &filesystems.Memory{}

	files, err := filesystem.ReadDir(fsys, "testdir")
	if err != nil {
		panic(err)
	}
	_ = files
}

func ExampleWalkDir() {
	var fsys filesystem.FileSystem = &filesystems.Memory{}

	_ = filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		return fs.SkipDir
	})
}
