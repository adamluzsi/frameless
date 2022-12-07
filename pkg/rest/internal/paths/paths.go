package paths

import (
	"path"
	"strings"
)

func Unshift(path string) (_pathParameter string, _remainingPath string) {
	parts := Split(path)
	if len(parts) == 0 {
		return "", path
	}
	param := parts[0]
	leftover := path[(strings.Index(path, param) + len(param)):]
	if len(leftover) == 0 || leftover[0] != '/' {
		leftover = `/` + leftover
	}
	return param, leftover
}

func Canonical(p string) string {
	np := Clean(p)
	if np == "/" {
		return np
	}
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		// Fast path for common case of p being the string we want:
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += "/"
		}
	}
	return np
}

func Clean(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	return path.Clean(p)
}

func Split(p string) []string {
	path := Canonical(p)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 && parts[0] == "" {
		return []string{}
	}
	return parts
}

func Join(parts ...string) string {
	return Clean(strings.Join(parts, "/"))
}
