package restapi

import (
	"github.com/adamluzsi/frameless/pkg/pathutil"
	"github.com/adamluzsi/frameless/pkg/restapi/internal"
	"net/http"
	"strings"
)

// Mount will help to register a handler on a request multiplexer in both as the concrete path to the handler and as a prefix match.
// example:
//
//	if pattern -> "/something"
//	registered as "/something" for exact match
//	registered as "/something/" for prefix match
func Mount(multiplexer multiplexer, pattern string, handler http.Handler) {
	pattern = pathutil.Clean(pattern)
	handler = http.StripPrefix(pattern, handler)
	handler = MountPoint(pattern, handler)
	multiplexer.Handle(pattern, handler)
	multiplexer.Handle(pattern+`/`, handler)
}

// multiplexer represents a http request multiplexer.
type multiplexer interface {
	Handle(pattern string, handler http.Handler)
}

func MountPoint(mountPoint Path, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, rc := internal.WithRoutingCountex(r)
		withMountPoint(rc, mountPoint)
		next.ServeHTTP(w, r)
	})
}

func withMountPoint(rc *internal.Routing, mountPoint Path) {
	rc.Path = pathutil.Canonical(strings.TrimPrefix(rc.Path, pathutil.Canonical(string(mountPoint))))
}
