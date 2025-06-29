package httpkit_test

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pathkit"

	. "go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/httpspec"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleRouter() {
	var router httpkit.Router

	router.Namespace("/path", func(r *httpkit.Router) {
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

		r.Resource("foo", httpkit.RESTHandler[Foo, FooID]{
			Mapping: dtokit.Mapping[Foo, FooDTO]{},
			Index: func(ctx context.Context) iter.Seq2[Foo, error] {
				foo := Foo{
					ID:  "42",
					Foo: "foo",
					Bar: "bar",
					Baz: "baz",
				}
				return iterkit.ToSeqE(iterkit.Slice1([]Foo{foo}))
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
		type exampleContextKey struct{}
		r = r.WithContext(context.WithValue(r.Context(), exampleContextKey{}, "example"))
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
	subject := testcase.Let(s, func(t *testcase.T) *httpkit.Router {
		return &httpkit.Router{}
	})

	httpspec.ItBehavesLikeHandlerMiddleware(s, func(t *testcase.T, next http.Handler) http.Handler {
		r := &httpkit.Router{}
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
			t.Must.Equal(httpkit.ErrPathNotFound.Code.String(), errDTO.Type.ID)
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

					routing, ok := internal.RoutingContext.Lookup(lastRequest.Get(t).Context())
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

					routing, ok := internal.RoutingContext.Lookup(lastRequest.Get(t).Context())
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
					t.Must.Equal(httpkit.ErrPathNotFound.Code.String(), errDTO.Type.ID)
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

					pparams := httpkit.PathParams(lastRequest.Get(t).Context())
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
	router := httpkit.Router{}

	router.Handle("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	router.Handle("/bar", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	router.Handle("/baz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	router.Mount("/qux", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	var call = func(path string) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
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
	var router httpkit.Router
	router.Namespace("/top", func(r *httpkit.Router) {
		r.Get("/sub", /* /top/sub */
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))

	})
}

func TestRouter_Handle(t *testing.T) {
	t.Run("behaves like http.ServeMux", func(t *testing.T) {
		var router httpkit.Router
		router.Handle("/path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("endpoint-path"))
		}))

		router.Handle("/path/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("catch-all-path"))
		}))

		t.Run("on matching request for a specific path", func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(
				rnd.Pick([]string{
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
				rnd.Pick([]string{
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
				rnd.Pick([]string{
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
		var router httpkit.Router

		router.Handle("/", // catch-all
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("top-catch-all"))
			}))

		router.Namespace("/top", func(r *httpkit.Router) {
			r.Namespace("/sub", func(r *httpkit.Router) {
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
		var r httpkit.Router
		r.Handle("/foo/bar/baz/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo/bar/baz/qux", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
	})
}

// func TestRouter_Mount(t *testing.T) {
// 	var router httpkit.Router
// }

func TestRouter_Namespace(t *testing.T) {
	var router httpkit.Router
	router.Namespace("/top", func(r *httpkit.Router) {
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
			rnd.Pick([]string{
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

func TestRouter_Sub(t *testing.T) {
	var router httpkit.Router

	sub := router.Sub("/tip/top")
	sub.Handle("/sub", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("handle"))
	}))

	t.Run("on matching request for a specific path", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			"/tip/top/sub",
			nil,
		)

		router.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
		assert.Equal(t, rr.Body.String(), "handle")
	})

	t.Run("on non matching request", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			rnd.Pick([]string{
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
			"/tip/top/oth",
			nil,
		)

		router.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusNotFound)
	})
}

func TestRouter_Sub_withPathParam(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := randomPathPart(t)
	t.OnFail(func() { t.Logf("expected path param: %s", exp) })

	var ro httpkit.Router

	t.OnFail(func() { t.LogPretty(ro.RouteInfo()) })

	sub := ro.Sub("/path/:param")
	sub.Handle("/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := httpkit.PathParams(r.Context())
		t.OnFail(func() { t.Log("expected path param:", exp) })
		assert.Equal(t, params["param"], exp)

		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("handle"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, pathkit.Join("/path", url.PathEscape(exp), "/endpoint"), nil)

	t.OnFail(func() { t.Logf("%s %s", req.Method, req.URL.RequestURI()) })

	ro.ServeHTTP(rr, req)
	assert.Equal(t, rr.Code, http.StatusTeapot)
	assert.Equal(t, rr.Body.String(), "handle")
}

func TestRouter_Handle_withDynamicPathParam(tt *testing.T) {
	t := testcase.NewT(tt)

	rndString := t.Random.StringNWithCharset(5, random.CharsetAlpha()+random.CharsetDigit())
	exp := fmt.Sprintf("%s %s!=/", rndString, "%")

	t.OnFail(func() {
		t.Logf("expected path param: %s", exp)
		t.Log("escaped path:", url.PathEscape(exp))
	})

	var ro httpkit.Router

	t.OnFail(func() { t.LogPretty(ro.RouteInfo()) })

	sub := ro.Sub("/path/:param")
	sub.Handle("/endpoint", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := httpkit.PathParams(r.Context())
		t.OnFail(func() { t.Log("expected path param:", exp) })
		assert.Equal(t, params["param"], exp)

		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("handle"))
	}))

	req := httptest.NewRequest(http.MethodGet, pathkit.Join("/path", url.PathEscape(exp), "/endpoint"), nil)
	rr := httptest.NewRecorder()
	ro.ServeHTTP(rr, req)

	resp := rr.Result()
	assert.Equal(t, resp.StatusCode, http.StatusTeapot)
	assert.Equal(t, string(assert.ReadAll(t, resp.Body)), "handle")
}

func TestNewRouter(t *testing.T) {
	r := httpkit.NewRouter(func(router *httpkit.Router) {
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
	var router httpkit.Router
	router.Get("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Post() {
	var router httpkit.Router
	router.Post("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Put() {
	var router httpkit.Router
	router.Put("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Patch() {
	var router httpkit.Router
	router.Patch("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Connect() {
	var router httpkit.Router
	router.Connect("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Delete() {
	var router httpkit.Router
	router.Delete("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Head() {
	var router httpkit.Router
	router.Head("/foo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func ExampleRouter_Handle() {
	var router httpkit.Router
	var handler http.Handler

	// single endpoint
	router.Handle("/foo", handler)

	// catch all endpoint
	router.Handle("/foo/", handler)
}

func TestRouter_httpVerbs(t *testing.T) {
	t.Run("Get", func(t *testing.T) {
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
		var router httpkit.Router
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
	var r httpkit.Router

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
	var r httpkit.Router
	var top, nested bool

	r.Use(mwWithContextValue("foo", "foo"))

	r.Namespace("/ns", func(r *httpkit.Router) {
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
	var r httpkit.Router
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
	var r httpkit.Router
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
	s := testcase.NewSpec(t)

	var hfn = func(ppkey, ppval string, code int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			val := httpkit.PathParams(r.Context())[ppkey]
			assert.NotEmpty(t, val)
			assert.Equal(t, ppval, val)
			w.WriteHeader(code)
		}
	}

	s.Test("top-level", func(t *testcase.T) {
		var r httpkit.Router
		r.Get("/:id", hfn("id", "foo", 100))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 100)
	})

	s.Test("top-level, but on different method", func(t *testcase.T) {
		var r httpkit.Router
		r.Get("/:id", hfn("id", "foo", 100))
		r.Post("/:nid", hfn("nid", "bar", 200))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/bar", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 200)
	})

	s.Test("nested path with dynamic part", func(t *testcase.T) {
		var r httpkit.Router
		r.Namespace("/:test", func(r *httpkit.Router) {
			r.Get("/", hfn("test", "baz", 300))
		})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/baz", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 300)
	})

	s.Test("nested path with dynamic part that overlaps with dynamic endpoint", func(t *testcase.T) {
		var r httpkit.Router
		r.Get("/:id", hfn("id", "foo", 100))
		r.Namespace("/:test", func(r *httpkit.Router) {
			r.Get("/", hfn("test", "baz", 400))
		})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/baz", nil)
		r.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, 400)
	})

	s.Test("query parameter might have escaped characters value", func(t *testcase.T) {
		var ro httpkit.Router

		exp := randomPathPart(t)

		ro.Namespace("/path/:val", func(ro *httpkit.Router) {
			ro.Get("/", hfn("val", exp, http.StatusTeapot))
		})

		target := pathkit.Join("/path/", url.PathEscape(exp))
		t.Log(target, exp, url.PathEscape(exp))
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rr := httptest.NewRecorder()
		ro.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusTeapot)
	})
}

func randomPathPart(tb testing.TB) string {
	t := testcase.ToT(&tb)
	return random.Unique(t.Random.String, ".")
}

var nullHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

func ExampleGetRouteInfo_httpServeMux() {
	mux := http.NewServeMux()
	mux.Handle("/foo", nullHandler)
	mux.Handle("/bar/", nullHandler)

	_ = httpkit.GetRouteInfo(mux)
	// ALL /foo
	// ALL /bar/
}

func ExampleGetRouteInfo_httpkitRouter() {
	var ro httpkit.Router
	ro.Get("/test", nullHandler)

	mux := http.NewServeMux()
	mux.Handle("/foo", nullHandler)
	mux.Handle("/bar/", nullHandler)

	ro.Mount("/mux", mux)

	_ = httpkit.GetRouteInfo(mux)
	// GET /test
	// ALL /foo
	// ALL /bar/
}

func TestGetRouteInfo(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {

		var ro httpkit.Router

		var expectedRouteCount int
		var assertCurRouteCount = func() {
			t.Helper()
			assert.Equal(t, len(ro.RouteInfo()), expectedRouteCount)
		}
		var assertIncRouteCountBy = func(n int) {
			t.Helper()
			expectedRouteCount += n
			assertCurRouteCount()
		}
		var assertIncRouteCount = func() {
			t.Helper()
			expectedRouteCount++
			assertCurRouteCount()
		}

		assertCurRouteCount()

		ro.Get("/root1", nullHandler)
		assertIncRouteCount()

		ro.Delete("/root2", nullHandler)
		assertIncRouteCount()

		ro.Namespace("/nested/namespace", func(ro *httpkit.Router) {
			ro.Get("/", nullHandler)
			assertIncRouteCount()
			ro.Get("/ep1", nullHandler)
			assertIncRouteCount()
			ro.Get("/ep2", nullHandler)
			assertIncRouteCount()
			ro.Handle("/baz/1", nullHandler)
			assertIncRouteCount()
			ro.Handle("/baz/2/", nullHandler)
			assertIncRouteCount()
		})

		assertCurRouteCount()

		ro.Namespace("/qux", func(ro *httpkit.Router) {
			ro.Get("/ep1", nullHandler)
			assertIncRouteCount()
			ro.Get("/ep2", nullHandler)
			assertIncRouteCount()
		})

		ro.Namespace("/http-methods-are-ordered-by-crud", func(ro *httpkit.Router) {
			ro.Get("/", nullHandler)
			assertIncRouteCount()
			ro.Post("/", nullHandler)
			assertIncRouteCount()
			ro.Put("/", nullHandler)
			assertIncRouteCount()
			ro.Patch("/", nullHandler)
			assertIncRouteCount()
			ro.Delete("/", nullHandler)
			assertIncRouteCount()
			ro.Connect("/", nullHandler)
			assertIncRouteCount()
		})

		sub := ro.Sub("/sub")
		assertCurRouteCount()
		sub.Get("/pe", nullHandler)
		assertIncRouteCount()

		ro.Namespace("/resource/:id", func(ro *httpkit.Router) {
			ro.Get("/", nullHandler)
			assertIncRouteCount()
		})

		ro.Mount("/mount-point-handler", nullHandler)
		assertIncRouteCount()

		mux := http.NewServeMux()
		mux.Handle("/pattern1", nullHandler)
		mux.Handle("/pattern2/", nullHandler)
		ro.Mount("/mux", mux)
		assertIncRouteCountBy(2)

		muxWithRouter := http.NewServeMux()

		var ro2 httpkit.Router
		ro2.Get("/httpkitRouterUnderMuxHandle", nullHandler)
		muxWithRouter.Handle("/path", &ro2)
		ro.Mount("/muxWithRouter", muxWithRouter)
		assertIncRouteCountBy(1)

		routes := ro.RouteInfo()

		assert.Equal(t, strings.Split(routes.String(), "\n"), []string{
			"POST    /http-methods-are-ordered-by-crud",
			"GET     /http-methods-are-ordered-by-crud",
			"PUT     /http-methods-are-ordered-by-crud",
			"PATCH   /http-methods-are-ordered-by-crud",
			"DELETE  /http-methods-are-ordered-by-crud",
			"CONNECT /http-methods-are-ordered-by-crud",
			"ALL     /mount-point-handler",
			"ALL     /mux/pattern1",
			"ALL     /mux/pattern2",
			"GET     /muxWithRouter/path/httpkitRouterUnderMuxHandle",
			"GET     /nested/namespace",
			"GET     /nested/namespace/ep1",
			"GET     /nested/namespace/ep2",
			"ALL     /nested/namespace/baz/1",
			"ALL     /nested/namespace/baz/2",
			"GET     /qux/ep1",
			"GET     /qux/ep2",
			"GET     /resource/:id",
			"GET     /root1",
			"DELETE  /root2",
			"GET     /sub/pe",
		})

	})

	t.Run("RESTHandler", func(t *testing.T) {
		var ro httpkit.Router

		ro.Resource("foos", httpkit.RESTHandler[Foo, FooID]{
			Create:     func(ctx context.Context, ptr *Foo) error { return nil },
			Index:      func(ctx context.Context) iter.Seq2[Foo, error] { return nil },
			Show:       func(ctx context.Context, id FooID) (ent Foo, found bool, err error) { return },
			Update:     func(ctx context.Context, ptr *Foo) error { return nil },
			Destroy:    func(ctx context.Context, id FooID) error { return nil },
			DestroyAll: func(ctx context.Context) error { return nil },

			ResourceRoutes: httpkit.NewRouter(func(r *httpkit.Router) {
				r.Get("/hello", nullHandler)
			}),
		})

		assert.Equal(t, strings.Split(ro.RouteInfo().String(), "\n"), []string{
			"POST   /foos",
			"GET    /foos",
			"DELETE /foos",
			"GET    /foos/:id",
			"PUT    /foos/:id",
			"DELETE /foos/:id",
			"GET    /foos/:id/hello",
		})
	})

	t.Run("Register", func(t *testing.T) {
		exp := httpkit.RouteInfo{httpkit.PathInfo{Method: http.MethodGet, Path: "/the/path"}}

		unreg := httpkit.RegisterRouteInformer(func(v StubHTTPHandler1) httpkit.RouteInfo {
			return exp
		})
		defer unreg()

		got := httpkit.GetRouteInfo(StubHTTPHandler1{})
		assert.Equal(t, exp, got)

		unreg()

		got = httpkit.GetRouteInfo(StubHTTPHandler1{})
		assert.Equal(t, got, httpkit.RouteInfo{httpkit.PathInfo{Method: "ALL", Path: "/"}})
	})

	t.Run("RouteInformer", func(t *testing.T) {
		unreg := httpkit.RegisterRouteInformer(func(v StubHTTPHandler2) httpkit.RouteInfo {
			return v.StubRouteInfo
		})
		defer unreg()

		exp := httpkit.RouteInfo{httpkit.PathInfo{Method: http.MethodGet, Path: "/the/path"}}
		got := httpkit.GetRouteInfo(StubHTTPHandler2{StubRouteInfo: exp})
		assert.Equal(t, exp, got)
	})
}

type StubHTTPHandler1 struct{}

func (f StubHTTPHandler1) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	panic("not implemented") // TODO: Implement
}

type StubHTTPHandler2 struct {
	StubRouteInfo httpkit.RouteInfo
}

func (f StubHTTPHandler2) RouteInfo() httpkit.RouteInfo {
	return f.StubRouteInfo
}

func (f StubHTTPHandler2) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	panic("not implemented") // TODO: Implement
}
