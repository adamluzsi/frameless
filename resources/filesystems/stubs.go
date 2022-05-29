package filesystems

import (
	"io/fs"
	"path/filepath"
	"time"
)

type fileInfo struct {
	path     string
	size     int64
	mode     fs.FileMode
	modeTime time.Time
	isDir    bool
	sys      any
}

func (fi fileInfo) Name() string       { return filepath.Base(fi.path) }
func (fi fileInfo) Size() int64        { return fi.size }
func (fi fileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return fi.modeTime }
func (fi fileInfo) IsDir() bool        { return fi.isDir }
func (fi fileInfo) Sys() any           { return fi.sys }

type dirEntry struct {
	path  string
	isDir bool
	mode  fs.FileMode
	info  fs.FileInfo
}

func (de dirEntry) Name() string               { return filepath.Base(de.path) }
func (de dirEntry) IsDir() bool                { return de.isDir }
func (de dirEntry) Type() fs.FileMode          { return de.mode.Type() }
func (de dirEntry) Info() (fs.FileInfo, error) { return de.info, nil }
