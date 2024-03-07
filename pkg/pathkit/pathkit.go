// Package pathkit implements utility routines for manipulating slash-separated paths.
//
// The path package should only be used for paths separated by forward
// slashes, such as the paths in URLs. This package does not deal with
// Windows paths with drive letters or backslashes; to manipulate
// operating system paths, use the path/filepath package.
package pathkit

import (
	"net/url"
	"path"
	"regexp"
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

// Join function makes it easy to combine different parts of a path,
// making sure slashes are handled correctly.
// If you provide a URL as the first thing in the Join function,
// it becomes the starting point for creating the final URL.
//
// Double slashes prefixes are preserved as part of the joining.
// Instances of double slashes (//) in a path commonly denote a relative URL path.
// When a browser or HTTP client encounters // at the path's start,
// it omits the leading slashes, interpreting the remaining path as relative to the current URL's path.
// For instance, if the present URL is https://example.com/some/path,
// and you reference //foo/bar, the resulting URL becomes https://example.com/some/foo/bar.
func Join(ps ...string) string {
	u := &url.URL{}
	if len(ps) == 0 {
		return separatorChar
	}
	var relativePath bool
	if strings.HasPrefix(ps[0], "//") {
		relativePath = true
	}
	if uri := ps[0]; isSchema.MatchString(uri) {
		if nu, err := url.Parse(uri); err == nil {
			u = nu
			ps = ps[1:]
		}
	}
	u = u.JoinPath(ps...)
	u.Path = Clean(u.Path)
	if relativePath {
		u.Path = separatorChar + u.Path
	}
	return u.String()
}

var isSchema = regexp.MustCompile(`^[^:]+:`)
