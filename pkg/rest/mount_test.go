package rest_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/pathutil"
	"github.com/adamluzsi/frameless/pkg/rest"
	"github.com/adamluzsi/frameless/pkg/rest/internal"
	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestMount(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		serveMux = testcase.Let(s, func(t *testcase.T) *http.ServeMux { return http.NewServeMux() })
		pattern  = testcase.LetValue[string](s, "/path")
		lastReq  = testcase.LetValue[*http.Request](s, nil)
		handler  = testcase.Let[http.Handler](s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lastReq.Set(t, r)
				w.WriteHeader(http.StatusTeapot)
			})
		})
	)
	act := func(t *testcase.T) {
		rest.Mount(
			serveMux.Get(t),
			pattern.Get(t),
			handler.Get(t),
		)
	}

	makeRequest := func(t *testcase.T, path string) *http.Response {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		serveMux.Get(t).ServeHTTP(w, r)
		return w.Result()
	}

	s.Then("it will mount the handler and set the mount point in the request context", func(t *testcase.T) {
		act(t)

		response := makeRequest(t, "/path/sub")
		t.Must.Equal(http.StatusTeapot, response.StatusCode)
		t.Must.NotNil(lastReq.Get(t))
		routing, ok := internal.LookupRouting(lastReq.Get(t).Context())
		t.Must.True(ok)
		t.Must.Equal("/sub", routing.Path)
	})

	s.When("handler is a rest.Handler", func(s *testcase.Spec) {
		repo := testcase.Let(s, func(t *testcase.T) *memory.Repository[Foo, FooID] {
			return memory.NewRepository[Foo, FooID](memory.NewMemory())
		})

		ent := testcase.Let(s, func(t *testcase.T) Foo {
			v := Foo{Foo: t.Random.Int()}
			crudtest.Create[Foo, FooID](t, repo.Get(t), context.Background(), &v)
			return v
		}).EagerLoading(s)

		handler.Let(s, func(t *testcase.T) http.Handler {
			return rest.Handler[Foo, FooID, FooDTO]{
				Resource: repo.Get(t),
				Mapping:  FooMapping{},
				Router: rest.NewRouter(func(r *rest.Router) {
					r.Mount("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						lastReq.Set(t, r)
						w.WriteHeader(http.StatusTeapot)
					}))
				}),
			}
		})

		s.Then("the handler is properly propagated", func(t *testcase.T) {
			act(t)
			resp := makeRequest(t, pathutil.Join(pattern.Get(t), strconv.Itoa(ent.Get(t).ID), "test", "foo"))
			t.Must.Equal(http.StatusTeapot, resp.StatusCode)
			t.Must.NotNil(lastReq.Get(t))

			routing, ok := internal.LookupRouting(lastReq.Get(t).Context())
			t.Must.True(ok)
			t.Must.Equal("/foo", routing.Path)
		})
	})

	s.When("mount pattern is not in a clean format", func(s *testcase.Spec) {
		pattern.LetValue(s, "//a/b/c/..")

		s.Then("mounted to the correct path", func(t *testcase.T) {
			act(t)

			response := makeRequest(t, "/a/b")
			t.Must.Equal(http.StatusTeapot, response.StatusCode)
		})
	})

	s.When(`pattern lack trailing slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `/path0`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path0/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)

			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern lack leading slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `path1/`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path1/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern lack leading and trailing slash`, func(s *testcase.Spec) {
		pattern.LetValue(s, `path2`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/path2/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})

	s.When(`pattern includes nested path`, func(s *testcase.Spec) {
		pattern.LetValue(s, `/test/this/out/`)

		s.Then(`it will be still available to call even for the under paths`, func(t *testcase.T) {
			act(t)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, `/test/this/out/123`, nil)
			serveMux.Get(t).ServeHTTP(w, r)
			t.Must.Equal(http.StatusTeapot, w.Result().StatusCode)
		})
	})
}

func TestMountPoint(tt *testing.T) {
	s := testcase.NewSpec(tt)
	t := testcase.NewT(tt, s)

	charset := random.CharsetDigit()
	mountPoint := rest.Path(fmt.Sprintf("/%s/%s/%s",
		t.Random.StringNC(3, charset),
		t.Random.StringNC(3, charset),
		t.Random.StringNC(3, charset),
	))
	var gotReq *http.Request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(http.StatusTeapot)
	})

	gotHandler := rest.MountPoint(mountPoint, handler)

	remainingPath := "/foo/bar/baz"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, string(mountPoint)+remainingPath, nil)
	gotHandler.ServeHTTP(rr, req)
	t.Must.Equal(http.StatusTeapot, rr.Code)
	t.Must.NotNil(gotReq)
	rc, ok := internal.LookupRouting(gotReq.Context())
	t.Must.True(ok)
	t.Must.Equal(remainingPath, rc.Path)
}
