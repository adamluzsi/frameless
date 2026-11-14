//go:build !windows

package fileio

// isTransient reports false, as on this platform, an open file doesn't stop it from being renamed or removed.
func isTransient(error) bool { return false }
