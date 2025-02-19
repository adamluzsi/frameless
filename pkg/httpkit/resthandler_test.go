package httpkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/relationship"
	"go.llib.dev/frameless/port/iterators"
	. "go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func _() {
	var h httpkit.RESTHandler[X, XID]
	var (
		creator     crud.Creator[X]
		allFinder   crud.AllFinder[X]
		byIDFinder  crud.ByIDFinder[X, XID]
		byIDDeleter crud.ByIDDeleter[XID]
		allDeleter  crud.AllDeleter
	)
	h.Create = creator.Create
	h.Index = allFinder.FindAll
	h.Show = byIDFinder.FindByID
	h.Destroy = byIDDeleter.DeleteByID
	h.DestroyAll = allDeleter.DeleteAll
}

func ExampleRESTHandler() {
	fooRepository := memory.NewRepository[X, XID](memory.NewMemory())
	fooRestfulResource := httpkit.RESTHandler[X, XID]{
		Create:     fooRepository.Create,
		Index:      fooRepository.FindAll,
		Show:       fooRepository.FindByID,
		Update:     fooRepository.Update,
		Destroy:    fooRepository.DeleteByID,
		DestroyAll: fooRepository.DeleteAll,
	}

	mux := http.NewServeMux()
	httpkit.Mount(mux, "/foos", fooRestfulResource)
}

