package httpkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
)

// RestHandler is an HTTP Handler that allows you to expose a resource such as a repository as a Restful API resource.
// Depending on what CRUD operation is supported by the Handler.RestHandler, the Handler supports the following actions:
type RestHandler[Entity, ID any] struct {
	// Create will create a new entity in the restful resource.
	// Create is a collection endpoint.
	// 		POST /
	Create func(ctx context.Context, ptr *Entity) error
	// Index will return the entities, optionally filtered with the query argument.
	// Index is a collection endpoint.
	//		GET /
	Index func(ctx context.Context) (iterators.Iterator[Entity], error)
	// Show will return a single entity, looked up by its ID.
	// Show is a resource endpoint.
	// 		GET /:id
	Show func(ctx context.Context, id ID) (ent Entity, found bool, err error)
	// Update will update/replace an entity with the new state.
	// Update is a resource endpoint.
	// 		PUT   /:id - update/replace
	// 		PATCH /:id - partial update (WIP)
	Update func(ctx context.Context, ptr *Entity) error
	// Destroy will delete an entity, identified by its id.
	// Destroy is a resource endpoint.
	// 		 Delete /:id
	Destroy func(ctx context.Context, id ID) error
	// DestroyAll will delete all entity.
	// DestroyAll is a collection endpoint
	// 		 Delete /
	DestroyAll func(ctx context.Context) error
	// Serialization is responsible to serialize and unserialize dtokit.
	// JSON, line separated JSON stream and FormUrlencoded formats are supported out of the box.
	//
	// Serialization is an optional field.
	// Unless you have specific needs in serialization, don't configure it.
	Serialization RestResourceSerialization[Entity, ID]
	// Mapping is the primary Entity to DTO mapping configuration.
	Mapping dtokit.Mapper[Entity]
	// MappingForMediaType defines a per MIMEType Mapping, that takes priority over Mapping
	MappingForMediaType map[string]dtokit.Mapper[Entity]
	// ErrorHandler is used to handle errors from the request, by mapping the error value into an error DTOMapping.
	ErrorHandler ErrorHandler
	// IDContextKey is an optional field used to store the parsed ID from the URL in the context.
	//
	// Default: IDContextKey[Entity, ID]{}
	IDContextKey any
	// IDAccessor [optional] tells how to look up or set the Entity's ID.
	//
	// Default: extid.Lookup / extid.Set
	IDAccessor extid.Accessor[Entity, ID]
	// ResourceRoutes field is an http.Handler that will receive resource-specific requests.
	// ResourceRoutes field is optional.
	// ResourceRoutes are resource endpoints.
	//
	// The http.Request.Context will contain the parsed ID from the request path,
	// and can be accessed with the IDContextKey.
	//
	// Example paths
	// 		/plural-resource-identifier-name/:id/sub-routes
	// 		/users/42/status
	// 		/users/42/jobs/13
	//
	// Request paths will be stripped from their prefix.
	// For example, "/users/42/jobs" will end up as "/jobs".
	ResourceRoutes http.Handler
	// BodyReadLimit is the max bytes that the handler is willing to read from the request body.
	//
	// The default value is DefaultBodyReadLimit, which is preset to 16MB.
	BodyReadLimit iokit.ByteSize
	// CollectionContext is called when a collection endpoint is called.
	//
	// applies to:
	// 	- CREATE
	// 	- INDEX
	CollectionContext func(context.Context) (context.Context, error)
	// ResourceContext is called when a resource endpoint is called.
	//
	// applies to:
	// 	- SHOW
	// 	- UPDATE
	// 	- DESTORY
	// 	- sub routes
	ResourceContext func(context.Context, ID) (context.Context, error)
}

type resource interface {
	resource()
	http.Handler
}

func (res RestHandler[Entity, ID]) resource() {}

var _ resource = RestHandler[any, any]{}

type idConverter[ID any] interface {
	FormatID(ID) (string, error)
	ParseID(string) (ID, error)
}

