# Worked example: a nested REST API, its spec, and a contract-tested client

This is a complete, verified example. It ran green with `go test -race -count=5` against the current `pkg/httpkit`. It contains:

- a `notes` domain with `User` and `Note` entities and their repository role interfaces;
- an `httpapi` adapter that serves `/api/v1/users/:user_id/notes` with `Router`, nested `RESTHandler`s, an auth check in `ResourceContext`, a DTO codec and an rfc7807 error handler that keeps the built-in statuses;
- a `testcase` spec of the handler, using `httptest` and the `memory` repositories;
- a `RESTClient` for the same API, run against `crudcontract` in-process through `RoundTripperFunc`.

Replace `example.com/app` with your module path.

## `domain/notes/notes.go`

```go
// Package notes is the domain: entities and the role interfaces it needs.
package notes

import "go.llib.dev/frameless/port/crud"

type UserID string

type User struct {
	ID   UserID `ext:"id"`
	Name string
}

type NoteID string

type Note struct {
	ID     NoteID `ext:"id"`
	UserID UserID // a Note belongs to a User: relationship.Related finds it by convention
	Title  string
	Body   string
}

type UserRepository interface {
	crud.Creator[User]
	crud.ByIDFinder[User, UserID]
}

type NoteRepository interface {
	crud.Creator[Note]
	crud.AllFinder[Note]
	crud.ByIDFinder[Note, NoteID]
	crud.Updater[Note]
	crud.ByIDDeleter[NoteID]
	crud.AllDeleter
}
```

## `adapter/httpapi/httpapi.go`

```go
// Package httpapi is the driving adapter: it exposes the notes domain over HTTP.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"example.com/app/domain/notes"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/jsonkit"
)

type Config struct {
	Users notes.UserRepository
	Notes notes.NoteRepository
}

func NewHandler(c Config) http.Handler {
	return httpkit.NewRouter(func(r *httpkit.Router) {
		r.Use(httpkit.WithAccessLog)
		r.Get("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		r.Namespace("/api/v1", func(r *httpkit.Router) {
			r.Resource("/users", usersAPI(c))
		})
	})
}

// ErrUnauthorized is an errorkit.UserError, so rfc7807 replies with its Code and Message.
var ErrUnauthorized = errorkit.UserError{
	Code:    "unauthorized",
	Message: "The request is not authorised for this user.",
}

var errorHandler = rfc7807.Handler{
	BaseURL: "/errors",
	Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
		httpkit.ErrorMapping(ctx, err, dto) // keep the built-in 404/405/409/413 statuses
		if errors.Is(err, ErrUnauthorized) {
			dto.Status = http.StatusUnauthorized
		}
	},
}

func usersAPI(c Config) httpkit.RESTHandler[notes.User, notes.UserID] {
	return httpkit.RESTHandler[notes.User, notes.UserID]{
		Create: c.Users.Create,
		Show:   c.Users.FindByID,

		ResourceContext: func(ctx context.Context, id notes.UserID) (context.Context, error) {
			req, _ := httpkit.LookupRequest(ctx)
			if req.Header.Get("X-User-ID") != string(id) { // stand-in for real authentication
				return ctx, ErrUnauthorized
			}
			return ctx, nil
		},
		ResourceRoutes: httpkit.NewRouter(func(r *httpkit.Router) {
			r.Resource("/notes", notesAPI(c))
		}),

		ErrorHandler: errorHandler,
	}
}

// NoteJSON is the wire format. The domain Note never gets json tags.
type NoteJSON struct {
	ID     string `json:"id,omitempty"`
	UserID string `json:"user_id,omitempty"` // a scoped PUT is ownership-checked, so it must round-trip
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
}

func notesCodec() *jsonkit.Codec {
	var c jsonkit.Codec
	jsonkit.CodecRegister(&c, "note", jsonkit.DataTransferObjectMapping[notes.Note, NoteJSON]{
		ToDTO: func(ctx context.Context, n notes.Note) (NoteJSON, error) {
			return NoteJSON{ID: string(n.ID), UserID: string(n.UserID), Title: n.Title, Body: n.Body}, nil
		},
		ToENT: func(ctx context.Context, dto NoteJSON) (notes.Note, error) {
			if strings.TrimSpace(dto.Title) == "" {
				return notes.Note{}, errors.New("title is required") // becomes 400 invalid-request-body
			}
			return notes.Note{
				ID:     notes.NoteID(dto.ID),
				UserID: notes.UserID(dto.UserID),
				Title:  dto.Title,
				Body:   dto.Body,
			}, nil
		},
	})
	return &c
}

func notesAPI(c Config) httpkit.RESTHandler[notes.Note, notes.NoteID] {
	return httpkit.RESTHandlerFromCRUD[notes.Note, notes.NoteID](c.Notes, func(h *httpkit.RESTHandler[notes.Note, notes.NoteID]) {
		h.MediaType = mediatype.JSON
		h.Codecs = httpkit.Codecs{mediatype.JSON: notesCodec()}
		h.ErrorHandler = errorHandler
	})
}
```

