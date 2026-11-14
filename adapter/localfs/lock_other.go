//go:build !(darwin || linux || windows)

package localfs

import (
	"context"
	"os"
	"slices"
	"sync"
)

// lockDir coordinates separate lockers for the same directory within this process.
// It does not lock between processes on this platform.
func lockDir(ctx context.Context, root *os.Root) (unlock func() error, _ error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	lock := refProcessDirLock(info)
	select {
	case lock.turn <- struct{}{}:
		return func() error {
			<-lock.turn
			unrefProcessDirLock(lock)
			return nil
		}, nil
	case <-ctx.Done():
		unrefProcessDirLock(lock)
		return nil, ctx.Err()
	}
}

var processDirLocks struct {
	sync.Mutex
	locks []*processDirLock
}

type processDirLock struct {
	info os.FileInfo
	turn chan struct{}
	refs int
}

// Identify the opened directory, not its spelling: relative paths and symlinks
// to the same directory must share a lock. References include waiting callers.
func refProcessDirLock(info os.FileInfo) *processDirLock {
	processDirLocks.Lock()
	defer processDirLocks.Unlock()
	for _, lock := range processDirLocks.locks {
		if os.SameFile(lock.info, info) {
			lock.refs++
			return lock
		}
	}
	lock := &processDirLock{info: info, turn: make(chan struct{}, 1), refs: 1}
	processDirLocks.locks = append(processDirLocks.locks, lock)
	return lock
}

func unrefProcessDirLock(lock *processDirLock) {
	processDirLocks.Lock()
	defer processDirLocks.Unlock()
	lock.refs--
	if lock.refs == 0 {
		i := slices.Index(processDirLocks.locks, lock)
		processDirLocks.locks = slices.Delete(processDirLocks.locks, i, i+1)
	}
}
