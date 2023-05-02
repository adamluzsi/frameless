package memory

import (
	"github.com/adamluzsi/frameless/ports/filesystem"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/adamluzsi/frameless/pkg/iokit"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type FileSystem struct {
	entries  map[string]*fileSystemEntry
	mutex    sync.RWMutex
	initOnce sync.Once
}

func (mfs *FileSystem) init() {
	mfs.initOnce.Do(func() {
		mfs.entries = make(map[string]*fileSystemEntry)
	})
}

func (mfs *FileSystem) rlock() func() {
	mfs.init()
	//mfs.mutex.RLock()
	//return mfs.mutex.RUnlock
	return func() {}
}

func (mfs *FileSystem) wlock() func() {
	mfs.init()
	//mfs.mutex.Lock()
	//return mfs.mutex.Unlock
	return func() {}
}

func (mfs *FileSystem) path(name string) string {
	abs, err := filepath.Abs(name)
	if err != nil {
		return name
	}
	return abs
}

func (mfs *FileSystem) isRoot(path string) bool {
	return path == mfs.path(".")
}

func (mfs *FileSystem) Stat(name string) (fs.FileInfo, error) {
	defer mfs.rlock()()
	path := mfs.path(name)
	entry, ok := mfs.entries[path]
	if !ok && mfs.isRoot(path) {
		entry, ok = mfs.rootEntry(path), true
	}
	if !ok {
		return nil, &fs.PathError{
			Op:   "stat",
			Path: path,
			Err:  os.ErrNotExist,
		}
	}
	return filesystem.FileInfo{
		Path:        entry.path,
		FileSize:    int64(len(entry.data)),
		FileMode:    entry.mode,
		ModifiedAt:  entry.modeTime,
		IsDirectory: entry.isDir,
	}, nil
}

func (mfs *FileSystem) OpenFile(name string, flag int, perm fs.FileMode) (filesystem.File, error) {
	path := mfs.path(name)
	if flag&os.O_CREATE != 0 {
		defer mfs.wlock()()
	} else {
		defer mfs.rlock()()
	}
	f, ok := mfs.entries[path]
	if ok && flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0 {
		return nil, &fs.PathError{
			Op:   "open",
			Path: path,
			Err:  os.ErrExist,
		}
	}
	if !ok && flag&os.O_CREATE != 0 {
		f, ok = &fileSystemEntry{
			path:     path,
			mode:     perm,
			modeTime: time.Now().UTC(),
			isDir:    false,
		}, true
		mfs.entries[path] = f
	}
	if !ok && mfs.isRoot(path) {
		f, ok = mfs.rootEntry(path), true
	}
	if !ok {
		return nil, &fs.PathError{
			Op:   "open",
			Path: path,
			Err:  os.ErrNotExist,
		}
	}
	if flag&os.O_TRUNC != 0 {
		f.data = []byte{}
	}
	return &FileSystemFile{
		entry:      f,
		openFlag:   flag,
		buffer:     iokit.NewBuffer(f.data),
		dirEntries: iterators.Slice(mfs.getDirEntriesFn(path)),
	}, nil
}

func (mfs *FileSystem) rootEntry(path string) *fileSystemEntry {
	return &fileSystemEntry{
		path:     path,
		mode:     0777,
		modeTime: time.Now().UTC(),
		isDir:    true,
	}
}

func (mfs *FileSystem) getDirEntriesFn(dirPath string) []fs.DirEntry {
	var des []fs.DirEntry
	for path, entry := range mfs.entries {
		dp := filepath.Dir(path)
		if dp != dirPath {
			continue
		}
		des = append(des, filesystem.DirEntry{FileInfo: entry.fileInfo()})
	}
	return des
}

func (mfs *FileSystem) Mkdir(name string, perm fs.FileMode) error {
	defer mfs.wlock()()
	path := mfs.path(name)
	if _, ok := mfs.entries[path]; ok {
		return &fs.PathError{
			Op:   "mkdir",
			Path: path,
			Err:  os.ErrExist,
		}
	}
	mfs.entries[path] = &fileSystemEntry{
		path:     path,
		mode:     perm | fs.ModeDir,
		modeTime: time.Now().UTC(),
		isDir:    true,
	}
	return nil
}

