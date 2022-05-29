package frameless

import (
	"io"
	"io/fs"
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
//
// permission cheat sheet:
//   +-----+---+--------------------------+
//   | rwx | 7 | Read, write and execute  |
//   | rw- | 6 | Read, write              |
//   | r-x | 5 | Read, and execute        |
//   | r-- | 4 | Read,                    |
//   | -wx | 3 | Write and execute        |
//   | -w- | 2 | Write                    |
//   | --x | 1 | Execute                  |
//   | --- | 0 | no permissions           |
//   +------------------------------------+
//
//   +------------+------+-------+
//   | Permission | Octal| Field |
//   +------------+------+-------+
//   | rwx------  | 0700 | User  |
//   | ---rwx---  | 0070 | Group |
//   | ------rwx  | 0007 | Other |
//   +------------+------+-------+
//
type FileSystem interface {
	// Stat returns a FileInfo describing the named file.
	// If there is an error, it will be of type *PathError.
	Stat(name string) (fs.FileInfo, error)
	// OpenFile is the generalized open call; most users will use Open
	// or Create instead. It opens the named file with specified flag
	// (O_RDONLY etc.). If the file does not exist, and the O_CREATE flag
	// is passed, it is created with mode perm (before umask). If successful,
	// methods on the returned File can be used for I/O.
	// If there is an error, it will be of type *PathError.
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	// Mkdir creates a new directory with the specified name and permission
	// bits (before umask).
	// If there is an error, it will be of type *PathError.
	Mkdir(name string, perm fs.FileMode) error
	// Remove removes the named file or (empty) directory.
	// If there is an error, it will be of type *PathError.
	Remove(name string) error
}