func (res RestHandler[Entity, ID]) getMapping(mediaType string) dtokit.Mapper[Entity] {
	mediaType = getMediaType(mediaType) // TODO: TEST ME
	if res.MappingForMediaType != nil {
		if mapping, ok := res.MappingForMediaType[mediaType]; ok {
			return mapping
		}
	}
	if res.Mapping != nil {
		return res.Mapping
	}
	return passthroughMappingMode[Entity]()
}

// passthroughMappingMode enables a passthrough mapping mode where entity is used as the DTO itself.
// Since we can't rule out that they don't use restapi.Resource with a DTO type in the first place.
func passthroughMappingMode[Entity any]() dtokit.Mapping[Entity, Entity] {
	return dtokit.Mapping[Entity, Entity]{}
}

// Mapper is a generic interface used for representing a DTO-Entity mapping relationship.
// Its primary function is to allow Resource to list various mappings,
// each with its own DTO type, for different MIMEType values.
// This means we can use different DTO types within the same restful Resource handler based on different content types,
// making it more flexible and adaptable to support different Serialization formats.
//
// It is implemented by DTOMapping.
type Mapper[Entity any] interface {
	newDTO() (dtoPtr any)
	toEnt(ctx context.Context, dtoPtr any) (Entity, error)
	toDTO(ctx context.Context, ent Entity) (DTO any, _ error)
}

func (res RestHandler[Entity, ID]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(internal.WithRequest(r.Context(), r))
	defer res.handlePanic(w, r)
	r, rc := internal.WithRoutingContext(r)
	switch rc.PathLeft {
	case `/`, ``:
		if res.CollectionContext != nil {
			cctx, err := res.CollectionContext(r.Context())
			if err != nil {
				res.getErrorHandler().HandleError(w, r, err)
				return
			}
			r = r.WithContext(cctx)
		}
		switch r.Method {
		case http.MethodGet:
			res.index(w, r)
		case http.MethodPost:
			res.create(w, r)
		case http.MethodDelete:
			res.destroyAll(w, r)
		default:
			res.errMethodNotAllowed(w, r)
		}
		return

	default: // dynamic path
		resourceID, rest := pathkit.Unshift(rc.PathLeft)
		rc.Travel(resourceID)

		id, err := res.Serialization.getIDConverter().ParseID(resourceID)
		if err != nil {
			defaultErrorHandler.HandleError(w, r, ErrMalformedID.With().Detail(err.Error()))
			return
		}

		if res.IDContextKey != nil {
			r = r.WithContext(context.WithValue(r.Context(), res.IDContextKey, id))
		}

		if res.ResourceContext != nil {
			rctx, err := res.ResourceContext(r.Context(), id)
			if err != nil {
				res.getErrorHandler().HandleError(w, r, err)
				return
			}
			r = r.WithContext(rctx)
		}

		if rest != "/" {
			if res.ResourceRoutes == nil {
				res.getErrorHandler().HandleError(w, r, ErrPathNotFound)
				return
			}
			res.ResourceRoutes.ServeHTTP(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			res.show(w, r, id)
		case http.MethodPut, http.MethodPatch:
			res.update(w, r, id)
		case http.MethodDelete:
			res.destroy(w, r, id)
		}
	}

}

func (res RestHandler[Entity, ID]) handlePanic(w http.ResponseWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
	if err, ok := v.(error); ok {
		res.errInternalServerError(w, r, err)
		return
	}
	res.errInternalServerError(w, r, fmt.Errorf("recover: %v", v))
}

func (res RestHandler[Entity, ID]) getErrorHandler() ErrorHandler {
	if res.ErrorHandler != nil {
		return res.ErrorHandler
	}
	return defaultErrorHandler
}

func (res RestHandler[Entity, ID]) errInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		fmt.Println("ERROR", err.Error())
	}
	res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
}

