package httpkit_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/random"
)

func ExampleRestClient() {
	var (
		ctx     = context.Background()
		fooRepo = httpkit.RestClient[testent.Foo, testent.FooID]{
			BaseURL:     "https://mydomain.dev/api/v1/foos",
			MediaType:   mediatype.JSON,
			Mapping:     dtokit.Mapping[testent.Foo, testent.FooDTO]{},
			Serializer:  serializers.JSON{},
			IDConverter: httpkit.IDConverter[testent.FooID]{},
			LookupID:    testent.Foo.LookupID,
		}
	)

	var ent = testent.Foo{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	err := fooRepo.Create(ctx, &ent)
	if err != nil {
		panic(err)
	}

	gotEnt, found, err := fooRepo.FindByID(ctx, ent.ID)
	if err != nil {
		panic(err)
	}
	_, _ = gotEnt, found

	err = fooRepo.Update(ctx, &ent)
	if err != nil {
		panic(err)
	}

	err = fooRepo.DeleteByID(ctx, ent.ID)
	if err != nil {
		panic(err)
	}
}

func ExampleRestClient_subresource() {
	barResourceClient := httpkit.RestClient[testent.Bar, testent.BarID]{
		BaseURL: "https://example.com/foos/:foo_id/bars",
		WithContext: func(ctx context.Context) context.Context {
			// here we define that this barResourceClient is the subresource of a Foo value (id=fooidvalue)
			return httpkit.WithPathParam(ctx, "foo_id", "fooidvalue")
		},
	}

	ctx := context.Background()
	_ = barResourceClient.FindAll(ctx)
	_, _, _ = barResourceClient.FindByID(ctx, "baridvalue")
}

func TestRestClient_crud(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := httpkit.RestResource[testent.Foo, testent.FooID]{}.WithCRUD(fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RestClient[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}

	crudcontractsConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		MakeEntity:      testent.MakeFoo,
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	crudcontracts.Creator[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
	crudcontracts.Finder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
	crudcontracts.Updater[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
	crudcontracts.Deleter[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
}

func TestRestClient_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RestResource[testent.Bar, testent.BarID]{}.WithCRUD(barRepo)
	fooAPI := httpkit.RestResource[testent.Foo, testent.FooID]{
		SubRoutes: httpkit.NewRouter(func(router *httpkit.Router) {
			router.Resource("/bars", barAPI)
		}),
	}.WithCRUD(fooRepo)

	api := httpkit.NewRouter(func(router *httpkit.Router) {
		router.Resource("/foos", fooAPI)
	})

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	foo := rnd.Make(testent.Foo{}).(testent.Foo)
	foo.ID = ""
	crudtest.Create[testent.Foo, testent.FooID](t, fooRepo, context.Background(), &foo)

	barClient := httpkit.RestClient[testent.Bar, testent.BarID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos/:foo_id/bars",
	}

	crudcontractsConfig := crudcontracts.Config[testent.Bar, testent.BarID]{
		MakeContext: func() context.Context {
			return httpkit.WithPathParam(context.Background(), "foo_id", foo.ID.String())
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	t.Run("Creator", crudcontracts.Creator[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Finder", crudcontracts.Finder[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Updater", crudcontracts.Updater[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Deleter", crudcontracts.Deleter[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
}

func TestRestClient_Resource_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RestResource[testent.Bar, testent.BarID]{}.WithCRUD(barRepo)
	fooAPI := httpkit.RestResource[testent.Foo, testent.FooID]{
		SubRoutes: httpkit.NewRouter(func(router *httpkit.Router) {
			router.Resource("/bars", barAPI)
		}),
	}.WithCRUD(fooRepo)

	api := httpkit.NewRouter(func(router *httpkit.Router) {
		router.Resource("/foos", fooAPI)
	})

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	foo := rnd.Make(testent.Foo{}).(testent.Foo)
	foo.ID = ""
	crudtest.Create[testent.Foo, testent.FooID](t, fooRepo, context.Background(), &foo)

	fooClient := httpkit.RestClient[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos",
	}

	barClient := httpkit.RestClient[testent.Bar, testent.BarID]{
		HTTPClient: fooClient.HTTPClient,
		BaseURL:    pathkit.Join(fooClient.BaseURL, ":foo_id", "/bars"),

		WithContext: func(ctx context.Context) context.Context {
			return httpkit.WithPathParam(ctx, "foo_id", foo.ID.String())
		},
	}

	crudcontractsConfig := crudcontracts.Config[testent.Bar, testent.BarID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	t.Run("Creator", crudcontracts.Creator[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Finder", crudcontracts.Finder[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Updater", crudcontracts.Updater[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Deleter", crudcontracts.Deleter[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
}
