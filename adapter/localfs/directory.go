package localfs

import (
	"context"
	"io/fs"
	"os"
	"strings"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/port/filesystem"
)

func localFileSystem(fsys filesystem.FileOpener) (FileSystem, bool) {
	switch fsys := fsys.(type) {
	case FileSystem:
		return fsys, true
	case *FileSystem:
		if fsys != nil {
			return *fsys, true
		}
	}
	return FileSystem{}, false
}

// openRepositoryDir scopes entity names to Dir. For the OS backend it also pins
// the opened directory and prevents symlink escapes from Dir, not just RootPath.
// Other backends retain their own path-resolution and confinement semantics.
func openRepositoryDir(fsys RepositoryFileSystem, dir string, lockCtx context.Context) (fileio.FileSystem, func() error, error) {
	if local, ok := localFileSystem(fsys); ok {
		var root *os.Root
		var err error
		if state := repositoryDirectoryLock(lockCtx, local, dir); state != nil {
			state.mutex.Lock()
			err = state.check()
			if err == nil {
				// Duplicate the pinned root while it is still owned. A borrower
				// closes only this view, never the caller's lock root.
				root, err = state.root.OpenRoot(".")
			}
			state.mutex.Unlock()
		} else {
			root, err = local.openRoot(dir)
		}
		if err != nil {
			return nil, nil, err
		}
		return rootedFileSystem{root}, root.Close, nil
	}
	return directoryFileSystem{FileSystem: fsys, dir: dir}, func() error { return nil }, nil
}

func repositoryDirectoryLock(ctx context.Context, fsys FileSystem, dir string) *lockerState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(directoryLockKey{fsys, dir}).(*lockerState)
	return state
}

func (fsys FileSystem) openRoot(name string) (*os.Root, error) {
	if fsys.RootPath == "" {
		return os.OpenRoot(name)
	}
	var dir *os.Root
	err := fsys.inRoot(func(root *os.Root) (err error) {
		dir, err = root.OpenRoot(name)
		return err
	})
	if err != nil {
		return nil, &fs.PathError{Op: "openroot", Path: name, Err: rootError(err)}
	}
	return dir, nil
}

// checkedFileSystem checks operation cancellation and ownership between file
// operations. This is not fencing: a lease can still be lost during an in-flight
// filesystem call. The backend must supply fencing for that stronger guarantee.
// Files are returned unchanged, preserving optional Sync and Chmod support.
type checkedFileSystem struct {
	fileio.FileSystem
	check func() error
}

func (f checkedFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	return f.FileSystem.OpenFile(name, flag, perm)
}

func (f checkedFileSystem) Stat(name string) (fs.FileInfo, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	return f.FileSystem.Stat(name)
}

func (f checkedFileSystem) Lstat(name string) (fs.FileInfo, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	return statEntry(f.FileSystem, name)
}

// statEntry preserves native no-clobber checks for dangling symlinks without
// requiring Lstat from filesystems which have no notion of symbolic links.
func statEntry(fsys filesystem.FileInfoReader, name string) (fs.FileInfo, error) {
	if fsys, ok := fsys.(interface {
		Lstat(string) (fs.FileInfo, error)
	}); ok {
		return fsys.Lstat(name)
	}
	return fsys.Stat(name)
}

func (f checkedFileSystem) Rename(oldpath, newpath string) error {
	if err := f.check(); err != nil {
		return err
	}
	return f.FileSystem.Rename(oldpath, newpath)
}

func (f checkedFileSystem) Remove(name string) error {
	// Entity names never start with a dot. Hidden names are our private
	// staging/temporary files, whose cleanup must work after cancellation.
	if !strings.HasPrefix(name, ".") {
		if err := f.check(); err != nil {
			return err
		}
	}
	return f.FileSystem.Remove(name)
}

// directoryFileSystem only receives repository-generated names. Do not clean Dir:
// cleaning dot segments could change how a backend resolves its symlinks.
type directoryFileSystem struct {
	FileSystem RepositoryFileSystem
	dir        string
}

func (d directoryFileSystem) path(name string) string { return d.dir + string(os.PathSeparator) + name }

func (d directoryFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	return d.FileSystem.OpenFile(d.path(name), flag, perm)
}

func (d directoryFileSystem) Stat(name string) (fs.FileInfo, error) {
	return d.FileSystem.Stat(d.path(name))
}

func (d directoryFileSystem) Lstat(name string) (fs.FileInfo, error) {
	return statEntry(d.FileSystem, d.path(name))
}

func (d directoryFileSystem) Mkdir(name string, perm fs.FileMode) error {
	return d.FileSystem.Mkdir(d.path(name), perm)
}

func (d directoryFileSystem) Remove(name string) error { return d.FileSystem.Remove(d.path(name)) }

func (d directoryFileSystem) Rename(oldpath, newpath string) error {
	return d.FileSystem.Rename(d.path(oldpath), d.path(newpath))
}

// rootedFileSystem adapts the covariant OpenFile return type of os.Root to the port.
// It keeps transient OS retries here, rather than in the generic file helpers.
type rootedFileSystem struct{ *os.Root }

func (r rootedFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	file, err := r.Root.OpenFile(name, flag, perm.Perm())
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: rootError(err)}
	}
	return file, nil
}

func (r rootedFileSystem) Stat(name string) (fs.FileInfo, error) {
	info, err := r.Root.Stat(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: rootError(err)}
	}
	return info, nil
}

func (r rootedFileSystem) Mkdir(name string, perm fs.FileMode) error {
	if err := r.Root.Mkdir(name, perm.Perm()); err != nil {
		return &fs.PathError{Op: "mkdir", Path: name, Err: rootError(err)}
	}
	return nil
}

func (r rootedFileSystem) Remove(name string) error {
	if err := fileio.Retry(func() error { return r.Root.Remove(name) }); err != nil {
		return &fs.PathError{Op: "remove", Path: name, Err: rootError(err)}
	}
	return nil
}

func (r rootedFileSystem) Rename(oldpath, newpath string) error {
	if err := fileio.Retry(func() error { return r.Root.Rename(oldpath, newpath) }); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: rootError(err)}
	}
	return nil
}
