package internal

import (
	"context"
	"net/http"
	"strings"

	"go.llib.dev/frameless/pkg/pathkit"
)

type routingCtxKey struct{}

type Routing struct {
	PathLeft string
	Current  string
}

func (routing Routing) Peek(path string) Routing {
	path = pathkit.Canonical(path)
	return Routing{
		PathLeft: pathkit.Canonical(strings.TrimPrefix(routing.PathLeft, path)),
		Current:  pathkit.Join(routing.Current, path),
	}
}

func (routing *Routing) Travel(path string) {
	*routing = routing.Peek(path)
}

func WithRoutingContext(request *http.Request) (*http.Request, *Routing) {
	ctx := request.Context()
	rc, ok := LookupRouting(ctx)
	if ok {
		return request, rc
	}
	nro := Routing{
		PathLeft: pathkit.Canonical(request.URL.Path),
		Current:  "/",
	}
	return request.WithContext(context.WithValue(ctx, routingCtxKey{}, &nro)), &nro
}

func LookupRouting(ctx context.Context) (*Routing, bool) {
	if ctx == nil {
		return nil, false
	}
	r, ok := ctx.Value(routingCtxKey{}).(*Routing)
	return r, ok
}