func (res RestHandler[Entity, ID]) errMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrMethodNotAllowed)
}

func (res RestHandler[Entity, ID]) errEntityNotFound(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
}

// DefaultBodyReadLimit is the maximum number of bytes that a restapi.Handler will read from the requester,
// if the Handler.BodyReadLimit is not provided.
var DefaultBodyReadLimit int = 16 * iokit.Megabyte

func (res RestHandler[Entity, ID]) getBodyReadLimit() int {
	if res.BodyReadLimit != 0 {
		return res.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

func (res RestHandler[Entity, ID]) index(w http.ResponseWriter, r *http.Request) {
	if res.Index == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	index, err := res.Index(ctx)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	defer func() {
		if err := index.Close(); err != nil {
			logger.Warn(ctx, "error during closing the index result resource",
				logging.ErrField(err))
		}
	}()
	if err := index.Err(); err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	resSer, resMIMEType := res.Serialization.responseBodySerializer(r) // TODO:TEST_ME
	resMapping := res.getMapping(resMIMEType)

	serMaker, ok := resSer.(codec.ListEncoderMaker)
	if !ok {
		const code = http.StatusNotAcceptable
		http.Error(w, http.StatusText(code), code)
		return
	}

	w.Header().Set(headerKeyContentType, resMIMEType)
	listEncoder := serMaker.MakeListEncoder(w)

	defer func() {
		if err := listEncoder.Close(); err != nil {
			logger.Warn(ctx, "finishing the index list encoding encountered an error",
				logging.ErrField(err))
			return
		}
	}()

	var n int
	for ; index.Next(); n++ {
		ent := index.Value()

		dto, err := resMapping.MapToiDTO(ctx, ent)
		if err != nil {
			logger.Warn(ctx, "error during index element DTO Mapping", logging.ErrField(err))
			break
		}

		if err := listEncoder.Encode(dto); err != nil {
			logger.Warn(ctx, "error during DTO value encoding", logging.ErrField(err))
			break
		}
	}

	if err := index.Err(); err != nil {
		logger.Error(ctx, "error during iterating index result",
			logging.Field("entity_type", reflectkit.TypeOf[Entity]().String()),
			logging.ErrField(err))

		if n == 0 { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
			return
		}
		return
	}
}

func (res RestHandler[Entity, ID]) create(w http.ResponseWriter, r *http.Request) {
	if res.Create == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	data, err := res.readAllBody(r)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	var (
		ctx                 = r.Context()
		reqSer, reqMIMEType = res.Serialization.requestBodySerializer(r)
		reqMapping          = res.getMapping(reqMIMEType)
	)

	dtoPtr := reqMapping.NewiDTO()
	if err := reqSer.Unmarshal(data, dtoPtr); err != nil {
		logger.Debug(ctx, "invalid request body", logging.ErrField(err))
		res.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody)
		return
	}

	ent, err := reqMapping.MapFromiDTOPtr(ctx, dtoPtr)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.Create(ctx, &ent); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist.With().Wrap(err))
			return
		}
		logger.Error(ctx, "error during restapi.Resource#Create operation", logging.ErrField(err))
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	var (
		resSer, resMIMEType = res.Serialization.responseBodySerializer(r)
		resMapping          = res.getMapping(resMIMEType)
	)

	dto, err := resMapping.MapToiDTO(ctx, ent)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err = resSer.Marshal(dto) // TODO:TEST_ME
	if err != nil {
		logger.Error(ctx, "error during Marshaling entity operation",
			logging.Field("type", reflectkit.TypeOf[Entity]().String()),
			logging.ErrField(err))
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.Header().Set(headerKeyContentType, resMIMEType)
	w.WriteHeader(http.StatusCreated)

	if _, err := w.Write(data); err != nil {
		logger.Debug(ctx, "error during writing response to the caller",
			logging.ErrField(err))
	}
}

