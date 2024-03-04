package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"io"
	"net/http"

	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/ports/crud"
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
	// Mapping takes care mapping back and forth Entity into a DTOMapping, and ID into a string.
	// ID needs mapping into a string because it is used as part of the restful paths.
	Mapping OldMapping[Entity, ID, DTO]
	// ErrorHandler is used to handle errors from the request, by mapping the error value into an error DTOMapping.
	ErrorHandler ErrorHandler
	// Router is the sub-router, where you can define routes related to entity related paths
	//  > .../:id/sub-routes
	Router *Router
	// BodyReadLimit is the max bytes that the handler is willing to read from the request body.
	//
	// The default value is DefaultBodyReadLimit, which is preset to 16MB.
	BodyReadLimit int64

	Operations[Entity, ID, DTO]

	// NoCreate will instruct the handler to not expose the `POST /` endpoint
	NoCreate bool
	// NoIndex will instruct the handler to not expose the `Get /` endpoint
	NoIndex bool
	// NoShow will instruct the handler to not expose the `Get /:id` endpoint
	NoShow bool
	// NoUpdate will instruct the handler to not expose the `PUT /:id` endpoint
	NoUpdate bool
	// NoDelete will instruct the handler to not expose the `DELETE /:id` endpoint
	NoDelete bool
}

func OperationsFromCRUD[Entity, ID, DTO any, Resource crud.ByIDFinder[Entity, ID]](resource Resource) Operations[Entity, ID, DTO] {
	var ops Operations[Entity, ID, DTO]

	return ops
}

// Operations is an optional config where you can customise individual restful operations.
type Operations[Entity, ID, DTO any] struct {
	// Index is an OPTIONAL field if you wish to customise the index operation's behaviour
	//   GET /
	//
	Index IndexOperation[Entity, ID, DTO]
	// Create is an OPTIONAL field if you wish to customise the create operation's behaviour
	//   POST /
	//
	Create CreateOperation[Entity, ID, DTO]
	// Show is an OPTIONAL field if you wish to customise the show operation's behaviour
	//   GET /:id
	//
	Show ShowOperation[Entity, ID, DTO]
	// Update is an OPTIONAL field if you wish to customise the update operation's behaviour
	//   PUT /:id
	//   PATCH /:id
	//
	Update UpdateOperation[Entity, ID, DTO]
	// Delete is an OPTIONAL field if you wish to customise the delete operation's behaviour
	//   DELETE /:id
	//
	Delete DeleteOperation[Entity, ID, DTO]
}

type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}

// OldMapping is the previous iteration's mapping solution.
// It was replaced because it coupled json serialization and the JSON DTOs with the Handler,
// excluding support for other serialisation formats.
//
// DEPRECATED
type OldMapping[Entity, ID, DTO any] interface {
	LookupID(Entity) (ID, bool)
	SetID(*Entity, ID)

	FormatID(ID) (string, error)
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
			if h.NoIndex {
				h.errMethodNotAllowed(w, r)
				return
			}
			h.index(w, r)
		case http.MethodPost:
			if h.NoCreate {
				h.errMethodNotAllowed(w, r)
				return
			}
			h.create(w, r)
		default:
			h.errMethodNotAllowed(w, r)
		}
		return

	default: // dynamic path
		resourceID, rest := pathkit.Unshift(rc.Path)
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
			if h.NoShow {
				h.errMethodNotAllowed(w, r)
				return
			}
			h.show(w, r, id)
		case http.MethodPut, http.MethodPatch:
			if h.NoUpdate {
				h.errMethodNotAllowed(w, r)
				return
			}
			h.update(w, r, id)
		case http.MethodDelete:
			if h.NoDelete {
				h.errMethodNotAllowed(w, r)
				return
			}
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

type BeforeHook http.HandlerFunc

