package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

// FileEvent is a change of one file watched by a Watch.
type FileEvent struct {
	// Path is the slash-separated path of the file in the FS of the Watch,
	// so the same FS can open it.
	Path string
	// Name is the file name of Path, without its directory.
	Name string
	// Change is what happened to the file.
	Change FileChange
}

// FileChange tells what happened to the file of a FileEvent.
type FileChange string

const (
	// FileChangeCreated means the file appeared since the watch last looked at its directory.
	FileChangeCreated FileChange = "created"
	// FileChangeWritten means the size or the modification time of the file changed
	// since the watch last looked at its directory.
	FileChangeWritten FileChange = "written"
	// FileChangeRemoved means the file is gone since the watch last looked at its directory,
	// removed or renamed away.
	FileChangeRemoved FileChange = "removed"
)

// Watch is a volatile broadcast of the changes of a directory's files, including the changes
// made outside of the process which watches it: by hand in an editor, or by another process.
//
// It is how a component which lives by the files of a directory, like a Repository,
// can notice the changes it didn't make itself, like an entity file edited by hand.
//
// A Watch only subscribes: what it announces comes from the directory, so there is nothing
// to publish to it. Change the files of the directory instead.
//
//	watch := localfs.Watch{FS: os.DirFS(settingsDir), FileExt: "json"}
//	for msg, err := range watch.Subscribe(ctx) {
//		if err != nil {
//			return err
//		}
//		reload(msg.Data().Name)
//	}
//
// Every subscription polls the directory on its own. Subscribe looks at the directory
// before it returns, and the subscription then looks again at every Interval,
// and yields what changed between the two looks. So every subscription receives
// the changes made after its Subscribe, and nothing from before it.
//
// A file counts as changed when its size or its modification time is not what
// the last look found. Polling coalesces by nature: whatever happens to a file between
// two looks is one change, so a burst of writes is announced once, and a subscription
// which lags behind misses nothing but the steps in between.
// A file which is created and removed between two looks is never announced, though,
// and on a file system with coarse modification times (FAT, HFS+, some network mounts),
// a rewrite which keeps the size of the file can go unnoticed.
//
// Only the regular files directly in Dir are watched, which suits the flat one-file-per-entity
// layout of Repository; a subdirectory is none of a Watch's business. Symbolic links
// are followed, so a link is watched as the file its readers get.
// Hidden files (with a leading dot) are never announced, so the temporary files of
// atomic writes, like the ones of every Repository write, don't disturb.
// With a FileExt, only the files carrying it are announced, the same way Repository
// takes only those as entities.
//
// A missing Dir is watched as an empty one, so its files are announced as created
// when it appears, and as removed when it goes away. A Dir which can't be read
// for another reason, like its permissions, is logged and retried at the next Interval,
// while the subscription keeps what it knew of it: a failed look never announces
// the files as removed.
//
// A Watch keeps no state of its own, so it can be copied, and shared by any number of subscriptions.
type Watch struct {
	// FS is the file system the watched directory is in,
	// like os.DirFS(dir), or a FileSystem.
	FS fs.FS
	// Dir [optional] is the slash-separated path of the watched directory in FS,
	// in the format of fs.ValidPath.
	//
	// Default: ".", the root of FS
	Dir string
	// FileExt [optional] is the extension of the interesting files, like "json" or "md".
	// Its leading dot is optional, so ".json" is the same as "json".
	// Without it every visible file of Dir is interesting.
	FileExt string
	// Interval [optional] is how often a subscription looks at Dir.
	//
	// Default: 1 second
	Interval time.Duration
}

var _ pubsub.Subscriber[FileEvent] = Watch{}

const defaultWatchInterval = time.Second

// Subscribe receives the changes of Dir from now on, until ctx is done
// or the consumer stops iterating; ACK and NACK of its messages have nothing to do.
//
// Subscribe looks at Dir before it returns, so a change made after it is received,
// even if the subscription is consumed only later.
// A Watch which is not configured well yields its error, and ends the subscription.
func (w Watch) Subscribe(ctx context.Context) pubsub.Subscription[FileEvent] {
	c, err := w.config()
	if err != nil {
		return func(yield func(pubsub.Message[FileEvent], error) bool) { yield(nil, err) }
	}
	if ctx.Err() != nil {
		return func(yield func(pubsub.Message[FileEvent], error) bool) {}
	}
	var baseline = c.look(ctx, watchSnapshot{})
	return func(yield func(pubsub.Message[FileEvent], error) bool) {
		var last = baseline
		for {
			select {
			case <-ctx.Done():
				return
			case <-clock.After(c.interval):
			}
			var next = c.look(ctx, last)
			for _, ev := range c.changes(last, next) {
				if ctx.Err() != nil {
					return
				}
				if !yield(pubsub.MakeMessage(ctx, ev, nil, nil), nil) {
					return
				}
			}
			last = next
		}
	}
}