func ExampleRESTHandler_withIndexFilteringByQuery() {
	fooRepository := memory.NewRepository[X, XID](memory.NewMemory())
	fooRestfulResource := httpkit.RESTHandler[X, XID]{
		Index: func(ctx context.Context) (iterators.Iterator[X], error) {
			foos, err := fooRepository.FindAll(ctx)
			if err != nil {
				return foos, err
			}
			req, _ := httpkit.LookupRequest(ctx)
			if bt := req.URL.Query().Get("bigger"); bt != "" {
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
	}

	mux := http.NewServeMux()
	httpkit.Mount(mux, "/foos", fooRestfulResource)
}

func ExampleRESTHandler_withMediaTypeConfiguration() {
	fooRepository := memory.NewRepository[X, XID](memory.NewMemory())
	fooRestfulResource := httpkit.RESTHandler[X, XID]{
		Create:  fooRepository.Create,
		Index:   fooRepository.FindAll,
		Show:    fooRepository.FindByID,
		Update:  fooRepository.Update,
		Destroy: fooRepository.DeleteByID,

		Mapping: dtokit.Mapping[X, XDTO]{},

		MediaType: mediatype.JSON, // we can set the preferred default media type in case the requester don't specify it.

		MediaTypeMappings: httpkit.MediaTypeMappings[X]{ // we can populate this with any media type we want
			mediatype.JSON: dtokit.Mapping[X, XDTO]{},
		},

		MediaTypeCodecs: httpkit.MediaTypeCodecs{ // we can populate with any custom codec for any custom media type
			mediatype.JSON: jsonkit.Codec{},
		},
	}

	mux := http.NewServeMux()
	httpkit.Mount(mux, "/foos", fooRestfulResource)
}

func TestRESTHandler_ServeHTTP(t *testing.T) {
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
	subject := testcase.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
		return httpkit.RESTHandlerFromCRUD[X, XID](resource.Get(t), func(h *httpkit.RESTHandler[X, XID]) {
			h.IDContextKey = FooIDContextKey{}
			h.MediaTypeCodecs = map[string]codec.Codec{
				mediatype.JSON: jsonkit.Codec{},
			}
			h.Mapping = dtokit.Mapping[X, XDTO]{}
		})
	})

	o := testcase.Let(s, func(t *testcase.T) *O {
		return &O{ID: OID(t.Random.IntBetween(1, 99))}
	})

	GivenWeHaveStoredFooWithDTO := func(s *testcase.Spec) (testcase.Var[X], testcase.Var[XDTO]) {
		return testcase.Let2(s, func(t *testcase.T) (X, XDTO) {
			// create ent and persist
			ent := X{N: t.Random.Int(), OID: o.Get(t).ID}
			t.Must.NoError(mdb.Get(t).Create(context.Background(), &ent))
			t.Defer(mdb.Get(t).DeleteByID, context.Background(), ent.ID)
			// map ent to DTO
			dto, err := XMapping{}.MapDTO(context.Background(), ent)
			t.Must.NoError(err)
			return ent, dto
		})
	}

	GivenWeHaveStoredFooDTO := func(s *testcase.Spec) testcase.Var[XDTO] {
		_, dto := GivenWeHaveStoredFooWithDTO(s)
		dto.EagerLoading(s)
		return dto
	}

	s.Describe("#ServeHTTP", func(s *testcase.Spec) {
		var (
			method  = testcase.LetValue(s, http.MethodGet)
			path    = testcase.LetValue(s, "/")
			body    = testcase.LetValue[[]byte](s, nil)
			Context = let.Context(s)
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method.Get(t), path.Get(t), bytes.NewReader(body.Get(t)))
			r = r.WithContext(Context.Get(t))
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
				t.Must.Equal(httpkit.ErrMethodNotAllowed.Code.String(), errDTO.Type.ID)
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

				s.When("handler is a subresource and ownership check passes", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
						sub := subject.Super(t)
						sub.Filters = append(sub.Filters, func(ctx context.Context, x X) bool {
							return true
						})
						return sub
					})

					s.Then("it will return back the entity", func(t *testcase.T) {
						rr := act(t)
						t.Must.NotEmpty(rr.Body.String())
						t.Must.Contain(respondsWithJSON[[]XDTO](t, rr), dto.Get(t))
					})
				})

				s.When("if filters block it", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
						sub := subject.Super(t)
						sub.Filters = append(sub.Filters, func(ctx context.Context, x X) bool {
							return false
						})
						return sub
					})

					s.Then("it will not return back the entity", func(t *testcase.T) {
						rr := act(t)
						t.Must.NotEmpty(rr.Body.String())
						t.Must.NotContain(respondsWithJSON[[]XDTO](t, rr), dto.Get(t))
					})
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
					t.Must.Equal(httpkit.ErrMethodNotAllowed.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("index is provided", func(s *testcase.Spec) {
				override := testcase.Let[func(ctx context.Context) (iterators.Iterator[X], error)](s, nil)

				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					h := subject.Super(t)
					h.Index = func(ctx context.Context) (iterators.Iterator[X], error) {
						return override.Get(t)(ctx)
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
					override.Let(s, func(t *testcase.T) func(ctx context.Context) (iterators.Iterator[X], error) {
						return func(ctx context.Context) (iterators.Iterator[X], error) {
							req, ok := httpkit.LookupRequest(ctx)
							if ok {
								receivedQuery.Set(t, req.URL.Query())
							}
							return iterators.SingleValue(x.Get(t)), nil
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

					override.Let(s, func(t *testcase.T) func(ctx context.Context) (iterators.Iterator[X], error) {
						return func(ctx context.Context) (iterators.Iterator[X], error) {
							return iterators.Error[X](expectedErr.Get(t)), nil
						}
					})

					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					rapi := subject.Super(t)
					rapi.Index = nil
					return rapi
				})

				ThenNotAllowed(s)
			})

			s.When("non empty iterator returned it is ensured to be closed", func(s *testcase.Spec) {
				isClosed := testcase.LetValue[bool](s, false)

				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					sub := subject.Super(t)
					sub.Index = func(ctx context.Context) (iterators.Iterator[X], error) {
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
					t.Must.Equal(httpkit.ErrMethodNotAllowed.Code.String(), errDTO.Type.ID)
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
						t.Must.Equal(httpkit.ErrEntityAlreadyExist.Code.String(), errDTO.Type.ID)
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
					t.Must.Equal(httpkit.ErrMethodNotAllowed.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("the request body is larger than the configured limit", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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
					t.Must.Equal(httpkit.ErrRequestEntityTooLarge.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("No Create flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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
					t.Must.Equal(httpkit.ErrMalformedID.Code.String(), errDTO.Type.ID)
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

			s.When("handler is a subresource and ownership check passes", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), o.Get(t))
				})

				s.Then(`it accept the request`, func(t *testcase.T) {
					rr := act(t)
					t.Must.NotEmpty(rr.Body.String())
					gotDTO := respondsWithJSON[XDTO](t, rr)
					t.Must.Equal(dto.Get(t), gotDTO)
				})
			})

			s.When("if filters block it", func(s *testcase.Spec) {
				othO := testcase.LetValue(s, O{ID: 123})

				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), othO.Get(t))
				})

				s.Then("replies back with not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
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
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("NoShow flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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

			s.When("handler is a subresource and ownership check passes", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), o.Get(t))
				})

				s.Then(`it accept the request`, func(t *testcase.T) {
					rr := act(t)
					t.Must.Empty(rr.Body.String())
					t.Must.Equal(http.StatusNoContent, rr.Code)
					ent, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
					t.Must.NoError(err)
					t.Must.True(found)
					t.Must.Equal(ent.N, updatedDTO.Get(t).X)
				})
			})

			s.When("if filters block it", func(s *testcase.Spec) {
				othO := testcase.LetValue(s, O{ID: 123})

				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), othO.Get(t))
				})

				s.Then("the it replies back with forbidden due to the filter", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
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
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("Update is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("NoUpdate flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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

			s.When("handler is a subresource and ownership check passes", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), o.Get(t))
				})

				s.Then(`it accept the request`, func(t *testcase.T) {
					rr := act(t)
					t.Must.Empty(rr.Body.String())
					t.Must.Equal(http.StatusNoContent, rr.Code)

					_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
					t.Must.NoError(err)
					t.Must.False(found, "expected that the entity is deleted")
				})
			})

			s.When("if filters block it", func(s *testcase.Spec) {
				othO := testcase.LetValue(s, O{ID: 123})

				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), othO.Get(t))
				})

				s.Then("the it replies back with forbidden due to the filter", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
			})

			s.When(".Show is not provided to verify the entity prior to deletion in a subresource context", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), o.Get(t))
				})

				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					sub := subject.Super(t)
					sub.Show = nil
					return sub
				})

				s.Then(`method not allowed returned`, func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)
				})

				s.And("DeletionIsContextAware", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
						sub := subject.Super(t)
						sub.ScopeAware = true
						return sub
					})

					s.Then(`it will delete the entity in the repository`, func(t *testcase.T) {
						rr := act(t)
						t.Must.Empty(rr.Body.String())
						t.Must.Equal(http.StatusNoContent, rr.Code)

						_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
						t.Must.NoError(err)
						t.Must.False(found, "expected that the entity is deleted")
					})
				})
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
					t.Must.Equal(httpkit.ErrEntityNotFound.Code.String(), errDTO.Type.ID)
				})
			})

			s.When("Delete is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("Destroy handler is unset", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
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

			s.When("the handler is a subresource", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					return internal.ContextRESTParentResourceValuePointer.ContextWith(Context.Super(t), o.Get(t))
				})

				s.And("Destroy and Index is not provided that would enable soft deleting", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
						h := subject.Super(t)
						h.Index = nil
						h.Destroy = nil
						return h
					})

					s.Then(`method not allowed returned`, func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)
					})
				})

				s.And("Index+Destroy is provided to enable soft deleting the subresource scoped values", func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						assert.NotNil(t, subject.Get(t).Index)
						assert.NotNil(t, subject.Get(t).Destroy)
					})

					_, othDTO := testcase.Let2(s, func(t *testcase.T) (X, XDTO) {
						// create ent and persist
						ent := X{N: t.Random.Int(), OID: random.Unique(func() OID { return OID(t.Random.Int()) }, o.Get(t).ID)}
						t.Must.NoError(mdb.Get(t).Create(context.Background(), &ent))
						t.Defer(mdb.Get(t).DeleteByID, context.Background(), ent.ID)
						// map ent to DTO
						dto, err := XMapping{}.MapDTO(context.Background(), ent)
						t.Must.NoError(err)
						return ent, dto
					})

					s.Then(`it will delete the entities related to the current REST Scope`, func(t *testcase.T) {
						rr := act(t)
						t.Must.Empty(rr.Body.String())
						t.Must.Equal(http.StatusNoContent, rr.Code)

						_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
						t.Must.NoError(err)
						t.Must.False(found, "expected that the entity is deleted")

						_, found, err = mdb.Get(t).FindByID(context.Background(), XID(othDTO.Get(t).ID))
						t.Must.NoError(err)
						t.Must.True(found, "expected that the unrelated entity is not deleted")
					})
				})

				s.And("DeletionIsContextAware", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
						sub := subject.Super(t)
						sub.ScopeAware = true
						return sub
					})

					s.Then(`it will delete the entity in the repository`, func(t *testcase.T) {
						rr := act(t)
						t.Must.Empty(rr.Body.String())
						t.Must.Equal(http.StatusNoContent, rr.Code)

						_, found, err := mdb.Get(t).FindByID(context.Background(), XID(dto.Get(t).ID))
						t.Must.NoError(err)
						t.Must.False(found, "expected that the entity is deleted")
					})
				})
			})

			s.When("DeleteAll is not supported by the Repository", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[X, XID] {
					return struct{ crud.ByIDFinder[X, XID] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			s.When("DestroyAll handler is unset", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					rapi := subject.Super(t)
					rapi.DestroyAll = nil
					return rapi
				})

				ThenNotAllowed(s)
			})
		})

		s.Describe(".ResourceRoutes", func(s *testcase.Spec) {
			var lastSubResourceRequest = testcase.LetValue[*http.Request](s, nil)
			var foo, dto = GivenWeHaveStoredFooWithDTO(s)

			subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
				sub := subject.Super(t)
				sub.ResourceRoutes = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Handle all routes with a simple HandlerFunc
					lastSubResourceRequest.Set(t, r)
					w.WriteHeader(http.StatusTeapot)
				})
				return sub
			})

			path.Let(s, func(t *testcase.T) string {
				return pathkit.Join(strconv.Itoa(dto.Get(t).ID), "bars")
			})

			s.Then("the .Routes will be used to route the request", func(t *testcase.T) {
				rr := act(t)
				t.Must.Equal(http.StatusTeapot, rr.Code)
				req := lastSubResourceRequest.Get(t)
				t.Must.NotNil(req)

				id, ok := req.Context().Value(FooIDContextKey{}).(XID)
				t.Must.True(ok)
				assert.Equal(t, foo.Get(t).ID, id)

				routing, ok := internal.RoutingContext.Lookup(req.Context())
				t.Must.True(ok)
				t.Must.Equal("/bars", routing.PathLeft)
			})

			s.And(".EntityRoutes is nil", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.RESTHandler[X, XID] {
					v := subject.Super(t)
					v.ResourceRoutes = nil
					return v
				})

				s.Then("path is not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusNotFound, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(httpkit.ErrPathNotFound.Code.String(), errDTO.Type.ID)
				})
			})
		})

	})
}

