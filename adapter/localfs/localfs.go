// Package localfs has suppliers using the local filesystem.
package localfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"go.llib.dev/frameless/port/filesystem"
)

// FileSystem provides local file system access through the filesystem.FileSystem interface.
type FileSystem struct {
	// RootPath is an optional parameter to jail the file system access for file access.
	RootPath string
}

func (fs FileSystem) path(name, op string) (string, error) {
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

func (fs FileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	path, err := fs.path(name, "open")
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm)
}

func (fs FileSystem) Mkdir(name string, perm fs.FileMode) error {
	path, err := fs.path(name, "mkdir")
	if err != nil {
		return err
	}
	return os.Mkdir(path, perm)
}

func (fs FileSystem) Remove(name string) error {
	path, err := fs.path(name, "remove")
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (fs FileSystem) Stat(name string) (fs.FileInfo, error) {
	path, err := fs.path(name, "stat")
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}
