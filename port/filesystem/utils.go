package filesystem

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// Open opens the named file for reading (os.O_RDONLY).
// The caller must close the returned file.
//
// Names follow the native path conventions of fsys.OpenFile. For names that
// have the same spelling in io/fs, Open prefers fs.FS.Open when available.
// If its handle does not implement File, Open closes it before falling back
// to OpenFile. Errors from opening or closing are returned unchanged.
func Open(fsys FileOpener, name string) (File, error) {
	localName, pathErr := filepath.Localize(name)
	if f, ok := fsys.(fs.FS); ok && pathErr == nil && localName == name {
		file, err := f.Open(name)
		if err != nil {
			return nil, err
		}
		if file, ok := file.(File); ok {
			return file, nil
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	return fsys.OpenFile(name, os.O_RDONLY, 0)
}

// Create creates or truncates the named file. If the file already exists,
// it is truncated. If the file does not exist, it is created with FileMode 0666
// (before umask). If successful, methods on the returned File can
// be used for I/O; the associated file descriptor has FileMode O_RDWR.
// If there is an error, it will be of type *PathError.
func Create(fsys FileOpener, name string) (File, error) {
	return fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// ReadFile reads the named file and returns its contents.
// A successful call returns err == nil, not err == EOF.
// Because ReadFile reads the whole file, it does not treat an EOF from Read
// as an error to be reported.
// The file is closed before returning; like os.ReadFile, errors from Close are ignored.
func ReadFile(fsys FileOpener, name string) ([]byte, error) {
	file, err := Open(fsys, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// WriteFile writes data to the named file, creating it if necessary.
// If the file does not exist, WriteFile creates it with permissions perm (before umask);
// otherwise WriteFile truncates it before writing, without changing permissions.
// Since WriteFile requires multiple system calls to complete,
// a failure mid-operation can leave the file in a partially written state.
// If both writing and closing fail, WriteFile returns the write error.
func WriteFile(fsys FileOpener, name string, data []byte, perm fs.FileMode) error {
	file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// MkdirAll creates a directory named name, along with any necessary parents,
// and returns nil, or else returns an error.
// The permission bits perm (before umask) are used for all directories that MkdirAll creates.
// If name is already a directory, MkdirAll does nothing and returns nil.
func MkdirAll(fsys interface {
	FileInfoReader
	DirectoryMaker
}, name string, perm fs.FileMode) error {
	if name == "" {
		return &fs.PathError{Op: "mkdir", Path: name, Err: syscall.ENOENT}
	}
	info, err := fsys.Stat(name)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: name, Err: syscall.ENOTDIR}
	}

	// Find the parent without cleaning the path: removing ".." can skip
	// missing ancestors, change symlink resolution, or bypass a backend's jail.
	i := len(name) - 1
	for i >= 0 && os.IsPathSeparator(name[i]) {
		i--
	}
	for i >= 0 && !os.IsPathSeparator(name[i]) {
		i--
	}
	if i > len(filepath.VolumeName(name)) {
		if err := MkdirAll(fsys, name[:i], perm); err != nil {
			return err
		}
	}

	if err := fsys.Mkdir(name, perm); err != nil {
		// Another caller may have made it, or name may end with "/.".
		if info, statErr := fsys.Stat(name); statErr == nil && info.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// ReadDir reads the named directory,
// returning all its directory entries sorted by filename.
// If an error occurs reading the directory,
// ReadDir returns the entries it was able to read before the error,
// along with the error.
func ReadDir(fsys FileOpener, name string) ([]fs.DirEntry, error) {
	f, err := Open(fsys, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dirs, err := f.ReadDir(-1)
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	return dirs, err
}

// WalkDir walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root.
//
// All errors that arise visiting files and directories are filtered by fn:
// see the fs.WalkDirFunc documentation for details.
//
// The files are walked in lexical order, which makes the output deterministic
// but requires WalkDir to read an entire directory into memory before proceeding
// to walk that directory.
//
// WalkDir does not follow symbolic links.
func WalkDir(fsys interface {
	FileInfoReader
	FileOpener
}, root string, fn fs.WalkDirFunc) error {
	info, err := fsys.Stat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fsys, root, DirEntry{
			FileInfo: FileInfo{
				Path:        root,
				FileSize:    info.Size(),
				FileMode:    info.Mode(),
				ModifiedAt:  info.ModTime(),
				IsDirectory: info.IsDir(),
				System:      info.Sys(),
			},
		}, fn)
	}
	if err == fs.SkipDir {
		return nil
	}
	return err
}

func walkDir(fsys interface {
	FileInfoReader
	FileOpener
}, name string, entry fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(name, entry, nil); err != nil || !entry.IsDir() {
		if err == fs.SkipDir && entry.IsDir() {
			err = nil // Successfully skipped directory.
		}
		return err
	}

	dirEntries, err := ReadDir(fsys, name)
	if err != nil {
		// Second call, to report ReadDir error.
		err = walkDirFn(name, entry, err)
		if err != nil {
			return err
		}
	}

	for _, dirEntry := range dirEntries {
		dirEntryName := filepath.Join(name, dirEntry.Name())
		if err := walkDir(fsys, dirEntryName, dirEntry, walkDirFn); err != nil {
			if err == fs.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
