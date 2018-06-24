package requests

import (
	"context"

	"github.com/adamluzsi/frameless"
)

func NewMock(ctx context.Context, data frameless.Iterator) *Mock {
	return &Mock{ctx: ctx, data: data}
}

type Mock struct {
	ctx  context.Context
	data frameless.Iterator
}

func (m *Mock) Context() context.Context {
	return m.ctx
}

func (m *Mock) Data() frameless.Iterator {
	return m.data
}
