package restapi_test

import (
	"context"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
	"net/http/httptest"
	"testing"
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
	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := restapi.Resource[testent.Foo, testent.FooID]{}.WithCRUD(fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := restapi.Client[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}

	makeFoo := func() testent.Foo {
		foo := rnd.Make(testent.Foo{}).(testent.Foo)
		foo.ID = ""
		return foo
	}

	crudcontracts.Creator[testent.Foo, testent.FooID](func(tb testing.TB) crudcontracts.CreatorSubject[testent.Foo, testent.FooID] {
		return crudcontracts.CreatorSubject[testent.Foo, testent.FooID]{
			Resource:        fooClient,
			MakeContext:     context.Background,
			MakeEntity:      makeFoo,
			SupportIDReuse:  true,
			SupportRecreate: true,
		}
	}).Test(t)

	crudcontracts.Finder[testent.Foo, testent.FooID](func(tb testing.TB) crudcontracts.FinderSubject[testent.Foo, testent.FooID] {
		return crudcontracts.FinderSubject[testent.Foo, testent.FooID]{
			Resource:    fooClient,
			MakeContext: context.Background,
			MakeEntity:  makeFoo,
		}
	}).Test(t)

	crudcontracts.Updater[testent.Foo, testent.FooID](func(tb testing.TB) crudcontracts.UpdaterSubject[testent.Foo, testent.FooID] {
		return crudcontracts.UpdaterSubject[testent.Foo, testent.FooID]{
			Resource:    fooClient,
			MakeContext: context.Background,
			MakeEntity:  makeFoo,
		}
	}).Test(t)

	crudcontracts.Deleter[testent.Foo, testent.FooID](func(tb testing.TB) crudcontracts.DeleterSubject[testent.Foo, testent.FooID] {
		return crudcontracts.DeleterSubject[testent.Foo, testent.FooID]{
			Resource:    fooClient,
			MakeContext: context.Background,
			MakeEntity:  makeFoo,
		}
	}).Test(t)
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

	makeBar := func() testent.Bar {
		v := rnd.Make(testent.Bar{}).(testent.Bar)
		v.ID = ""
		return v
	}

	makeContext := func() context.Context {
		return restapi.WithPathParam(context.Background(), "foo_id", foo.ID.String())
	}

	t.Run("Create", crudcontracts.Creator[testent.Bar, testent.BarID](func(tb testing.TB) crudcontracts.CreatorSubject[testent.Bar, testent.BarID] {
		return crudcontracts.CreatorSubject[testent.Bar, testent.BarID]{
			Resource:        barClient,
			MakeContext:     makeContext,
			MakeEntity:      makeBar,
			SupportIDReuse:  true,
			SupportRecreate: true,
		}
	}).Test)

	t.Run("Finder", crudcontracts.Finder[testent.Bar, testent.BarID](func(tb testing.TB) crudcontracts.FinderSubject[testent.Bar, testent.BarID] {
		return crudcontracts.FinderSubject[testent.Bar, testent.BarID]{
			Resource:    barClient,
			MakeContext: makeContext,
			MakeEntity:  makeBar,
		}
	}).Test)

	t.Run("Updater", crudcontracts.Updater[testent.Bar, testent.BarID](func(tb testing.TB) crudcontracts.UpdaterSubject[testent.Bar, testent.BarID] {
		return crudcontracts.UpdaterSubject[testent.Bar, testent.BarID]{
			Resource:    barClient,
			MakeContext: makeContext,
			MakeEntity:  makeBar,
		}
	}).Test)

	t.Run("Deleter", crudcontracts.Deleter[testent.Bar, testent.BarID](func(tb testing.TB) crudcontracts.DeleterSubject[testent.Bar, testent.BarID] {
		return crudcontracts.DeleterSubject[testent.Bar, testent.BarID]{
			Resource:    barClient,
			MakeContext: makeContext,
			MakeEntity:  makeBar,
		}
	}).Test)
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
