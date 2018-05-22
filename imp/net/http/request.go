package http

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/adamluzsi/frameless/dataprovider"
)

type Request struct {
	srcRequest      *http.Request
	iteratorBuilder func(io.Reader) dataprovider.Iterator
}

func NewRequest(r *http.Request, payloadDecoderBuilder func(io.Reader) dataprovider.Iterator) *Request {
	return &Request{
		srcRequest:      r,
		iteratorBuilder: payloadDecoderBuilder,
	}
}

func (r *Request) Context() context.Context {
	return r.srcRequest.Context()
}

type options url.Values

func (o options) Get(key interface{}) (interface{}, bool) {
	vs, ok := o[key.(string)]
	return vs, ok
}

func (r *Request) Options() dataprovider.Getter {
	return options(r.srcRequest.URL.Query())
}

func (r *Request) Data() dataprovider.Iterator {
	return r.iteratorBuilder(r.srcRequest.Body)
}

func (r *Request) Close() error {
	return r.srcRequest.Body.Close()
}
