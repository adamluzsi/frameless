package cachecontracts

import (
	"context"

	"github.com/adamluzsi/testcase"
)

var ctx = testcase.Var[context.Context]{
	ID: `context.Context`,
	Init: func(t *testcase.T) context.Context {
		return context.Background()
	},
}

func ctxGet(t *testcase.T) context.Context {
	return ctx.Get(t)
}
