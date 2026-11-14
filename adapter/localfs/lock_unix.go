//go:build darwin || linux

package localfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockDir locks the directory of root with flock(2), so the processes which change the directory take turns.
// The lock is polled, so waiting for it can stop when ctx is done.
func lockDir(ctx context.Context, root *os.Root) (unlock func() error, _ error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	err = pollLock(ctx, func() (bool, error) {
		err := flock(dir, syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("localfs: locking %s: %w", root.Name(), err)
		}
		return false, nil
	})
	if err != nil {
		return nil, errors.Join(err, dir.Close())
	}
	return func() error {
		return errors.Join(flock(dir, syscall.LOCK_UN), dir.Close())
	}, nil
}

func flock(file *os.File, how int) error {
	conn, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var flockErr error
	err = conn.Control(func(fd uintptr) {
		for {
			if flockErr = syscall.Flock(int(fd), how); !errors.Is(flockErr, syscall.EINTR) {
				return
			}
		}
	})
	if err != nil {
		return err
	}
	return flockErr
}
