package restapi_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func ExampleClient() {
	var (
		ctx     = context.Background()
		fooRepo = restapi.Client[testent.Foo, testent.FooID]{
			BaseURL:     "https://mydomain.dev/api/v1/foos",
			MIMEType:    restapi.JSON,
			Mapping:     restapi.DTOMapping[testent.Foo, testent.FooDTO]{},
			Serializer:  serializers.JSON{},
			IDConverter: restapi.IDConverter[testent.FooID]{},
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

func TestClient_crud(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := restapi.Resource[testent.Foo, testent.FooID]{}.WithCRUD(fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := restapi.Client[testent.Foo, testent.FooID]{
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

func TestClient_subresource(t *testing.T) {
	logger.LogWithTB(t)
	logger.Default.Level = logger.LevelDebug

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := restapi.Resource[testent.Bar, testent.BarID]{}.WithCRUD(barRepo)
	fooAPI := restapi.Resource[testent.Foo, testent.FooID]{
		SubRoutes: restapi.NewRouter(func(router *restapi.Router) {
			router.Resource("/bars", barAPI)
		}),
	}.WithCRUD(fooRepo)

	api := restapi.NewRouter(func(router *restapi.Router) {
		router.Resource("/foos", fooAPI)
	})

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	foo := rnd.Make(testent.Foo{}).(testent.Foo)
	foo.ID = ""
	crudtest.Create[testent.Foo, testent.FooID](t, fooRepo, context.Background(), &foo)

	barClient := restapi.Client[testent.Bar, testent.BarID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos/:foo_id/bars",
	}

	crudcontractsConfig := crudcontracts.Config[testent.Bar, testent.BarID]{
		MakeContext: func() context.Context {
			return restapi.WithPathParam(context.Background(), "foo_id", foo.ID.String())
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	t.Run("Creator", crudcontracts.Creator[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Finder", crudcontracts.Finder[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Updater", crudcontracts.Updater[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Deleter", crudcontracts.Deleter[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
}

func TestWithPathParam(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := restapi.WithPathParam(ctx, "foo", "A")
		ctx2 := restapi.WithPathParam(ctx1, "bar", "B")
		ctx3 := restapi.WithPathParam(ctx2, "foo", "C")

		assert.Equal(t, restapi.PathParams(ctx), map[string]string{})
		assert.Equal(t, restapi.PathParams(ctx1), map[string]string{
			"foo": "A",
		})
		assert.Equal(t, restapi.PathParams(ctx2), map[string]string{
			"foo": "A",
			"bar": "B",
		})
		assert.Equal(t, restapi.PathParams(ctx3), map[string]string{
			"foo": "C",
			"bar": "B",
		})
	})
	t.Run("variable can be set in the context", func(t *testing.T) {
		ctx := context.Background()
		ctx = restapi.WithPathParam(ctx, "foo", "A")
		assert.Equal(t, restapi.PathParams(ctx), map[string]string{"foo": "A"})
	})
	t.Run("variable can be overwritten with WithPathParam", func(t *testing.T) {
		ctx := context.Background()
		ctx = restapi.WithPathParam(ctx, "foo", "A")
		ctx = restapi.WithPathParam(ctx, "foo", "C")
		assert.Equal(t, restapi.PathParams(ctx), map[string]string{"foo": "C"})
	})
	t.Run("variable overwriting is not mutating the original context", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := restapi.WithPathParam(ctx, "foo", "A")
		ctx2 := restapi.WithPathParam(ctx1, "foo", "C")
		assert.Equal(t, restapi.PathParams(ctx1), map[string]string{"foo": "A"})
		assert.Equal(t, restapi.PathParams(ctx2), map[string]string{"foo": "C"})
	})
}
