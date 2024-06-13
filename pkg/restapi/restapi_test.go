package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/restapi/rfc7807"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/iterators"
	. "go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleResource() {
	fooRepository := memory.NewRepository[X, XID](memory.NewMemory())
	fooRestfulResource := restapi.Resource[X, XID]{
		Create: fooRepository.Create,
		Index: func(ctx context.Context, query url.Values) (iterators.Iterator[X], error) {
			foos := fooRepository.FindAll(ctx)

			if bt := query.Get("bigger"); bt != "" {
				bigger, err := strconv.Atoi(bt)
				if err != nil {
					return nil, err
				}
				foos = iterators.Filter(foos, func(foo X) bool {
					return bigger < foo.N
				})
			}

			return foos, nil
		},

		Show: fooRepository.FindByID,

		Update: func(ctx context.Context, id XID, ptr *X) error {
			ptr.ID = id
			return fooRepository.Update(ctx, ptr)
		},
		Destroy: fooRepository.DeleteByID,

		Mapping: restapi.DTOMapping[X, XDTO]{},

		MappingForMIME: map[restapi.MIMEType]restapi.Mapping[X]{
			restapi.JSON: restapi.DTOMapping[X, XDTO]{},
		},
	}

	mux := http.NewServeMux()
	restapi.Mount(mux, "/foos", fooRestfulResource)
}

