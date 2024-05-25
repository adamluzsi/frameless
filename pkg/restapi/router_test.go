package restapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/restapi/rfc7807"
	"go.llib.dev/frameless/ports/iterators"
	. "go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/httpspec"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func Example() {

	var r restapi.Router

	r.Namespace("/path", func(r *restapi.Router) {
		r.Use(SampleMiddleware)

		r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		}))

		r.Post("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		}))
	})

	type FooIDContextKey struct{}
	// Register a restful resource
	// 		GET 	/foos
	// 		POST 	/foos
	// 		GET 	/foos/:id
	// 		PUT 	/foos/:id
	// 		DELETE	/foos/:id
	//
	r.Resource("/foos", restapi.Resource[Foo, FooID]{
		// Mapping between Foo and FooDTO.
		// FooDTO in this case used to communicate with the API callers.
		Mapping:      restapi.DTOMapping[Foo, FooDTO]{},
		IDContextKey: FooIDContextKey{},
		SubRoutes: restapi.NewRouter(func(subRoutes *restapi.Router) {
			subRoutes.Get("/activate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fooID := r.Context().Value(FooIDContextKey{}).(FooID)
				// activate foo by foo_id
				logger.Debug(r.Context(), "activating foo", logger.Field("foo_id", fooID))
			}))
		}),
	}.WithCRUD(&FooRepository{}))

}

func ExampleRouter() {
	var router restapi.Router

	router.Namespace("/path", func(r *restapi.Router) {
		r.Use(SampleMiddleware)

		r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

		r.Post("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

		r.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// sub route catch-all handle
		}))

		r.Resource("foo", restapi.Resource[Foo, FooID]{
			Mapping: restapi.DTOMapping[Foo, FooDTO]{},
			Index: func(ctx context.Context, query url.Values) (iterators.Iterator[Foo], error) {
				foo := Foo{
					ID:  "42",
					Foo: "foo",
					Bar: "bar",
					Baz: "baz",
				}
				return iterators.Slice([]Foo{foo}), nil
			},
		})
	})

	router.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// handler that catches all requests that doesn't match anything directly
	}))

	router.Handle("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /foo endpoint for all methods
	}))
}

func SampleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "example", "example"))
		next.ServeHTTP(w, r)
	})
}

