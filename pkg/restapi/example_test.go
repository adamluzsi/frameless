package restapi_test

import (
	"log"
	"net/http"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/restapi"
)

func ExampleRoutes() {
	m := memory.NewMemory()
	fooRepository := memory.NewRepository[X, XID](m)
	barRepository := memory.NewRepository[Y, string](m)

	r := restapi.NewRouter(func(router *restapi.Router) {
		router.Resource("/v1/api/foos", restapi.Resource[X, XID]{
			Mapping: restapi.DTOMapping[X, XDTO]{},
			SubRoutes: restapi.NewRouter(func(router *restapi.Router) {
				router.Resource("/bars", restapi.Resource[Y, string]{}.
					WithCRUD(barRepository))
			}),
		}.WithCRUD(fooRepository))
	})

	// Generated endpoints:
	//
	// Foo Index  - GET       /v1/api/foos
	// Foo Create - POST      /v1/api/foos
	// Foo Show   - GET       /v1/api/foos/:foo_id
	// Foo Update - PATCH/PUT /v1/api/foos/:foo_id
	// Foo Delete - DELETE    /v1/api/foos/:foo_id
	//
	// Bar Index  - GET       /v1/api/foos/:foo_id/bars
	// Bar Create - POST      /v1/api/foos/:foo_id/bars
	// Bar Show   - GET       /v1/api/foos/:foo_id/bars/:bar_id
	// Bar Update - PATCH/PUT /v1/api/foos/:foo_id/bars/:bar_id
	// Bar Delete - DELETE    /v1/api/foos/:foo_id/bars/:bar_id
	//
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalln(err.Error())
	}
}