func TestRESTHandler_formUrlencodedRequestBodyIsSupported(t *testing.T) {
	ctx := context.Background()

	var got Foo
	res := httpkit.RESTHandler[Foo, FooID]{
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

	client := httpkit.RESTClient[Foo, FooID]{
		HTTPClient: &http.Client{
			Transport: httpkit.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				rr := httptest.NewRecorder()
				res.ServeHTTP(rr, request)
				return rr.Result(), nil
			}),
		},
		MediaType: mediatype.FormUrlencoded,
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

func TestRESTHandler_WithCRUD_onNotEmptyOperations(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()

	var createC, indexC, showC, updateC, destroyC, destroyAllC bool
	fooRepo := memory.NewRepository[Foo, FooID](mem)
	fooAPI := httpkit.RESTHandlerFromCRUD[Foo, FooID](fooRepo, func(h *httpkit.RESTHandler[Foo, FooID]) {
		h.Create = func(ctx context.Context, ptr *Foo) error {
			createC = true
			ptr.ID = FooID(rnd.StringNC(5, random.CharsetAlpha()))
			return nil
		}
		h.Index = func(ctx context.Context) (iterators.Iterator[Foo], error) {
			indexC = true
			return iterators.Empty[Foo](), nil
		}
		h.Show = func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			showC = true
			return Foo{ID: id}, true, nil
		}
		h.Update = func(ctx context.Context, ptr *Foo) error {
			updateC = true
			return nil
		}
		h.Destroy = func(ctx context.Context, id FooID) error {
			destroyC = true
			return nil
		}
		h.DestroyAll = func(ctx context.Context) error {
			destroyAllC = true
			return nil
		}
	})

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

	resource := httpkit.RESTHandlerFromCRUD[Foo, FooID](fooRepository, func(h *httpkit.RESTHandler[Foo, FooID]) {
		h.Mapping = dtokit.Mapping[Foo, FooCustomDTO]{
			ToENT: func(ctx context.Context, dto FooCustomDTO) (Foo, error) {
				return dto.Foo, nil
			},
			ToDTO: func(ctx context.Context, ent Foo) (FooCustomDTO, error) {
				return FooCustomDTO{Foo: ent}, nil
			},
		}
	})

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
		r.Header.Set("Content-Type", mediatype.JSON)
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
		r.Header.Set("Accept", mediatype.JSON)
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
	var r httpkit.Router

	ctx := context.Background()

	foo := Foo{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	r.Resource("foo", httpkit.RESTHandler[Foo, FooID]{
		Index: func(ctx context.Context) (iterators.Iterator[Foo], error) {
			return iterators.SingleValue(foo), nil
		},
		Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			return foo, true, nil
		},
		Mapping: dtokit.Mapping[Foo, FooDTO]{},
	})

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo", nil)
		req.Header.Set("Content-Type", mediatype.JSON)
		r.ServeHTTP(rr, req)

		var index []FooDTO
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &index))
		assert.NotEmpty(t, index)
		assert.True(t, len(index) == 1)
	}

	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/foo/42", nil)
		req.Header.Set("Content-Type", mediatype.JSON)
		r.ServeHTTP(rr, req)

		var show FooDTO
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &show))
		assert.NotEmpty(t, show)
		assert.Equal(t, dtokit.MustMap[FooDTO](ctx, foo), show)
	}
}

