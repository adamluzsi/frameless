package filesystem_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"

	ffs "go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/testcase/assert"
)

func makeFS(tb testing.TB) filesystem.FileSystem {
	if _, ok := os.LookupEnv("USE_FS"); ok {
		return &ffs.FileSystem{RootPath: tb.TempDir()}
	}
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
