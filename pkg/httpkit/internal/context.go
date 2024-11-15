package internal

import (
	"context"
	"net/http"

	"go.llib.dev/frameless/pkg/contextkit"
)

type ctxKeyRequest struct{}

func WithRequest(ctx context.Context, req *http.Request) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequest{}, req)
}

func LookupRequest(ctx context.Context) (*http.Request, bool) {
	if ctx == nil {
		return nil, false
	}
	if req, ok := ctx.Value(ctxKeyRequest{}).(*http.Request); ok {
		return req, true
	}
	return nil, false
}

var ContextRESTParentResourceValuePointer contextkit.ValueHandler[ctxKeyRESTResourceValue, any]

type ctxKeyRESTResourceValue struct{}
