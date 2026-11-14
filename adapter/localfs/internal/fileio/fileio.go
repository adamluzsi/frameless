// Package fileio is the file handling of the localfs Repository and its transactions:
// naming and listing the files of a directory, and writing them atomically.
package fileio

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"go.llib.dev/frameless/port/filesystem"
)

// FileSystem is the file system which fileio writes into, within an existing directory.
// It renames the written files into place; directory creation is left to the caller.
type FileSystem interface {
	filesystem.FileOpener
	filesystem.FileInfoReader
	filesystem.Remover
	filesystem.Renamer
}

// ListFiles returns the names of the regular files in the root directory which end with the suffix, in order.
// Hidden files are left out, which also covers the temporary files of an ongoing write.
func ListFiles(root filesystem.FileOpener, suffix string) ([]string, error) {
	entries, err := filesystem.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, suffix) || !entry.Type().IsRegular() {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// EscapeFileName turns an ID into a file name, which ends with the suffix, like a file extension.
//
// Every byte which isn't safe in a file name across the major platforms is percent-encoded,
// as well as a leading dot, so the file doesn't become a hidden file, which ListFiles leaves out.
func EscapeFileName(id string, suffix string) string {
	var name strings.Builder
	for i := 0; i < len(id); i++ {
		if c := id[i]; IsFileNameSafe(c) && !(i == 0 && c == '.') {
			name.WriteByte(c)
		} else {
			fmt.Fprintf(&name, "%%%02X", c)
		}
	}
	name.WriteString(suffix)
	return name.String()
}

// IsFileNameSafe reports whether the byte is safe in a file name across the major platforms.
func IsFileNameSafe(c byte) bool {
	return 'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

// CreateFile writes data into a new hidden file, then renames it into place.
// It fails with an fs.ErrExist error if the destination already exists.
// The existence check uses Lstat when supported, so dangling symlinks count as existing;
// otherwise it uses Stat, which may follow symlinks.
// The caller must hold a lock covering the existence check and rename.
// This is not an atomic no-replace operation: an uncoordinated writer's destination
// created after the existence check may be overwritten.
// Publication is atomic when the filesystem's Rename is atomic.
func CreateFile(root FileSystem, name string, data []byte, perm fs.FileMode) error {
	stat := root.Stat
	if root, ok := root.(interface {
		Lstat(string) (fs.FileInfo, error)
	}); ok {
		stat = root.Lstat
	}
	if _, err := stat(name); err == nil {
		return &fs.PathError{Op: "create", Path: name, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return writeFile(root, name, data, perm, false)
}

// ReplaceFile writes data into a new hidden file, then renames it into place, replacing an existing file.
// The permission is set to perm exactly when the file supports Chmod; otherwise the creation mode applies.
// Replacement is atomic when the filesystem's Rename is atomic.
func ReplaceFile(root interface {
	filesystem.FileOpener
	filesystem.Remover
	filesystem.Renamer
}, name string, data []byte, perm fs.FileMode) error {
	return writeFile(root, name, data, perm, true)
}

func writeFile(root interface {
	filesystem.FileOpener
	filesystem.Remover
	filesystem.Renamer
}, name string, data []byte, perm fs.FileMode, exactPerm bool) error {
	tmp := "." + name + ".tmp-" + rand.Text()
	if err := WriteNewFile(root, tmp, data, perm, exactPerm); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return errors.Join(err, root.Remove(tmp))
	}
	return nil
}

// WriteNewFile writes data into a new file, syncing it when the file supports Sync.
// It fails with an fs.ErrExist error if the file already exists.
// On failure it closes the file before removing it, reporting any cleanup errors as well.
// When exactPerm is set and the file supports Chmod, perm is applied without the umask.
func WriteNewFile(root interface {
	filesystem.FileOpener
	filesystem.Remover
}, name string, data []byte, perm fs.FileMode, exactPerm bool) (rErr error) {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer func() {
		rErr = errors.Join(rErr, file.Close())
		if rErr != nil {
			rErr = errors.Join(rErr, root.Remove(name))
		}
	}()
	if n, err := file.Write(data); err != nil {
		return err
	} else if n < len(data) {
		return io.ErrShortWrite
	}
	if exactPerm {
		if file, ok := file.(interface{ Chmod(fs.FileMode) error }); ok {
			if err := file.Chmod(perm); err != nil {
				return err
			}
		}
	}
	if file, ok := file.(interface{ Sync() error }); ok {
		return file.Sync()
	}
	return nil
}

// DirPerm derives the permission of a directory from the permission of its files:
// whoever can read the files can list the directory as well, and the owner can always change it.
func DirPerm(filePerm fs.FileMode) fs.FileMode {
	filePerm = filePerm.Perm()
	return filePerm | 0o700 | (filePerm&0o044)>>2
}
