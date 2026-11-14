//go:build windows

package fileio

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isTransient reports whether the error is likely to go away by itself,
// as another process, like a virus scanner or a search indexer, holds the file open for a moment.
//
// A missing file is not transient, as the callers rely on fs.ErrNotExist.
func isTransient(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
