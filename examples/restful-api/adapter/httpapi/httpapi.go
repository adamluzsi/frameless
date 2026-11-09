package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"restapi/domain/mydomain"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/frameless/pkg/jsonkit"
)

type Config struct {
	UserRepository mydomain.UserRepository
	NoteRepository mydomain.NoteRepository
}

func NewServer(c Config) (*http.Server, error) {

	port, _, err := env.Lookup[int]("PORT", env.DefaultValue("8080"))
	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: MakeHandler(c),
	}, nil
}

func MakeHandler(c Config) http.Handler {
	var router httpkit.Router
	router.Resource("/users", MakeUsersAPI(c))

	return &router
}

var errorHandler = rfc7807.Handler{
	Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
		// keep the status codes of httpkit's own errors, like 404 for httpkit.ErrEntityNotFound.
		httpkit.ErrorMapping(ctx, err, dto)
		switch {
		case errors.Is(err, ErrUnauthorized):
			dto.Type.ID = "unauthorized"
			dto.Title = "Unauthorized request"
			dto.Status = http.StatusUnauthorized
			dto.Detail = err.Error()
		}
	},
}

const ErrUnauthorized errorkit.Error = "Unauthorized"

// jsonCodec is the JSON codec of the API.
// It writes and reads the domain entities through their JSON DTOs,
// so the JSON format of the API doesn't change with the domain types.
// The type IDs only show up when an entity is held by an interface, like an `any` field.
var jsonCodec = func() *jsonkit.Codec {
	var c jsonkit.Codec
	jsonkit.CodecRegister(&c, "restapi:user", dtoUserMapping)
	jsonkit.CodecRegister(&c, "restapi:note", dtoNoteMapping)
	return &c
}()

//* USER API - BEGIN *//

func MakeUsersAPI(c Config) httpkit.RESTHandler[mydomain.User, mydomain.UserID] {
	return httpkit.RESTHandler[mydomain.User, mydomain.UserID]{
		Create: c.UserRepository.Create,
		Index:  c.UserRepository.FindAll,
		Show:   c.UserRepository.FindByID,

		ResourceRoutes: httpkit.NewRouter(func(r *httpkit.Router) {
			r.Resource("/notes", MakeNotesAPI(c))
		}),

		ErrorHandler: errorHandler,

		MediaType: mediatype.JSON,
		Codecs: httpkit.Codecs{
			mediatype.JSON: jsonCodec,
		},

		ResourceContext: func(ctx context.Context, id mydomain.UserID) (context.Context, error) {
			req, _ := httpkit.LookupRequest(ctx)

			if err := ValidateIsAuthorisedForUser(c, req, id); err != nil {
				return nil, err
			}

			return ctxUserID.ContextWith(ctx, id), nil
		},
	}
}

type ctxKeyUserID struct{}

var ctxUserID contextkit.ValueHandler[ctxKeyUserID, mydomain.UserID]

func ValidateIsAuthorisedForUser(c Config, req *http.Request, id mydomain.UserID) error {
	// get proof that the UserID belongs to the current  user req.Header.Get("Au")
	return nil
}

var dtoUserMapping = jsonkit.DataTransferObjectMapping[mydomain.User, UserJSONDTO]{
	ToENT: func(ctx context.Context, dto UserJSONDTO) (mydomain.User, error) {
		return mydomain.User(dto), nil
	},
	ToDTO: func(ctx context.Context, ent mydomain.User) (UserJSONDTO, error) {
		return UserJSONDTO(ent), nil
	},
}

type UserJSONDTO struct {
	ID       mydomain.UserID `json:"id,omitempty"`
	Username string          `json:"username"`
}

//* USER API - END *//

func MakeNotesAPI(c Config) httpkit.RESTHandler[mydomain.Note, mydomain.NoteID] {
	return httpkit.RESTHandler[mydomain.Note, mydomain.NoteID]{
		// Codecs don't receive the request context,
		// so the Note is scoped to the authorised user here instead of in the DTO mapping.
		Create: func(ctx context.Context, ptr *mydomain.Note) error {
			userID, ok := ctxUserID.Lookup(ctx)
			if !ok {
				return ErrUnauthorized
			}
			ptr.UserID = userID
			return c.NoteRepository.Create(ctx, ptr)
		},
		Index:      c.NoteRepository.FindAll,
		Show:       c.NoteRepository.FindByID,
		Destroy:    c.NoteRepository.DeleteByID,
		DestroyAll: c.NoteRepository.DeleteAll,

		MediaType: mediatype.JSON,
		Codecs: httpkit.Codecs{
			mediatype.JSON: jsonCodec,
		},
	}
}

type NoteJSONDTO struct {
	ID     mydomain.NoteID `json:"id"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	UserID mydomain.UserID `json:"user_id"`
}

var dtoNoteMapping = jsonkit.DataTransferObjectMapping[mydomain.Note, NoteJSONDTO]{
	// The UserID is not read from the request body,
	// as the Note belongs to the user of the request path, see MakeNotesAPI.
	ToENT: func(ctx context.Context, dto NoteJSONDTO) (mydomain.Note, error) {
		return mydomain.Note{
			ID:    dto.ID,
			Title: dto.Title,
			Body:  dto.Body,
		}, nil
	},
	ToDTO: func(ctx context.Context, ent mydomain.Note) (NoteJSONDTO, error) {
		return NoteJSONDTO(ent), nil
	},
}
