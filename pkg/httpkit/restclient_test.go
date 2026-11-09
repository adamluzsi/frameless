package httpkit_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/iotest"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleRESTClient() {
	var (
		ctx     = context.Background()
		fooRepo = httpkit.RESTClient[testent.Foo, testent.FooID]{
			BaseURL:   "https://mydomain.dev/api/v1/foos",
			MediaType: mediatype.JSON,
			Codecs: httpkit.Codecs{
				mediatype.JSON: &jsonkit.Codec{},
			},
			// leave IDFormatter empty for using the default id formatter, or provide your own
			IDFormatter: func(fi testent.FooID) (string, error) {
				return httpkit.IDConverter[testent.FooID]{}.Format(fi)
			},
			IDA: func(f *testent.Foo) *testent.FooID {
				return &f.ID
			},
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
	_ = barResourceClient.FindAll(ctx)
	_, _, _ = barResourceClient.FindByID(ctx, "baridvalue")
}

func TestRESTClient(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(func(t *testcase.T) { logger.Testing(t) })

	var (
		ctx  = let.Context(s)
		repo = testcase.Let(s, func(t *testcase.T) *memory.Repository[testent.Foo, testent.FooID] {
			return memory.NewRepository[testent.Foo, testent.FooID](memory.NewMemory())
		})
		api = testcase.Let(s, func(t *testcase.T) http.Handler {
			return httpkit.NewRouter(func(r *httpkit.Router) {
				r.Resource("/foos", httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](repo.Get(t)))
			})
		})
		server = testcase.Let(s, func(t *testcase.T) *httptest.Server {
			srv := httptest.NewServer(api.Get(t))
			t.Defer(srv.Close)
			return srv
		})
		baseURL = testcase.Let(s, func(t *testcase.T) string {
			return server.Get(t).URL + "/foos"
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) httpkit.RESTClient[testent.Foo, testent.FooID] {
		return httpkit.RESTClient[testent.Foo, testent.FooID]{
			HTTPClient: server.Get(t).Client(),
			BaseURL:    baseURL.Get(t),
		}
	})

	var (
		id = testcase.Let(s, func(t *testcase.T) testent.FooID {
			return testent.FooID(t.Random.StringNC(8, random.CharsetAlpha()))
		})
		foo = testcase.Let(s, func(t *testcase.T) testent.Foo {
			v := testent.MakeFoo(t)
			v.ID = id.Get(t)
			crudtest.Create[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), &v)
			return v
		})
		// reservedID has characters that are reserved in a URL path.
		reservedID = func(t *testcase.T) testent.FooID {
			return testent.FooID("a b/c?d#e%f" + t.Random.StringNC(3, random.CharsetAlpha()))
		}
	)

	s.Describe("#FindByID", func(s *testcase.Spec) {
		act := func(t *testcase.T) (testent.Foo, bool, error) {
			return subject.Get(t).FindByID(ctx.Get(t), foo.Get(t).ID)
		}

		s.Then("it finds the stored entity", func(t *testcase.T) {
			got, found, err := act(t)
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, foo.Get(t), got)
		})

		s.When("the ID has characters that are reserved in a URL path", func(s *testcase.Spec) {
			id.Let(s, reservedID)

			s.Then("it escapes the ID and finds the stored entity", func(t *testcase.T) {
				got, found, err := act(t)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, foo.Get(t), got)
			})
		})

		s.When("the BaseURL has a path parameter, which value has characters reserved in a URL path", func(s *testcase.Spec) {
			var (
				tenant         = testcase.LetValue(s, "a b/c?d")
				receivedTenant = testcase.LetValue(s, "")
			)
			api.Let(s, func(t *testcase.T) http.Handler {
				return httpkit.NewRouter(func(r *httpkit.Router) {
					r.Namespace("/tenants/:tenant", func(r *httpkit.Router) {
						r.Use(func(next http.Handler) http.Handler {
							return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								receivedTenant.Set(t, httpkit.PathParams(r.Context())["tenant"])
								next.ServeHTTP(w, r)
							})
						})
						r.Resource("/foos", httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](repo.Get(t)))
					})
				})
			})
			baseURL.Let(s, func(t *testcase.T) string {
				return server.Get(t).URL + "/tenants/:tenant/foos"
			})
			ctx.Let(s, func(t *testcase.T) context.Context {
				return httpkit.WithPathParam(ctx.Super(t), "tenant", tenant.Get(t))
			})

			s.Then("it escapes the path parameter value in the request path", func(t *testcase.T) {
				got, found, err := act(t)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, foo.Get(t), got)
				assert.Equal(t, tenant.Get(t), receivedTenant.Get(t))
			})
		})

		s.When("reading the response body fails", func(s *testcase.Spec) {
			expErr := let.Error(s)
			subject.Let(s, func(t *testcase.T) httpkit.RESTClient[testent.Foo, testent.FooID] {
				c := subject.Super(t)
				c.HTTPClient = &http.Client{
					Transport: httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
						data, err := json.Marshal(foo.Get(t))
						if err != nil {
							return nil, err
						}
						body := io.MultiReader(bytes.NewReader(data), iotest.ErrReader(expErr.Get(t)))
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{mediatype.JSON}},
							Body:       io.NopCloser(body),
							Request:    r,
						}, nil
					}),
				}
				return c
			})

			s.Then("it returns the read error", func(t *testcase.T) {
				_, found, err := act(t)
				assert.ErrorIs(t, expErr.Get(t), err)
				assert.False(t, found)
			})
		})
	})

	s.Describe("#Update", func(s *testcase.Spec) {
		updated := testcase.Let(s, func(t *testcase.T) testent.Foo {
			v := foo.Get(t)
			v.Bar = random.Unique(func() string { return t.Random.StringNC(8, random.CharsetAlpha()) }, v.Bar)
			return v
		})
		act := func(t *testcase.T) error {
			v := updated.Get(t)
			return subject.Get(t).Update(ctx.Get(t), &v)
		}

		s.Then("it updates the stored entity", func(t *testcase.T) {
			assert.NoError(t, act(t))
			exp := updated.Get(t)
			crudtest.HasEntity[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), &exp)
		})

		s.When("the ID has characters that are reserved in a URL path", func(s *testcase.Spec) {
			id.Let(s, reservedID)

			s.Then("it escapes the ID and updates the stored entity", func(t *testcase.T) {
				assert.NoError(t, act(t))
				exp := updated.Get(t)
				crudtest.HasEntity[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), &exp)
			})
		})
	})

	s.Describe("#DeleteByID", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).DeleteByID(ctx.Get(t), foo.Get(t).ID)
		}

		s.Then("it deletes the stored entity", func(t *testcase.T) {
			assert.NoError(t, act(t))
			crudtest.IsAbsent[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), foo.Get(t).ID)
		})

		s.When("the ID has characters that are reserved in a URL path", func(s *testcase.Spec) {
			id.Let(s, reservedID)

			s.Then("it escapes the ID and deletes the stored entity", func(t *testcase.T) {
				assert.NoError(t, act(t))
				crudtest.IsAbsent[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), foo.Get(t).ID)
			})
		})
	})

	s.Describe("#DeleteAll", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).DeleteAll(ctx.Get(t))
		}

		s.Then("it deletes every stored entity", func(t *testcase.T) {
			v := foo.Get(t)
			assert.NoError(t, act(t))
			crudtest.IsAbsent[testent.Foo, testent.FooID](t, repo.Get(t), context.Background(), v.ID)
		})

		s.When("the server replies with not found", func(s *testcase.Spec) {
			api.Let(s, func(t *testcase.T) http.Handler {
				return httpkit.NewRouter()
			})

			s.Then("it returns crud.ErrNotFound", func(t *testcase.T) {
				assert.ErrorIs(t, crud.ErrNotFound, act(t))
			})
		})
	})

	s.Describe("#FindAll", func(s *testcase.Spec) {
		act := func(t *testcase.T) ([]testent.Foo, error) {
			return iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
		}

		s.Then("it lists the stored entities", func(t *testcase.T) {
			v := foo.Get(t)
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, []testent.Foo{v}, got)
		})

		s.When("the server replies with not found", func(s *testcase.Spec) {
			api.Let(s, func(t *testcase.T) http.Handler {
				return httpkit.NewRouter()
			})

			s.Then("it yields crud.ErrNotFound", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, crud.ErrNotFound, err)
			})
		})

		s.When("the server replies with an unexpected status code", func(s *testcase.Spec) {
			api.Let(s, func(t *testcase.T) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", mediatype.JSON)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`[]`))
				})
			})

			s.Then("it yields an error that tells the unexpected status code", func(t *testcase.T) {
				_, err := act(t)
				var unexpected httpkit.ClientErrUnexpectedResponse
				assert.True(t, errors.As(err, &unexpected))
				assert.Equal(t, http.StatusInternalServerError, unexpected.StatusCode)
			})
		})
	})
}