func TestRouter(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		lastRequest = testcase.LetValue[*http.Request](s, nil)
		handler     = testcase.Let(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lastRequest.Set(t, r)
				w.WriteHeader(http.StatusTeapot)
			})
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) *restapi.Router {
		return &restapi.Router{}
	})

	httpspec.ItBehavesLikeHandlerMiddleware(s, func(t *testcase.T, next http.Handler) http.Handler {
		r := &restapi.Router{}
		r.Mount("/", next)
		return r
	})

	s.Describe(".ServeHTTP", func(s *testcase.Spec) {
		var (
			method = testcase.LetValue(s, http.MethodGet)
			path   = testcase.LetValue(s, "/")
			body   = testcase.LetValue[[]byte](s, nil)
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method.Get(t), path.Get(t), bytes.NewReader(body.Get(t)))
			r.Header.Set("Content-Type", "application/json")
			subject.Get(t).ServeHTTP(w, r)
			return w
		}

		s.Then("it will reply 404, path not found on empty router", func(t *testcase.T) {
			rr := act(t)
			t.Must.Equal(http.StatusNotFound, rr.Code)

			errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
			t.Must.NotEmpty(errDTO)
			t.Must.Equal(restapi.ErrPathNotFound.ID.String(), errDTO.Type.ID)
		})

		ThenHandlerIsReachable := func(s *testcase.Spec, registeredPath testcase.Var[string]) {
			s.When("the request path is the registered path", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) string {
					return registeredPath.Get(t)
				})

				s.Then("it proxy the call to the registered handler", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusTeapot, rr.Code)
					t.Must.NotNil(lastRequest.Get(t))
				})

				s.Then("it configure the path to not include the routed path", func(t *testcase.T) {
					act(t)
					t.Must.NotNil(lastRequest.Get(t))
					lastRequest.Get(t)

					routing, ok := internal.LookupRouting(lastRequest.Get(t).Context())
					t.Must.True(ok)
					t.Must.Equal("/", routing.PathLeft)
				})
			})

			s.When("the request path contains the registered path", func(s *testcase.Spec) {
				const pathRest = "/foo/bar/baz"
				path.Let(s, func(t *testcase.T) string {
					return string(registeredPath.Get(t)) + pathRest
				})

				s.Then("it proxy the call to the registered http.Handler", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusTeapot, rr.Code)
					t.Must.NotNil(lastRequest.Get(t))
				})

				s.Then("it configure the path to not include the routed path", func(t *testcase.T) {
					act(t)
					t.Must.NotNil(lastRequest.Get(t))
					lastRequest.Get(t)

					routing, ok := internal.LookupRouting(lastRequest.Get(t).Context())
					t.Must.True(ok)
					t.Must.Equal(pathRest, routing.PathLeft)
				})
			})
		}

		ThenItRoutesTheRequestToTheHandler := func(s *testcase.Spec, registeredPath testcase.Var[string]) {
			ThenHandlerIsReachable(s, registeredPath)

			s.When("the request path doesn't match the registered path", func(s *testcase.Spec) {
				const pathPrefix = "/foo/bar/baz"
				path.Let(s, func(t *testcase.T) string {
					return pathkit.Join(pathPrefix, registeredPath.Get(t))
				})

				s.Then("it return path not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrPathNotFound.ID.String(), errDTO.Type.ID)
				})
			})
		}

		s.When("routes are registered", func(s *testcase.Spec) {
			registeredPath := testcase.Let(s, func(t *testcase.T) string {
				path := t.Random.StringNC(5, random.CharsetAlpha())
				return fmt.Sprintf("/%s", url.PathEscape(path))
			})
			s.Before(func(t *testcase.T) {
				subject.Get(t).On(method.Get(t), registeredPath.Get(t), handler.Get(t))
			})

			ThenItRoutesTheRequestToTheHandler(s, registeredPath)
		})

		s.When("path is registered", func(s *testcase.Spec) {
			registeredPath := testcase.Let(s, func(t *testcase.T) string {
				path := t.Random.StringNC(5, random.CharsetAlpha())
				return fmt.Sprintf("/%s", url.PathEscape(path))
			})
			s.Before(func(t *testcase.T) {
				subject.Get(t).Mount(registeredPath.Get(t), handler.Get(t))
			})

			ThenHandlerIsReachable(s, registeredPath)

			s.And("registered path is a path variable section", func(s *testcase.Spec) {
				registeredPath.LetValue(s, "/:pathvar")
				path.Let(s, func(t *testcase.T) string {
					return registeredPath.Get(t)
				})

				ThenHandlerIsReachable(s, registeredPath)

				s.Then("the path variable is available", func(t *testcase.T) {
					rr := act(t)
					assert.Equal(t, rr.Code, http.StatusTeapot)

					pparams := restapi.PathParams(lastRequest.Get(t).Context())
					assert.NotEmpty(t, pparams)
					assert.Equal(t,
						strings.TrimPrefix(registeredPath.Get(t), "/"),
						pparams["pathvar"])
				})

				s.And("if a non-dynamic path is also registered", func(s *testcase.Spec) {
					fixPathHandlerResponseCode := testcase.Let(s, func(t *testcase.T) int {
						return random.Pick(t.Random,
							http.StatusInternalServerError,
							http.StatusUnauthorized,
							http.StatusNotAcceptable,
						)
					})
					const endpointPath = "/fixpath"
					s.Before(func(t *testcase.T) {
						subject.Get(t).Mount(endpointPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(fixPathHandlerResponseCode.Get(t))
						}))
					})

					ThenHandlerIsReachable(s, registeredPath)

					s.Then("the non-dynamic path can be accessed, and not interpreted as a path parameter", func(t *testcase.T) {
						w := httptest.NewRecorder()
						r := httptest.NewRequest(method.Get(t), endpointPath, nil)
						r.Header.Set("Content-Type", "application/json")
						subject.Get(t).ServeHTTP(w, r)
						assert.Equal(t, fixPathHandlerResponseCode.Get(t), w.Code)
					})
				})
			})
		})
	})
}

