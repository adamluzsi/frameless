//go:build windows

package localfs

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockFileName is the file of the directory which is locked on Windows, as a directory can't be locked there.
// It is a hidden file, so it is not taken as an entity file.
//
// It is never removed, as a process which waits for its lock would keep waiting on the removed file,
// while another process locks a new one, and they would both hold the lock.
const lockFileName = ".lock"

// lockDir locks the directory of root with LockFileEx on its lock file, see lockFileName,
// so the processes which change the directory take turns.
// The lock is polled, so waiting for it can stop when ctx is done.
func lockDir(ctx context.Context, root *os.Root) (unlock func() error, _ error) {
	file, err := root.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("localfs: locking %s: %w", root.Name(), err)
	}
	err = pollLock(ctx, func() (bool, error) {
		err := lockFile(file, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY)
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("localfs: locking %s: %w", root.Name(), err)
		}
		return false, nil
	})
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return func() error {
		return errors.Join(unlockFile(file), file.Close())
	}, nil
}

// lockFile locks the first byte of the file, which is enough, as every process locks the same byte.
func lockFile(file *os.File, flags uint32) error {
	return controlHandle(file, func(h windows.Handle) error {
		return windows.LockFileEx(h, flags, 0, 1, 0, new(windows.Overlapped))
	})
}

func unlockFile(file *os.File) error {
	return controlHandle(file, func(h windows.Handle) error {
		return windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
	})
}

// controlHandle calls fn with the handle of the file, which stays valid until fn returns.
func controlHandle(file *os.File, fn func(windows.Handle) error) error {
	conn, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var fnErr error
	if err := conn.Control(func(fd uintptr) { fnErr = fn(windows.Handle(fd)) }); err != nil {
		return err
	}
	return fnErr
}
