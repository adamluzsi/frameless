package filesystems_test

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	filesystemcontracts "github.com/adamluzsi/frameless/ports/filesystem/contracts"

	"github.com/adamluzsi/frameless/adapters/filesystems"
	"github.com/adamluzsi/testcase"
)

func TestLocal_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem{
		Subject: func(tb testing.TB) filesystems.FileSystem {
			return filesystems.Local{
				RootPath: t.TempDir(),
			}
		},
	}.Test(t)
}

func TestLocal_rootPath(t *testing.T) {
	s := testcase.NewSpec(t)

	getSysTmpDir := func(t *testcase.T) string {
		t.Helper()
		tmpDir := os.TempDir()
		stat, err := os.Stat(tmpDir)
		t.Must.Nil(err)
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

	touchFile := func(t *testcase.T, fs filesystems.FileSystem, name string) error {
		t.Helper()
		file, err := fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, filesystems.ModeUserRWX)
		if err == nil {
			t.Must.Nil(file.Close())
			t.Cleanup(func() { _ = fs.Remove(name) })
		}
		return err
	}

	s.Test("without .RootPath set, fs is not jailed", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := filesystems.Local{}

		name := tmpFile(t, tmpDir)
		t.Must.Nil(touchFile(t, fs, name))
		_, err := os.Stat(name)
		t.Must.Nil(err)

		name = tmpFile(t, getSysTmpDir(t))
		t.Must.Nil(touchFile(t, fs, name))
		_, err = os.Stat(name)
		t.Must.Nil(err)
	})

	s.Test("with .RootPath set, fs is jailed and path used as relative path", func(t *testcase.T) {
		tmpDir := t.TempDir()
		fs := filesystems.Local{RootPath: tmpDir}

		name := makeName(t)
		t.Must.Nil(touchFile(t, fs, name))
		_, err := os.Stat(filepath.Join(tmpDir, name))
		t.Must.Nil(err)
		_, err = fs.Stat(name)
		t.Must.Nil(err)
		t.Must.Nil(fs.Mkdir(makeName(t), filesystems.ModeUserRWX))
		t.Must.Nil(fs.Remove(name))

		path := filepath.Join("..", name)
		t.Must.ErrorIs(syscall.EACCES, touchFile(t, fs, path))
		t.Must.ErrorIs(syscall.EACCES, fs.Mkdir(path, 0700))
		t.Must.ErrorIs(syscall.EACCES, func() error { _, err := fs.Stat(path); return err }())
		t.Must.ErrorIs(syscall.EACCES, fs.Remove(path))
	})
}
