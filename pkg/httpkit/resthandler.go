package httpkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
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

// RESTHandler implements an http.Handler that adheres to the Representational State of Resource (REST) architectural style.
//
// What is REST?
//
// REST, short for Representational State Transfer,
// is an architectural style for designing networked applications.
// It was introduced by Roy Fielding in his 2000 PhD dissertation.
//
// The primary goals of a RESTful API are to:
// * Provide a uniform interface for interacting with resources (CRUD over HTTP)
// * Separate concerns between client and server
// * Use standard HTTP methods (e.g., GET, POST, PUT, DELETE) to manipulate resources
//
// This RESTHandler provides a foundation for building RESTful APIs that meet these goals.
type RESTHandler[ENT, ID any] struct {
	// Create will create a new entity in the restful resource.
	// Create is a collection endpoint.
	// 		POST /
	Create func(ctx context.Context, ptr *ENT) error
	// Index will return the entities, optionally filtered with the query argument.
	// Index is a collection endpoint.
	//		GET /
	Index func(ctx context.Context) (iterators.Iterator[ENT], error)
	// Show will return a single entity, looked up by its ID.
	// Show is a resource endpoint.
	// 		GET /:id
	Show func(ctx context.Context, id ID) (ent ENT, found bool, err error)
	// Update will update/replace an entity with the new state.
	// Update is a resource endpoint.
	// 		PUT   /:id - update/replace
	// 		PATCH /:id - partial update (WIP)
	Update func(ctx context.Context, ptr *ENT) error
	// Destroy will delete an entity, identified by its id.
	// Destroy is a resource endpoint.
	// 		 Delete /:id
	Destroy func(ctx context.Context, id ID) error
	// DestroyAll will delete all entity.
	// DestroyAll is a collection endpoint
	// 		 Delete /
	DestroyAll func(ctx context.Context) error
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
	// Mapping [optional] is the generic ENT to DTO mapping configuration.
	//
	// default: the ENT type itself is used as the DTO type.
	Mapping dtokit.Mapper[ENT]
	// MediaType [optional] configures what MediaType the handler should use, when the request doesn't defines it.
	//
	// default: DefaultCodec.MediaType
	MediaType mediatype.MediaType
	// MediaTypeMappings [optional] defines a per MediaType DTO Mapping,
	// that takes priority over the Mapping.
	//
	// default: Mapping is used.
	MediaTypeMappings MediaTypeMappings[ENT]
	// MediaTypeCodecs [optional] contains per media type related codec which is used to marshal and unmarshal data in the response and response body.
	//
	// default: will use httpkit.DefaultCodecs
	MediaTypeCodecs MediaTypeCodecs
	// ErrorHandler [optional] is used to handle errors from the request, by mapping the error value into an error DTO Mapping.
	ErrorHandler ErrorHandler
	// IDContextKey is an optional field used to store the parsed ID from the URL in the context.
	//
	// default: IDContextKey[ENT, ID]{}
	IDContextKey any
	// IDParser [optional] is the ID converter which is used to parse the ID value from the request path.
	//
	// default: IDConverter[ID]{}.ParseID(rawID)
	IDParser func(string) (ID, error)
	// IDAccessor [optional] tells how to look up or set the ENT's ID.
	//
	// Default: extid.Lookup / extid.Set
	IDAccessor extid.Accessor[ENT, ID]
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

func (res RESTHandler[ENT, ID]) getMapping(mediaType string) dtokit.Mapper[ENT] {
	mediaType, _ = lookupMediaType(mediaType) // TODO: TEST ME
	if res.MediaTypeMappings != nil {
		if mapping, ok := res.MediaTypeMappings[mediaType]; ok {
			return mapping
		}
	}
	if res.Mapping != nil {
		return res.Mapping
	}
	return passthroughMappingMode[ENT]()
}

// passthroughMappingMode enables a passthrough mapping mode where entity is used as the DTO itself.
// Since we can't rule out that they don't use httpkit.Resource with a DTO type in the first place.
func passthroughMappingMode[ENT any]() dtokit.Mapping[ENT, ENT] {
	return dtokit.Mapping[ENT, ENT]{}
}

// Mapper is a generic interface used for representing a DTO-ENT mapping relationship.
// Its primary function is to allow Resource to list various mappings,
// each with its own DTO type, for different MIMEType values.
// This means we can use different DTO types within the same restful Resource handler based on different content types,
// making it more flexible and adaptable to support different Serialization formats.
//
// It is implemented by DTOMapping.
type Mapper[ENT any] interface {
	newDTO() (dtoPtr any)
	toEnt(ctx context.Context, dtoPtr any) (ENT, error)
	toDTO(ctx context.Context, ent ENT) (DTO any, _ error)
}

func (res RESTHandler[ENT, ID]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

		id, err := res.getIDParser(resourceID)
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

func (res RESTHandler[ENT, ID]) handlePanic(w http.ResponseWriter, r *http.Request) {
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

func (res RESTHandler[ENT, ID]) getErrorHandler() ErrorHandler {
	if res.ErrorHandler != nil {
		return res.ErrorHandler
	}
	return defaultErrorHandler
}

func (res RESTHandler[ENT, ID]) errInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		fmt.Println("ERROR", err.Error())
	}
	res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
}

func (res RESTHandler[ENT, ID]) errMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrMethodNotAllowed)
}

