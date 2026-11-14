package memory

import (
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/testcase/clock"
)

// FileSystem is an in-memory filesystem.FileSystem, and an fs.FS.
//
// Names are resolved against the current working directory, as the os package does.
// The working directory (".") is the root directory, and like the directories above it, it always exists,
// so names can be created in them, but they can't be removed or renamed.
// As on disk, a name below a file fails with syscall.ENOTDIR, rather than fs.ErrNotExist.
// Rename fails with the same errors as os.Rename does on Unix, and as it is made under a lock, it is atomic.
// Open follows the fs.FS conventions, and only accepts paths that fs.ValidPath allows.
//
// FileSystem is safe for concurrent use, and its zero value is ready to use.
type FileSystem struct {
	mutex   sync.RWMutex
	entries map[string]*fileSystemEntry

	initOnce    sync.Once
	rootModTime time.Time
}

var (
	_ filesystem.FileSystem = (*FileSystem)(nil)
	_ fs.FS                 = (*FileSystem)(nil)
	_ fs.StatFS             = (*FileSystem)(nil)
)

func (mfs *FileSystem) init() {
	mfs.initOnce.Do(func() {
		mfs.entries = make(map[string]*fileSystemEntry)
		mfs.rootModTime = clock.Now()
	})
}

func (mfs *FileSystem) path(name string) string {
	abs, err := filepath.Abs(name)
	if err != nil {
		return name
	}
	return abs
}

// lookup finds the entry of an absolute path. The caller must hold mfs.mutex.
func (mfs *FileSystem) lookup(path string) (*fileSystemEntry, bool) {
	if entry, ok := mfs.entries[path]; ok {
		return entry, true
	}
	if mfs.isRootOrAbove(path) {
		return &fileSystemEntry{
			mode:    fs.ModeDir | 0777,
			modTime: mfs.rootModTime,
			isDir:   true,
		}, true
	}
	return nil, false
}

// isRootOrAbove reports whether path is the root directory, or one of the directories above it.
// These are not stored, but they exist on disk, so the FileSystem has them as well.
func (mfs *FileSystem) isRootOrAbove(path string) bool {
	for dir := mfs.path("."); ; dir = filepath.Dir(dir) {
		if dir == path {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

// checkParent verifies that the directory which path would be created in exists. The caller must hold mfs.mutex.
func (mfs *FileSystem) checkParent(path string) error {
	parent, err := mfs.resolve(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.isDir {
		return syscall.ENOTDIR
	}
	return nil
}

// resolve finds the entry of an absolute path, as the path resolution of an OS file system does:
// when there is no such entry, it fails with ENOTDIR if a directory on the way is a file, otherwise with fs.ErrNotExist.
// The caller must hold mfs.mutex.
func (mfs *FileSystem) resolve(path string) (*fileSystemEntry, error) {
	if entry, ok := mfs.lookup(path); ok {
		return entry, nil
	}
	// Entries are only made in an existing directory, and a directory can't be removed while it has entries,
	// so everything above the closest existing entry is a directory, and that entry alone tells what went wrong.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if entry, ok := mfs.lookup(dir); ok {
			if !entry.isDir {
				return nil, syscall.ENOTDIR
			}
			return nil, fs.ErrNotExist
		}
		if filepath.Dir(dir) == dir {
			return nil, fs.ErrNotExist
		}
	}
}

// children returns the paths of the entries directly inside dirPath. The caller must hold mfs.mutex.
func (mfs *FileSystem) children(dirPath string) []string {
	var children []string
	for path := range mfs.entries {
		if path != dirPath && filepath.Dir(path) == dirPath {
			children = append(children, path)
		}
	}
	for dir := mfs.path("."); filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
		if filepath.Dir(dir) == dirPath {
			children = append(children, dir)
			break
		}
	}
	return children
}

func (mfs *FileSystem) Stat(name string) (fs.FileInfo, error) {
	mfs.init()
	path := mfs.path(name)
	mfs.mutex.RLock()
	defer mfs.mutex.RUnlock()
	entry, err := mfs.resolve(path)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: err}
	}
	return entry.fileInfo(path), nil
}

// Open opens the named file for reading, as fs.FS defines it.
func (mfs *FileSystem) Open(name string) (fs.File, error) {
	path, err := filepath.Localize(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	file, err := mfs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if pathErr, ok := err.(*fs.PathError); ok {
			pathErr.Path = name
		}
		return nil, err
	}
	return file, nil
}