func TestRESTClient_crud(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}

	crudcontractsConfig := crudcontract.Config[testent.Foo, testent.FooID]{
		MakeEntity:        testent.MakeFoo,
		SupportIDReuse:    true,
		SupportRecreate:   true,
		LazyNotFoundError: true,
	}

	s := testcase.NewSpec(t)
	crudcontract.Creator[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Spec(s)
	crudcontract.Finder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Spec(s)
	crudcontract.ByIDsFinder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Spec(s)
	crudcontract.Updater[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Spec(s)
	crudcontract.Deleter[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Spec(s)
}

func TestRESTClient_FindAll_withDisableStreaming(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	fooAPI := httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](fooRepo)
	srv := httptest.NewServer(fooAPI)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
		HTTPClient:       srv.Client(),
		BaseURL:          srv.URL,
		DisableStreaming: true,
	}

	crudcontractsConfig := crudcontract.Config[testent.Foo, testent.FooID]{
		MakeEntity: testent.MakeFoo,
	}

	crudcontract.AllFinder[testent.Foo, testent.FooID](fooClient, crudcontractsConfig).Test(t)
}

func TestRESTClient_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RESTHandlerFromCRUD[testent.Bar, testent.BarID](barRepo, func(h *httpkit.RESTHandler[testent.Bar, testent.BarID]) {
		h.ScopeAware = true
	})

	fooAPI := httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](fooRepo, func(h *httpkit.RESTHandler[testent.Foo, testent.FooID]) {
		h.ResourceRoutes = httpkit.NewRouter(func(router *httpkit.Router) {
			router.Resource("/bars", barAPI)
		})
	})

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

	crudcontractsConfig := crudcontract.Config[testent.Bar, testent.BarID]{
		MakeContext: func(testing.TB) context.Context {
			return httpkit.WithPathParam(context.Background(), "foo_id", foo.ID.String())
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
		MakeEntity: func(t testing.TB) testent.Bar {
			v := rnd.Make(testent.Bar{}).(testent.Bar)
			v.ID = ""
			v.FooID = foo.ID
			return v
		},
	}

	t.Run("Creator", crudcontract.Creator[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Finder", crudcontract.Finder[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Updater", crudcontract.Updater[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Deleter", crudcontract.Deleter[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
}

func TestRESTClient_Resource_subresource(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	barAPI := httpkit.RESTHandlerFromCRUD(barRepo)

	fooAPI := httpkit.RESTHandlerFromCRUD(fooRepo, func(h *httpkit.RESTHandler[testent.Foo, testent.FooID]) {
		h.ResourceRoutes = httpkit.NewRouter(func(router *httpkit.Router) {
			router.Resource("/bars", barAPI)
		})
	})

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
		HTTPClient: srv.Client(),
		BaseURL:    pathkit.Join(fooClient.BaseURL, ":foo_id", "/bars"),

		WithContext: func(ctx context.Context) context.Context {
			return httpkit.WithPathParam(ctx, "foo_id", foo.ID.String())
		},
	}

	crudcontractsConfig := crudcontract.Config[testent.Bar, testent.BarID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
		MakeEntity: func(t testing.TB) testent.Bar {
			v := rnd.Make(testent.Bar{}).(testent.Bar)
			v.ID = ""
			v.FooID = foo.ID
			return v
		},
	}

	t.Run("Creator", crudcontract.Creator[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Finder", crudcontract.Finder[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Updater", crudcontract.Updater[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
	t.Run("Deleter", crudcontract.Deleter[testent.Bar, testent.BarID](barClient, crudcontractsConfig).Test)
}

var _ = func() struct{} {
	gob.Register(testent.Foo{})
	gob.Register(testent.FooID(""))
	return struct{}{}
}()

func TestRESTClient_withMediaTypeCodecs(t *testing.T) {
	logger.Testing(t)

	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)

	fooAPI := httpkit.RESTHandlerFromCRUD[testent.Foo, testent.FooID](fooRepo, func(h *httpkit.RESTHandler[testent.Foo, testent.FooID]) {
		h.MediaType = GobMediaType
		h.Codecs = httpkit.Codecs{
			GobMediaType: &GobBundle{},
		}
	})

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

		MediaType: GobMediaType,

		Codecs: httpkit.Codecs{
			GobMediaType: &GobBundle{},
		},
	}

	crudcontract.Creator[testent.Foo, testent.FooID](fooClient).Test(t)
	crudcontract.Finder[testent.Foo, testent.FooID](fooClient).Test(t)
	crudcontract.ByIDsFinder[testent.Foo, testent.FooID](fooClient).Test(t)
	crudcontract.Updater[testent.Foo, testent.FooID](fooClient).Test(t)
	crudcontract.Deleter[testent.Foo, testent.FooID](fooClient).Test(t)
}

const GobMediaType = "application/gob"

type GobBundle struct {
	regonce sync.Once
}

// Marshal encodes a value v into a byte slice.
func (c *GobBundle) Marshal(v any) (_ []byte, _ error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal decodes a byte slice into a provided pointer ptr.
func (c *GobBundle) Unmarshal(data []byte, ptr any) (_ error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(ptr); err != nil {
		return err
	}
	return nil
}

func TestRESTClient_bodyReadLimit(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	fooRepo := memory.NewRepository[testent.Foo, testent.FooID](mem)

	foo := rnd.Make(testent.Foo{}).(testent.Foo)
	foo.ID = ""
	crudtest.Create[testent.Foo, testent.FooID](t, fooRepo, context.Background(), &foo)
	fooValPath := pathkit.Join("/", foo.ID.String())

	fooAPI := httpkit.NewRouter()

	// FindAll / FindByIDs
	fooAPI.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]testent.Foo{foo})
	}))

	// DeleteAll
	fooAPI.Delete("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "OK"})
	}))

	// FindByID
	fooAPI.Get(fooValPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(foo)
	}))

	// Update
	fooAPI.Put(fooValPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))

	// DeleteByID
	fooAPI.Delete(fooValPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))

	api := httpkit.NewRouter(func(router *httpkit.Router) {
		router.Mount("/foos", fooAPI)
	})

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	fooClient := httpkit.RESTClient[testent.Foo, testent.FooID]{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL + "/foos",

		MediaType: GobMediaType,

		Codecs: httpkit.Codecs{
			GobMediaType: &GobBundle{},
		},

		BodyReadLimit: 1,
	}

	var err error

	err = fooClient.Create(t.Context(), &foo)
	assert.ErrorIs(t, httpkit.ErrResponseEntityTooLarge, err)

	_, _, err = fooClient.FindByID(t.Context(), foo.ID)
	assert.ErrorIs(t, httpkit.ErrResponseEntityTooLarge, err)

	_, err = iterkit.CollectE(fooClient.FindByIDs(t.Context(), foo.ID))
	assert.ErrorIs(t, httpkit.ErrResponseEntityTooLarge, err)

	err = fooClient.Update(t.Context(), &foo)
	assert.ErrorIs(t, httpkit.ErrResponseEntityTooLarge, err)

	err = fooClient.DeleteByID(t.Context(), foo.ID)
	assert.ErrorIs(t, httpkit.ErrResponseEntityTooLarge, err)
}
