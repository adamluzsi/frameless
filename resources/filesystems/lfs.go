package filesystems

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/adamluzsi/frameless"
)

// LFS provides local file system access through the frameless.FileSystem interface.
type LFS struct {
	// RootPath is an optional parameter to jail the file system access for file access.
	RootPath string
}

func (fs LFS) path(name, op string) (string, error) {
	if fs.RootPath == "" {
		return name, nil
	}

	root, err := filepath.Abs(fs.RootPath)
	if err != nil {
		return "", err
	}

	path, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(path, root) {
		return "", &os.PathError{
			Op:   op,
			Path: name,
			Err:  syscall.EACCES,
		}
	}

	return path, nil
}

func (fs LFS) OpenFile(name string, flag int, perm fs.FileMode) (frameless.File, error) {
	path, err := fs.path(name, "open")
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm)
}

func (fs LFS) Mkdir(name string, perm fs.FileMode) error {
	path, err := fs.path(name, "mkdir")
	if err != nil {
		return err
	}
	return os.Mkdir(path, perm)
}

func (fs LFS) Remove(name string) error {
	path, err := fs.path(name, "remove")
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (fs LFS) Stat(name string) (fs.FileInfo, error) {
	path, err := fs.path(name, "stat")
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}
