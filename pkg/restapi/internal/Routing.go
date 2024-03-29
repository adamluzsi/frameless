package internal

import (
	"context"
	"net/http"

	"go.llib.dev/frameless/pkg/pathkit"
)

type routingCtxKey struct{}

type Routing struct {
	Path string
}

func WithRoutingCountex(request *http.Request) (*http.Request, *Routing) {
	ctx := request.Context()
	rc, ok := LookupRouting(ctx)
	if ok {
		return request, rc
	}
	nro := Routing{Path: pathkit.Canonical(request.URL.Path)}
	return request.WithContext(context.WithValue(ctx, routingCtxKey{}, &nro)), &nro
}

func LookupRouting(ctx context.Context) (*Routing, bool) {
	if ctx == nil {
		return nil, false
	}
	r, ok := ctx.Value(routingCtxKey{}).(*Routing)
	return r, ok
}
