package filesystem_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/port/filesystem"

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
	it := assert.MakeIt(t)
	fsys := makeFS(t)
	name := "test.txt"
	file, err := filesystem.Create(fsys, name)
	assert.NoError(it, err)
	fileInfo, err := file.Stat()
	assert.NoError(it, err)
	assert.False(it, fileInfo.IsDir())
	assert.True(it, fileInfo.Mode()&filesystem.ModeUserRW != 0)

	data := "Hello, world!"
	n, err := file.Write([]byte(data))
	assert.NoError(it, err)
	assert.Equal(it, len([]byte(data)), n)
	assert.NoError(it, file.Close())

	file, err = filesystem.Open(fsys, name)
	assert.NoError(it, err)

	bs, err := io.ReadAll(file)
	assert.NoError(it, err)

	assert.Equal(it, data, string(bs))
}

func TestReadDir(t *testing.T) {
	it := assert.MakeIt(t)
	fsys := makeFS(t)
	dirName := "test.d"

	t.Log("on missing dir, it yields error")
	_, err := filesystem.ReadDir(fsys, dirName)
	it.Must.ErrorIs(fs.ErrNotExist, err)

	t.Log("on empty dir, returns an empty list")
	assert.NoError(it, fsys.Mkdir(dirName, filesystem.ModeUserRWX))
	dirEntries, err := filesystem.ReadDir(fsys, dirName)
	assert.NoError(it, err)
	it.Must.Empty(dirEntries)

	t.Log("on dir with entries, return the list in sorted order")
	for _, fileName := range []string{"c", "a", "b"} {
		filePath := filepath.Join(dirName, fileName)
		f, err := filesystem.Create(fsys, filePath)
		assert.NoError(it, err)
		assert.NoError(it, f.Close())
		t.Cleanup(func() { fsys.Remove(filePath) })
	}
	dirEntries, err = filesystem.ReadDir(fsys, dirName)
	assert.NoError(it, err)
	assert.Equal(it, 3, len(dirEntries))
	assert.Equal(it, "a", dirEntries[0].Name())
	assert.Equal(it, "b", dirEntries[1].Name())
	assert.Equal(it, "c", dirEntries[2].Name())
}

// TestWalkDir
//
// TODO: make stub tests to cover all rainy-Path
func TestWalkDir(t *testing.T) {
	it := assert.MakeIt(t)
	fsys := makeFS(t)

	touchFile := func(tb testing.TB, name string) {
		file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filesystem.ModeUserRWX)
		assert.NoError(tb, err)
		assert.NoError(tb, file.Close())
	}

	assert.NoError(it, fsys.Mkdir("a", filesystem.ModeUserRWX))
	touchFile(t, "a/1")
	touchFile(t, "a/2")
	touchFile(t, "a/3")
	assert.NoError(it, fsys.Mkdir("b", filesystem.ModeUserRWX))
	touchFile(t, "b/4")
	touchFile(t, "b/5")
	touchFile(t, "b/6")
	assert.NoError(it, fsys.Mkdir("a/c", filesystem.ModeUserRWX))
	touchFile(t, "a/c/7")
	touchFile(t, "a/c/8")
	touchFile(t, "a/c/9")

	var names []string
	assert.NoError(it, filesystem.WalkDir(fsys, "a", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(it, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(it, []string{"a", "1", "2", "3", "c", "7", "8", "9"}, names)

	names = nil
	assert.NoError(it, filesystem.WalkDir(fsys, "a/c", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(it, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(it, []string{"c", "7", "8", "9"}, names)

	names = nil
	assert.NoError(it, filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Log(err.Error())
		}
		assert.NoError(it, err)
		names = append(names, d.Name())
		return nil
	}))
	assert.Equal(it, []string{".", "a", "1", "2", "3", "c", "7", "8", "9", "b", "4", "5", "6"}, names)

	names = nil
	assert.NoError(it, filesystem.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		assert.NoError(it, err)
		names = append(names, d.Name())

		// intentionally skipping after name is already added,
		// to confirm that the restapi of the directory is skipped
		if err == nil && d.IsDir() && d.Name() == "a" {
			return fs.SkipDir
		}
		return nil
	}))
	assert.Equal(it, []string{".", "a", "b", "4", "5", "6"}, names)
}
