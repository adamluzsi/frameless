package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"sync"
	"uuid"

	"go.llib.dev/frameless/adapter/localfs/internal/fileio"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/filesystem"
)

var (
	_ comproto.OnePhaseCommitProtocol = (*Repository[struct{}, string])(nil)
	_ comproto.InTx                   = (*Repository[struct{}, string])(nil)
)

// BeginTx begins a transaction, and returns its context.
// The operations which get the context of the transaction take part in it,
// until it ends with CommitTx or RollbackTx, or with a rollback when ctx is done.
//
// The isolation of the transactions is serialisable:
// at its first operation, a transaction locks Dir until it ends,
// so transactions and changes outside a transaction take turns through Locker.
// With the default local filesystem locker, this also coordinates processes on Linux, macOS and Windows.
// The reads from outside of a transaction don't wait for the lock;
// they see Dir as it was before the transaction, until its commit.
// As a commit moves the changes into place one file at a time,
// a read during the commit may see a part of the changes.
// Waiting for the lock stops when the context of the operation is done.
// Mind that a change from outside of a transaction waits for the transaction,
// even when the goroutine of the transaction makes it.
//
// The changes of a transaction are kept in hidden files of Dir, named after the entity file and the transaction:
// ".<entity file>.<transaction id>".
// If the process dies before the transaction ends, they are left behind,
// but they are hidden from the operations of the repository.
//
// BeginTx can be called again within a transaction, but such a nested transaction is part of the outer one:
// its changes are committed with the outer transaction, and its rollback rolls back the outer transaction too.
func (r *Repository[ENT, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return r.txm().BeginTx(ctx)
}

// CommitTx ends the transaction of the context, and moves its changes into place.
func (r *Repository[ENT, ID]) CommitTx(ctx context.Context) error {
	return r.txm().CommitTx(ctx)
}

// RollbackTx ends the transaction of the context, and discards its changes.
func (r *Repository[ENT, ID]) RollbackTx(ctx context.Context) error {
	return r.txm().RollbackTx(ctx)
}

// InTx reports whether the context has a transaction of the repository which hasn't ended yet.
func (r *Repository[ENT, ID]) InTx(ctx context.Context) bool {
	m := r.txm()
	if !m.InTx(ctx) {
		return false
	}
	tx, ok := m.LookupTx(ctx)
	if !ok || tx == nil {
		return false
	}
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	return tx.ended == nil && tx.check(ctx) == nil
}

func (r *Repository[ENT, ID]) txm() txkit.Manager[Repository[ENT, ID], fileTx, struct{}] {
	return txkit.Manager[Repository[ENT, ID], fileTx, struct{}]{
		R: r,
		Begin: func(ctx context.Context, r *Repository[ENT, ID]) (*fileTx, error) {
			suffix, err := r.fileSuffix()
			if err != nil {
				return nil, err
			}
			return beginTx(ctx, r.acquireFiles, suffix), nil
		},
		Commit:   func(ctx context.Context, tx *fileTx) error { return tx.commit(ctx) },
		Rollback: func(_ context.Context, tx *fileTx) error { return tx.rollback() },
	}
}

func (r *Repository[ENT, ID]) lookupTx(ctx context.Context) (transaction, bool) {
	tx, ok := r.txm().LookupTx(ctx)
	if !ok || tx == nil {
		return nil, false
	}
	return tx, true
}

// fileTx is a transaction of a Repository.
type fileTx struct {
	// id names the files of the transaction's changes.
	id      string
	acquire func(context.Context) (dirFiles, func() error, error)
	// suffix is what the names of the entity files end with, see Repository.fileSuffix.
	suffix string

	// mutex makes the operations of the transaction take turns,
	// as its context can be shared between goroutines.
	mutex sync.Mutex
	// stop stops the rollback of the transaction when the context of BeginTx is done.
	stop     func() bool
	stopLock func() bool
	ctx      context.Context
	lockErr  func() error
	// root is the directory, which is locked from the first operation of the transaction until its end.
	root       fileio.FileSystem
	releaseDir func() error
	// changes maps the name of each changed entity file to the name of the file which has its new content,
	// or to an empty name, if the entity file is deleted.
	changes map[string]string
	// ended is the error for using the transaction after it ended. It is nil while the transaction is on.
	ended error
	// abandoned tells that the transaction is rolled back, because the context of BeginTx is done.
	abandoned bool
}

func beginTx(ctx context.Context, acquire func(context.Context) (dirFiles, func() error, error), suffix string) *fileTx {
	tx := &fileTx{
		id:      uuid.NewV4().String(),
		ctx:     ctx,
		acquire: acquire,
		suffix:  suffix,
		changes: make(map[string]string),
	}
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	// Like a database/sql transaction, an abandoned transaction is rolled back when its context is done,
	// so it doesn't keep Dir locked.
	tx.stop = context.AfterFunc(ctx, func() { _ = tx.abandon(context.Cause(ctx)) })
	return tx
}

func (tx *fileTx) do(ctx context.Context, fn func(files) error) error {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.ended != nil {
		return tx.ended
	}
	if err := tx.check(ctx); err != nil {
		return err
	}
	if tx.root == nil {
		dir, release, err := tx.acquire(ctx)
		if err != nil {
			return err
		}
		tx.root, tx.releaseDir, tx.lockErr = dir.root, release, dir.lockErr
		tx.stopLock = context.AfterFunc(dir.lockCtx, func() { _ = tx.abandon(dir.lockErr()) })
	}
	check := func() error { return tx.check(ctx) }
	if err := check(); err != nil {
		return err
	}
	err := fn(txFiles{tx: tx, root: checkedFileSystem{FileSystem: tx.root, check: check}})
	if cause := tx.lockErr(); cause != nil {
		tx.abandoned = true
		return errors.Join(err, cause, tx.end(cause))
	}
	return errors.Join(err, check())
}

func (tx *fileTx) check(ctx context.Context) error {
	if tx.lockErr != nil {
		if err := tx.lockErr(); err != nil {
			return err
		}
	}
	return errors.Join(lockContextError(tx.ctx), lockContextError(ctx))
}

func (tx *fileTx) commit(ctx context.Context) error {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.ended != nil {
		return tx.ended
	}
	err := tx.check(ctx)
	if err == nil && tx.root != nil {
		err = tx.apply(ctx)
	}
	err = errors.Join(err, tx.check(ctx))
	return errors.Join(err, tx.end(fmt.Errorf("localfs: the transaction is finished: %w", errors.Join(txkit.ErrTxDone, err))))
}

// apply moves the changes into place, in the order of the entity files.
// On failure, the rest of the changes are not applied.
func (tx *fileTx) apply(ctx context.Context) error {
	root := checkedFileSystem{FileSystem: tx.root, check: func() error { return tx.check(ctx) }}
	names := slices.Sorted(maps.Keys(tx.changes))
	for i, name := range names {
		var err error
		if staged := tx.changes[name]; staged == "" {
			if err = root.Remove(name); errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		} else {
			err = root.Rename(staged, name)
		}
		if err != nil {
			return fmt.Errorf("localfs: the commit of transaction %s failed after %d of %d changes: %w", tx.id, i, len(names), err)
		}
		delete(tx.changes, name)
	}
	return nil
}

// rollback ends the transaction, and discards its changes.
func (tx *fileTx) rollback() error {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.ended != nil {
		if tx.abandoned {
			return nil // it is rolled back already
		}
		return tx.ended
	}
	return tx.end(fmt.Errorf("localfs: the transaction is rolled back: %w", txkit.ErrTxDone))
}

// abandon rolls back the transaction, as the context of BeginTx is done.
func (tx *fileTx) abandon(cause error) error {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.ended != nil {
		return nil
	}
	tx.abandoned = true
	return tx.end(fmt.Errorf("localfs: the transaction is rolled back, as its context is done: %w", cause))
}

// end ends the transaction for the given reason, and discards the changes which are not committed.
func (tx *fileTx) end(reason error) error {
	tx.ended = reason
	tx.stop()
	if tx.stopLock != nil {
		tx.stopLock()
	}
	if tx.root == nil {
		return nil // it had no operation, so it hasn't locked anything
	}
	return tx.release()
}

// release removes the files of the changes which are not committed, and unlocks the directory.
func (tx *fileTx) release() error {
	var errs []error
	for _, staged := range tx.changes {
		if staged == "" {
			continue
		}
		if err := tx.root.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	clear(tx.changes)
	errs = append(errs, tx.releaseDir())
	return errors.Join(errs...)
}

// txFiles are the entity files of a transaction: the files of the locked directory, with the changes of the transaction.
// They are used while the mutex of the transaction is held.
type txFiles struct {
	tx   *fileTx
	root fileio.FileSystem
}

// lookup returns the name of the file which has the content of the named entity file within the transaction.
func (f txFiles) lookup(op, name string) (string, error) {
	staged, changed := f.tx.changes[name]
	if !changed {
		return name, nil
	}
	if staged == "" {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	return staged, nil
}

func (f txFiles) read(name string) ([]byte, error) {
	file, err := f.lookup("open", name)
	if err != nil {
		return nil, err
	}
	return filesystem.ReadFile(f.root, file)
}

func (f txFiles) stat(name string) (fs.FileInfo, error) {
	file, err := f.lookup("stat", name)
	if err != nil {
		return nil, err
	}
	return f.root.Stat(file)
}

func (f txFiles) list() ([]string, error) {
	committed, err := fileio.ListFiles(f.root, f.tx.suffix)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range committed {
		if staged, changed := f.tx.changes[name]; !changed || staged != "" {
			names = append(names, name)
		}
	}
	for name, staged := range f.tx.changes {
		if _, found := slices.BinarySearch(committed, name); staged != "" && !found {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (f txFiles) create(name string, data []byte, perm fs.FileMode) error {
	entry, err := f.lookup("create", name)
	if err == nil {
		_, err = statEntry(f.root, entry)
	}
	if err == nil {
		return &fs.PathError{Op: "create", Path: name, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return f.stage(name, data, perm, false)
}

func (f txFiles) replace(name string, data []byte, perm fs.FileMode) error {
	return f.stage(name, data, perm, true)
}

func (f txFiles) remove(name string) error {
	if _, err := f.stat(name); err != nil {
		return err
	}
	if err := f.discard(name); err != nil {
		return err
	}
	f.tx.changes[name] = ""
	return nil
}

// stage writes the new content of the named entity file into the file of the transaction's change.
func (f txFiles) stage(name string, data []byte, perm fs.FileMode, exactPerm bool) error {
	if staged := f.tx.changes[name]; staged != "" {
		return fileio.ReplaceFile(f.root, staged, data, perm)
	}
	staged := "." + name + "." + f.tx.id
	if err := fileio.WriteNewFile(f.root, staged, data, perm, exactPerm); err != nil {
		return err
	}
	f.tx.changes[name] = staged
	return nil
}

// discard removes the change of the named entity file.
func (f txFiles) discard(name string) error {
	staged, changed := f.tx.changes[name]
	if !changed {
		return nil
	}
	if staged != "" {
		if err := f.root.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	delete(f.tx.changes, name)
	return nil
}