func TestRouter_race(t *testing.T) {
	router := restapi.Router{}

	router.Handle("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	router.Handle("/bar", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	router.Handle("/baz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	router.Mount("/qux", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	var call = func(path string) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo", nil)
		router.ServeHTTP(rr, req)
	}

	testcase.Race(
		func() { call("/foo") },
		func() { call("/bar") },
		func() { call("/baz") },
		func() { call("/qux") },
	)
}

func ExampleRouter_Namespace() {
	var router restapi.Router
	router.Namespace("/top", func(r *restapi.Router) {
		r.Get("/sub", /* /top/sub */
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

	})
}

func TestRouter_Handle(t *testing.T) {
	t.Run("behaves like http.ServeMux", func(t *testing.T) {
		var router restapi.Router
		router.Handle("/path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("endpoint-path"))
		}))

		router.Handle("/path/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("catch-all-path"))
		}))

		t.Run("on matching request for a specific path", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(
				rnd.SliceElement([]string{
					http.MethodGet,
					http.MethodHead,
					http.MethodPost,
					http.MethodPut,
					http.MethodPatch,
					http.MethodDelete,
					http.MethodConnect,
					http.MethodOptions,
					http.MethodTrace,
				}).(string),
				"/path",
				nil,
			)

			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Body.String(), "endpoint-path")
		})

		t.Run("on matching request for sub paths", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(
				rnd.SliceElement([]string{
					http.MethodGet,
					http.MethodHead,
					http.MethodPost,
					http.MethodPut,
					http.MethodPatch,
					http.MethodDelete,
					http.MethodConnect,
					http.MethodOptions,
					http.MethodTrace,
				}).(string),
				"/path/oth",
				nil,
			)

			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Body.String(), "catch-all-path")
		})

		t.Run("on non matching request", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(
				rnd.SliceElement([]string{
					http.MethodGet,
					http.MethodHead,
					http.MethodPost,
					http.MethodPut,
					http.MethodPatch,
					http.MethodDelete,
					http.MethodConnect,
					http.MethodOptions,
					http.MethodTrace,
				}).(string),
				"/not-endpoint-path",
				nil,
			)

			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		})
	})
	t.Run("mux that matched the most of the path is used in case of no direct endpoint", func(t *testing.T) {
		var router restapi.Router

		router.Handle("/", // catch-all
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("top-catch-all"))
			}))

		router.Namespace("/top", func(r *restapi.Router) {
			r.Namespace("/sub", func(r *restapi.Router) {
				r.Handle("/", // sub-catch-all
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte("sub-catch-all"))
					}))

				r.On(http.MethodGet, "/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("endpoint"))
				}))
			})
		})

		t.Run("endpoint can be reached", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/top/sub/endpoint", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusOK)
			assert.Equal(t, rr.Body.String(), "endpoint")
		})

		t.Run("closest matching catch-all is used", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/top/sub/catch-all", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusOK)
			assert.Equal(t, rr.Body.String(), "sub-catch-all")
		})

		t.Run("top level catch-all is used if present", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/top/oth/nok", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusOK)
			assert.Equal(t, rr.Body.String(), "top-catch-all")
		})
	})
	t.Run("Handle call with non root path", func(t *testing.T) {
		var r restapi.Router
		r.Handle("/foo/bar/baz/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo/bar/baz/qux", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
	})
}

func TestRouter_Namespace(t *testing.T) {
	var router restapi.Router
	router.Namespace("/top", func(r *restapi.Router) {
		r.Handle("/sub", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("handle"))
		}))
	})

	t.Run("on matching request for a specific path", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			"/top/sub",
			nil,
		)

		router.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
		assert.Equal(t, rr.Body.String(), "handle")
	})

	t.Run("on non matching request", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			rnd.SliceElement([]string{
				http.MethodGet,
				http.MethodHead,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete,
				http.MethodConnect,
				http.MethodOptions,
				http.MethodTrace,
			}).(string),
			"/top/oth",
			nil,
		)

		router.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusNotFound)
	})
}

func TestNewRouter(t *testing.T) {
	r := restapi.NewRouter(func(router *restapi.Router) {
		router.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(100)
		}))
		router.Get("/path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(101)
		}))
	})

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	r.ServeHTTP(rr1, req1)
	assert.Equal(t, rr1.Code, 100)

	req2 := httptest.NewRequest(http.MethodGet, "/path", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	assert.Equal(t, rr2.Code, 101)
}

