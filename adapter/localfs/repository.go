package localfs

import (
	"context"
	"encoding"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"uuid"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/guard"
)

// RepositoryFileSystem is the file system a Repository keeps its entity files in.
// Repository writes each file under a temporary name, then renames it into place.
type RepositoryFileSystem interface {
	filesystem.FileOpener
	filesystem.FileInfoReader
	filesystem.DirectoryMaker
	filesystem.Remover
	filesystem.Renamer
}

// Repository is a CRUD repository, which keeps each entity encoded by its Codec in a file of its Dir, named after the entity's ID.
// The files are meant to be human-friendly, so they can be read, edited and versioned by hand.
//
// It suits a small number of entities, like settings, credentials or the state of a local tool:
//
//	settings := &localfs.Repository[Setting, SettingID]{FS: localfs.FileSystem{RootPath: root}, Dir: "settings", FileExt: "json"}
//
// Changes are serialised through Locker, including transactions (see BeginTx).
// File replacement is atomic only when FS.Rename is atomic.
//
// Repository is ready to use once its Dir is set. Use it through a pointer;
// do not copy it or change its configuration after first use.
type Repository[ENT, ID any] struct {
	// FS [optional] stores the entity files.
	//
	// Default: FileSystem{}, for backwards-compatible local OS access.
	FS RepositoryFileSystem
	// Locker [optional] serialises changes, including the existence check of Create.
	// All writers sharing FS and Dir must participate in the same locking protocol.
	// Re-entry returning the supplied context unchanged borrows caller ownership:
	// the caller must keep it alive until all operations/transactions finish and
	// serialize its use across repositories. Calls on this repository are serialized
	// locally as well. Decorators must preserve that re-entry identity and context values.
	// A native Locker with matching FS and Dir shares its pinned root with Repository.
	// Lock-loss checks cannot fence a filesystem call already in flight.
	//
	// Default: a localfs.Locker for FileSystem (value or pointer), otherwise a
	// per-repository memory.NewLocker. Share an explicit Locker between repositories
	// using the same non-local filesystem and directory.
	Locker guard.Locker
	// Dir is the directory of the entity files, resolved within FS.
	// It is created when it doesn't exist yet.
	Dir string
	// FileMode [optional] is the permission of the entity files.
	// Dir gets the same permission, plus the right to list it for whoever can read the files.
	//
	// default: 0600, so only the owner can read the files, which suits credentials as well.
	FileMode fs.FileMode
	// IDA [optional] is the ID Accessor that maps the ID to the ENT field.
	IDA extid.Accessor[ENT, ID]
	// MakeID [optional] makes the ID for an entity which is created without one.
	//
	// default: a UUID v4, when ID is a string type.
	MakeID func(context.Context) (ID, error)
	// Codec [optional] encodes the entities into the content of their files, and decodes them back.
	//
	// default: jsonkit.Codec, with its output indented, so the files are easy to read and edit by hand.
	Codec codec.Codec
	// FileExt [optional] is the extension of the entity files, like "json" for the default Codec.
	// Only the files with the extension are taken as entities, so Dir can have other files as well.
	// Its leading dot is optional, so ".json" is the same as "json".
	//
	// default: no extension, so the files are named after the IDs only,
	// and every file of Dir is taken as an entity, except for the hidden ones.
	FileExt string

	initLocker    sync.Once
	defaultLocker guard.Locker
	writers       dirLock
}

var (
	_ crud.Creator[struct{}]            = (*Repository[struct{}, string])(nil)
	_ crud.Saver[struct{}]              = (*Repository[struct{}, string])(nil)
	_ crud.ByIDFinder[struct{}, string] = (*Repository[struct{}, string])(nil)
	_ crud.AllFinder[struct{}]          = (*Repository[struct{}, string])(nil)
	_ crud.Updater[struct{}]            = (*Repository[struct{}, string])(nil)
	_ crud.ByIDDeleter[string]          = (*Repository[struct{}, string])(nil)
	_ crud.AllDeleter                   = (*Repository[struct{}, string])(nil)
)

