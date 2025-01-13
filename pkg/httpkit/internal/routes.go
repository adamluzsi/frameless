package internal

import (
	"net/http"
	"strings"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/pathkit"
)

var RoutingContext contextkit.ValueHandler[routingCtxKey, *Routing]

type routingCtxKey struct{}

type Routing struct {
	RequestURI string
	PathLeft   string
	Current    string
}

func (routing Routing) Peek(p string) Routing {
	p = pathkit.Canonical(p)
	return Routing{
		PathLeft: pathkit.Canonical(strings.TrimPrefix(routing.PathLeft, p)),
		Current:  pathkit.Join(routing.Current, p),
	}
}

func (routing *Routing) Travel(p string) { *routing = routing.Peek(p) }

func WithRoutingContext(request *http.Request) (*http.Request, *Routing) {
	ctx := request.Context()
	rc, ok := RoutingContext.Lookup(ctx)
	if ok {
		return request, rc
	}
	p := pathkit.Canonical(request.URL.EscapedPath())

	nro := Routing{
		RequestURI: request.RequestURI,
		PathLeft:   p,
		Current:    "/",
	}
	return request.WithContext(RoutingContext.ContextWith(ctx, &nro)), &nro
}