// watchConfig is the configuration of a Watch, with its defaults applied.
type watchConfig struct {
	fsys     fs.FS
	dir      string
	suffix   string
	interval time.Duration
}

func (w Watch) config() (watchConfig, error) {
	if w.FS == nil {
		return watchConfig{}, errors.New("localfs: the FS of the Watch is not set")
	}
	var dir = w.Dir
	if dir == "" {
		dir = "."
	}
	if !fs.ValidPath(dir) {
		return watchConfig{}, fmt.Errorf("localfs: the Dir of the Watch is not a valid fs.FS path: %q", w.Dir)
	}
	suffix, err := watchFileSuffix(w.FileExt)
	if err != nil {
		return watchConfig{}, err
	}
	var interval = w.Interval
	if interval <= 0 {
		interval = defaultWatchInterval
	}
	return watchConfig{fsys: w.FS, dir: dir, suffix: suffix, interval: interval}, nil
}

// watchSnapshot is what a look found in the watched directory.
type watchSnapshot struct {
	// files are the interesting files of the directory, by name.
	files map[string]watchStamp
	// err is why the look failed; files are then what the look before it found.
	err error
}

// watchStamp is what tells whether a file changed between two looks.
type watchStamp struct {
	size    int64
	modTime time.Time
}

func (s watchStamp) equal(oth watchStamp) bool {
	return s.size == oth.size && s.modTime.Equal(oth.modTime)
}

// look reads the interesting files of the directory.
//
// A directory which can't be read says nothing about its files, so the files of prev
// are kept until it can be read again, and only the start of the trouble is logged,
// not every look.
func (c watchConfig) look(ctx context.Context, prev watchSnapshot) watchSnapshot {
	entries, err := fs.ReadDir(c.fsys, c.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return watchSnapshot{}
	}
	if err != nil {
		if prev.err == nil {
			logger.Warn(ctx, "localfs: the Watch can't read its directory, and keeps trying",
				logging.Field("dir", c.dir), logging.ErrField(err))
		}
		return watchSnapshot{files: prev.files, err: err}
	}
	var files = make(map[string]watchStamp, len(entries))
	for _, entry := range entries {
		var name = entry.Name()
		if entry.IsDir() || !watchInteresting(name, c.suffix) {
			continue
		}
		info, err := fs.Stat(c.fsys, path.Join(c.dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed since the directory was read, or a dangling link
		}
		if err != nil {
			if stamp, ok := prev.files[name]; ok {
				files[name] = stamp
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files[name] = watchStamp{size: info.Size(), modTime: info.ModTime()}
	}
	return watchSnapshot{files: files}
}

// changes are the file changes between two looks, in the order of the file names.
func (c watchConfig) changes(last, next watchSnapshot) []FileEvent {
	var events []FileEvent
	for name, stamp := range next.files {
		old, ok := last.files[name]
		switch {
		case !ok:
			events = append(events, c.event(name, FileChangeCreated))
		case !old.equal(stamp):
			events = append(events, c.event(name, FileChangeWritten))
		}
	}
	for name := range last.files {
		if _, ok := next.files[name]; !ok {
			events = append(events, c.event(name, FileChangeRemoved))
		}
	}
	slices.SortFunc(events, func(a, b FileEvent) int { return strings.Compare(a.Name, b.Name) })
	return events
}

func (c watchConfig) event(name string, op FileChange) FileEvent {
	return FileEvent{Path: path.Join(c.dir, name), Name: name, Change: op}
}

// watchInteresting reports whether a file name is one the watch announces:
// never a hidden file, and with a suffix only the files carrying it.
func watchInteresting(name, suffix string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	if suffix == "" {
		return true
	}
	return strings.HasSuffix(name, suffix)
}

// watchFileSuffix turns a FileExt into the suffix the interesting names end with,
// the same rule as Repository#fileSuffix, without a repository in hand.
func watchFileSuffix(ext string) (string, error) {
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		return "", nil
	}
	for i := range len(ext) {
		if !fileio.IsFileNameSafe(ext[i]) {
			return "", fmt.Errorf("localfs: the FileExt of the Watch has a character which is not safe in a file name: %q", ext)
		}
	}
	return "." + ext, nil
}
