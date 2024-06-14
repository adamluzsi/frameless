package restapi

import (
	"net/http"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
)

// Mount will help to register a handler on a request multiplexer in both as the concrete path to the handler and as a prefix match.
// example:
//
//	if pattern -> "/something"
//	registered as "/something" for exact match
//	registered as "/something/" for prefix match
func Mount(multiplexer httpkit.Multiplexer, pattern string, handler http.Handler) {
	pattern = pathkit.Clean(pattern)
	handler = MountPoint(pattern, handler)
	httpkit.Mount(multiplexer, pattern, handler)
}

func MountPoint(mountPoint string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, rc := internal.WithRoutingContext(r)
		rc.Travel(mountPoint)
		next.ServeHTTP(w, r)
	})
}
