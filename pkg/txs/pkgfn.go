package txs

import (
	"context"
)

var DefaultManager = Manager{}

func Begin(ctx context.Context) context.Context {
	return DefaultManager.Begin(ctx)
}

func Finish(returnError *error, tx context.Context) {
	DefaultManager.Finish(returnError, tx)
}

func OnRollback(ctx context.Context, fn func()) {
	if err := DefaultManager.OnRollback(ctx, fn); err != nil {
		// implementation error, no
		panic(err.Error())
	}
}