func TestRESTHandler_withContext(t *testing.T) {
	type CollectionProbeKey struct{}
	type ResourceProbeKey struct{}
	val := rnd.Error().Error()

	var (
		lastID            FooID
		ResourceRoutesRan bool
	)

	h := httpkit.RESTHandler[Foo, FooID]{
		Index: func(ctx context.Context) (iterators.Iterator[Foo], error) {
			assert.Equal[any](t, ctx.Value(CollectionProbeKey{}), val)
			assert.Nil(t, ctx.Value(ResourceProbeKey{}))
			return iterators.Empty[Foo](), nil
		},
		Create: func(ctx context.Context, ptr *Foo) error {
			assert.Equal[any](t, ctx.Value(CollectionProbeKey{}), val)
			assert.Nil(t, ctx.Value(ResourceProbeKey{}))
			return nil
		},
		Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
			assert.Equal[any](t, ctx.Value(ResourceProbeKey{}), val)
			assert.Nil(t, ctx.Value(CollectionProbeKey{}))
			return Foo{ID: id}, true, nil
		},
		Update: func(ctx context.Context, ptr *Foo) error {
			assert.Equal[any](t, ctx.Value(ResourceProbeKey{}), val)
			assert.Nil(t, ctx.Value(CollectionProbeKey{}))
			return nil
		},
		Destroy: func(ctx context.Context, id FooID) error {
			assert.Equal[any](t, ctx.Value(ResourceProbeKey{}), val)
			assert.Nil(t, ctx.Value(CollectionProbeKey{}))
			return nil
		},
		DestroyAll: func(ctx context.Context) error {
			assert.Equal[any](t, ctx.Value(CollectionProbeKey{}), val)
			assert.Nil(t, ctx.Value(ResourceProbeKey{}))
			return nil
		},
		ResourceRoutes: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			assert.Equal[any](t, ctx.Value(ResourceProbeKey{}), val)
			assert.Nil(t, ctx.Value(CollectionProbeKey{}))
			ResourceRoutesRan = true
		}),
		CollectionContext: func(ctx context.Context) (context.Context, error) {
			return context.WithValue(ctx, CollectionProbeKey{}, val), nil
		},
		ResourceContext: func(ctx context.Context, id FooID) (context.Context, error) {
			lastID = id
			return context.WithValue(ctx, ResourceProbeKey{}, val), nil
		},
		ScopeAware: true,
	}

	data, err := json.Marshal(MakeFoo(t))
	assert.NoError(t, err)
	var mkbody = func() io.Reader {
		return bytes.NewReader(data)
	}

	// assertions are triggered in the controllers

	{ // INDEX: GET /
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
	}
	{ // CREATE: POST /
		req := httptest.NewRequest(http.MethodPost, "/", mkbody())
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
	}
	{ // DESTROY ALL: DELETE /
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
	}
	{ // SHOW: GET /{id}
		req := httptest.NewRequest(http.MethodGet, pathkit.Join("/", "foo-id-1"), mkbody())
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, lastID, "foo-id-1")
	}
	{ // UPDATE: PUT /{id}
		req := httptest.NewRequest(http.MethodPut, pathkit.Join("/", "foo-id-2"), mkbody())
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, lastID, "foo-id-2")
	}
	{ // DESTROY: DELETE /{id}
		req := httptest.NewRequest(http.MethodDelete, pathkit.Join("/", "foo-id-3"), mkbody())
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, lastID, "foo-id-3")
	}
	{ // Resrouce Route
		req := httptest.NewRequest(http.MethodGet, pathkit.Join("/", "foo-id-4", "resource-path"), nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, rr.Code, http.StatusOK)
		assert.Equal(t, lastID, "foo-id-4")
		assert.True(t, ResourceRoutesRan)
	}

	t.Run("rainy", func(t *testing.T) {
		expErr := rnd.Error()

		h := httpkit.RESTHandler[Foo, FooID]{
			Index: func(ctx context.Context) (iterators.Iterator[Foo], error) {
				t.Error("Index was not expected to be called")
				return iterators.Empty[Foo](), nil
			},
			Show: func(ctx context.Context, id FooID) (ent Foo, found bool, err error) {
				t.Error("Show was not exepcted to be called")
				return Foo{}, false, nil
			},

			ResourceRoutes: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("ResourceRoutes was not expecte to be called")
			}),

			ErrorHandler: rfc7807.Handler{
				Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
					if errors.Is(err, expErr) {
						dto.Type.ID = "OK"
						dto.Status = http.StatusTeapot
					}
				},
			},

			CollectionContext: func(ctx context.Context) (context.Context, error) {
				return ctx, expErr
			},
			ResourceContext: func(ctx context.Context, id FooID) (context.Context, error) {
				return ctx, expErr
			},
		}

		{ // INDEX: GET /
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusTeapot, rr.Code)
		}
		{ // SHOW: GET /{id}
			req := httptest.NewRequest(http.MethodGet, pathkit.Join("/", "id"), nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusTeapot, rr.Code)
		}
		{ // Resrouce Route
			req := httptest.NewRequest(http.MethodGet, pathkit.Join("/", "id", "resource-path"), nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusTeapot, rr.Code)
		}
	})
}