func (mfs *FileSystem) Remove(name string) error {
	defer mfs.wlock()()
	path := mfs.path(name)
	f, ok := mfs.entries[path]
	if !ok {
		return &fs.PathError{
			Op:   "remove",
			Path: path,
			Err:  os.ErrNotExist,
		}
	}
	if f.isDir && 0 < len(mfs.getDirEntriesFn(path)) {
		return &fs.PathError{
			Op:   "remove",
			Path: path,
			Err:  syscall.ENOTEMPTY,
		}
	}
	delete(mfs.entries, path)
	return nil
}

type fileSystemEntry struct {
	path     string
	mode     fs.FileMode
	modeTime time.Time
	isDir    bool
	data     []byte
	mutex    sync.Mutex
}

func (entry *fileSystemEntry) fileInfo() filesystem.FileInfo {
	return filesystem.FileInfo{
		Path:        entry.path,
		FileSize:    int64(len(entry.data)),
		FileMode:    entry.mode,
		ModifiedAt:  entry.modeTime,
		IsDirectory: entry.isDir,
		System:      nil,
	}
}

type FileSystemFile struct {
	entry    *fileSystemEntry
	openFlag int
	buffer   *iokit.Buffer
	mutex    sync.Mutex

	dirEntries iterators.Iterator[fs.DirEntry]
}

func (f *FileSystemFile) fileWriteLock() func() {
	f.mutex.Lock()
	return f.mutex.Unlock
}

func (f *FileSystemFile) Close() error {
	defer f.fileWriteLock()()
	if err := f.Sync(); err != nil {
		return err
	}
	return f.buffer.Close()
}

func (f *FileSystemFile) Sync() error {
	f.entry.mutex.Lock()
	defer f.entry.mutex.Unlock()
	f.entry.data = f.buffer.Bytes()
	f.entry.modeTime = time.Now().UTC()
	return nil
}

func (f *FileSystemFile) Stat() (fs.FileInfo, error) {
	defer f.fileWriteLock()()
	return filesystem.FileInfo{
		Path:        f.entry.path,
		FileSize:    int64(len(f.buffer.Bytes())),
		FileMode:    f.entry.mode,
		ModifiedAt:  f.entry.modeTime,
		IsDirectory: f.entry.isDir,
		System:      nil,
	}, nil
}

func (f *FileSystemFile) Read(bytes []byte) (int, error) {
	if !filesystem.HasOpenFlagRead(f.openFlag) {
		return 0, &fs.PathError{
			Op:   "read",
			Path: f.entry.path,
			Err:  os.ErrPermission,
		}
	}
	defer f.fileWriteLock()()
	return f.buffer.Read(bytes)
}

func (f *FileSystemFile) Write(p []byte) (n int, err error) {
	if !filesystem.HasOpenFlagWrite(f.openFlag) {
		return 0, &fs.PathError{
			Op:   "write",
			Path: f.entry.path,
			Err:  os.ErrPermission,
		}
	}
	defer f.Sync()
	defer f.fileWriteLock()()
	if f.openFlag&os.O_APPEND != 0 {
		if _, err := f.buffer.Seek(0, io.SeekEnd); err != nil {
			return 0, err
		}
	}
	return f.buffer.Write(p)
}

func (f *FileSystemFile) Seek(offset int64, whence int) (int64, error) {
	defer f.fileWriteLock()()
	return f.buffer.Seek(offset, whence)
}

func (f *FileSystemFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.entry.isDir {
		return nil, &fs.PathError{
			Op:   "fdopendir",
			Path: f.entry.path,
			Err:  syscall.ENOTDIR,
		}
	}
	defer f.fileWriteLock()()
	if n < 0 {
		return iterators.Collect(f.dirEntries)
	}
	if n == 0 {
		return []fs.DirEntry{}, nil
	}
	var des []fs.DirEntry
	for i := 0; i < n; i++ {
		if !f.dirEntries.Next() {
			break
		}
		des = append(des, f.dirEntries.Value())
	}
	if err := f.dirEntries.Err(); err != nil {
		return nil, err
	}
	if len(des) == 0 {
		return nil, io.EOF
	}
	return des, nil
}
