package filesystem_test

import (
	"github.com/adamluzsi/frameless/ports/filesystem"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	ffs "github.com/adamluzsi/frameless/adapters/filesystems"
	"github.com/adamluzsi/testcase/assert"
)

func makeFS(tb testing.TB) ffs.FileSystem {
	if _, ok := os.LookupEnv("USE_FS"); ok {
		return &ffs.Local{RootPath: tb.TempDir()}
	}
	return &ffs.Memory{}
}

func Test_createAndOpen(t *testing.T) {
	it := assert.MakeIt(t)
	fsys := makeFS(t)
	name := "test.txt"
	file, err := filesystem.Create(fsys, name)
	it.Must.Nil(err)
	fileInfo, err := file.Stat()
	it.Must.Nil(err)
	it.Must.False(fileInfo.IsDir())
	it.Must.True(fileInfo.Mode()&ffs.ModeUserRW != 0)

	data := "Hello, world!"
	n, err := file.Write([]byte(data))
	it.Must.Nil(err)
	it.Must.Equal(len([]byte(data)), n)
	it.Must.Nil(file.Close())

	file, err = filesystem.Open(fsys, name)
	it.Must.Nil(err)

	bs, err := io.ReadAll(file)
	it.Must.Nil(err)

	it.Must.Equal(data, string(bs))
}

func TestReadDir(t *testing.T) {
	it := assert.MakeIt(t)
	fsys := makeFS(t)
	dirName := "test.d"

	t.Log("on missing dir, it yields error")
	_, err := filesystem.ReadDir(fsys, dirName)
	it.Must.ErrorIs(fs.ErrNotExist, err)

	t.Log("on empty dir, returns an empty list")
	it.Must.Nil(fsys.Mkdir(dirName, ffs.ModeUserRWX))
	dirEntries, err := filesystem.ReadDir(fsys, dirName)
	it.Must.Nil(err)
	it.Must.Empty(dirEntries)

	t.Log("on dir with entries, return the list in sorted order")
	for _, fileName := range []string{"c", "a", "b"} {
		filePath := filepath.Join(dirName, fileName)
		f, err := filesystem.Create(fsys, filePath)
		it.Must.Nil(err)
		it.Must.Nil(f.Close())
		t.Cleanup(func() { fsys.Remove(filePath) })
	}
	dirEntries, err = filesystem.ReadDir(fsys, dirName)
	it.Must.Nil(err)
	it.Must.Equal(3, len(dirEntries))
	it.Must.Equal("a", dirEntries[0].Name())
	it.Must.Equal("b", dirEntries[1].Name())
	it.Must.Equal("c", dirEntries[2].Name())
}

// TestWalkDir
//
// TODO: make stub tests to cover all rainy-Path
func TestWalkDir(t *testing.T) {
	it := assert.MakeIt(t)
	fsys := makeFS(t)

	touchFile := func(tb testing.TB, name string) {
		file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, ffs.ModeUserRWX)
		assert.Must(tb).Nil(err)
		assert.Must(tb).Nil(file.Close())
	}

	it.Must.Nil(fsys.Mkdir("a", ffs.ModeUserRWX))
	touchFile(t, "a/1")
	touchFile(t, "a/2")
	touchFile(t, "a/3")
	it.Must.Nil(fsys.Mkdir("b", ffs.ModeUserRWX))
	touchFile(t, "b/4")
	touchFile(t, "b/5")
	touchFile(t, "b/6")
	it.Must.Nil(fsys.Mkdir("a/c", ffs.ModeUserRWX))
	touchFile(t, "a/c/7")
	touchFile(t, "a/c/8")
	touchFile(t, "a/c/9")

	var names []string
	it.Must.Nil(filesystem.WalkDir(fsys, "a", func(path string, d fs.DirEntry, err error) error {
		it.Must.Nil(err)
		names = append(names, d.Name())
		return nil
	}))
	it.Must.Equal([]string{"a", "1", "2", "3", "c", "7", "8", "9"}, names)

	names = nil
	it.Must.Nil(filesystem.WalkDir(fsys, "a/c", func(path string, d fs.DirEntry, err error) error {
		it.Must.Nil(err)
		names = append(names, d.Name())
		return nil
	}))
	it.Must.Equal([]string{"c", "7", "8", "9"}, names)

	names = nil
	it.Must.Nil(filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Log(err.Error())
		}
		it.Must.Nil(err)
		names = append(names, d.Name())
		return nil
	}))
	it.Must.Equal([]string{".", "a", "1", "2", "3", "c", "7", "8", "9", "b", "4", "5", "6"}, names)

	names = nil
	it.Must.Nil(filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		it.Must.Nil(err)
		names = append(names, d.Name())

		// intentionally skipping after name is already added,
		// to confirm that the rest of the directory is skipped
		if err == nil && d.IsDir() && d.Name() == "a" {
			return fs.SkipDir
		}
		return nil
	}))
	it.Must.Equal([]string{".", "a", "b", "4", "5", "6"}, names)
}