func (res RESTHandler[ENT, ID]) errEntityNotFound(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
}

// DefaultBodyReadLimit is the maximum number of bytes that a httpkit.Handler will read from the requester,
// if the Handler.BodyReadLimit is not provided.
var DefaultBodyReadLimit int = 16 * iokit.Megabyte

func (res RESTHandler[ENT, ID]) getBodyReadLimit() int {
	if res.BodyReadLimit != 0 {
		return res.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

func (res RESTHandler[ENT, ID]) index(w http.ResponseWriter, r *http.Request) {
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

	resCodec, resMediaType := res.responseBodyCodec(r, res.MediaType) // TODO:TEST_ME
	resMapping := res.getMapping(resMediaType)

	w.Header().Set(headerKeyContentType, resMediaType)

	serMaker, ok := resCodec.(codec.ListEncoderMaker)
	if !ok {
		vs, err := iterators.Collect(index)
		if err != nil {
			res.getErrorHandler().HandleError(w, r, err)
			return
		}

		data, err := resCodec.Marshal(vs)
		if err != nil {
			res.getErrorHandler().HandleError(w, r, err)
			return
		}

		if _, err := w.Write(data); err != nil {
			logger.Debug(ctx, "failed index write response in non streaming mode due to the codec not supporting ListEncoderMaker",
				logging.ErrField(err))
		}

		return
	}

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

		dto, err := resMapping.MapToIDTO(ctx, ent)
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
			logging.Field("entity_type", reflectkit.TypeOf[ENT]().String()),
			logging.ErrField(err))

		if n == 0 { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
			return
		}
		return
	}
}

