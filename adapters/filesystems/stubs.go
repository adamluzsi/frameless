package filesystems

import (
	"io/fs"
	"path/filepath"
	"time"
)

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

type DirEntry struct {
	FileInfo fs.FileInfo
}

func (de DirEntry) Name() string               { return de.FileInfo.Name() }
func (de DirEntry) IsDir() bool                { return de.FileInfo.IsDir() }
func (de DirEntry) Type() fs.FileMode          { return de.FileInfo.Mode().Type() }
func (de DirEntry) Info() (fs.FileInfo, error) { return de.FileInfo, nil }