func ExampleRouter_Get() {
	var router restapi.Router
	router.Get("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Post() {
	var router restapi.Router
	router.Post("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Put() {
	var router restapi.Router
	router.Put("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Patch() {
	var router restapi.Router
	router.Patch("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Connect() {
	var router restapi.Router
	router.Connect("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Delete() {
	var router restapi.Router
	router.Delete("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Head() {
	var router restapi.Router
	router.Head("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Handle() {
	var router restapi.Router
	var handler http.Handler

	// single endpoint
	router.Handle("/foo", handler)

	// catch all endpoint
	router.Handle("/foo/", handler)
}

func TestRouter_httpVerbs(t *testing.T) {
	t.Run("Get", func(t *testing.T) {
		var router restapi.Router
		router.Get("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Post", func(t *testing.T) {
		var router restapi.Router
		router.Post("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Put", func(t *testing.T) {
		var router restapi.Router
		router.Put("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Patch", func(t *testing.T) {
		var router restapi.Router
		router.Patch("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Head", func(t *testing.T) {
		var router restapi.Router
		router.Head("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodHead, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Delete", func(t *testing.T) {
		var router restapi.Router
		router.Delete("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Connect", func(t *testing.T) {
		var router restapi.Router
		router.Connect("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodConnect, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Options", func(t *testing.T) {
		var router restapi.Router
		router.Options("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodOptions, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("Trace", func(t *testing.T) {
		var router restapi.Router
		router.Trace("/foo",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodTrace, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/foo", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
	t.Run("root path", func(t *testing.T) {
		var router restapi.Router
		router.Get("/",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusTeapot)
		}
		{
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			router.ServeHTTP(rr, req)
			assert.Equal(t, rr.Code, http.StatusNotFound)
		}
	})
}

func TestRouter_Use(t *testing.T) {
	var r restapi.Router

	r.Use(mwWithContextValue("foo", "foo"))

	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		assert.Equal[any](t, "foo", r.Context().Value("foo"))
	}))

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
	}
}

func TestRouter_Use_nesting(t *testing.T) {
	var r restapi.Router
	var top, nested bool

	r.Use(mwWithContextValue("foo", "foo"))

	r.Namespace("/ns", func(r *restapi.Router) {
		r.Use(mwWithContextValue("bar", "bar"))

		r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			nested = true
			assert.Equal[any](t, "foo", r.Context().Value("foo"))
			assert.Equal[any](t, "bar", r.Context().Value("bar"))
		}))
	})

	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		assert.Equal[any](t, "foo", r.Context().Value("foo"))
		assert.Nil(t, r.Context().Value("bar"))
		top = true
	}))

	{
		t.Log("top level endpoints only have the middlewares from the top")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
		assert.True(t, top)
	}
	{
		t.Log("nested endpoints only have the middlewares from their scope")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ns", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
		assert.True(t, nested)
	}
}

func TestRouter_Use_mux(t *testing.T) {
	var r restapi.Router
	r.Use(mwWithContextValue("key", "val"))

	r.Handle("/path/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal[any](t, r.Context().Value("key"), "val")
		w.WriteHeader(http.StatusTeapot)
	}))

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/path/goes/here", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
	}
}

func TestRouter_Use_404(t *testing.T) {
	var r restapi.Router
	var ok bool

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok = true
			next.ServeHTTP(w, r)
		})
	})

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/path/not/found", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusNotFound)
		assert.True(t, ok)
	}
}

func mwWithContextValue(key, value any) httpkit.MiddlewareFactoryFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), key, value))
			next.ServeHTTP(w, r)
		})
	}
}

func TestRouter_withPathParams(t *testing.T) {
	var hfn = func(ppkey, ppval string, code int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			val := restapi.PathParams(r.Context())[ppkey]
			assert.NotEmpty(t, val)
			assert.Equal(t, ppval, val)
			w.WriteHeader(code)
		}
	}

	t.Run("top-level", func(t *testing.T) {
		var r restapi.Router
		r.Get("/:id", hfn("id", "foo", 100))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 100)
	})
	t.Run("top-level, but on different method", func(t *testing.T) {
		var r restapi.Router
		r.Get("/:id", hfn("id", "foo", 100))
		r.Post("/:nid", hfn("nid", "bar", 200))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/bar", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 200)
	})
	t.Run("nested path with dynamic part", func(t *testing.T) {
		var r restapi.Router
		r.Namespace("/:test", func(r *restapi.Router) {
			r.Get("/", hfn("test", "baz", 300))
		})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/baz", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 300)
	})
	t.Run("nested path with dynamic part that overlaps with dynamic endpoint", func(t *testing.T) {
		var r restapi.Router
		r.Get("/:id", hfn("id", "foo", 100))
		r.Namespace("/:test", func(r *restapi.Router) {
			r.Get("/", hfn("test", "baz", 400))
		})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/baz", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 400)
	})
}