func (h Handler[Entity, ID, DTO]) useBeforeHook(hook BeforeHook, w http.ResponseWriter, r *http.Request) (_continue bool) {
	if hook == nil {
		return true
	}
	usageTracker := &usageTrackerResponseWriter{ResponseWriter: w}
	hook(usageTracker, r)
	return !usageTracker.used
}

type usageTrackerResponseWriter struct {
	http.ResponseWriter
	used bool
}

func (rw *usageTrackerResponseWriter) Write(bs []byte) (int, error) {
	rw.used = true
	return rw.ResponseWriter.Write(bs)
}

func (rw *usageTrackerResponseWriter) WriteHeader(statusCode int) {
	rw.used = true
	rw.ResponseWriter.WriteHeader(statusCode)
}

//
//type Overrides[Entity, ID, DTO any] struct {
//	Create func(r *http.Request, ent Entity) (ID, error)
//	Index  func(r *http.Request) iterators.Iterator[Entity]
//	Show   func(r *http.Request, id ID) (Entity, error)
//	Update func(r *http.Request, ent Entity, id ID) (Entity, error)
//	Delete func(r *http.Request, id ID) error
//}

// SetIDByExtIDTag is a OldMapping tool that allows you to extract Entity ID using the `ext:"id"` tag.
//
// DEPRECATED
type SetIDByExtIDTag[Entity, ID any] struct{}

func (m SetIDByExtIDTag[Entity, ID]) LookupID(ent Entity) (ID, bool) {
	return extid.Lookup[ID](ent)
}

func (m SetIDByExtIDTag[Entity, ID]) SetID(ptr *Entity, id ID) {
	if err := extid.Set(ptr, id); err != nil {
		panic(fmt.Errorf("%T entity type doesn't have extid tag.\n%w", *new(Entity), err))
	}
}

// CreateOperation
//
// DEPRECATED
type CreateOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) create(w http.ResponseWriter, r *http.Request) {
	if !h.useBeforeHook(h.Operations.Create.BeforeHook, w, r) {
		return
	}

	creator, ok := h.Resource.(crud.Creator[Entity])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	defer r.Body.Close()

	bodyReadLimit := h.getBodyReadLimit()
	bytes, err := io.ReadAll(io.LimitReader(r.Body, bodyReadLimit))
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	var dto DTO
	if err := json.Unmarshal(bytes, &dto); err != nil {
		if int(bodyReadLimit) == len(bytes) {
			h.getErrorHandler().HandleError(w, r,
				ErrRequestEntityTooLarge.With().
					Detailf("Body Limit: %d", bodyReadLimit))
			return
		}

		h.getErrorHandler().HandleError(w, r,
			ErrInvalidRequestBody.With().
				Detail(err.Error()))
		return
	}

	ctx := r.Context()

	entity, err := h.Mapping.MapEntity(ctx, dto)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := json.Unmarshal(bytes, &dto); err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	if id, ok := h.Mapping.LookupID(entity); ok {
		_, found, err := h.Resource.FindByID(ctx, id)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		if found {
			h.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist)
			return
		}
	}

	if err := creator.Create(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) {
			err = ErrEntityAlreadyExist.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	dto, err = h.Mapping.MapDTO(ctx, entity)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusCreated, dto)
}