What the routes do:

| Request | Result |
| --- | --- |
| `GET /api/v1/users/1/notes` | 200, only the notes whose `UserID` is `1` (ownership filter) |
| `POST /api/v1/users/1/notes` `{"title":"x"}` | 201, `UserID` set to `1` from the URL |
| `PUT /api/v1/users/1/notes/4` with `user_id` `1` | 204, the whole note is replaced |
| `PUT …/notes/4` without `user_id` | 404: the new state fails the ownership check |
| any `/api/v1/users/1/…` without `X-User-ID: 1` | 401 `unauthorized` from `ResourceContext` |
| `GET /api/v1/users` | 405 `rest-method-not-allowed` (no `Index`) |

## `adapter/httpapi/httpapi_test.go`

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"example.com/app/adapter/httpapi"
	"example.com/app/domain/notes"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/port/crud/crudcontract"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestNewHandler(t *testing.T) {
	s := testcase.NewSpec(t)
	logger.Testing(t)

	var (
		mem   = let.Var(s, func(t *testcase.T) *memory.Memory { return memory.NewMemory() })
		users = let.Var(s, func(t *testcase.T) *memory.Repository[notes.User, notes.UserID] {
			return memory.NewRepository[notes.User, notes.UserID](mem.Get(t))
		})
		noteRepo = let.Var(s, func(t *testcase.T) *memory.Repository[notes.Note, notes.NoteID] {
			return memory.NewRepository[notes.Note, notes.NoteID](mem.Get(t))
		})
		subject = let.Var(s, func(t *testcase.T) http.Handler {
			return httpapi.NewHandler(httpapi.Config{Users: users.Get(t), Notes: noteRepo.Get(t)})
		})
		user = let.Var(s, func(t *testcase.T) notes.User {
			u := notes.User{Name: t.Random.String()}
			assert.Must(t).NoError(users.Get(t).Create(context.Background(), &u))
			return u
		})
		authUserID = let.Var(s, func(t *testcase.T) notes.UserID { return user.Get(t).ID })
	)

	notesPath := func(t *testcase.T, rest ...string) string {
		return pathkit.Join(append([]string{"/api/v1/users", url.PathEscape(string(user.Get(t).ID)), "notes"}, rest...)...)
	}

	s.Describe("GET /api/v1/users/:id/notes", func(s *testcase.Spec) {
		var (
			mine = let.Var(s, func(t *testcase.T) notes.Note {
				n := notes.Note{UserID: user.Get(t).ID, Title: t.Random.String()}
				assert.Must(t).NoError(noteRepo.Get(t).Create(context.Background(), &n))
				return n
			})
			theirs = let.Var(s, func(t *testcase.T) notes.Note {
				n := notes.Note{UserID: notes.UserID(t.Random.UUID()), Title: t.Random.String()}
				assert.Must(t).NoError(noteRepo.Get(t).Create(context.Background(), &n))
				return n
			})
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodGet, notesPath(t), nil)
			r.Header.Set("X-User-ID", string(authUserID.Get(t)))
			rr := httptest.NewRecorder()
			subject.Get(t).ServeHTTP(rr, r)
			return rr
		}

		s.Then("it lists only the notes of the user, in the DTO format", func(t *testcase.T) {
			mine.Get(t)
			theirs.Get(t)

			rr := act(t)
			assert.Equal(t, http.StatusOK, rr.Code)

			var got []httpapi.NoteJSON
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Equal(t, []httpapi.NoteJSON{{
				ID:     string(mine.Get(t).ID),
				UserID: string(user.Get(t).ID),
				Title:  mine.Get(t).Title,
			}}, got)
		})

		s.When("the caller is not the user", func(s *testcase.Spec) {
			authUserID.Let(s, func(t *testcase.T) notes.UserID { return notes.UserID(t.Random.UUID()) })

			s.Then("it replies 401 with an rfc7807 error", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

				var dto rfc7807.DTO
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
				assert.Equal(t, httpapi.ErrUnauthorized.Code.String(), dto.Type.ID)
			})
		})
	})

	s.Describe("POST /api/v1/users/:id/notes", func(s *testcase.Spec) {
		body := let.Var(s, func(t *testcase.T) string {
			return `{"title":"` + t.Random.StringNC(8, "abcdef") + `"}`
		})
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodPost, notesPath(t), strings.NewReader(body.Get(t)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-User-ID", string(authUserID.Get(t)))
			rr := httptest.NewRecorder()
			subject.Get(t).ServeHTTP(rr, r)
			return rr
		}

		s.Then("it creates the note under the user from the URL", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, http.StatusCreated, rr.Code)

			var dto httpapi.NoteJSON
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
			stored, found, err := noteRepo.Get(t).FindByID(context.Background(), notes.NoteID(dto.ID))
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, user.Get(t).ID, stored.UserID)
		})

		s.When("the title is missing", func(s *testcase.Spec) {
			body.LetValue(s, `{"body":"no title"}`)

			s.Then("it replies 400 invalid-request-body", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusBadRequest, rr.Code)
				var dto rfc7807.DTO
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
				assert.Equal(t, httpkit.ErrInvalidRequestBody.Code.String(), dto.Type.ID)
			})
		})
	})

	s.Describe("PUT /api/v1/users/:id/notes/:id", func(s *testcase.Spec) {
		var (
			note = let.Var(s, func(t *testcase.T) notes.Note {
				n := notes.Note{UserID: user.Get(t).ID, Title: "old", Body: "dropped by PUT"}
				assert.Must(t).NoError(noteRepo.Get(t).Create(context.Background(), &n))
				return n
			})
			body = let.Var(s, func(t *testcase.T) string {
				return `{"title":"new","user_id":"` + string(user.Get(t).ID) + `"}`
			})
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodPut, notesPath(t, url.PathEscape(string(note.Get(t).ID))), strings.NewReader(body.Get(t)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-User-ID", string(authUserID.Get(t)))
			rr := httptest.NewRecorder()
			subject.Get(t).ServeHTTP(rr, r)
			return rr
		}

		s.Then("it replaces the whole note and replies 204", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, http.StatusNoContent, rr.Code)
			got, _, err := noteRepo.Get(t).FindByID(context.Background(), note.Get(t).ID)
			assert.NoError(t, err)
			assert.Equal(t, "new", got.Title)
			assert.Equal(t, "", got.Body)
		})

		s.When("the body omits the parent reference", func(s *testcase.Spec) {
			body.LetValue(s, `{"title":"new"}`)

			s.Then("the ownership check fails with 404", func(t *testcase.T) {
				assert.Equal(t, http.StatusNotFound, act(t).Code)
			})
		})
	})
}
```

## A `RESTClient` for the same API, under `crudcontract`

The client is a crud repository of the wire type. Running `crudcontract` against it proves the server and client agree on status codes, the ID in the path, and the codec. `RoundTripperFunc` serves the requests in-process, so no port is opened.

```go
func TestRESTClient_againstNotesAPI(t *testing.T) {
	logger.Testing(t)
	mem := memory.NewMemory()
	users := memory.NewRepository[notes.User, notes.UserID](mem)
	noteRepo := memory.NewRepository[notes.Note, notes.NoteID](mem)
	u := notes.User{Name: "bob"}
	assert.NoError(t, users.Create(context.Background(), &u))

	h := httpapi.NewHandler(httpapi.Config{Users: users, Notes: noteRepo})
	client := httpkit.RESTClient[httpapi.NoteJSON, string]{
		BaseURL: "http://api.example/api/v1/users/:user_id/notes",
		HTTPClient: &http.Client{Transport: httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if err := r.Context().Err(); err != nil {
				return nil, err // a real transport fails on a cancelled ctx too; crudcontract checks it
			}
			r.Header.Set("X-User-ID", string(u.ID))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			return rr.Result(), nil
		})},
		// RESTClient escapes the ID and the path params itself, so they are given as they are.
		WithContext: func(ctx context.Context) context.Context {
			return httpkit.WithPathParam(ctx, "user_id", string(u.ID))
		},
	}

	conf := crudcontract.Config[httpapi.NoteJSON, string]{
		MakeEntity: func(tb testing.TB) httpapi.NoteJSON {
			return httpapi.NoteJSON{UserID: string(u.ID), Title: testcase.ToT(&tb).Random.StringNC(8, "abcdef")}
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}
	t.Run("Creator", crudcontract.Creator[httpapi.NoteJSON, string](client, conf).Test)
	t.Run("Finder", crudcontract.Finder[httpapi.NoteJSON, string](client, conf).Test)
	t.Run("Updater", crudcontract.Updater[httpapi.NoteJSON, string](client, conf).Test)
	t.Run("Deleter", crudcontract.Deleter[httpapi.NoteJSON, string](client, conf).Test)
}
```

Notes:

- The client works on `NoteJSON`, the wire type. To return domain `Note`s instead, give the client `Codecs: httpkit.Codecs{mediatype.JSON: notesCodec()}`. Then the codec is shared between the server and the client, so keep it in the adapter package and export a constructor.
- Without the `ctx.Err()` check in the `RoundTripperFunc`, the `Deleter` contract fails its "context is cancelled" cases, because `httptest.ResponseRecorder` ignores cancellation. With `httptest.NewServer` and `srv.Client()` you don't need the check.