func TestResource_ServeHTTP(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(func(t *testcase.T) { logger.Testing(t) })

	type FooIDContextKey struct{}

	var (
		mdb = testcase.Let(s, func(t *testcase.T) *memory.Repository[X, XID] {
			m := memory.NewMemory()
			return memory.NewRepository[X, XID](m)
		})
		resource = testcase.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
			return mdb.Get(t)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
		return restapi.Resource[X, XID]{
			IDContextKey: FooIDContextKey{},
			Serialization: restapi.ResourceSerialization[X, XID]{
				Serializers: map[restapi.MIMEType]restapi.Serializer{
					restapi.JSON: serializers.JSON{},
				},
				IDConverter: restapi.IDConverter[XID]{},
			},
			Mapping: restapi.DTOMapping[X, XDTO]{},
		}.WithCRUD(resource.Get(t))
	})

	GivenWeHaveStoredFooDTO := func(s *testcase.Spec) testcase.Var[XDTO] {
		return testcase.Let(s, func(t *testcase.T) XDTO {
			// create ent and persist
			ent := X{N: t.Random.Int()}
			t.Must.NoError(mdb.Get(t).Create(context.Background(), &ent))
			t.Defer(mdb.Get(t).DeleteByID, context.Background(), ent.ID)
			// map ent to DTO
			dto, err := XMapping{}.MapDTO(context.Background(), ent)
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
				t.Must.Empty(respondsWithJSON[[]XDTO](t, rr))
			})

			s.When("we have entity in the repository", func(s *testcase.Spec) {
				dto := GivenWeHaveStoredFooDTO(s)

				s.Then("it will return back the entity", func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					t.Must.Contain(respondsWithJSON[[]XDTO](t, rr), dto.Get(t))
				})
			})

			s.When("we have multiple entities in the repository", func(s *testcase.Spec) {
				dto1 := GivenWeHaveStoredFooDTO(s)
				dto2 := GivenWeHaveStoredFooDTO(s)
				dto3 := GivenWeHaveStoredFooDTO(s)

				s.Then("it will return back the entity", func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					t.Must.ContainExactly([]XDTO{dto1.Get(t), dto2.Get(t), dto3.Get(t)},
						respondsWithJSON[[]XDTO](t, rr))
				})
			})

			s.When("FindAll is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
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
				override := testcase.Let[func(query url.Values) iterators.Iterator[X]](s, nil)

				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					h := subject.Super(t)
					h.Index = func(ctx context.Context, query url.Values) (iterators.Iterator[X], error) {
						return override.Get(t)(query), nil
					}
					return h
				})

				s.And("it returns values without an issue", func(s *testcase.Spec) {
					x := testcase.Let(s, func(t *testcase.T) X {
						return X{
							ID: XID(t.Random.Int()),
							N:  t.Random.Int(),
						}
					})

					receivedQuery := testcase.LetValue[url.Values](s, nil)
					override.Let(s, func(t *testcase.T) func(q url.Values) iterators.Iterator[X] {
						return func(q url.Values) iterators.Iterator[X] {
							receivedQuery.Set(t, q)
							return iterators.SingleValue(x.Get(t))
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

					s.Then("the result will be based on the value returned by the controller function", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusOK, rr.Code)
						t.Must.ContainExactly(
							[]XDTO{{ID: int(x.Get(t).ID), X: x.Get(t).N}},
							respondsWithJSON[[]XDTO](t, rr))
					})
				})

				s.And("the returned result has an issue", func(s *testcase.Spec) {
					expectedErr := let.Error(s)

					override.Let(s, func(t *testcase.T) func(q url.Values) iterators.Iterator[X] {
						return func(q url.Values) iterators.Iterator[X] {
							return iterators.Error[X](expectedErr.Get(t))
						}
					})

					subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
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

			s.When("Index is not set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					rapi := subject.Super(t)
					rapi.Index = nil
					return rapi
				})

				ThenNotAllowed(s)
			})

			s.When("non empty iterator returned it is ensured to be closed", func(s *testcase.Spec) {
				isClosed := testcase.LetValue[bool](s, false)

				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					sub := subject.Super(t)
					sub.Index = func(ctx context.Context, query url.Values) (iterators.Iterator[X], error) {
						i := iterators.Slice([]X{{ID: 1, N: 1}, {ID: 2, N: 2}})
						stub := iterators.Stub(i)
						stub.StubClose = func() error {
							isClosed.Set(t, true)
							return i.Close()
						}
						return stub, nil
					}
					return sub
				})

				s.Test("iterator is closed on finish", func(t *testcase.T) {
					rr := act(t)
					rr.Result()
					assert.True(t, isClosed.Get(t))
				})
			})
		})

		s.Describe(`#create`, func(s *testcase.Spec) {
			var (
				_   = method.LetValue(s, http.MethodPost)
				_   = path.LetValue(s, `/`)
				dto = testcase.Let(s, func(t *testcase.T) XDTO {
					return XDTO{X: t.Random.Int()}
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
				gotDTO := respondsWithJSON[XDTO](t, rr)
				t.Must.Equal(dto.Get(t).X, gotDTO.X)
				t.Must.NotEmpty(gotDTO.ID)

				ent, found, err := mdb.Get(t).FindByID(context.Background(), XID(gotDTO.ID))
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(ent.N, gotDTO.X)
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
				mdb.Let(s, func(t *testcase.T) *memory.Repository[X, XID] {
					m := mdb.Super(t)
					// configure if needed the *memory.Repository to accept supplied ID value
					return m
				})

				dto.Let(s, func(t *testcase.T) XDTO {
					d := dto.Super(t)
					d.ID = int(time.Now().Unix())
					return d
				})

				s.Then(`it will create a new entity in the repository with the given entity`, func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					gotDTO := respondsWithJSON[XDTO](t, rr)
					t.Must.Equal(dto.Get(t), gotDTO)
					t.Must.NotEmpty(gotDTO.ID)

					ent, found, err := mdb.Get(t).FindByID(context.Background(), XID(gotDTO.ID))
					t.Must.NoError(err)
					t.Must.True(found)
					t.Must.Equal(ent.N, gotDTO.X)
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
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
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					h := subject.Super(t)
					h.BodyReadLimit = 3
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
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
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
				gotDTO := respondsWithJSON[XDTO](t, rr)
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
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
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

				updatedDTO = testcase.Let(s, func(t *testcase.T) XDTO {
					v := dto.Get(t)
					v.X = t.Random.Int()
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
				ent, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(ent.N, updatedDTO.Get(t).X)
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), XID(dto.Get(t).ID)))
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("NoUpdate flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					rapi := subject.Super(t)
					rapi.Update = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(`#destroy`, func(s *testcase.Spec) {
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

				_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
				t.Must.NoError(err)
				t.Must.False(found, "expected that the entity is deleted")
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), XID(dto.Get(t).ID)))
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("Destroy handler is unset", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					rapi := subject.Super(t)
					rapi.Destroy = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(`#destroy-all`, func(s *testcase.Spec) {
			var (
				dto = GivenWeHaveStoredFooDTO(s)
				_   = method.LetValue(s, http.MethodDelete)
				_   = path.LetValue(s, "/")
			)

			s.Then(`it will delete the entity in the repository`, func(t *testcase.T) {
				rr := act(t)
				t.Must.Empty(rr.Body.String())
				t.Must.Equal(http.StatusNoContent, rr.Code)

				_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
				t.Must.NoError(err)
				t.Must.False(found, "expected that the entity is deleted")
			})

			s.When("DeleteAll is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("DestroyAll handler is unset", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					rapi := subject.Super(t)
					rapi.DestroyAll = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(".ResourceRoutes", func(s *testcase.Spec) {
			var lastSubResourceRequest = testcase.LetValue[*http.Request](s, nil)

			subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
				sub := subject.Super(t)
				sub.SubRoutes = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Handle all routes with a simple HandlerFunc
					lastSubResourceRequest.Set(t, r)
					w.WriteHeader(http.StatusTeapot)
				})
				return sub
			})

			path.Let(s, func(t *testcase.T) string {
				return "/42/bars"
			})

			s.Then("the .Routes will be used to route the request", func(t *testcase.T) {
				rr := act(t)
				t.Must.Equal(http.StatusTeapot, rr.Code)
				req := lastSubResourceRequest.Get(t)
				t.Must.NotNil(req)

				id, ok := req.Context().Value(FooIDContextKey{}).(XID)
				t.Must.True(ok)
				assert.Equal(t, 42, id)

				routing, ok := internal.LookupRouting(req.Context())
				t.Must.True(ok)
				t.Must.Equal("/bars", routing.PathLeft)
			})

			s.And(".EntityRoutes is nil", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Resource[X, XID] {
					v := subject.Super(t)
					v.SubRoutes = nil
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

func TestResource_formUrlencodedRequestBodyIsSupported(t *testing.T) {
	ctx := context.Background()

	var got Foo
	res := restapi.Resource[Foo, FooID]{
		Create: func(ctx context.Context, ptr *Foo) error {
			ptr.ID = "ok"
			got = *ptr
			return nil
		},
		Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			if got.ID != id {
				return ent, false, nil
			}
			return got, true, nil
		},
	}

	client := restapi.Client[Foo, FooID]{
		HTTPClient: &http.Client{
			Transport: httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				rr := httptest.NewRecorder()
				res.ServeHTTP(rr, request)
				return rr.Result(), nil
			}),
		},
		MIMEType: restapi.FormUrlencoded,
	}

	exp := Foo{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}
	assert.NoError(t, client.Create(ctx, &exp))
	assert.NotEmpty(t, exp.ID)
	assert.Equal(t, exp, got)

	got2, found, err := client.FindByID(ctx, exp.ID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, exp, got2)
}

func TestResource_WithCRUD_onNotEmptyOperations(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	var createC, indexC, showC, updateC, destroyC, destroyAllC bool
	fooRepo := memory.NewRepository[Foo, FooID](mem)
	fooAPI := restapi.Resource[Foo, FooID]{
		Create: func(ctx context.Context, ptr *Foo) error {
			createC = true
			ptr.ID = FooID(rnd.StringNC(5, random.CharsetAlpha()))
			return nil
		},
		Index: func(ctx context.Context, query url.Values) (iterators.Iterator[Foo], error) {
			indexC = true
			return iterators.Empty[Foo](), nil
		},
		Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			showC = true
			return Foo{ID: id}, true, nil
		},
		Update: func(ctx context.Context, id FooID, ptr *Foo) error {
			updateC = true
			return nil
		},
		Destroy: func(ctx context.Context, id FooID) error {
			destroyC = true
			return nil
		},
		DestroyAll: func(ctx context.Context, query url.Values) error {
			destroyAllC = true
			return nil
		},
	}.WithCRUD(fooRepo)

	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/", nil))

	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/42", nil))
	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/42", strings.NewReader("{}")))
	fooAPI.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/42", strings.NewReader("{}")))

	assert.True(t, createC)
	assert.True(t, indexC)
	assert.True(t, destroyAllC)

	assert.True(t, showC)
	assert.True(t, updateC)
	assert.True(t, destroyC)
}

func TestDTOMapping_manual(t *testing.T) {
	fooRepository := memory.NewRepository[Foo, FooID](memory.NewMemory())

	// FooCustomDTO is not a proper DTO.
	// The only reason we use this is to ensure that the custom mapping is used
	// instead of the default dtos mapping.
	type FooCustomDTO struct{ Foo }

	resource := restapi.Resource[Foo, FooID]{
		Mapping: restapi.DTOMapping[Foo, FooCustomDTO]{
			ToEnt: func(ctx context.Context, dto FooCustomDTO) (Foo, error) {
				return dto.Foo, nil
			},
			ToDTO: func(ctx context.Context, ent Foo) (FooCustomDTO, error) {
				return FooCustomDTO{Foo: ent}, nil
			},
		},
	}.WithCRUD(fooRepository)

	example := FooCustomDTO{
		Foo: Foo{
			Foo: "foo",
			Bar: "bar",
			Baz: "baz",
		},
	}

	var id FooID
	{
		t.Log("given we create an entity with our custom DTO")
		data, err := json.Marshal(example)
		assert.NoError(t, err)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
		r.Header.Set("Content-Type", restapi.JSON.String())
		resource.ServeHTTP(w, r)
		assert.Equal(t, w.Code, http.StatusCreated)

		var response FooCustomDTO
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		id = response.Foo.ID
		assert.NotEmpty(t, id)
	}
	{
		t.Log("then we are able to retrieve this entity through the custom DTO")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, pathkit.Join("/", id.String()), nil)
		r.Header.Set("Accept", restapi.JSON.String())
		resource.ServeHTTP(w, r)
		assert.Equal(t, w.Code, http.StatusOK)

		var response FooCustomDTO
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		expected := example
		expected.ID = id
		assert.Equal(t, response, expected)
	}
}

func TestRouter_Resource(t *testing.T) {
	var r restapi.Router

	ctx := context.Background()

	foo := Foo{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	r.Resource("foo", restapi.Resource[Foo, FooID]{
		Index: func(ctx context.Context, query url.Values) (iterators.Iterator[Foo], error) {
			return iterators.SingleValue(foo), nil
		},
		Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			return foo, true, nil
		},
		Mapping: restapi.DTOMapping[Foo, FooDTO]{},
	})

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo", nil)
		req.Header.Set("Content-Type", restapi.JSON.String())
		r.ServeHTTP(rr, req)

		var index []FooDTO
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &index))
		assert.NotEmpty(t, index)
		assert.True(t, len(index) == 1)
	}

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo/42", nil)
		req.Header.Set("Content-Type", restapi.JSON.String())
		r.ServeHTTP(rr, req)

		var show FooDTO
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &show))
		assert.NotEmpty(t, show)
		assert.Equal(t, dtokit.MustMap[FooDTO](ctx, foo), show)
	}
}
