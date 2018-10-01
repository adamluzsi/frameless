package http

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/adamluzsi/frameless"
)

type Request struct {
	ctx             context.Context
	srcRequest      *http.Request
	iteratorBuilder func(io.Reader) frameless.Iterator
}

func NewRequest(r *http.Request, b func(io.Reader) frameless.Iterator) frameless.Request {
	return NewRequestWithContext(contextBy(r.Context(), r.URL.Query()), r, b)
}

func NewRequestWithContext(ctx context.Context, r *http.Request, b func(io.Reader) frameless.Iterator) frameless.Request {
	return &Request{
		ctx:             ctx,
		srcRequest:      r,
		iteratorBuilder: b,
	}
}

func contextBy(ctx context.Context, vs url.Values) context.Context {
	for k, v := range vs {
		ctx = context.WithValue(ctx, k, v[0])
	}

	return ctx
}

func (r *Request) Context() context.Context {
	return r.ctx
}

func (r *Request) Data() frameless.Iterator {
	return r.iteratorBuilder(r.srcRequest.Body)
}
