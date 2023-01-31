package restapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/adamluzsi/frameless/pkg/pathutil"
	"github.com/adamluzsi/frameless/pkg/restapi/internal"
	"github.com/adamluzsi/frameless/ports/crud"
)

type Handler[Entity, ID, DTO any] struct {
	Resource     crud.ByIDFinder[Entity, ID]
	Mapping      Mapping[Entity, ID, DTO]
	ErrorHandler ErrorHandler
	Router       *Router

	// BodyReadLimit is the max bytes that the handler is willing to read from the request body.
	BodyReadLimit int64
}

type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}

type Mapping[Entity, ID, DTO any] interface {
	LookupID(Entity) (ID, bool)
	SetID(*Entity, ID)

	EncodeID(ID) (string, error)
	ParseID(string) (ID, error)

	ContextWithID(context.Context, ID) context.Context
	ContextLookupID(ctx context.Context) (ID, bool)

	MapEntity(context.Context, DTO) (Entity, error)
	MapDTO(context.Context, Entity) (DTO, error)
}

func (h Handler[Entity, ID, DTO]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer h.handlePanic(w, r)
	r, rc := internal.WithRoutingCountex(r)
	switch rc.Path {
	case `/`, ``:
		switch r.Method {
		case http.MethodGet:
			h.index(w, r)
		case http.MethodPost:
			h.create(w, r)
		default:
			h.errMethodNotAllowed(w, r)
		}
		return

	default: // dynamic path
		resourceID, rest := pathutil.Unshift(rc.Path)
		withMountPoint(rc, Path(resourceID))

		id, err := h.Mapping.ParseID(resourceID)
		if err != nil {
			defaultErrorHandler.HandleError(w, r, ErrMalformedID.With().Detail(err.Error()))
			return
		}

		r = r.WithContext(h.Mapping.ContextWithID(r.Context(), id))

		if rest != "/" {
			h.route(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.show(w, r, id)
		case http.MethodPut, http.MethodPatch:
			h.update(w, r, id)
		case http.MethodDelete:
			h.delete(w, r, id)
		}
	}
}

func (h Handler[Entity, ID, DTO]) handlePanic(w http.ResponseWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
}

func (h Handler[Entity, ID, DTO]) getErrorHandler() ErrorHandler {
	if h.ErrorHandler != nil {
		return h.ErrorHandler
	}
	return defaultErrorHandler
}

func (h Handler[Entity, ID, DTO]) errInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		fmt.Println("ERROR", err.Error())
	}
	h.getErrorHandler().HandleError(w, r, ErrInternalServerError)
}

func (h Handler[Entity, ID, DTO]) errMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.getErrorHandler().HandleError(w, r, ErrMethodNotAllowed)
}

func (h Handler[Entity, ID, DTO]) errEntityNotFound(w http.ResponseWriter, r *http.Request) {
	h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
}

func (h Handler[Entity, ID, DTO]) route(w http.ResponseWriter, r *http.Request) {
	if h.Router == nil {
		h.getErrorHandler().HandleError(w, r, ErrPathNotFound)
		return
	}

	h.Router.ServeHTTP(w, r)
}
