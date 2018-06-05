package httprouter

import (
	"context"
	"io"
	"net/http"

	"github.com/adamluzsi/frameless"
	fhttp "github.com/adamluzsi/frameless/integrations/net/http"
	"github.com/julienschmidt/httprouter"
)

func NewRequest(r *http.Request, b func(io.Reader) frameless.Iterator, ps httprouter.Params) frameless.Request {
	return fhttp.NewRequestWithContext(contextBy(r.Context(), ps), r, b)
}

func contextBy(c context.Context, ps httprouter.Params) context.Context {
	ctx := c

	for _, p := range ps {
		ctx = context.WithValue(ctx, p.Key, p.Value)
	}

	return ctx
}
