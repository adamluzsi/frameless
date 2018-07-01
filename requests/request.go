package requests

import (
	"context"

	"github.com/adamluzsi/frameless"
)

func New(ctx context.Context, data frameless.Iterator) *Request {
	return &Request{ctx: ctx, data: data}
}

type Request struct {
	ctx  context.Context
	data frameless.Iterator
}

func (m *Request) Context() context.Context {
	return m.ctx
}

func (m *Request) Data() frameless.Iterator {
	return m.data
}
