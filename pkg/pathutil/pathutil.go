// Package pathutil implements utility routines for manipulating slash-separated paths.
//
// The path package should only be used for paths separated by forward
// slashes, such as the paths in URLs. This package does not deal with
// Windows paths with drive letters or backslashes; to manipulate
// operating system paths, use the path/filepath package.
package pathutil

import (
	"path"
	"strings"
)

const (
	separatorRune = '/'
	separatorChar = string(separatorRune)
)

func Unshift(path string) (_pathParameter string, _remainingPath string) {
	parts := Split(path)
	if len(parts) == 0 {
		return "", path
	}
	param := parts[0]
	leftover := path[(strings.Index(path, param) + len(param)):]
	if len(leftover) == 0 || leftover[0] != separatorRune {
		leftover = separatorChar + leftover
	}
	return param, leftover
}

func Canonical(p string) string {
	np := Clean(p)
	if np == separatorChar {
		return np
	}
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == separatorRune && np != separatorChar {
		// Fast path for common case of p being the string we want:
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += separatorChar
		}
	}
	return np
}

func Clean(p string) string {
	if p == "" {
		return separatorChar
	}
	if p[0] != separatorRune {
		p = separatorChar + p
	}
	return path.Clean(p)
}

func Split(p string) []string {
	path := Canonical(p)
	path = strings.TrimPrefix(path, separatorChar)
	path = strings.TrimSuffix(path, separatorChar)
	parts := strings.Split(path, separatorChar)
	if len(parts) == 1 && parts[0] == "" {
		return []string{}
	}
	return parts
}

func Join(parts ...string) string {
	return Clean(strings.Join(parts, separatorChar))
}
