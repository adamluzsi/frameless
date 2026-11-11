package httpkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"go.llib.dev/testcase/tchttp"
)

func ExampleTransactionMiddleware() {
	m := memory.NewMemory() // a commit manager, like a database connection
	fooRepository := memory.NewRepository[X, XID](m)

	var router httpkit.Router
	router.Resource("/foos", httpkit.RESTHandlerFromCRUD[X, XID](fooRepository))

	handler := httpkit.TransactionMiddleware{
		Next:          &router,
		CommitManager: m,
		// ErrorHandler is optional, and replies when the transaction can't begin or commit.
		ErrorHandler: rfc7807.Handler{Mapping: httpkit.ErrorMapping},
	}

	_ = http.ListenAndServe(":8080", handler)
}

func TestTransactionMiddleware(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(func(t *testcase.T) { logger.Testing(t) })

	var (
		repo = let.Var(s, func(t *testcase.T) *memory.Repository[X, XID] {
			return memory.NewRepository[X, XID](memory.NewMemory())
		})
		commitManager = let.Var(s, func(t *testcase.T) *commitManagerStub {
			return &commitManagerStub{OnePhaseCommitProtocol: repo.Get(t)}
		})
		recorder = let.Var(s, func(t *testcase.T) *httptest.ResponseRecorder {
			return httptest.NewRecorder()
		})
		code = let.VarOf(s, http.StatusCreated)
		body = let.Var(s, func(t *testcase.T) string {
			return t.Random.StringNC(16, random.CharsetAlpha())
		})
		// sentDuringNext is what the client received by the time next finished writing its reply.
		sentDuringNext = let.VarOf(s, "")
		next           = let.Var(s, func(t *testcase.T) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ent := X{N: t.Random.Int()}
				assert.Must(t).NoError(repo.Get(t).Create(r.Context(), &ent))
				w.WriteHeader(code.Get(t))
				_, _ = w.Write([]byte(body.Get(t)))
				sentDuringNext.Set(t, recorder.Get(t).Body.String())
			})
		})
	)
	subject := let.Var(s, func(t *testcase.T) httpkit.TransactionMiddleware {
		return httpkit.TransactionMiddleware{
			Next:          next.Get(t),
			CommitManager: commitManager.Get(t),
		}
	})

	storedEntities := func(t *testcase.T) []X {
		vs, err := iterkit.CollectE(repo.Get(t).FindAll(context.Background()))
		assert.Must(t).NoError(err)
		return vs
	}

	tchttp.HandlerMiddleware(func(t *testcase.T, next http.Handler) http.Handler {
		return httpkit.TransactionMiddleware{Next: next, CommitManager: memory.NewMemory()}
	}).Spec(s)

	s.When("the request changes state", func(s *testcase.Spec) {
		tchttp.HandlerMiddleware(func(t *testcase.T, next http.Handler) http.Handler {
			return httpkit.TransactionMiddleware{Next: next, CommitManager: memory.NewMemory()}
		}, tchttp.HandlerMiddlewareOption{
			Request: func(t *testcase.T) *http.Request {
				return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(t.Random.String()))
			},
		}).Spec(s)
	})

	s.Describe("#ServeHTTP", func(s *testcase.Spec) {
		method := let.VarOf(s, http.MethodPost)

		act := func(t *testcase.T) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method.Get(t), "/", nil)
			subject.Get(t).ServeHTTP(recorder.Get(t), r)
			return recorder.Get(t)
		}

		// customErrorHandler replies with 503 and the error message,
		// so its replies can't be mistaken for the default ErrorHandler's 500.
		customErrorHandler := rfc7807.Handler{Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
			dto.Type.ID = "transaction-failure"
			dto.Status = http.StatusServiceUnavailable
			dto.Detail = err.Error()
		}}

		s.Then("the changes of the request are committed", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, code.Get(t), rr.Code)
			assert.Equal(t, body.Get(t), rr.Body.String())

			assert.Equal(t, 1, len(storedEntities(t)))
		})

		s.Then("the reply is held back until the commit", func(t *testcase.T) {
			act(t)

			assert.Empty(t, sentDuringNext.Get(t))
		})

		s.When("the reply has an error status code", func(s *testcase.Spec) {
			code.Let(s, func(t *testcase.T) int {
				return random.Pick(t.Random,
					http.StatusBadRequest,
					http.StatusNotFound,
					http.StatusConflict,
					http.StatusInternalServerError,
				)
			})

			s.Then("the changes are rolled back, and the reply is sent as is", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, code.Get(t), rr.Code)
				assert.Equal(t, body.Get(t), rr.Body.String())

				assert.Empty(t, storedEntities(t))
			})
		})

		s.When("next panics after it made changes", func(s *testcase.Spec) {
			next.Let(s, func(t *testcase.T) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ent := X{N: t.Random.Int()}
					assert.Must(t).NoError(repo.Get(t).Create(r.Context(), &ent))
					panic("boom")
				})
			})

			s.Then("the changes are rolled back, and the panic is passed on", func(t *testcase.T) {
				got := assert.Panic(t, func() { act(t) })
				assert.Equal[any](t, "boom", got)

				assert.Empty(t, storedEntities(t))
			})
		})

		s.When("the transaction can't begin", func(s *testcase.Spec) {
			expErr := let.Error(s)
			commitManager.Let(s, func(t *testcase.T) *commitManagerStub {
				cm := commitManager.Super(t)
				cm.BeginTxErr = expErr.Get(t)
				return cm
			})

			s.Then("it replies with an error, without calling next", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusInternalServerError, rr.Code)

				assert.Empty(t, storedEntities(t))
			})

			s.And("an ErrorHandler is configured", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.TransactionMiddleware {
					mw := subject.Super(t)
					mw.ErrorHandler = customErrorHandler
					return mw
				})

				s.Then("the ErrorHandler replies the error", func(t *testcase.T) {
					rr := act(t)
					assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
					dto := respondsWithJSON[rfc7807.DTO](t, rr)
					assert.Equal(t, "transaction-failure", dto.Type.ID)
					assert.Equal(t, expErr.Get(t).Error(), dto.Detail)

					assert.Empty(t, storedEntities(t))
				})
			})
		})

		s.When("the transaction can't be committed", func(s *testcase.Spec) {
			expErr := let.Error(s)
			commitManager.Let(s, func(t *testcase.T) *commitManagerStub {
				cm := commitManager.Super(t)
				cm.CommitTxErr = expErr.Get(t)
				return cm
			})

			s.Then("it replies with an error instead of the reply of next", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusInternalServerError, rr.Code)
				assert.NotContains(t, rr.Body.String(), body.Get(t))

				assert.Empty(t, storedEntities(t))
			})

			s.And("an ErrorHandler is configured", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) httpkit.TransactionMiddleware {
					mw := subject.Super(t)
					mw.ErrorHandler = customErrorHandler
					return mw
				})

				s.Then("the ErrorHandler replies the error instead of the reply of next", func(t *testcase.T) {
					rr := act(t)
					assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
					dto := respondsWithJSON[rfc7807.DTO](t, rr)
					assert.Equal(t, "transaction-failure", dto.Type.ID)
					assert.Equal(t, expErr.Get(t).Error(), dto.Detail)

					assert.Empty(t, storedEntities(t))
				})
			})
		})

		s.When("the request method is safe, so it doesn't change state", func(s *testcase.Spec) {
			method.Let(s, func(t *testcase.T) string {
				return random.Pick(t.Random,
					http.MethodGet,
					http.MethodHead,
					http.MethodOptions,
					http.MethodTrace,
				)
			})

			s.Then("the reply streams to the client while next writes it", func(t *testcase.T) {
				act(t)

				assert.Equal(t, body.Get(t), sentDuringNext.Get(t))
			})

			s.And("the transaction can't begin", func(s *testcase.Spec) {
				expErr := let.Error(s)
				commitManager.Let(s, func(t *testcase.T) *commitManagerStub {
					cm := commitManager.Super(t)
					cm.BeginTxErr = expErr.Get(t)
					return cm
				})

				s.Then("next serves the request without a transaction", func(t *testcase.T) {
					rr := act(t)
					assert.Equal(t, code.Get(t), rr.Code)
					assert.Equal(t, body.Get(t), rr.Body.String())
				})
			})
		})
	})
}
