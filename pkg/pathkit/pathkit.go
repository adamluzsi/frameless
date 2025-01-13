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

func Unshift(rawPath string) (_pathParameter string, _remainingPath string) {
	parts := Split(rawPath)
	if len(parts) == 0 {
		return "", rawPath
	}
	param := parts[0]
	leftover := rawPath[(strings.Index(rawPath, param) + len(param)):]
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

func Split(path string) []string {
	path = Canonical(path)
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
	cleanJoin(u)
	if relativePath {
		u.Path = separatorChar + u.Path
		u.RawPath = separatorChar + u.RawPath
	}
	return u.String()
}

func cleanJoin(u *url.URL) error {
	raw := u.RawPath
	if len(raw) == 0 {
		raw = u.EscapedPath()
	}
	rpath := Clean(raw)
	if rpath == raw {
		return nil
	}
	uri, err := url.ParseRequestURI(rpath)
	if err != nil {
		return err
	}
	u.Path = uri.Path
	u.RawPath = uri.RawPath
	return nil
}

var isSchema = regexp.MustCompile(`^[^:]+:`)

// SplitBase will split a given uri into a base url and a request path.
func SplitBase(uri string) (_baseURL string, _path string) {
	if !isSchema.MatchString(uri) {
		return "", uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", uri
	}
	uPath := &url.URL{
		Opaque:      u.Opaque,
		Path:        u.Path,
		RawPath:     u.RawPath,
		ForceQuery:  u.ForceQuery,
		RawQuery:    u.RawQuery,
		Fragment:    u.Fragment,
		RawFragment: u.RawFragment,
	}
	uBase := &url.URL{
		Scheme:   u.Scheme,
		User:     u.User,
		Host:     u.Host,
		OmitHost: u.OmitHost,
	}
	return uBase.String(), uPath.String()
}