func (r *Repository[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	if ptr == nil {
		return fmt.Errorf("localfs: nil %T given to %T#Create", ptr, r)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ent := *ptr
	id, err := r.lookupID(ent)
	if err != nil {
		return err
	}
	if zerokit.IsZero(id) {
		if id, err = r.makeID(ctx); err != nil {
			return err
		}
		if err := r.IDA.Set(&ent, id); err != nil {
			return err
		}
	}
	data, err := r.marshal(ent)
	if err != nil {
		return err
	}
	name, err := r.fileName(id)
	if err != nil {
		return err
	}
	err = r.write(ctx, func(dir files) error {
		if err := dir.create(name, data, r.fileMode()); errors.Is(err, fs.ErrExist) {
			return crud.ErrAlreadyExists.F("localfs: %T already exists with id: %v", ent, id)
		} else {
			return err
		}
	})
	if err != nil {
		return err
	}
	*ptr = ent
	return nil
}

// Save creates the entity, or when it already exists, then it updates it.
func (r *Repository[ENT, ID]) Save(ctx context.Context, ptr *ENT) error {
	if ptr == nil {
		return fmt.Errorf("localfs: nil %T given to %T#Save", ptr, r)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := r.lookupID(*ptr)
	if err != nil {
		return err
	}
	if zerokit.IsZero(id) {
		return r.Create(ctx, ptr)
	}
	data, err := r.marshal(*ptr)
	if err != nil {
		return err
	}
	name, err := r.fileName(id)
	if err != nil {
		return err
	}
	return r.write(ctx, func(dir files) error {
		perm := r.fileMode()
		if info, err := dir.stat(name); err == nil {
			perm = info.Mode().Perm()
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return dir.replace(name, data, perm)
	})
}

func (r *Repository[ENT, ID]) FindByID(ctx context.Context, id ID) (ENT, bool, error) {
	var zero ENT
	if err := ctx.Err(); err != nil {
		return zero, false, err
	}
	if zerokit.IsZero(id) {
		return zero, false, nil
	}
	name, err := r.fileName(id)
	if err != nil {
		return zero, false, err
	}
	dir, closeDir, err := r.view(ctx)
	if err != nil {
		return zero, false, err
	}
	defer closeDir()
	ent, err := r.read(dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	return ent, true, nil
}

// FindAll returns the entities in the order of their file names.
func (r *Repository[ENT, ID]) FindAll(ctx context.Context) iterkit.SeqE[ENT] {
	return iterkit.From(func(yield func(ENT) bool) error {
		dir, closeDir, err := r.view(ctx)
		if err != nil {
			return err
		}
		defer closeDir()
		names, err := dir.list()
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			ent, err := r.read(dir, name)
			if errors.Is(err, fs.ErrNotExist) {
				continue // deleted since the directory was listed
			}
			if err != nil {
				return err
			}
			if !yield(ent) {
				return nil
			}
		}
		return nil
	})
}

func (r *Repository[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	if ptr == nil {
		return fmt.Errorf("localfs: nil %T given to %T#Update", ptr, r)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := r.lookupID(*ptr)
	if err != nil {
		return err
	}
	if zerokit.IsZero(id) {
		return r.errNotFound(id)
	}
	data, err := r.marshal(*ptr)
	if err != nil {
		return err
	}
	name, err := r.fileName(id)
	if err != nil {
		return err
	}
	return r.write(ctx, func(dir files) error {
		info, err := dir.stat(name)
		if errors.Is(err, fs.ErrNotExist) {
			return r.errNotFound(id)
		}
		if err != nil {
			return err
		}
		return dir.replace(name, data, info.Mode().Perm())
	})
}

func (r *Repository[ENT, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if zerokit.IsZero(id) {
		return r.errNotFound(id)
	}
	name, err := r.fileName(id)
	if err != nil {
		return err
	}
	return r.write(ctx, func(dir files) error {
		if err := dir.remove(name); errors.Is(err, fs.ErrNotExist) {
			return r.errNotFound(id)
		} else {
			return err
		}
	})
}

func (r *Repository[ENT, ID]) DeleteAll(ctx context.Context) error {
	return r.write(ctx, func(dir files) error {
		names, err := dir.list()
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := dir.remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		return nil
	})
}

// fileReader reads the entity files of Dir.
type fileReader interface {
	// read returns the content of the named file.
	read(name string) ([]byte, error)
	// list returns the names of the entity files, in order.
	list() ([]string, error)
}

// files are the entity files of Dir, as an operation sees them:
// outside of a transaction, they are the files of Dir, and within a transaction, they include its changes.
type files interface {
	fileReader
	stat(name string) (fs.FileInfo, error)
	// create writes a new file, which is subject to the umask. It fails with fs.ErrExist if the file already exists.
	create(name string, data []byte, perm fs.FileMode) error
	// replace writes a file with the exact perm permission, replacing the file when it already exists.
	replace(name string, data []byte, perm fs.FileMode) error
	remove(name string) error
}

// transaction is a transaction of a Repository (see tx.go).
type transaction interface {
	// do runs fn with the files as the transaction sees them.
	// The operations of a transaction take turns, so the check and the change of an operation are not interleaved.
	do(ctx context.Context, fn func(files) error) error
}

// view returns the entity files for reading.
// Outside of a transaction, it doesn't wait for the lock of Dir.
// The returned function closes the view.
func (r *Repository[ENT, ID]) view(ctx context.Context) (fileReader, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if tx, ok := r.lookupTx(ctx); ok {
		return txReader{ctx: ctx, tx: tx}, func() error { return nil }, nil
	}
	dir, err := r.openFiles()
	if err != nil {
		return nil, nil, err
	}
	return dir, dir.close, nil
}

// write runs fn with the entity files, while Dir is locked.
func (r *Repository[ENT, ID]) write(ctx context.Context, fn func(files) error) (rErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := r.lookupTx(ctx); ok {
		return tx.do(ctx, fn)
	}
	dir, release, err := r.acquireFiles(ctx)
	if err != nil {
		return err
	}
	defer func() { rErr = errors.Join(rErr, release()) }()
	check := func() error { return errors.Join(dir.lockErr(), lockContextError(ctx)) }
	if err := check(); err != nil {
		return err
	}
	dir.root = checkedFileSystem{FileSystem: dir.root, check: check}
	return errors.Join(fn(dir), check())
}

// openFiles opens a read view without holding the writer lock.
func (r *Repository[ENT, ID]) openFiles() (dirFiles, error) {
	if err := r.prepareDir(); err != nil {
		return dirFiles{}, err
	}
	return r.bindFiles(nil)
}

func (r *Repository[ENT, ID]) prepareDir() error {
	if r.Dir == "" {
		return errors.New("localfs: the Dir of the repository is not set")
	}
	if _, err := r.fileSuffix(); err != nil {
		return err
	}
	return filesystem.MkdirAll(r.fileSystem(), r.Dir, fileio.DirPerm(r.fileMode()))
}

func (r *Repository[ENT, ID]) bindFiles(lockCtx context.Context) (dirFiles, error) {
	suffix, err := r.fileSuffix()
	if err != nil {
		return dirFiles{}, err
	}
	root, closeRoot, err := openRepositoryDir(r.fileSystem(), r.Dir, lockCtx)
	if err != nil {
		return dirFiles{}, err
	}
	return dirFiles{root: root, close: closeRoot, suffix: suffix}, nil
}

func (r *Repository[ENT, ID]) fileSystem() RepositoryFileSystem {
	if r.FS == nil {
		return FileSystem{}
	}
	return r.FS
}

func (r *Repository[ENT, ID]) locker() guard.Locker {
	if r.Locker != nil {
		return r.Locker
	}
	r.initLocker.Do(func() {
		if fsys, ok := localFileSystem(r.fileSystem()); ok {
			r.defaultLocker = &Locker{FS: fsys, Dir: r.Dir}
		} else {
			r.defaultLocker = memory.NewLocker()
		}
	})
	return r.defaultLocker
}

// acquireFiles keeps a writer's lock until all file operations and cleanup finish.
func (r *Repository[ENT, ID]) acquireFiles(ctx context.Context) (dirFiles, func() error, error) {
	if err := lockContextError(ctx); err != nil {
		return dirFiles{}, nil, err
	}
	if err := r.prepareDir(); err != nil {
		return dirFiles{}, nil, err
	}
	lockCtx, unlock, err := acquireRepositoryLock(ctx, r.locker())
	if err != nil {
		return dirFiles{}, nil, err
	}
	// Acquire the local turn AFTER ownership: otherwise an independent waiter
	// could block the owner trying to re-enter. Keep this turn through cleanup.
	releaseTurn, err := r.writers.take(ctx)
	if err != nil {
		return dirFiles{}, nil, errors.Join(err, unlock())
	}
	release := func() error {
		defer releaseTurn()
		return unlock()
	}
	dir, err := r.bindFiles(lockCtx)
	if err != nil {
		return dirFiles{}, nil, errors.Join(err, release())
	}
	dir.lockCtx = lockCtx
	dir.lockErr = func() error { return lockContextError(lockCtx) }
	if local, ok := localFileSystem(r.fileSystem()); ok {
		if state := repositoryDirectoryLock(lockCtx, local, r.Dir); state != nil {
			dir.lockErr = func() error { return errors.Join(lockContextError(lockCtx), state.lockError()) }
		}
	}
	return dir, func() error { return errors.Join(dir.close(), release()) }, nil
}

// fileName returns the name of the entity's file.
func (r *Repository[ENT, ID]) fileName(id ID) (string, error) {
	text, err := formatID(id)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("localfs: the id of %T is formatted as an empty text: %#v", *new(ENT), id)
	}
	suffix, err := r.fileSuffix()
	if err != nil {
		return "", err
	}
	return fileio.EscapeFileName(text, suffix), nil
}

// fileSuffix returns what the names of the entity files end with: the dot and the FileExt, or nothing without a FileExt.
func (r *Repository[ENT, ID]) fileSuffix() (string, error) {
	ext := strings.TrimPrefix(r.FileExt, ".")
	if ext == "" {
		return "", nil
	}
	for i := range len(ext) {
		if !fileio.IsFileNameSafe(ext[i]) {
			return "", fmt.Errorf("localfs: the FileExt of %T has a character which is not safe in a file name: %q", r, r.FileExt)
		}
	}
	return "." + ext, nil
}

func (r *Repository[ENT, ID]) lookupID(ent ENT) (ID, error) {
	id, ok := r.IDA.Lookup(ent)
	if !ok {
		return id, fmt.Errorf("localfs: %T has no ID field, which could be marked with the `ext:\"id\"` tag", ent)
	}
	return id, nil
}

func (r *Repository[ENT, ID]) makeID(ctx context.Context) (ID, error) {
	if r.MakeID != nil {
		return r.MakeID(ctx)
	}
	var id ID
	v := reflect.ValueOf(&id).Elem()
	if v.Kind() != reflect.String {
		return id, fmt.Errorf("localfs: %T needs a MakeID to make an ID of type %T", r, id)
	}
	v.SetString(uuid.NewV4().String())
	return id, nil
}

func (r *Repository[ENT, ID]) codec() codec.Codec {
	if r.Codec == nil {
		return indentedJSON{}
	}
	return r.Codec
}

func (r *Repository[ENT, ID]) marshal(ent ENT) ([]byte, error) {
	data, err := r.codec().Marshal(ent)
	if err != nil {
		return nil, fmt.Errorf("localfs: failed to encode %T: %w", ent, err)
	}
	return data, nil
}

// read returns the entity of the named file.
func (r *Repository[ENT, ID]) read(dir fileReader, name string) (ENT, error) {
	var ent ENT
	data, err := dir.read(name)
	if err != nil {
		return ent, err
	}
	if err := r.codec().Unmarshal(data, &ent); err != nil {
		return ent, fmt.Errorf("localfs: failed to decode %T from %s: %w", ent, name, err)
	}
	return ent, nil
}

func (r *Repository[ENT, ID]) fileMode() fs.FileMode {
	if r.FileMode == 0 {
		return 0o600
	}
	return r.FileMode.Perm()
}

func (r *Repository[ENT, ID]) errNotFound(id ID) error {
	return crud.ErrNotFound.F("localfs: %T not found with id: %v", *new(ENT), id)
}

// formatID returns the ID as text, preferring its encoding.TextMarshaler implementation,
// so an ID type like uuid.UUID has the same text as in its JSON.
func formatID[ID any](id ID) (string, error) {
	if m, ok := any(id).(encoding.TextMarshaler); ok {
		text, err := m.MarshalText()
		return string(text), err
	}
	if v := reflect.ValueOf(id); v.Kind() == reflect.String {
		return v.String(), nil
	}
	return fmt.Sprint(id), nil
}

// indentedJSON is the default Codec of the Repository:
// the jsonkit.Codec, with its output indented and ended with a newline, like a file edited by hand.
// Its strings are reformatted without escaping the HTML characters, like <, > and &,
// which jsonkit.Codec escapes like encoding/json does by default.
type indentedJSON struct{ jsonkit.Codec }

func (c indentedJSON) Marshal(v any) ([]byte, error) {
	data, err := c.Codec.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := jsontext.Value(data)
	err = out.Indent(jsontext.WithIndent("  "), jsontext.PreserveRawStrings(false), jsontext.EscapeForHTML(false))
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// dirFiles are the entity files of a directory, whose names end with the suffix.
type dirFiles struct {
	root    fileio.FileSystem
	close   func() error
	suffix  string
	lockCtx context.Context
	lockErr func() error
}

func (d dirFiles) read(name string) ([]byte, error) { return filesystem.ReadFile(d.root, name) }

func (d dirFiles) stat(name string) (fs.FileInfo, error) { return d.root.Stat(name) }

func (d dirFiles) list() ([]string, error) { return fileio.ListFiles(d.root, d.suffix) }

func (d dirFiles) create(name string, data []byte, perm fs.FileMode) error {
	return fileio.CreateFile(d.root, name, data, perm)
}

func (d dirFiles) replace(name string, data []byte, perm fs.FileMode) error {
	return fileio.ReplaceFile(d.root, name, data, perm)
}

func (d dirFiles) remove(name string) error { return d.root.Remove(name) }

// txReader reads the files of a transaction, taking turns with the other operations of the transaction at each read.
type txReader struct {
	ctx context.Context
	tx  transaction
}

func (r txReader) read(name string) (data []byte, err error) {
	err = r.tx.do(r.ctx, func(dir files) error {
		data, err = dir.read(name)
		return err
	})
	return data, err
}

func (r txReader) list() (names []string, err error) {
	err = r.tx.do(r.ctx, func(dir files) error {
		names, err = dir.list()
		return err
	})
	return names, err
}
