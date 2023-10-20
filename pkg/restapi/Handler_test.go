package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/testcase/let"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/restapi/rfc7807"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func TestHandler(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		mdb = testcase.Let(s, func(t *testcase.T) *memory.Repository[Foo, int] {
			m := memory.NewMemory()
			return memory.NewRepository[Foo, int](m)
		})
		resource = testcase.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, int] {
			return mdb.Get(t)
		})
		mapping                = testcase.LetValue[restapi.Mapping[Foo, int, FooDTO]](s, FooMapping{})
		lastSubResourceRequest = testcase.LetValue[*http.Request](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
		return restapi.Handler[Foo, int, FooDTO]{
			Resource: resource.Get(t),
			Mapping:  mapping.Get(t),
			Router: restapi.NewRouter(func(router *restapi.Router) {
				router.MountRoutes(restapi.Routes{
					// match anything
					"/": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						lastSubResourceRequest.Set(t, r)
						http.Error(w, "", http.StatusTeapot)
					}),
				})
			}),
		}
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

		type WithHookFunc func(*testcase.T, *restapi.Handler[Foo, int, FooDTO], restapi.BeforeHook)

		itSupportsBeforeHook := func(s *testcase.Spec, withHook WithHookFunc) {
			s.When("BeforeHook is provided", func(s *testcase.Spec) {
				mw := testcase.Let[restapi.BeforeHook](s, nil)

				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					h := subject.Super(t)
					withHook(t, &h, mw.Get(t))
					return h
				})

				s.And("BeforeHook propagates the request forward to the next handler", func(s *testcase.Spec) {
					var (
						wasCalled = testcase.LetValue[bool](s, false)
						lastReq   = testcase.LetValue[*http.Request](s, nil)
					)
					mw.Let(s, func(t *testcase.T) restapi.BeforeHook {
						return func(w http.ResponseWriter, r *http.Request) {
							wasCalled.Set(t, true)
							lastReq.Set(t, r)
						}
					})

					s.Then("BeforeHook is called", func(t *testcase.T) {
						act(t)

						t.Must.True(wasCalled.Get(t), "it was expected that mw was called")
					})

					s.Then("last request is valid", func(t *testcase.T) {
						path.Set(t, path.Get(t)+"?foo=bar")
						act(t)

						r := lastReq.Get(t)
						t.Must.NotNil(r,
							"it was expected that the BeforeHook received a non-nil request")
						t.Must.Equal("bar", r.URL.Query().Get("foo"),
							"it is expected that the received request contains request data")
					})
				})

				s.And("the BeforeHook interrupts the request processing by using http.ResponseWriter.Write", func(s *testcase.Spec) {
					body := let.String(s)
					mw.Let(s, func(t *testcase.T) restapi.BeforeHook {
						return func(w http.ResponseWriter, r *http.Request) {
							_, _ = w.Write([]byte(body.Get(t)))
						}
					})

					s.Then("BeforeHook will be the last is called", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusOK, rr.Code)
						t.Must.Equal(body.Get(t), rr.Body.String())
					})
				})

				s.And("the BeforeHook interrupts the request processing by using http.ResponseWriter.WriteHeader", func(s *testcase.Spec) {
					mw.Let(s, func(t *testcase.T) restapi.BeforeHook {
						return func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusTeapot)
						}
					})

					s.Then("BeforeHook will be the last is called", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusTeapot, rr.Code)
						t.Must.Empty(rr.Body.String())
					})
				})
			})
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, int] {
					return struct{ crud.ByIDFinder[Foo, int] }{ByIDFinder: mdb.Get(t)}
				})

				s.Then("it will respond with StatusMethodNotAllowed, page not found", func(t *testcase.T) {
					rr := act(t)
					t.Must.Equal(http.StatusMethodNotAllowed, rr.Code)

					errDTO := respondsWithJSON[rfc7807.DTO](t, rr)
					t.Must.NotEmpty(errDTO)
					t.Must.Equal(restapi.ErrMethodNotAllowed.ID.String(), errDTO.Type.ID)
				})
			})

			s.When("index override is provided", func(s *testcase.Spec) {
				override := testcase.Let[func(r *http.Request) iterators.Iterator[Foo]](s, nil)

				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					h := subject.Super(t)
					h.Operations.Index.Override = override.Get(t)
					return h
				})

				s.And("it returns values without an issue", func(s *testcase.Spec) {
					foo := testcase.Let(s, func(t *testcase.T) Foo {
						return Foo{
							ID:  FooID(t.Random.Int()),
							Foo: t.Random.Int(),
						}
					})

					receivedRequest := testcase.LetValue[*http.Request](s, nil)
					override.Let(s, func(t *testcase.T) func(r *http.Request) iterators.Iterator[Foo] {
						return func(r *http.Request) iterators.Iterator[Foo] {
							receivedRequest.Set(t, r)
							return iterators.SingleValue(foo.Get(t))
						}
					})

					s.Then("override is used and the actual HTTP request passed to it", func(t *testcase.T) {
						path.Set(t, path.Get(t)+"?foo=bar")
						act(t)
						r := receivedRequest.Get(t)
						t.Must.NotNil(r,
							"it was expected that the override populate the receivedRequest variable")
						t.Must.Equal("bar", r.URL.Query().Get("foo"),
							"it is expected that the override has access to a valid request object")
					})

					s.Then("the result will be based on the value returned by the override", func(t *testcase.T) {
						rr := act(t)
						t.Must.Equal(http.StatusOK, rr.Code)
						t.Must.ContainExactly(
							[]FooDTO{{ID: foo.Get(t).ID, Foo: foo.Get(t).Foo}},
							respondsWithJSON[[]FooDTO](t, rr))
					})
				})

				s.And("the returned result has an issue", func(s *testcase.Spec) {
					expectedErr := let.Error(s)

					override.Let(s, func(t *testcase.T) func(r *http.Request) iterators.Iterator[Foo] {
						return func(r *http.Request) iterators.Iterator[Foo] {
							return iterators.Error[Foo](expectedErr.Get(t))
						}
					})

					subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
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

			itSupportsBeforeHook(s, func(t *testcase.T, r *restapi.Handler[Foo, int, FooDTO], bh restapi.BeforeHook) {
				r.Operations.Index.BeforeHook = bh
			})

			s.When("NoIndex flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					rapi := subject.Super(t)
					rapi.NoIndex = true
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

				ent, found, err := mdb.Get(t).FindByID(context.Background(), gotDTO.ID)
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
				mdb.Let(s, func(t *testcase.T) *memory.Repository[Foo, int] {
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

					ent, found, err := mdb.Get(t).FindByID(context.Background(), gotDTO.ID)
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, int] {
					return struct{ crud.ByIDFinder[Foo, int] }{ByIDFinder: mdb.Get(t)}
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
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
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

			itSupportsBeforeHook(s, func(t *testcase.T, r *restapi.Handler[Foo, int, FooDTO], bh restapi.BeforeHook) {
				r.Operations.Create.BeforeHook = bh
			})

			s.When("No Create flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					rapi := subject.Super(t)
					rapi.NoCreate = true
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

			itSupportsBeforeHook(s, func(t *testcase.T, r *restapi.Handler[Foo, int, FooDTO], bh restapi.BeforeHook) {
				r.Operations.Show.BeforeHook = func(w http.ResponseWriter, r *http.Request) {
					id, ok := mapping.Get(t).ContextLookupID(r.Context())
					t.Must.True(ok, "expected to find the ID in the context")
					t.Must.Equal(dto.Get(t).ID, id)
					bh(w, r)
				}
			})

			s.When("NoShow flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					rapi := subject.Super(t)
					rapi.NoShow = true
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
				ent, found, err := mdb.Get(t).FindByID(context.Background(), dto.Get(t).ID)
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(ent.Foo, updatedDTO.Get(t).Foo)
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), dto.Get(t).ID))
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, int] {
					return struct{ crud.ByIDFinder[Foo, int] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			itSupportsBeforeHook(s, func(t *testcase.T, r *restapi.Handler[Foo, int, FooDTO], bh restapi.BeforeHook) {
				r.Operations.Update.BeforeHook = func(w http.ResponseWriter, r *http.Request) {
					id, ok := mapping.Get(t).ContextLookupID(r.Context())
					t.Must.True(ok, "expected to find the ID in the context")
					t.Must.Equal(dto.Get(t).ID, id)
					bh(w, r)
				}
			})

			s.When("NoUpdate flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					rapi := subject.Super(t)
					rapi.NoUpdate = true
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

				_, found, err := mdb.Get(t).FindByID(context.Background(), dto.Get(t).ID)
				t.Must.NoError(err)
				t.Must.False(found, "expected that the entity is deleted")
			})

			WhenIDInThePathIsMalformed(s)

			s.When("the referenced entity is absent", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(mdb.Get(t).DeleteByID(context.Background(), dto.Get(t).ID))
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
				resource.Let(s, func(t *testcase.T) crud.ByIDFinder[Foo, int] {
					return struct{ crud.ByIDFinder[Foo, int] }{ByIDFinder: mdb.Get(t)}
				})

				ThenNotAllowed(s)
			})

			itSupportsBeforeHook(s, func(t *testcase.T, r *restapi.Handler[Foo, int, FooDTO], bh restapi.BeforeHook) {
				r.Operations.Delete.BeforeHook = func(w http.ResponseWriter, r *http.Request) {
					id, ok := mapping.Get(t).ContextLookupID(r.Context())
					t.Must.True(ok, "expected to find the ID in the context")
					t.Must.Equal(dto.Get(t).ID, id)
					bh(w, r)
				}
			})

			s.When("NoDelete flag is set", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					rapi := subject.Super(t)
					rapi.NoDelete = true
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

				id, ok := subject.Get(t).Mapping.ContextLookupID(req.Context())
				t.Must.True(ok)
				t.Must.Equal(42, id)

				routing, ok := internal.LookupRouting(req.Context())
				t.Must.True(ok)
				t.Must.Equal("/bars", routing.Path)
			})

			s.And(".Routes is nil", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) restapi.Handler[Foo, int, FooDTO] {
					v := subject.Super(t)
					v.Router = nil
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

func respondsWithJSON[DTO any](t *testcase.T, recorder *httptest.ResponseRecorder) DTO {
	var dto DTO
	t.Log("body:", recorder.Body.String())
	t.Must.NotEmpty(recorder.Body.Bytes())
	t.Must.NoError(json.Unmarshal(recorder.Body.Bytes(), &dto))
	return dto
}
