// Package localfs has suppliers using the local file system.
//
//   - FileSystem implements filesystem.FileSystem on top of the os package, optionally jailed into a RootPath.
//   - Repository is a crud repository over a RepositoryFileSystem, keeping each entity in a human-friendly file named after its ID.
//     It supports transactions through comproto.OnePhaseCommitProtocol.
//   - Locker implements guard.Locker for a local directory, with inter-process locking on Linux, macOS and Windows.
//   - Watch is a volatile pubsub subscriber of the changes of a directory's files in an fs.FS, including the ones made by hand or by another process.
//
// FileSystem implements every role of filesystem.FileSystem, and Renamer too,
// so it satisfies RepositoryFileSystem.
//
// Repository writes through hidden temporary files renamed into place; atomic visibility depends on the backend's Rename.
// Changes are serialised through its Locker. With the default local filesystem backend,
// Linux and macOS lock Dir with flock(2), and Windows locks its persistent hidden ".lock" file with LockFileEx.
// Other filesystems default to per-repository in-process locking, or can use an injected shared Locker.
// Transaction commits publish files individually, not as an all-or-nothing multi-file operation.
package localfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
)

// FileSystem provides local file system access through filesystem.FileOpener,
// FileInfoReader, DirectoryMaker, Remover and Renamer. It also implements fs.FS.
type FileSystem struct {
	// RootPath is an optional parameter to jail the file system access into a directory.
	//
	// When set, names are resolved by os.Root and must be relative.
	// ".." segments and relative symbolic links are allowed if resolution never leaves the root.
	// Absolute names are rejected. Symlinks followed during resolution must have relative targets,
	// even if an absolute target would point inside the root.
	// Jailbreak attempts fail with syscall.EACCES, with confinement subject to os.Root's platform limitations.
	// RootPath itself may be a symbolic link.
	//
	// File and directory creation currently uses only perm.Perm() (before umask), ignoring other mode bits.
	// This accepts irrelevant type bits such as fs.ModeDir; preserving Unix setuid, setgid and sticky bits
	// is not currently implemented.
	RootPath string
}

var (
	_ fs.FS                     = FileSystem{}
	_ filesystem.FileSystem     = FileSystem{}
	_ filesystem.FileInfoReader = FileSystem{}
	_ filesystem.FileOpener     = FileSystem{}
	_ filesystem.DirectoryMaker = FileSystem{}
	_ filesystem.Remover        = FileSystem{}
	_ filesystem.Renamer        = FileSystem{}
	_ RepositoryFileSystem      = FileSystem{}
)

// Open opens the named file for reading, following the conventions of fs.FS, like os.DirFS does.
//
// Unlike OpenFile, it only accepts slash-separated names which fs.ValidPath allows,
// so absolute names, empty segments and "." or ".." segments are rejected with fs.ErrInvalid.
// Without a RootPath, names are relative to the current working directory.
func (fsys FileSystem) Open(name string) (fs.File, error) {
	localName, err := filepath.Localize(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	file, err := fsys.OpenFile(localName, os.O_RDONLY, filemode.UserR)
	if err != nil {
		if pathErr, ok := err.(*fs.PathError); ok {
			pathErr.Path = name
		}
		return nil, err
	}
	return file, nil
}

func (fsys FileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	if fsys.RootPath == "" {
		return os.OpenFile(name, flag, perm)
	}
	var file *os.File
	err := fsys.inRoot(func(root *os.Root) (err error) {
		file, err = root.OpenFile(name, flag, perm.Perm())
		return err
	})
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: rootError(err)}
	}
	return file, nil
}

func (fsys FileSystem) Mkdir(name string, perm fs.FileMode) error {
	if fsys.RootPath == "" {
		return os.Mkdir(name, perm)
	}
	err := fsys.inRoot(func(root *os.Root) error {
		return root.Mkdir(name, perm.Perm())
	})
	if err != nil {
		return &fs.PathError{Op: "mkdir", Path: name, Err: rootError(err)}
	}
	return nil
}

// Remove removes the named file or (empty) directory with os.Remove.
// On Windows, it is retried for a moment while another process, like a virus scanner, holds the file open.
func (fsys FileSystem) Remove(name string) error {
	if fsys.RootPath == "" {
		return fileio.Retry(func() error { return os.Remove(name) })
	}
	err := fsys.inRoot(func(root *os.Root) error {
		return fileio.Retry(func() error { return root.Remove(name) })
	})
	if err != nil {
		return &fs.PathError{Op: "remove", Path: name, Err: rootError(err)}
	}
	return nil
}

// Rename renames (moves) oldpath to newpath with os.Rename.
// On Windows, it is retried for a moment while another process, like a virus scanner, holds the file open.
func (fsys FileSystem) Rename(oldpath, newpath string) error {
	if fsys.RootPath == "" {
		return fileio.Retry(func() error { return os.Rename(oldpath, newpath) })
	}
	err := fsys.inRoot(func(root *os.Root) error {
		return fileio.Retry(func() error { return root.Rename(oldpath, newpath) })
	})
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: rootError(err)}
	}
	return nil
}

func (fsys FileSystem) Stat(name string) (fs.FileInfo, error) {
	if fsys.RootPath == "" {
		return os.Stat(name)
	}
	var info fs.FileInfo
	err := fsys.inRoot(func(root *os.Root) (err error) {
		info, err = root.Stat(name)
		return err
	})
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: rootError(err)}
	}
	return info, nil
}

// inRoot runs op with RootPath opened as an os.Root, which keeps every name of op within RootPath.
//
// RootPath is opened for each operation, rather than once,
// so the FileSystem stays a plain value, and it follows RootPath when it is made or replaced later.
func (fsys FileSystem) inRoot(op func(root *os.Root) error) error {
	root, err := os.OpenRoot(fsys.RootPath)
	if err != nil {
		// os.OpenRoot can report a non-directory RootPath with an error other than syscall.ENOTDIR.
		if info, statErr := os.Stat(fsys.RootPath); statErr == nil && !info.IsDir() {
			return syscall.ENOTDIR
		}
		return err
	}
	defer root.Close()
	return op(root)
}

// msgPathEscapes is the message of the error which os.Root reports for a name that would escape it.
// The os package doesn't export the error itself.
// Should the message ever change, such a name is still denied by os.Root, just with a different error.
const msgPathEscapes = "path escapes from parent"

// rootError unwraps the cause of a failed os.Root operation,
// so the FileSystem can report it like the os package does,
// under the name of the os operation (open rather than openat) and of the name it was given.
// A name which would escape RootPath is reported as syscall.EACCES.
func rootError(err error) error {
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	var linkError *os.LinkError
	if errors.As(err, &linkError) {
		err = linkError.Err
	}
	if err.Error() == msgPathEscapes {
		return syscall.EACCES
	}
	return err
}
