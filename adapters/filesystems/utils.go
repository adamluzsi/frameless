package filesystems

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Open opens the named file for reading. If successful, methods on
// the returned file can be used for reading; the associated file
// descriptor has FileMode O_RDONLY.
// If there is an error, it will be of type *PathError.
func Open(fsys FileSystem, name string) (File, error) {
	return fsys.OpenFile(name, os.O_RDONLY, 0)
}

// Create creates or truncates the named file. If the file already exists,
// it is truncated. If the file does not exist, it is created with FileMode 0666
// (before umask). If successful, methods on the returned File can
// be used for I/O; the associated file descriptor has FileMode O_RDWR.
// If there is an error, it will be of type *PathError.
func Create(fsys FileSystem, name string) (File, error) {
	return fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// ReadDir reads the named directory,
// returning all its directory entries sorted by filename.
// If an error occurs reading the directory,
// ReadDir returns the entries it was able to read before the error,
// along with the error.
func ReadDir(fsys FileSystem, name string) ([]fs.DirEntry, error) {
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
func WalkDir(fsys FileSystem, root string, fn fs.WalkDirFunc) error {
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

func walkDir(fsys FileSystem, name string, entry fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
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