func (res RestHandler[Entity, ID]) show(w http.ResponseWriter, r *http.Request, id ID) {
	if res.Show == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	entity, found, err := res.Show(ctx, id)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		res.errEntityNotFound(w, r)
		return
	}

	resSer, resMIMEType := res.Serialization.responseBodySerializer(r)
	mapping := res.getMapping(resMIMEType)

	w.Header().Set(headerKeyContentType, resMIMEType)

	dto, err := mapping.MapToiDTO(ctx, entity)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err := resSer.Marshal(dto)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		logger.Debug(ctx, "error while writing back the response", logging.ErrField(err))
		return
	}
}

func (res RestHandler[Entity, ID]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if res.Update == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	var (
		ctx                 = r.Context()
		reqSer, reqMIMEType = res.Serialization.requestBodySerializer(r)
		reqMapping          = res.getMapping(reqMIMEType)
	)

	data, err := res.readAllBody(r)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	dtoPtr := reqMapping.NewiDTO()

	if err := reqSer.Unmarshal(data, dtoPtr); err != nil {
		res.getErrorHandler().HandleError(w, r,
			ErrInvalidRequestBody.With().Detail(err.Error()))
		return
	}

	if res.Show != nil { // TODO:TEST_ME
		ctx := r.Context()
		_, found, err := res.Show(ctx, id)
		if err != nil {
			res.getErrorHandler().HandleError(w, r, err)
			return
		}
		if !found {
			res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
	}

	entity, err := reqMapping.MapFromiDTOPtr(ctx, dtoPtr)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.IDAccessor.Set(&entity, id); err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.Update(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrNotFound) { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res RestHandler[Entity, ID]) destroy(w http.ResponseWriter, r *http.Request, id ID) {
	if res.Destroy == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	var ctx = r.Context()

	if res.Show != nil { // TODO:TEST_ME
		ctx := r.Context()
		_, found, err := res.Show(ctx, id)
		if err != nil {
			res.getErrorHandler().HandleError(w, r, err)
			return
		}
		if !found {
			res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
	}

	if err := res.Destroy(ctx, id); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res RestHandler[Entity, ID]) destroyAll(w http.ResponseWriter, r *http.Request) {
	if res.DestroyAll == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	if err := res.DestroyAll(r.Context()); err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res RestHandler[Entity, ID]) readAllBody(r *http.Request) (_ []byte, returnErr error) {
	return bodyReadAll(r.Body, res.getBodyReadLimit())
}

func (res RestHandler[Entity, ID]) WithCRUD(repo crud.ByIDFinder[Entity, ID]) RestHandler[Entity, ID] {
	if repo, ok := repo.(crud.Creator[Entity]); ok && res.Create == nil {
		res.Create = repo.Create
	}
	if repo, ok := repo.(crud.AllFinder[Entity]); ok && res.Index == nil {
		res.Index = repo.FindAll // TODO: handle query
	}
	if repo, ok := repo.(crud.ByIDFinder[Entity, ID]); ok && res.Show == nil {
		res.Show = repo.FindByID
	}
	if repo, ok := repo.(crud.Updater[Entity]); ok && res.Update == nil {
		res.Update = repo.Update
	}
	if repo, ok := repo.(crud.AllDeleter); ok && res.DestroyAll == nil {
		res.DestroyAll = repo.DeleteAll // TODO: handle query
	}
	if repo, ok := repo.(crud.ByIDDeleter[ID]); ok && res.Destroy == nil {
		res.Destroy = repo.DeleteByID
	}
	return res
}

func bodyReadAll(body io.ReadCloser, bodyReadLimit iokit.ByteSize) (_ []byte, returnErr error) {
	data, err := iokit.ReadAllWithLimit(body, bodyReadLimit)
	if errors.Is(err, iokit.ErrReadLimitReached) {
		return nil, ErrRequestEntityTooLarge
	}
	return data, err
}

const (
	headerKeyContentType = "Content-Type"
	headerKeyAccept      = "Accept"
)
