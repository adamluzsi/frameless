package rest

import (
	"github.com/adamluzsi/frameless/pkg/rest/internal"
	"github.com/adamluzsi/frameless/pkg/rest/internal/paths"
	"net/http"
	"strings"
)

func MountPoint(mountPoint Path, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, rc := internal.WithRoutingCountex(r)
		withMountPoint(rc, mountPoint)
		next.ServeHTTP(w, r)
	})
}

func withMountPoint(rc *internal.Routing, mountPoint Path) {
	rc.Path = paths.Canonical(strings.TrimPrefix(rc.Path, paths.Canonical(string(mountPoint))))
}