func (mfs *FileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	mfs.init()
	path := mfs.path(name)
	if flag&os.O_CREATE != 0 {
		mfs.mutex.Lock()
		defer mfs.mutex.Unlock()
	} else {
		mfs.mutex.RLock()
		defer mfs.mutex.RUnlock()
	}
	entry, err := mfs.resolve(path)
	if errors.Is(err, fs.ErrNotExist) && flag&os.O_CREATE != 0 {
		return mfs.create(path, flag, perm)
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	if flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0 {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrExist}
	}
	if entry.isDir && filesystem.HasOpenFlagWrite(flag) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.EISDIR}
	}
	if flag&os.O_TRUNC != 0 && !entry.isDir {
		entry.truncate()
	}
	file := &FileSystemFile{path: path, entry: entry, openFlag: flag}
	if entry.isDir {
		file.dirEntries = mfs.dirEntries(path)
	}
	return file, nil
}

// create makes a new file at path, and opens it. The caller must hold mfs.mutex for writing.
func (mfs *FileSystem) create(path string, flag int, perm fs.FileMode) (filesystem.File, error) {
	if err := mfs.checkParent(path); err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	entry := &fileSystemEntry{
		mode:    perm,
		modTime: clock.Now(),
	}
	mfs.entries[path] = entry
	return &FileSystemFile{path: path, entry: entry, openFlag: flag}, nil
}

// dirEntries takes a snapshot of a directory's entries. The caller must hold mfs.mutex.
func (mfs *FileSystem) dirEntries(dirPath string) []fs.DirEntry {
	var entries []fs.DirEntry
	for _, path := range mfs.children(dirPath) {
		child, _ := mfs.lookup(path)
		entries = append(entries, filesystem.DirEntry{FileInfo: child.fileInfo(path)})
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries
}

func (mfs *FileSystem) Mkdir(name string, perm fs.FileMode) error {
	mfs.init()
	path := mfs.path(name)
	mfs.mutex.Lock()
	defer mfs.mutex.Unlock()
	if _, ok := mfs.lookup(path); ok {
		return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	}
	if err := mfs.checkParent(path); err != nil {
		return &fs.PathError{Op: "mkdir", Path: path, Err: err}
	}
	mfs.entries[path] = &fileSystemEntry{
		mode:    perm | fs.ModeDir,
		modTime: clock.Now(),
		isDir:   true,
	}
	return nil
}

func (mfs *FileSystem) Remove(name string) error {
	mfs.init()
	path := mfs.path(name)
	mfs.mutex.Lock()
	defer mfs.mutex.Unlock()
	if mfs.isRootOrAbove(path) {
		return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrPermission}
	}
	entry, err := mfs.resolve(path)
	if err != nil {
		return &fs.PathError{Op: "remove", Path: path, Err: err}
	}
	if entry.isDir && 0 < len(mfs.children(path)) {
		return &fs.PathError{Op: "remove", Path: path, Err: syscall.ENOTEMPTY}
	}
	delete(mfs.entries, path)
	return nil
}

func (mfs *FileSystem) Rename(oldpath, newpath string) error {
	mfs.init()
	oldPath, newPath := mfs.path(oldpath), mfs.path(newpath)
	mfs.mutex.Lock()
	defer mfs.mutex.Unlock()
	if err := mfs.rename(oldPath, newPath); err != nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

// rename moves the entry of oldPath, along with the entries below it, to newPath,
// failing with the same errors as os.Rename does on Unix. The caller must hold mfs.mutex for writing.
func (mfs *FileSystem) rename(oldPath, newPath string) error {
	if mfs.isRootOrAbove(oldPath) {
		return fs.ErrPermission
	}
	source, err := mfs.resolve(oldPath)
	if err != nil {
		return err
	}
	target, targetExists := mfs.lookup(newPath)
	if targetExists && target.isDir {
		return syscall.EEXIST
	}
	if oldPath == newPath {
		return nil
	}
	if err := mfs.checkParent(newPath); err != nil {
		return err
	}
	if targetExists && source.isDir {
		return syscall.ENOTDIR
	}
	if source.isDir && isBelow(newPath, oldPath) {
		return syscall.EINVAL
	}
	moved := make(map[string]*fileSystemEntry)
	for path, entry := range mfs.entries {
		if path == oldPath || isBelow(path, oldPath) {
			moved[newPath+strings.TrimPrefix(path, oldPath)] = entry
			delete(mfs.entries, path)
		}
	}
	maps.Copy(mfs.entries, moved)
	return nil
}

// isBelow reports whether path is inside the dir directory, at any depth.
func isBelow(path, dir string) bool {
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// fileSystemEntry is a file or a directory. It doesn't know its own path,
// as a rename moves it to another one, while the open files keep the path they were opened with.
type fileSystemEntry struct {
	mode  fs.FileMode
	isDir bool

	mutex   sync.Mutex
	data    []byte
	modTime time.Time
}

func (entry *fileSystemEntry) fileInfo(path string) filesystem.FileInfo {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	return filesystem.FileInfo{
		Path:        path,
		FileSize:    int64(len(entry.data)),
		FileMode:    entry.mode,
		ModifiedAt:  entry.modTime,
		IsDirectory: entry.isDir,
	}
}

func (entry *fileSystemEntry) truncate() {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	entry.data = nil
	entry.modTime = clock.Now()
}

// FileSystemFile is an open file of a FileSystem.
//
// Like an OS file descriptor, it has its own offset,
// but it shares the file's content with every other open handle of the same file,
// so writes are visible to them right away.
// It follows the file when the file is renamed, but it keeps the name it was opened with, as an *os.File does.
type FileSystemFile struct {
	path     string
	entry    *fileSystemEntry
	openFlag int

	mutex      sync.Mutex
	offset     int64
	closed     bool
	dirEntries []fs.DirEntry
}

func (f *FileSystemFile) pathError(op string, err error) error {
	return &fs.PathError{Op: op, Path: f.path, Err: err}
}

func (f *FileSystemFile) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return f.pathError("close", fs.ErrClosed)
	}
	f.closed = true
	return nil
}

