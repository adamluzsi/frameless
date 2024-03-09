package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/restapi/rfc7807"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/httpspec"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ExampleResource() {
	fooRepository := memory.NewRepository[Foo, FooID](memory.NewMemory())
	fooRestfulResource := restapi.Resource[Foo, FooID]{
		Create: fooRepository.Create,
		Index: func(ctx context.Context, query url.Values) (iterators.Iterator[Foo], error) {
			foos := fooRepository.FindAll(ctx)

			if bt := query.Get("bigger"); bt != "" {
				bigger, err := strconv.Atoi(bt)
				if err != nil {
					return nil, err
				}
				foos = iterators.Filter(foos, func(foo Foo) bool {
					return bigger < foo.Foo
				})
			}

			return foos, nil
		},

		Show: fooRepository.FindByID,

		Update: func(ctx context.Context, id FooID, ptr *Foo) error {
			ptr.ID = id
			return fooRepository.Update(ctx, ptr)
		},
		Destroy: fooRepository.DeleteByID,

		Mapping: restapi.Mapping[Foo]{
			restapi.JSON: restapi.DTOMapping[Foo, FooDTO]{},
		},
	}

	mux := http.NewServeMux()
	restapi.Mount(mux, "/foos", fooRestfulResource)
}

func TestResource_ServeHTTP(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(func(t *testcase.T) { logger.LogWithTB(t) })

	var (
		mdb = testcase.Let(s, func(t *testcase.T) *memory.Repository[Foo, FooID] {
			m := memory.NewMemory()
			return memory.NewRepository[Foo, FooID](m)
		})
		resource = testcase.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, FooID] {
			return mdb.Get(t)
		})
		lastSubResourceRequest = testcase.LetValue[*http.Request](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
		return restapi.MakeCRUDResource[Foo, FooID](resource.Get(t), restapi.Resource[Foo, FooID]{
			Serialization: restapi.Serialization[Foo, FooID]{
				Serializers: map[restapi.MIMEType]restapi.Serializer{
					restapi.JSON: restapi.JSONSerializer{},
				},
				IDConverter: restapi.IDConverter[FooID]{},
			},
			EntityRoutes: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle all routes with a simple HandlerFunc
				lastSubResourceRequest.Set(t, r)
				http.Error(w, "", http.StatusTeapot)
			}),
		})

	})

	GivenWeHaveStoredFooDTO := func(s *testcase.Spec) testcase.Var[FooDTO] {
		return testcase.Let(s, func(t *testcase.T) FooDTO {
			// create ent and persist
			ent := Foo{Foo: t.Random.Int()}
			t.Must.NoError(mdb.Get(t).Create(context.Background(), &ent))
			t.Defer(mdb.Get(t).DeleteByID, context.Background(), ent.ID)
			// map ent to DTO
			dto, err := FooMapping{}.MapDTO(context.Background(), ent)
			t.Must.NoError(err)
			return dto
		}).EagerLoading(s)
	}

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

		ThenNotAllowed := func(s *testcase.Spec) {
			s.Then("it will respond with 405, page not found", func(t *testcase.T) {
				rr := act(t)
				t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)
				errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
				t.Must.NotEmpty(errDTO)
				t.Must.Equal(restapi.ErrMethodNotAllowed.ID.String(), errDTO.Type.ID)
			})
		}

		s.Describe(`#index`, func(s *testcase.Spec) {
			method.LetValue(s, http.MethodGet)
			path.LetValue(s, `/`)

			s.Then(`it will return an empty result`, func(t *testcase.T) {
				rr := act(t)
				t.Must.NotEmpty(rr.Body.String())
				t.Must.Empty(respondsWithJSON[[]FooDTO](t, rr))
			})

			s.When("we have entity in the repository", func(s *testcase.Spec) {
				dto := GivenWeHaveStoredFooDTO(s)

				s.Then("it will return back the entity", func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					t.Must.Contain(respondsWithJSON[[]FooDTO](t, rr), dto.Get(t))
				})
			})

			s.When("we have multiple entities in the repository", func(s *testcase.Spec) {
				dto1 := GivenWeHaveStoredFooDTO(s)
				dto2 := GivenWeHaveStoredFooDTO(s)
				dto3 := GivenWeHaveStoredFooDTO(s)

				s.Then("it will return back the entity", func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					t.Must.ContainExactly([]FooDTO{dto1.Get(t), dto2.Get(t), dto3.Get(t)},
						respondsWithJSON[[]FooDTO](t, rr))
				})
			})

			s.When("FindAll is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, FooID] {
					return struct{ crud.ByIDFinder[Foo, FooID] }{ByIDFinder: mdb.Get(t)}
				})

				s.Then("it will respond with StatusMethodNotAllowed, page not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrMethodNotAllowed.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("index is provided", func(s *testcase.Spec) {
				override := testcase.Let[func(query url.Values) iterators.Iterator[Foo]](s, nil)

				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					h := subject.Super(t)
					h.Index = func(ctx context.Context, query url.Values) (iterators.Iterator[Foo], error) {
						return override.Get(t)(query), nil
					}
					return h
				})

				s.And("it returns values without an issue", func(s *testcase.Spec) {
					foo := testcase.Let(s, func(t *testcase.T) Foo {
						return Foo{
							ID:  FooID(t.Random.Int()),
							Foo: t.Random.Int(),
						}
					})

					receivedQuery := testcase.LetValue[url.Values](s, nil)
					override.Let(s, func(t *testcase.T) func(q url.Values) iterators.Iterator[Foo] {
						return func(q url.Values) iterators.Iterator[Foo] {
							receivedQuery.Set(t, q)
							return iterators.SingleValue(foo.Get(t))
						}
					})

					s.Then("override is used and the actual HTTP request passed to it", func(t *testcase.T) {
						path.Set(t, path.Get(t)+"?foo=bar")
						act(t)
						r := receivedQuery.Get(t)
						t.Must.NotNil(r,
							"it was expected that the override populate the receivedRequest variable")
						t.Must.Equal("bar", r.Get("foo"),
							"it is expected that the override has access to a valid request object")
					})

					s.Then("the result will be based on the value returned by the override", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusOK, rr.Code)
						t.Must.ContainExactly(
							[]FooDTO{{ID: int(foo.Get(t).ID), Foo: foo.Get(t).Foo}},
							respondsWithJSON[[]FooDTO](t, rr))
					})
				})

				s.And("the returned result has an issue", func(s *testcase.Spec) {
					expectedErr := let.Error(s)

					override.Let(s, func(t *testcase.T) func(q url.Values) iterators.Iterator[Foo] {
						return func(q url.Values) iterators.Iterator[Foo] {
							return iterators.Error[Foo](expectedErr.Get(t))
						}
					})

					subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
						h := subject.Super(t)
						h.ErrorHandler = rfc7807.Handler{
							Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
								t.Must.ErrorIs(expectedErr.Get(t), err)
								dto.Detail = err.Error()
								dto.Status = http.StatusTeapot
							},
						}
						return h
					})

					s.Then("then the error is propagated back", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusTeapot, rr.Code)

						errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
						t.Must.NotEmpty(errDTO)
						t.Must.Equal(expectedErr.Get(t).Error(), errDTO.Detail)
					})
				})
			})

			s.When("NoIndex flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					rapi := subject.Super(t)
					rapi.Index = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(`#create`, func(s *testcase.Spec) {
			var (
				_   = method.LetValue(s, http.MethodPost)
				_   = path.LetValue(s, `/`)
				dto = testcase.Let(s, func(t *testcase.T) FooDTO {
					return FooDTO{Foo: t.Random.Int()}
				})
				_ = body.Let(s, func(t *testcase.T) []byte {
					bs, err := json.Marshal(dto.Get(t))
					t.Must.NoError(err)
					return bs
				})
			)

			s.Then(`it will responds with the persisted entity's DTO that includes the populated ID field`, func(t *testcase.T) {
				rr := act(t)
				t.Must.Equal(http.StatusCreated, rr.Code)
				t.Must.NotEmpty(rr.Body.String())
				gotDTO := respondsWithJSON[FooDTO](t, rr)
				t.Must.Equal(dto.Get(t).Foo, gotDTO.Foo)
				t.Must.NotEmpty(gotDTO.ID)

				ent, found, err := mdb.Get(t).FindByID(context.Background(), FooID(gotDTO.ID))
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(ent.Foo, gotDTO.Foo)
			})

			s.When("the method is not supported", func(s *testcase.Spec) {
				method.Let(s, func(t *testcase.T) string {
					return t.Random.StringNC(5, strings.ToUpper(random.CharsetAlpha()))
				})

				s.Then("it replies back with method not supported error", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrMethodNotAllowed.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("ID is supplied and the repository allow pre populated ID fields", func(s *testcase.Spec) {
				mdb.Let(s, func(t *testcase.T) *memory.Repository[Foo, FooID] {
					m := mdb.Super(t)
					// configure if needed the *memory.Repository to accept supplied ID value
					return m
				})

				dto.Let(s, func(t *testcase.T) FooDTO {
					d := dto.Super(t)
					d.ID = int(time.Now().Unix())
					return d
				})

				s.Then(`it will create a new entity in the repository with the given entity`, func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					gotDTO := respondsWithJSON[FooDTO](t, rr)
					t.Must.Equal(dto.Get(t), gotDTO)
					t.Must.NotEmpty(gotDTO.ID)

					ent, found, err := mdb.Get(t).FindByID(context.Background(), FooID(gotDTO.ID))
					t.Must.NoError(err)
					t.Must.True(found)
					t.Must.Equal(ent.Foo, gotDTO.Foo)
				})

				s.And("the entity was already created", func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						t.Must.Equal(http.StatusCreated, act(t).Code)
					})

					s.Then("it will fail to create the resource", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusConflict, rr.Code)
						errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
						t.Must.Equal(restapi.ErrEntityAlreadyExist.ID.String(), errDTO.Type.ID)
					})
				})
			})

			s.When("Create is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, FooID] {
					return struct{ crud.ByIDFinder[Foo, FooID] }{ByIDFinder: mdb.Get(t)}
				})

				s.Then("it will respond with StatusMethodNotAllowed, page not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrMethodNotAllowed.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("the request body is larger than the configured limit", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					h := subject.Super(t)
					h.BodyReadLimitByteSize = 3
					return h
				})

				s.Then("it will fail because the request body is too large", func(t *testcase.T) {
					rr := act(t)
					t.Log(rr.Body.String())
					t.Must.Equal(http.StatusRequestEntityTooLarge, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrRequestEntityTooLarge.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("No Create flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					rapi := subject.Super(t)
					rapi.Create = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		WhenIDInThePathIsMalformed := func(s *testcase.Spec) {
			s.When("ID in the path is malformed", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf("/%s",
						t.Random.StringNC(t.Random.IntB(1, 5), random.CharsetAlpha()))
				})

				s.Then("it will fail on parsing the ID", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusBadRequest, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrMalformedID.ID.String(), errDTO.Type.ID)
				})
			})
		}

		s.Describe(`#show`, func(s *testcase.Spec) {
			var (
				dto = GivenWeHaveStoredFooDTO(s)
				_   = method.LetValue(s, http.MethodGet)
				_   = path.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf("/%d", dto.Get(t).ID)
				})
			)

			s.Then(`it will show the requested entity`, func(t *testcase.T) {
				rr := act(t)
				t.Must.NotEmpty(rr.Body.String())
				gotDTO := respondsWithJSON[FooDTO](t, rr)
				t.Must.Equal(dto.Get(t), gotDTO)
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the requested entity is not found", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf("/%d", t.Random.Int()+42)
				})

				s.Then("it will respond with 404, entity not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrEntityNotFound.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("NoShow flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					rapi := subject.Super(t)
					rapi.Show = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(`#update`, func(s *testcase.Spec) {
			var (
				dto = GivenWeHaveStoredFooDTO(s)
				_   = method.Let(s, func(t *testcase.T) string {
					return t.Random.SliceElement([]string{
						http.MethodPut,
						http.MethodPatch,
					}).(string)
				})
				_ = path.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf("/%d", dto.Get(t).ID)
				})

				updatedDTO = testcase.Let(s, func(t *testcase.T) FooDTO {
					v := dto.Get(t)
					v.Foo = t.Random.Int()
					return v
				})
				_ = body.Let(s, func(t *testcase.T) []byte {
					bs, err := json.Marshal(updatedDTO.Get(t))
					t.Must.NoError(err)
					return bs
				})
			)

			s.Then(`it will update the entity in the repository`, func(t *testcase.T) {
				rr := act(t)
				t.Must.Empty(rr.Body.String())
				t.Must.Equal(http.StatusNoContent, rr.Code)
				ent, found, err := mdb.Get(t).FindByID(context.Background(), FooID(dto.Get(t).ID))
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(ent.Foo, updatedDTO.Get(t).Foo)
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), FooID(dto.Get(t).ID)))
				})

				s.Then("it will respond with 404, entity not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrEntityNotFound.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("Update is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, FooID] {
					return struct{ crud.ByIDFinder[Foo, FooID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("NoUpdate flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					rapi := subject.Super(t)
					rapi.Update = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(`#delete`, func(s *testcase.Spec) {
			var (
				dto = GivenWeHaveStoredFooDTO(s)
				_   = method.LetValue(s, http.MethodDelete)
				_   = path.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf("/%d", dto.Get(t).ID)
				})
			)

			s.Then(`it will delete the entity in the repository`, func(t *testcase.T) {
				rr := act(t)
				t.Must.Empty(rr.Body.String())
				t.Must.Equal(http.StatusNoContent, rr.Code)

				_, found, err := mdb.Get(t).FindByID(context.Background(), FooID(dto.Get(t).ID))
				t.Must.NoError(err)
				t.Must.False(found, "expected that the entity is deleted")
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), FooID(dto.Get(t).ID)))
				})

				s.Then("it will respond with 404, entity not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrEntityNotFound.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("Delete is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, FooID] {
					return struct{ crud.ByIDFinder[Foo, FooID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("NoDelete flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					rapi := subject.Super(t)
					rapi.Destroy = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.When("pathkit that leads to sub resource endpoints called", func(s *testcase.Spec) {
			path.Let(s, func(t *testcase.T) string {
				return "/42/bars"
			})

			s.Then("the .Routes will be used to route the request", func(t *testcase.T) {
				rr := act(t)
				t.Must.Equal(http.StatusTeapot, rr.Code)
				req := lastSubResourceRequest.Get(t)
				t.Must.NotNil(req)

				id, ok := subject.Get(t).ContextLookupID(req.Context())
				t.Must.True(ok)
				assert.Equal(t, 42, id)

				routing, ok := internal.LookupRouting(req.Context())
				t.Must.True(ok)
				t.Must.Equal("/bars", routing.Path)
			})

			s.And(".EntityRoutes is nil", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[Foo, FooID] {
					v := subject.Super(t)
					v.EntityRoutes = nil
					return v
				})

				s.Then("path is not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrPathNotFound.ID.String(), errDTO.Type.ID)
				})
			})
		})
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

func TestIDConverter(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
		return restapi.IDConverter[string]{}
	})

	s.Describe(".FormatID", func(s *testcase.Spec) {
		var (
			id = let.String(s)
		)
		act := func(t *testcase.T) (string, error) {
			return subject.Get(t).FormatID(id.Get(t))
		}

		s.When("Format func is provided", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = func(s string) (string, error) {
					return "format-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				t.Must.ErrorIs(err, fmt.Errorf("boom"))
				t.Must.Equal(got, "format-ok")
			})
		})

		s.When("Format func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				t.Must.NoError(err)
				t.Must.Equal(got, id.Get(t))
			})
		})
	})
	s.Describe(".ParseID", func(s *testcase.Spec) {
		var (
			id  = let.String(s)
			raw = id.Bind(s)
		)
		act := func(t *testcase.T) (string, error) {
			return subject.Get(t).ParseID(raw.Get(t))
		}

		s.When("Parse func is provided", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = func(s string) (string, error) {
					return "parse-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				t.Must.ErrorIs(err, fmt.Errorf("boom"))
				t.Must.Equal(got, "parse-ok")
			})
		})

		s.When("Parse func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				t.Must.NoError(err)
				t.Must.Equal(got, id.Get(t))
			})
		})
	})

	s.Context("types handled by default without Parse and Format functions", func(s *testcase.Spec) {
		const answer = "The Answer to Life, the Universe and Everything Is Forty-Two"
		type IntID int
		type StringID string

		s.Test("int", func(t *testcase.T) {
			idc := restapi.IDConverter[int]{}
			id := int(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int based", func(t *testcase.T) {
			idc := restapi.IDConverter[IntID]{}
			id := IntID(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int8", func(t *testcase.T) {
			idc := restapi.IDConverter[int8]{}
			id := int8(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int16", func(t *testcase.T) {
			idc := restapi.IDConverter[int16]{}
			id := int16(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int32", func(t *testcase.T) {
			idc := restapi.IDConverter[int32]{}
			id := int32(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int64", func(t *testcase.T) {
			idc := restapi.IDConverter[int64]{}
			id := int64(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("string", func(t *testcase.T) {
			idc := restapi.IDConverter[string]{}
			id := answer
			encoded := answer

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("string based", func(t *testcase.T) {
			idc := restapi.IDConverter[StringID]{}
			id := StringID(answer)
			encoded := answer

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})
	}, testcase.Group("defaults"))
}
