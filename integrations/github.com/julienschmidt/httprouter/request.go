package httprouter

import (
	"context"
	"net/http"

	"github.com/adamluzsi/frameless/dataproviders"
	"github.com/julienschmidt/httprouter"
)

type Request struct {
	srcRequest      *http.Request
	iteratorBuilder dataproviders.IteratorBuilder
	params          httprouter.Params
}

func NewRequest(r *http.Request, b dataproviders.IteratorBuilder, p httprouter.Params) *Request {
	return &Request{
		srcRequest:      r,
		iteratorBuilder: b,
		params:          p,
	}
}

func (r *Request) Context() context.Context {
	return r.srcRequest.Context()
}

type options struct{ httprouter.Params }

func (o options) Get(key interface{}) interface{} {
	return o.Params.ByName(key.(string))
}

func (o options) Lookup(key interface{}) (interface{}, bool) {
	vs, ok := o.LookupAll(key)
	return vs[0], ok
}

func (o options) GetAll(key interface{}) []interface{} {
	vs, _ := o.LookupAll(key)

	return vs
}

func (o options) LookupAll(key interface{}) ([]interface{}, bool) {

	name := key.(string)
	values := []interface{}{}

	for _, param := range o.Params {
		if param.Key == name {
			values = append(values, param.Value)
		}
	}

	return values, len(values) != 0
}

func (r *Request) Options() dataproviders.Getter {
	return &options{r.params}
}

func (r *Request) Data() dataproviders.Iterator {
	return r.iteratorBuilder(r.srcRequest.Body)
}

func (r *Request) Close() error {
	return r.srcRequest.Body.Close()
}