func (res RESTHandler[ENT, ID]) create(w http.ResponseWriter, r *http.Request) {
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
		ctx                    = r.Context()
		reqCodec, reqMediaType = res.requestBodyCodec(r, res.MediaType)
		reqMapping             = res.getMapping(reqMediaType)
	)

	dtoPtr := reqMapping.NewDTO()
	if err := reqCodec.Unmarshal(data, dtoPtr); err != nil {
		logger.Debug(ctx, "invalid request body", logging.ErrField(err))
		res.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody)
		return
	}

	ent, err := reqMapping.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.Create(ctx, &ent); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist.With().Wrap(err))
			return
		}
		logger.Error(ctx, "error during httpkit.Resource#Create operation", logging.ErrField(err))
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	var (
		resSer, resMIMEType = res.responseBodyCodec(r, res.MediaType)
		resMapping          = res.getMapping(resMIMEType)
	)

	dto, err := resMapping.MapToIDTO(ctx, ent)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err = resSer.Marshal(dto) // TODO:TEST_ME
	if err != nil {
		logger.Error(ctx, "error during Marshaling entity operation",
			logging.Field("type", reflectkit.TypeOf[ENT]().String()),
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

func (res RESTHandler[ENT, ID]) show(w http.ResponseWriter, r *http.Request, id ID) {
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

	resSer, resMIMEType := res.responseBodyCodec(r, res.MediaType)
	mapping := res.getMapping(resMIMEType)

	w.Header().Set(headerKeyContentType, resMIMEType)

	dto, err := mapping.MapToIDTO(ctx, entity)
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

func (res RESTHandler[ENT, ID]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if res.Update == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	var (
		ctx                 = r.Context()
		reqSer, reqMIMEType = res.requestBodyCodec(r, res.MediaType)
		reqMapping          = res.getMapping(reqMIMEType)
	)

	data, err := res.readAllBody(r)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	dtoPtr := reqMapping.NewDTO()

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

	entity, err := reqMapping.MapFromDTO(ctx, dtoPtr)
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

func (res RESTHandler[ENT, ID]) destroy(w http.ResponseWriter, r *http.Request, id ID) {
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

func (res RESTHandler[ENT, ID]) destroyAll(w http.ResponseWriter, r *http.Request) {
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

func (res RESTHandler[ENT, ID]) readAllBody(r *http.Request) (_ []byte, returnErr error) {
	return bodyReadAll(r.Body, res.getBodyReadLimit())
}

func (res RESTHandler[ENT, ID]) WithCRUD(repo crud.ByIDFinder[ENT, ID]) RESTHandler[ENT, ID] {
	if repo, ok := repo.(crud.Creator[ENT]); ok && res.Create == nil {
		res.Create = repo.Create
	}
	if repo, ok := repo.(crud.AllFinder[ENT]); ok && res.Index == nil {
		res.Index = repo.FindAll // TODO: handle query
	}
	if repo, ok := repo.(crud.ByIDFinder[ENT, ID]); ok && res.Show == nil {
		res.Show = repo.FindByID
	}
	if repo, ok := repo.(crud.Updater[ENT]); ok && res.Update == nil {
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

func (m RESTHandler[ENT, ID]) getIDParser(rawID string) (ID, error) {
	if m.IDParser != nil {
		return m.IDParser(rawID)
	}
	return IDConverter[ID]{}.ParseID(rawID)
}

func (res RESTHandler[ENT, ID]) restHandler() {}

var _ restHandler = RESTHandler[any, any]{}

func (m RESTHandler[ENT, ID]) requestBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	return m.contentTypeCodec(r, fallbackMediaType)
}

func (m RESTHandler[ENT, ID]) lookupByContentType(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType, bool) {
	if mediaType, ok := m.getRequestBodyMediaType(r); ok { // TODO: TEST ME
		if c, ok := m.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType, true
		}
	}
	if c, ok := m.MediaTypeCodecs.Lookup(fallbackMediaType); ok {
		return c, fallbackMediaType, true

	}
	return nil, "", false
}

func (m RESTHandler[ENT, ID]) contentTypeCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	if mediaType, ok := m.getRequestBodyMediaType(r); ok { // TODO: TEST ME
		if c, ok := m.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType
		}
	}
	if c, ok := m.MediaTypeCodecs.Lookup(fallbackMediaType); ok {
		return c, fallbackMediaType

	}
	return defaultCodec.Codec, defaultCodec.MediaType
}

func (m RESTHandler[ENT, ID]) responseBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	var accept = r.Header.Get(headerKeyAccept)
	if accept == "" {
		return m.contentTypeCodec(r, fallbackMediaType)
	}
	for _, mediaType := range strings.Fields(accept) {
		if c, ok := m.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType
		}
	}
	return m.contentTypeCodec(r, fallbackMediaType)
}

func (m RESTHandler[ENT, ID]) getRequestBodyMediaType(r *http.Request) (mediatype.MediaType, bool) {
	return lookupMediaType(r.Header.Get(headerKeyContentType))
}
