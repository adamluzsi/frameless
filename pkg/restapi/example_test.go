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
		router.MountRoutes(restapi.Routes{
			"/v1/api/foos": restapi.Handler[X, XID, XDTO]{
				Resource: fooRepository,
				Mapping:  XMapping{},
				Router: restapi.NewRouter(func(router *restapi.Router) {
					router.MountRoutes(restapi.Routes{
						"/bars": restapi.Handler[Y, string, YDTO]{
							Resource: barRepository,
							Mapping:  YMapping{},
						}})
				}),
			},
		})
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

func ExampleHandler() {
	m := memory.NewMemory()
	fooRepository := memory.NewRepository[X, XID](m)

	h := restapi.Handler[X, XID, XDTO]{
		Resource: fooRepository,
		Mapping:  XMapping{},
	}

	if err := http.ListenAndServe(":8080", h); err != nil {
		log.Fatalln(err.Error())
	}
}