func (h Handler[Entity, ID, DTO]) getBodyReadLimit() int64 {
	if h.BodyReadLimit != 0 {
		return h.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

// DeleteOperation
//
// DEPRECATED
type DeleteOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) delete(w http.ResponseWriter, r *http.Request, id ID) {
	if !h.useBeforeHook(h.Operations.Delete.BeforeHook, w, r) {
		return
	}

	deleter, ok := h.Resource.(crud.ByIDDeleter[ID])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	_, found, err := h.Resource.FindByID(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
		return
	}

	if err := deleter.DeleteByID(ctx, id); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			err = ErrEntityNotFound.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// IndexOperation
//
// DEPRECATED
type IndexOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
	Override   func(r *http.Request) iterators.Iterator[Entity]
}

func (ctrl IndexOperation[Entity, ID, DTO]) handle(h Handler[Entity, ID, DTO], r *http.Request) (iterators.Iterator[Entity], bool) {
	if ctrl.Override != nil {
		return ctrl.Override(r), true
	}

	finder, ok := h.Resource.(crud.AllFinder[Entity])
	if !ok {
		return nil, false
	}

	return finder.FindAll(r.Context()), true
}

func (h Handler[Entity, ID, DTO]) index(w http.ResponseWriter, r *http.Request) {
	if !h.useBeforeHook(h.Operations.Index.BeforeHook, w, r) {
		return
	}
	iter, ok := h.Operations.Index.handle(h, r)
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	h.writeIterJSON(w, r, iter)
}

func (h Handler[Entity, ID, DTO]) writeIterJSON(w http.ResponseWriter, r *http.Request, iter iterators.Iterator[Entity]) {
	if err := iter.Err(); err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	defer func() { _ = iter.Close() }()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte("[")); err != nil {
		return
	}
	if iter.Next() {
		v := iter.Value()

		bytes, err := json.Marshal(&v)
		if err != nil {
			return
		}
		if _, err := w.Write(bytes); err != nil {
			return
		}
	}
	for iter.Next() {
		v := iter.Value()

		bytes, err := json.Marshal(&v)
		if err != nil {
			h.errInternalServerError(w, r, err)
		}
		if _, err := w.Write([]byte(`,`)); err != nil {

		}
		if _, err := w.Write(bytes); err != nil {
			return
		}
	}
	if _, err := w.Write([]byte(`]`)); err != nil {
		return
	}
	_, _ = w.Write([]byte("\n"))
}

// ShowOperation
//
// DEPRECATED
type ShowOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) show(w http.ResponseWriter, r *http.Request, id ID) {
	if !h.useBeforeHook(h.Operations.Show.BeforeHook, w, r) {
		return
	}

	finder, ok := h.Resource.(crud.ByIDFinder[Entity, ID])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	entity, found, err := finder.FindByID(r.Context(), id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.errEntityNotFound(w, r)
		return
	}

	dto, err := h.Mapping.MapDTO(r.Context(), entity)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, dto)
}

func (h Handler[Entity, ID, DTO]) writeJSON(w http.ResponseWriter, r *http.Request, code int, dto DTO) {
	bytes, err := json.Marshal(dto)
	if err != nil {
		h.errInternalServerError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(bytes)
	_, _ = w.Write([]byte("\n"))
}

// UpdateOperation
//
// DEPRECATED
type UpdateOperation[Entity, ID, DTO any] struct {
	BeforeHook BeforeHook
}

func (h Handler[Entity, ID, DTO]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if !h.useBeforeHook(h.Operations.Update.BeforeHook, w, r) {
		return
	}

	updater, ok := h.Resource.(crud.Updater[Entity])
	if !ok {
		h.errMethodNotAllowed(w, r)
		return
	}
	defer r.Body.Close()

	bodyReadLimit := h.getBodyReadLimit()
	bytes, err := io.ReadAll(io.LimitReader(r.Body, bodyReadLimit))
	if err != nil {
		defaultErrorHandler.HandleError(w, r,
			ErrRequestEntityTooLarge.With().
				Detailf("Body Limit: %d", bodyReadLimit))
		return
	}

	var dto DTO
	if err := json.Unmarshal(bytes, &dto); err != nil {
		h.getErrorHandler().HandleError(w, r,
			ErrInvalidRequestBody.With().
				Detail(err.Error()))
		return
	}

	ctx := r.Context()

	entity, err := h.Mapping.MapEntity(ctx, dto)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	h.Mapping.SetID(&entity, id)

	_, found, err := h.Resource.FindByID(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
		return
	}

	if err := updater.Update(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) {
			err = ErrEntityAlreadyExist.With().Wrap(err)
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
