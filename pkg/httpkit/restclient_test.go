package httpkit_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/random"
)

func ExampleRESTClient() {
	var (
		ctx     = context.Background()
		fooRepo = httpkit.RESTClient[testent.Foo, testent.FooID]{
			BaseURL:   "https://mydomain.dev/api/v1/foos",
			MediaType: mediatype.JSON,
			Mapping:   dtokit.Mapping[testent.Foo, testent.FooDTO]{},
			Codec:     jsonkit.Codec{},
			// leave IDFormatter empty for using the default id formatter, or provide your own
			IDFormatter: func(fi testent.FooID) (string, error) {
				return httpkit.IDConverter[testent.FooID]{}.Format(fi)
			},
			LookupID: testent.Foo.LookupID,
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

func ExampleRESTClient_subresource() {
	barResourceClient := httpkit.RESTClient[testent.Bar, testent.BarID]{
		BaseURL: "https://example.com/foos/:foo_id/bars",
		WithContext: func(ctx context.Context) context.Context {
			// here we define that this barResourceClient is the subresource of a Foo value (id=fooidvalue)
			return httpkit.WithPathParam(ctx, "foo_id", "fooidvalue")
		},
	}

	ctx := context.Background()
	_, _ = barResourceClient.FindAll(ctx)
	_, _, _ = barResourceClient.FindByID(ctx, "baridvalue")
}

func TestRESTClient_crud(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := httpkit.RESTHandler[testent.Foo, testent.FooID]{}.WithCRUD(fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
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
	crudcontracts.ByIDsFinder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
	crudcontracts.Updater[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
	crudcontracts.Deleter[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
}

func TestRESTClient_FindAll_withDisableStreaming(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := httpkit.RESTHandler[testent.Foo, testent.FooID]{}.WithCRUD(fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
		HTTPClient:       srv.Client(),
		BaseURL:          srv.URL,
		DisableStreaming: true,
	}

	crudcontractsConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		MakeEntity: testent.MakeFoo,
	}

	crudcontracts.AllFinder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
}

func TestRESTClient_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RESTHandler[testent.Bar, testent.BarID]{}.WithCRUD(barRepo)
	fooAPI := httpkit.RESTHandler[testent.Foo, testent.FooID]{
		ResourceRoutes: httpkit.NewRouter(func(router *httpkit.Router) {
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

	barClient := httpkit.RESTClient[testent.Bar, testent.BarID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos/:foo_id/bars",
	}

	crudcontractsConfig := crudcontracts.Config[testent.Bar, testent.BarID]{
		MakeContext: func(testing.TB) context.Context {
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

func TestRESTClient_Resource_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RESTHandler[testent.Bar, testent.BarID]{}.WithCRUD(barRepo)
	fooAPI := httpkit.RESTHandler[testent.Foo, testent.FooID]{
		ResourceRoutes: httpkit.NewRouter(func(router *httpkit.Router) {
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

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos",
	}

	barClient := httpkit.RESTClient[testent.Bar, testent.BarID]{
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