// Sync has nothing to flush, as writes reach the FileSystem immediately.
func (f *FileSystemFile) Sync() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return f.pathError("sync", fs.ErrClosed)
	}
	return nil
}

func (f *FileSystemFile) Stat() (fs.FileInfo, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return nil, f.pathError("stat", fs.ErrClosed)
	}
	return f.entry.fileInfo(f.path), nil
}

func (f *FileSystemFile) Read(p []byte) (int, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return 0, f.pathError("read", fs.ErrClosed)
	}
	if !filesystem.HasOpenFlagRead(f.openFlag) {
		return 0, f.pathError("read", fs.ErrPermission)
	}
	if len(p) == 0 {
		return 0, nil
	}
	f.entry.mutex.Lock()
	defer f.entry.mutex.Unlock()
	if f.entry.isDir {
		return 0, f.pathError("read", syscall.EISDIR)
	}
	if int64(len(f.entry.data)) <= f.offset {
		return 0, io.EOF
	}
	n := copy(p, f.entry.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *FileSystemFile) Write(p []byte) (int, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return 0, f.pathError("write", fs.ErrClosed)
	}
	if !filesystem.HasOpenFlagWrite(f.openFlag) {
		return 0, f.pathError("write", fs.ErrPermission)
	}
	if len(p) == 0 {
		return 0, nil
	}
	f.entry.mutex.Lock()
	defer f.entry.mutex.Unlock()
	if f.openFlag&os.O_APPEND != 0 {
		f.offset = int64(len(f.entry.data))
	}
	if gap := f.offset + int64(len(p)) - int64(len(f.entry.data)); 0 < gap {
		f.entry.data = append(f.entry.data, make([]byte, gap)...)
	}
	n := copy(f.entry.data[f.offset:], p)
	f.offset += int64(n)
	f.entry.modTime = clock.Now()
	return n, nil
}

func (f *FileSystemFile) Seek(offset int64, whence int) (int64, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return 0, f.pathError("seek", fs.ErrClosed)
	}
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = f.offset
	case io.SeekEnd:
		base = f.entry.fileInfo(f.path).Size()
	default:
		return 0, f.pathError("seek", syscall.EINVAL)
	}
	position := base + offset
	if position < 0 {
		return 0, f.pathError("seek", syscall.EINVAL)
	}
	f.offset = position
	return position, nil
}

// ReadDir reads the directory's entries, as they were when the directory was opened.
func (f *FileSystemFile) ReadDir(n int) ([]fs.DirEntry, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.closed {
		return nil, f.pathError("readdir", fs.ErrClosed)
	}
	if !f.entry.isDir {
		return nil, f.pathError("fdopendir", syscall.ENOTDIR)
	}
	if n <= 0 {
		entries := f.dirEntries
		f.dirEntries = nil
		return entries, nil
	}
	if len(f.dirEntries) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(f.dirEntries))
	entries := f.dirEntries[:n:n]
	f.dirEntries = f.dirEntries[n:]
	return entries, nil
}
