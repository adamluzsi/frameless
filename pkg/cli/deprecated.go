package cli

import "reflect"

// Response
//
// Deprecated: use cli.ResponseWriter instead
type Response = ResponseWriter

// Mux
//
// Deprecated: use cli.ServeMux instead
type Mux = ServeMux

// ConfigureHandler
//
// Deprecated: use Build instead in order to access Middlewares too.
func ConfigureHandler[H Handler](ptr *H, r *Request) error {
	return metaFor(reflect.ValueOf(ptr)).Configure(r)
}
