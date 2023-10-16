package restapi_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/restapi/rfc7807"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/httpspec"
	"go.llib.dev/testcase/random"
)

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

		ThenRouteTheRequests := func(s *testcase.Spec, registeredPath testcase.Var[string]) {
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

			s.When("the request path is the registered path", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) string {
					return registeredPath.Get(t)
				})

				s.Then("it proxy the call to the registeredHandler", func(t *testcase.T) {
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
					t.Must.Equal("/", routing.Path)
				})
			})

			s.When("the request path contains the registered path", func(s *testcase.Spec) {
				const pathRest = "/foo/bar/baz"
				path.Let(s, func(t *testcase.T) string {
					return string(registeredPath.Get(t)) + pathRest
				})

				s.Then("it proxy the call to the registeredHandler", func(t *testcase.T) {
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
					t.Must.Equal(pathRest, routing.Path)
				})
			})
		}

		s.When("Routes are registered with .RegisterRoutes", func(s *testcase.Spec) {
			registeredPath := testcase.Let(s, func(t *testcase.T) restapi.Path {
				path := t.Random.StringNC(5, random.CharsetAlpha())
				return fmt.Sprintf("/%s", url.PathEscape(path))
			})
			s.Before(func(t *testcase.T) {
				subject.Get(t).MountRoutes(restapi.Routes{
					registeredPath.Get(t): handler.Get(t),
				})
			})

			ThenRouteTheRequests(s, registeredPath)
		})

		s.When("path is registered", func(s *testcase.Spec) {
			registeredPath := testcase.Let(s, func(t *testcase.T) restapi.Path {
				path := t.Random.StringNC(5, random.CharsetAlpha())
				return fmt.Sprintf("/%s", url.PathEscape(path))
			})
			s.Before(func(t *testcase.T) {
				subject.Get(t).Mount(registeredPath.Get(t), handler.Get(t))
			})

			ThenRouteTheRequests(s, registeredPath)
		})
	})
}

func TestRouter_race(t *testing.T) {
	router := restapi.Router{}

	registerRoutes := func() {
		router.MountRoutes(restapi.Routes{
			"/foo": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			"/bar": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			"/baz": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		})
	}
	registerRoute := func() {
		router.Mount("/qux", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	}

	testcase.Race(
		registerRoutes, registerRoutes, registerRoutes,
		registerRoute, registerRoute, registerRoute,
	)
}
