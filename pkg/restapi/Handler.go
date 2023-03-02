package restapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/adamluzsi/frameless/pkg/pathutil"
	"github.com/adamluzsi/frameless/pkg/restapi/internal"
	"github.com/adamluzsi/frameless/ports/crud"
)

// Handler is a HTTP Handler that allows you to expose a resource such as a repository as a Restful API resource.
// Depending on what CRUD operation is supported by the Handler.Resource, the Handler support the following actions:
//   - Index:  GET    /
//   - Show:   GET    /:id
//   - Create: POST   /
//   - Update: PUT    /:id
//   - Delete: Delete /:id
type Handler[Entity, ID, DTO any] struct {
	// Resource is the CRUD Resource object that we wish to expose as a restful API resource.
	Resource crud.ByIDFinder[Entity, ID]
	// Mapping takes care mapping back and forth Entity into a DTO, and ID into a string.
	// ID needs mapping into a string because it is used as part of the restful paths.
	Mapping Mapping[Entity, ID, DTO]
	// ErrorHandler is used to handle errors from the request, by mapping the error value into an error DTO.
	ErrorHandler ErrorHandler
	// Router is the sub-router, where you can define routes related to entity related paths
	//  > .../:id/sub-routes
	Router *Router
	// BodyReadLimit is the max bytes that the handler is willing to read from the request body.
	BodyReadLimit int64
	// Index is the Restful API's index actions controller.
	// Index is an OPTIONAL field.
	// If you need to customise the index action, you can do so by providing a configured restapi.Index.
	Index Index[Entity, ID, DTO]
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

//
//type Overrides[Entity, ID, DTO any] struct {
//	Create func(r *http.Request, ent Entity) (ID, error)
//	Index  func(r *http.Request) iterators.Iterator[Entity]
//	Show   func(r *http.Request, id ID) (Entity, error)
//	Update func(r *http.Request, ent Entity, id ID) (Entity, error)
//	Delete func(r *http.Request, id ID) error
//}
