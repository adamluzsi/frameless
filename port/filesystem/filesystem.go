package filesystem

import (
	"io"
	"io/fs"
	"path/filepath"
	"time"
)

type File interface {
	io.Closer
	io.Reader
	io.Writer
	io.Seeker
	fs.File
	fs.ReadDirFile
}

// FileSystem is a header interface for representing a file-system.
// It combines FileOpener, FileInfoReader, DirectoryMaker and Remover.
//
// Consumers that need less should depend only on the role interfaces they use,
// and compose Renamer when they need to rename files.
// Unlike fs.FS, names follow the implementation's native path conventions.
//
// permission cheat sheet:
//
//	+-----+---+--------------------------+
//	| rwx | 7 | Read, write and execute  |
//	| rw- | 6 | Read, write              |
//	| r-x | 5 | Read, and execute        |
//	| r-- | 4 | Read,                    |
//	| -wx | 3 | Write and execute        |
//	| -w- | 2 | Write                    |
//	| --x | 1 | Execute                  |
//	| --- | 0 | no permissions           |
//	+------------------------------------+
//
//	+------------+------+-------+
//	| Permission | Octal| Field |
//	+------------+------+-------+
//	| rwx------  | 0700 | User  |
//	| ---rwx---  | 0070 | Group |
//	| ------rwx  | 0007 | Other |
//	+------------+------+-------+
type FileSystem interface {
	FileInfoReader
	FileOpener
	DirectoryMaker
	Remover
}

// FileInfoReader reads metadata for a named file or directory.
type FileInfoReader interface {
	// Stat returns a FileInfo describing the named file or directory.
	// If there is an error, it will be of type *fs.PathError.
	Stat(name string) (fs.FileInfo, error)
}

var _ FileInfoReader = fs.StatFS(nil)

// FileOpener opens or creates files with the requested access flags and permissions.
type FileOpener interface {
	// OpenFile opens the named file with the specified flags (os.O_RDONLY, etc.).
	// If the file does not exist and os.O_CREATE is set, it is created with
	// permissions perm (before umask).
	// If there is an error, it will be of type *fs.PathError.
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
}

// DirectoryMaker creates directories.
type DirectoryMaker interface {
	// Mkdir creates a new directory with the specified name and permission
	// bits (before umask).
	// If there is an error, it will be of type *fs.PathError.
	Mkdir(name string, perm fs.FileMode) error
}

// Remover removes files and empty directories.
type Remover interface {
	// Remove removes the named file or (empty) directory.
	// If there is an error, it will be of type *fs.PathError.
	Remove(name string) error
}

// Renamer renames or moves files and directories.
type Renamer interface {
	// Rename renames (moves) oldName to newName.
	// If newName already exists and is not a directory, Rename replaces it.
	// If newName already exists and is a directory, Rename returns an error.
	// The file keeps its content and mode.
	// Whether replacing newName is atomic depends on the implementation,
	// as with os.Rename, which is atomic on Unix, but not on every other platform.
	// If there is an error, it will be of type *os.LinkError.
	Rename(oldName, newName string) error
}

type FileInfo struct {
	Path        string
	FileSize    int64
	FileMode    fs.FileMode
	ModifiedAt  time.Time
	IsDirectory bool
	System      any
}

func (fi FileInfo) Name() string       { return filepath.Base(fi.Path) }
func (fi FileInfo) Size() int64        { return fi.FileSize }
func (fi FileInfo) Mode() fs.FileMode  { return fi.FileMode }
func (fi FileInfo) ModTime() time.Time { return fi.ModifiedAt }
func (fi FileInfo) IsDir() bool        { return fi.IsDirectory }
func (fi FileInfo) Sys() any           { return fi.System }

type DirEntry struct{ FileInfo fs.FileInfo }

func (de DirEntry) Name() string               { return de.FileInfo.Name() }
func (de DirEntry) IsDir() bool                { return de.FileInfo.IsDir() }
func (de DirEntry) Type() fs.FileMode          { return de.FileInfo.Mode().Type() }
func (de DirEntry) Info() (fs.FileInfo, error) { return de.FileInfo, nil }