func TestRESTHandler_nestedOwnershipConstraint(t *testing.T) {
	type User struct {
		ID string
	}
	type Note struct {
		ID     string
		UserID string

		Attachments []string
	}
	type Attachment struct {
		ID string
		BS []byte
	}
	var (
		userRepo = &memory.Repository[User, string]{}
		noteRepo = &memory.Repository[Note, string]{}
		attaRepo = &memory.Repository[Attachment, string]{}
	)
	var (
		attaResource = httpkit.RESTHandler[Attachment, string]{
			Create: attaRepo.Create,
			Index:  attaRepo.FindAll,
			Show:   attaRepo.FindByID,
		}
		noteResource = httpkit.RESTHandler[Note, string]{
			Index:  noteRepo.FindAll,
			Show:   noteRepo.FindByID,
			Update: noteRepo.Update,

			ResourceRoutes: httpkit.NewRouter(func(r *httpkit.Router) {
				r.Resource("attachments", attaResource)
			}),
		}
		userResource = httpkit.RESTHandler[User, string]{
			Index: userRepo.FindAll,
			Show:  userRepo.FindByID,

			ResourceRoutes: httpkit.NewRouter(func(r *httpkit.Router) {
				r.Resource("notes", noteResource)
			}),
		}
		router = httpkit.NewRouter(func(r *httpkit.Router) {
			r.Resource("users", userResource)
		})
	)

	var ctx = context.Background()

	t.Log("given we have some users")
	user1 := User{}
	crudtest.Create[User, string](t, userRepo, ctx, &user1)
	user2 := User{}
	crudtest.Create[User, string](t, userRepo, ctx, &user2)

	t.Log("each has its own note")
	note1 := Note{UserID: user1.ID}
	crudtest.Create[Note, string](t, noteRepo, ctx, &note1)
	note2 := Note{UserID: user2.ID}
	crudtest.Create[Note, string](t, noteRepo, ctx, &note2)

	t.Log("and each note has its own attachment")
	attachment1 := Attachment{BS: []byte(rnd.Domain())}
	crudtest.Create[Attachment, string](t, attaRepo, ctx, &attachment1)
	assert.NoError(t, relationship.Associate(&note1, &attachment1))
	crudtest.Update[Note, string](t, noteRepo, ctx, &note1)

	attachment2 := Attachment{BS: []byte(rnd.Domain())}
	crudtest.Create[Attachment, string](t, attaRepo, ctx, &attachment2)
	assert.NoError(t, relationship.Associate(&note2, &attachment2))
	crudtest.Update[Note, string](t, noteRepo, ctx, &note2)

	t.Run("when notes of a given user is requested, we only receive back its notes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		path := pathkit.Join("users", user1.ID, "notes")
		req := httptest.NewRequest(http.MethodGet, path, nil)
		t.Log(path)
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var got []Note
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, len(got), 1)
		assert.Equal(t, got[0], note1)
	})

	t.Run("when attachments requested of a given note, only the related attachment(s) returned", func(t *testing.T) {
		rr := httptest.NewRecorder()
		path := pathkit.Join("users", user1.ID, "notes")
		req := httptest.NewRequest(http.MethodGet, path, nil)
		t.Log(path)
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var got []Note
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, len(got), 1)
		assert.Equal(t, got[0], note1)
	})

	t.Run("on sub resource create parent reference many updated", func(t *testing.T) {
		rr := httptest.NewRecorder()
		path := pathkit.Join("users", user1.ID, "notes", note1.ID, "attachments")
		ent := Attachment{BS: []byte(rnd.Domain())}
		reqBody, err := json.Marshal(ent)
		assert.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(reqBody))
		t.Log(req.Method, req.URL.String())
		router.ServeHTTP(rr, req)

		t.Log("attachment created")
		assert.Equal(t, http.StatusCreated, rr.Code)
		var got Attachment
		assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, ent.BS, got.BS)
		assert.NotEmpty(t, got.ID)
		gotAttach, found, err := attaRepo.FindByID(ctx, got.ID)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, got, gotAttach)

		t.Log("parent relationship is also updated")
		gotNote, found, err := noteRepo.FindByID(ctx, note1.ID)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Contain(t, gotNote.Attachments, gotAttach.ID)
	})
}

func TestRESTHandler_idWithEscapedChars(tt *testing.T) {
	t := testcase.NewT(tt)
	exp := randomPathPart(t)

	type T struct{ ID string }

	h := httpkit.RESTHandler[T, string]{
		Show: func(ctx context.Context, id string) (ent T, found bool, err error) {
			t.Should.Equal(exp, id)
			return T{ID: id}, true, nil
		},

		IDParser: func(s string) (string, error) {
			return s, nil
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, pathkit.Clean(url.PathEscape(exp)), nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, rr.Result().StatusCode, http.StatusOK)
}
