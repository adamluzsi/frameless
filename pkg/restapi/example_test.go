package restapi_test

import (
	"log"
	"net/http"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/restapi"
)

func ExampleRoutes() {
	m := memory.NewMemory()
	fooRepository := memory.NewRepository[Foo, int](m)
	barRepository := memory.NewRepository[Bar, string](m)

	r := restapi.NewRouter(func(router *restapi.Router) {
		router.MountRoutes(restapi.Routes{
			"/v1/api/foos": restapi.Handler[Foo, int, FooDTO]{
				Resource: fooRepository,
				Mapping:  FooMapping{},
				Router: restapi.NewRouter(func(router *restapi.Router) {
					router.MountRoutes(restapi.Routes{
						"/bars": restapi.Handler[Bar, string, BarDTO]{
							Resource: barRepository,
							Mapping:  BarMapping{},
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
	fooRepository := memory.NewRepository[Foo, int](m)

	h := restapi.Handler[Foo, int, FooDTO]{
		Resource: fooRepository,
		Mapping:  FooMapping{},
	}

	if err := http.ListenAndServe(":8080", h); err != nil {
		log.Fatalln(err.Error())
	}
}