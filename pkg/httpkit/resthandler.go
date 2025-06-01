package httpkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/crud/relationship"
)

// RESTHandler implements an http.Handler that adheres to the Representational State of Resource (REST) architectural style.
//
// ## What is REST?
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
// ## Automatic Resource Relationships
//
// One of the key features of our RESTHandler is its ability to automatically infer relationships between resources.
// This means that when you define a nested URL structure, such as `/users/:user_id/notes/:note_id/attachments/:attachment_id`,
// our handler will automatically associate the corresponding entities and persist their relationships.
//
// For example, if we have three entities: User, Note, and Attachment, where:
//
// * A Note belongs to a User (identified by `Note#UserID`)
// * A Note has many Attachments (identified by `Note#Attachments`)
//
// When someone creates a new Note, our handler will automatically infer the UserID from the URL parameter `:user_id`.
// Similarly, when accessing the path `/users/:user_id/notes`, our handler will return only the notes that are scoped to the specified user.
//
// ## Ownership Constraints
//
// But what happens if you want to restrict access to certain resources based on their relationships?
// That's where ownership constraints come in. When you make a controller "not aware of the REST scope", our handler will apply an ownership constraint, which limits sub-resources to only those that are owned by the parent resource.
//
// To illustrate this, let's say we have the same entities as before: User, Note, and Attachment.
// If someone tries to access `/users/:user_id/notes`, they will only see notes that belong to the specified user.
// If they try to create a new note with an invalid or missing `:user_id` parameter, our handler will prevent the creation of the note.
//
// This feature helps ensure data consistency and security by enforcing relationships between resources.
//
// ## Conclusion
//
// Our RESTHandler provides a solid foundation for building RESTful APIs that meet the primary goals of REST.
// With its automatic resource relationship inference and ownership constraint features,
// you can focus on building robust and scalable applications with ease.
type RESTHandler[ENT, ID any] struct {
	// Create will create a new entity in the restful resource.
	// Create is a collection endpoint.
	// 		POST /
	Create func(ctx context.Context, ptr *ENT) error
	// Index will return the entities, optionally filtered with the query argument.
	// Index is a collection endpoint.
	//		GET /
	Index func(ctx context.Context) iter.Seq2[ENT, error]
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
	// If BodyReadLimit set to -1, then body io reading is not limited.
	//
	// default: DefaultBodyReadLimit, which is preset to 16MB.
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
	// Filters [optional]
	//
	// Filters allow the definition of client side requested server side response filtering.
	// Such as limiting the results of the Index endpoint by query parameters.
	// This approach enables efficient retrieval of specific subsets of resources
	// without requiring the client to fetch and process the entire collection.
	Filters []func(context.Context, ENT) bool
	// ScopeAware flags the RESTHandler that is is aware of the REST scope, such as being a nested resource.
	//   > RESTHandler[Note, NoteID] that is a subresource of a RESTHandler[User, UserID]
	//   > /users/:user_id/notes -> accessed notes should belong to a given :user_id only.
	//
	// If the current handler is not ScopeAware, then we assume so does its REST methods,
	// and when the RESTHandler used as a subresource ("/users/:user_id/notes"),
	// to avoid unwanted consequences, the DestroyAll operation will be either disabled or replaced with a sequenced of deletion using a scoped id list.
	//
	// Additionally, the Destroy operation is also disabled
	// unless the Show command is used to retrieve the ENT for validation with Constraint.
	ScopeAware bool
	// DisableOwnershipConstraint will disable the ownership check when the RESTHandler is in a sub-resource scope.
	DisableOwnershipConstraint bool
	// CommitManager [WIP]
	//
	// CommitManager is meant to make the API interaction transactional.
	CommitManager comproto.OnePhaseCommitProtocol
}

func (h RESTHandler[ENT, ID]) getMapping(mediaType string) dtokit.Mapper[ENT] {
	mediaType, _ = lookupMediaType(mediaType) // TODO: TEST ME
	if h.MediaTypeMappings != nil {
		if mapping, ok := h.MediaTypeMappings[mediaType]; ok {
			return mapping
		}
	}
	if h.Mapping != nil {
		return h.Mapping
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

func (h RESTHandler[ENT, ID]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer h.handlePanic(w, r)
	ctx := r.Context()
	ctx = internal.WithRequest(ctx, r)
	if h.CommitManager != nil {
		var err error
		ctx, err = h.CommitManager.BeginTx(ctx)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		defer h.CommitManager.CommitTx(ctx)
	}
	r = r.WithContext(ctx)
	r, rc := internal.WithRoutingContext(r)
	switch rc.PathLeft {
	case `/`, ``:
		if h.CollectionContext != nil {
			colCTX, err := h.CollectionContext(r.Context())
			if err != nil {
				h.getErrorHandler().HandleError(w, r, err)
				return
			}
			r = r.WithContext(colCTX)
		}
		switch r.Method {
		case http.MethodGet:
			h.index(w, r)
		case http.MethodPost:
			h.create(w, r)
		case http.MethodDelete:
			h.destroyAll(w, r)
		default:
			h.errMethodNotAllowed(w, r)
		}
		return

	default: // dynamic path
		rawPathResourceID, rest := pathkit.Unshift(rc.PathLeft)
		rc.Travel(rawPathResourceID)

		rawResourceID, err := url.PathUnescape(rawPathResourceID)
		if err != nil {
			defaultErrorHandler.HandleError(w, r, ErrMalformedID.Wrap(err))
			return
		}

		id, err := h.getIDParser(rawResourceID)
		if err != nil {
			defaultErrorHandler.HandleError(w, r, ErrMalformedID.Wrap(err))
			return
		}

		r = r.WithContext(h.contextWithID(r.Context(), id))

		if h.ResourceContext != nil {
			resCTX, err := h.ResourceContext(r.Context(), id)
			if err != nil {
				h.getErrorHandler().HandleError(w, r, err)
				return
			}
			r = r.WithContext(resCTX)
		}

		if rest != "/" {
			h.serveResourceRoute(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			h.show(w, r, id)
		case http.MethodPut, http.MethodPatch:
			h.update(w, r, id)
		case http.MethodDelete:
			h.destroy(w, r, id)
		}
	}

}

func (h RESTHandler[ENT, ID]) contextWithID(ctx context.Context, id ID) context.Context {
	// TODO: test that IDContextKey always there
	ctx = context.WithValue(ctx, IDContextKey[ENT, ID]{}, id)
	if h.IDContextKey != nil {
		ctx = context.WithValue(ctx, h.IDContextKey, id)
	}
	return ctx
}

func (h RESTHandler[ENT, ID]) handlePanic(w http.ResponseWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
	if err, ok := v.(error); ok {
		h.errInternalServerError(w, r, err)
		return
	}
	h.errInternalServerError(w, r, fmt.Errorf("recover: %v", v))
}

func (h RESTHandler[ENT, ID]) getErrorHandler() ErrorHandler {
	if h.ErrorHandler != nil {
		return h.ErrorHandler
	}
	return defaultErrorHandler
}

func (h RESTHandler[ENT, ID]) errInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		fmt.Println("ERROR", err.Error())
	}
	h.getErrorHandler().HandleError(w, r, ErrInternalServerError)
}

func (h RESTHandler[ENT, ID]) errMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.getErrorHandler().HandleError(w, r, ErrMethodNotAllowed)
}

func (h RESTHandler[ENT, ID]) errEntityNotFound(w http.ResponseWriter, r *http.Request) {
	h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
}

// DefaultBodyReadLimit is the maximum number of bytes that a httpkit.Handler will read from the requester,
// if the Handler.BodyReadLimit is not provided.
var DefaultBodyReadLimit int = 16 * iokit.Megabyte

func (h RESTHandler[ENT, ID]) getBodyReadLimit() int {
	if h.BodyReadLimit != 0 {
		return h.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

func (h RESTHandler[ENT, ID]) index(w http.ResponseWriter, r *http.Request) {
	if h.Index == nil {
		h.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	index := h.indexIter(ctx)

	if len(h.Filters) != 0 {
		index = iterkit.OnSeqEValue(index, func(i iter.Seq[ENT]) iter.Seq[ENT] {
			return iterkit.Filter(i, func(v ENT) bool {
				for _, filter := range h.Filters {
					if !filter(ctx, v) {
						return false
					}
				}
				return true
			})
		})
	}

	resCodec, resMediaType := h.responseBodyCodec(r, h.MediaType) // TODO:TEST_ME
	resMapping := h.getMapping(resMediaType)

	w.Header().Set(headerKeyContentType, resMediaType)

	serMaker, ok := resCodec.(codec.ListEncoderMaker)
	if !ok {
		vs, err := iterkit.CollectE(index)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}

		data, err := resCodec.Marshal(vs)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}

		if _, err := w.Write(data); err != nil {
			logger.Debug(ctx, "failed index write response in non streaming mode due to the codec not supporting ListEncoderMaker",
				logging.ErrField(err))
		}

		return
	}

	next, stop := iter.Pull2(index)
	defer stop()

	ent, err, ok := next()
	if !ok {
		_ = serMaker.MakeListEncoder(w).Close()
		return
	}
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	listEncoder := serMaker.MakeListEncoder(w)

	index = iterkit.Merge2(iterkit.ToSeqE(iterkit.Of(ent)), iterkit.FromPull2(next))

	defer func() {
		if err := listEncoder.Close(); err != nil {
			logger.Warn(ctx, "finishing the index list encoding encountered an error",
				logging.ErrField(err))
			return
		}
	}()

	for ent, err := range index {
		if err != nil {
			logger.Error(ctx, "error during iterating index result",
				logging.Field("entity_type", reflectkit.TypeOf[ENT]().String()),
				logging.ErrField(err))
			break
		}

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
}

func (h RESTHandler[ENT, ID]) indexIter(ctx context.Context) iter.Seq2[ENT, error] {
	index := h.Index(ctx)
	if _, ok := internal.ContextRESTParentResourceValuePointer.Lookup(ctx); ok {
		index = iterkit.OnSeqEValue(index, func(i iter.Seq[ENT]) iter.Seq[ENT] {
			return iterkit.Filter(i, func(v ENT) bool {
				return h.isOwnershipOK(ctx, v)
			})
		})
	}
	return index
}

func (h RESTHandler[ENT, ID]) create(w http.ResponseWriter, r *http.Request) {
	if h.Create == nil {
		h.errMethodNotAllowed(w, r)
		return
	}

	data, err := h.readAllBody(r)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	var (
		ctx                    = r.Context()
		reqCodec, reqMediaType = h.requestBodyCodec(r, h.MediaType)
		reqMapping             = h.getMapping(reqMediaType)
	)

	dtoPtr := reqMapping.NewDTO()
	if err := reqCodec.Unmarshal(data, dtoPtr); err != nil {
		logger.Debug(ctx, "invalid request body", logging.ErrField(err))
		h.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody)
		return
	}

	ent, err := reqMapping.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	parentPointer, hasParent := internal.ContextRESTParentResourceValuePointer.Lookup(ctx)
	if hasParent && relationship.HasReference(ent, parentPointer) {
		if err := relationship.Associate(parentPointer, &ent); err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
	}

	if !h.ScopeAware { // TODO: testme
		if hasParent && relationship.HasReference(ent, parentPointer) && !h.isOwnershipOK(ctx, ent) {
			h.getErrorHandler().HandleError(w, r, ErrForbidden)
			return
		}
	}

	if err := h.Create(ctx, &ent); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) { // TODO:TEST_ME
			h.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist.Wrap(err))
			return
		}
		logger.Error(ctx, "error during httpkit.Resource#Create operation", logging.ErrField(err))
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	if hasParent && relationship.HasReference(parentPointer, ent) {
		if err := relationship.Associate(parentPointer, &ent); err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
	}

	var (
		resSer, resMIMEType = h.responseBodyCodec(r, h.MediaType)
		resMapping          = h.getMapping(resMIMEType)
	)

	dto, err := resMapping.MapToIDTO(ctx, ent)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err = resSer.Marshal(dto) // TODO:TEST_ME
	if err != nil {
		logger.Error(ctx, "error during Marshaling entity operation",
			logging.Field("type", reflectkit.TypeOf[ENT]().String()),
			logging.ErrField(err))
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.Header().Set(headerKeyContentType, resMIMEType)
	w.WriteHeader(http.StatusCreated)

	if _, err := w.Write(data); err != nil {
		logger.Debug(ctx, "error during writing response to the caller",
			logging.ErrField(err))
	}
}

func (h RESTHandler[ENT, ID]) serveResourceRoute(w http.ResponseWriter, r *http.Request) {
	if h.ResourceRoutes == nil {
		h.getErrorHandler().HandleError(w, r, ErrPathNotFound)
		return
	}

	ctx := r.Context()

	if h.Show == nil {
		logger.Warn(ctx, "error, ResourceRoute requested on a RESTHandler that can't confirm the existence of the resource")
		h.errMethodNotAllowed(w, r)
		return
	}

	id, _ := ctx.Value(h.idContextKey()).(ID)
	entity, found, err := h.Show(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.errEntityNotFound(w, r)
		return
	}

	fingerprintBefore := fmt.Sprintf("%#v", entity)
	r = r.WithContext(internal.ContextRESTParentResourceValuePointer.ContextWith(ctx, &entity))
	h.ResourceRoutes.ServeHTTP(w, r)

	fingerprintAfter := fmt.Sprintf("%#v", entity)

	if fingerprintBefore != fingerprintAfter {
		logFieldType := logging.Field("type", reflectkit.TypeOf[ENT]().String())
		if h.Update != nil {
			if err := h.Update(ctx, &entity); err != nil {
				logger.Error(ctx, "failed to update REST parent",
					logging.ErrField(err),
					logFieldType)
			}
		} else {
			const msg = "The subresource likely altered the parent entity due to relationship reference changes, but we can't persist it because the RESTHandler#Update action is missing."
			logger.Debug(ctx, msg, logFieldType)
		}
	}
}

func (h RESTHandler[ENT, ID]) show(w http.ResponseWriter, r *http.Request, id ID) {
	if h.Show == nil {
		h.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	entity, found, err := h.Show(ctx, id)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}
	if !found {
		h.errEntityNotFound(w, r)
		return
	}

	if !h.isOwnershipOK(ctx, entity) {
		h.errEntityNotFound(w, r)
		return
	}

	resSer, resMIMEType := h.responseBodyCodec(r, h.MediaType)
	mapping := h.getMapping(resMIMEType)

	w.Header().Set(headerKeyContentType, resMIMEType)

	dto, err := mapping.MapToIDTO(ctx, entity)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err := resSer.Marshal(dto)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		logger.Debug(ctx, "error while writing back the response", logging.ErrField(err))
		return
	}
}

func (h RESTHandler[ENT, ID]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if h.Update == nil {
		h.errMethodNotAllowed(w, r)
		return
	}
	var (
		ctx                 = r.Context()
		reqSer, reqMIMEType = h.requestBodyCodec(r, h.MediaType)
		reqMapping          = h.getMapping(reqMIMEType)
	)

	data, err := h.readAllBody(r)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	dtoPtr := reqMapping.NewDTO()

	if err := reqSer.Unmarshal(data, dtoPtr); err != nil {
		h.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody.Wrap(err))
		return
	}

	if h.Show != nil { // TODO:TEST_ME
		ctx := r.Context()
		v, found, err := h.Show(ctx, id)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		if !found {
			h.errEntityNotFound(w, r)
			return
		}
		if !h.isOwnershipOK(ctx, v) {
			h.errEntityNotFound(w, r)
			return
		}
	} else if h.isSubResourceContext(ctx) {
		h.errMethodNotAllowed(w, r)
		return
	}

	entity, err := reqMapping.MapFromDTO(ctx, dtoPtr)
	if err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := h.IDAccessor.Set(&entity, id); err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	if !h.isOwnershipOK(ctx, entity) {
		h.errEntityNotFound(w, r)
		return
	}

	if err := h.Update(ctx, &entity); err != nil {
		if errors.Is(err, crud.ErrNotFound) { // TODO:TEST_ME
			h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h RESTHandler[ENT, ID]) destroy(w http.ResponseWriter, r *http.Request, id ID) {
	if h.Destroy == nil {
		h.errMethodNotAllowed(w, r)
		return
	}

	var ctx = r.Context()

	var filterChecked bool
	if h.Show != nil { // TODO:TEST_ME
		ctx := r.Context()
		v, found, err := h.Show(ctx, id)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		if !found {
			h.errEntityNotFound(w, r)
			return
		}
		if !h.isOwnershipOK(ctx, v) {
			h.errEntityNotFound(w, r)
			return
		}
		filterChecked = true
	}

	if !filterChecked && h.ScopeAware {
		filterChecked = true
	}

	if !filterChecked && !h.DisableOwnershipConstraint {
		h.errMethodNotAllowed(w, r)
		return
	}

	if err := h.Destroy(ctx, id); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			h.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h RESTHandler[ENT, ID]) destroyAll(w http.ResponseWriter, r *http.Request) {
	if h.DestroyAll == nil {
		h.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	if _, ok := internal.ContextRESTParentResourceValuePointer.Lookup(ctx); ok && !h.ScopeAware {
		ok, err := h.trySoftDeleteAll(ctx)
		if err != nil {
			h.getErrorHandler().HandleError(w, r, err)
			return
		}
		if ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.errMethodNotAllowed(w, r)
		return
	}

	if err := h.DestroyAll(ctx); err != nil {
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res RESTHandler[ENT, ID]) trySoftDeleteAll(ctx context.Context) (ok bool, rerr error) {
	if res.Destroy == nil || res.Index == nil {
		return false, nil
	}

	if res.CommitManager != nil {
		var err error
		ctx, err = res.CommitManager.BeginTx(ctx)
		if err != nil {
			return true, err
		}
		defer comproto.FinishOnePhaseCommit(&rerr, res.CommitManager, ctx)
	}

	all := res.Index(ctx)

	var ids []ID
	for v, err := range all {
		if err != nil {
			return true, err
		}
		id, ok := res.IDAccessor.Lookup(v)
		if ok {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		if err := res.Destroy(ctx, id); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (res RESTHandler[ENT, ID]) readAllBody(r *http.Request) ([]byte, error) {
	data, err := bodyReadAll(r.Body, res.getBodyReadLimit())
	if errors.Is(err, iokit.ErrReadLimitReached) {
		return nil, ErrRequestEntityTooLarge
	}
	return data, err
}

func RESTHandlerFromCRUD[ENT, ID any](repo crud.ByIDFinder[ENT, ID], conf ...func(h *RESTHandler[ENT, ID])) RESTHandler[ENT, ID] {
	var h RESTHandler[ENT, ID]
	h.Show = repo.FindByID
	if repo, ok := repo.(crud.Creator[ENT]); ok && h.Create == nil {
		h.Create = repo.Create
	}
	if repo, ok := repo.(crud.AllFinder[ENT]); ok && h.Index == nil {
		h.Index = repo.FindAll // TODO: handle query
	}
	if repo, ok := repo.(crud.Updater[ENT]); ok && h.Update == nil {
		h.Update = repo.Update
	}
	if repo, ok := repo.(crud.AllDeleter); ok && h.DestroyAll == nil {
		h.DestroyAll = repo.DeleteAll // TODO: handle query
	}
	if repo, ok := repo.(crud.ByIDDeleter[ID]); ok && h.Destroy == nil {
		h.Destroy = repo.DeleteByID
	}
	for _, init := range conf {
		init(&h)
	}
	return h
}

func bodyReadAll(body io.ReadCloser, bodyReadLimit iokit.ByteSize) (_ []byte, returnErr error) {
	if bodyReadLimit < 0 {
		return io.ReadAll(body)
	}
	if bodyReadLimit == 0 {
		bodyReadLimit = DefaultBodyReadLimit
	}
	return iokit.ReadAllWithLimit(body, bodyReadLimit)
}

const (
	headerKeyContentType = "Content-Type"
	headerKeyAccept      = "Accept"
)

func (h RESTHandler[ENT, ID]) getIDParser(rawID string) (ID, error) {
	if h.IDParser != nil {
		return h.IDParser(rawID)
	}
	return IDConverter[ID]{}.ParseID(rawID)
}

func (h RESTHandler[ENT, ID]) restHandler() {}

var _ restHandler = RESTHandler[any, any]{}

func (h RESTHandler[ENT, ID]) requestBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	return h.contentTypeCodec(r, fallbackMediaType)
}

func (h RESTHandler[ENT, ID]) lookupByContentType(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType, bool) {
	if mediaType, ok := h.getRequestBodyMediaType(r); ok { // TODO: TEST ME
		if c, ok := h.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType, true
		}
	}
	if c, ok := h.MediaTypeCodecs.Lookup(fallbackMediaType); ok {
		return c, fallbackMediaType, true

	}
	return nil, "", false
}

func (h RESTHandler[ENT, ID]) contentTypeCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	if mediaType, ok := h.getRequestBodyMediaType(r); ok { // TODO: TEST ME
		if c, ok := h.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType
		}
	}
	if c, ok := h.MediaTypeCodecs.Lookup(fallbackMediaType); ok {
		return c, fallbackMediaType

	}
	return defaultCodec.Codec, defaultCodec.MediaType
}

func (h RESTHandler[ENT, ID]) responseBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	var accept = r.Header.Get(headerKeyAccept)
	if accept == "" {
		return h.contentTypeCodec(r, fallbackMediaType)
	}
	for _, mediaType := range strings.Fields(accept) {
		if c, ok := h.MediaTypeCodecs.Lookup(mediaType); ok {
			return c, mediaType
		}
	}
	return h.contentTypeCodec(r, fallbackMediaType)
}

func (h RESTHandler[ENT, ID]) getRequestBodyMediaType(r *http.Request) (mediatype.MediaType, bool) {
	return lookupMediaType(r.Header.Get(headerKeyContentType))
}

func (h RESTHandler[ENT, ID]) idContextKey() any {
	if h.IDContextKey != nil {
		return h.IDContextKey
	}
	return IDContextKey[ENT, ID]{}
}

func (h RESTHandler[ENT, ID]) isSubResourceContext(ctx context.Context) bool {
	_, ok := internal.ContextRESTParentResourceValuePointer.Lookup(ctx)
	return ok
}

func (h RESTHandler[ENT, ID]) isOwnershipOK(ctx context.Context, v ENT) bool {
	if h.DisableOwnershipConstraint {
		return true
	}
	return RESTOwnershipCheck(ctx, v)
}

func (h RESTHandler[ENT, ID]) RouteInfo() RouteInfo {
	var (
		routes       []PathInfo
		resourcePath = "/:id"
	)
	if h.Create != nil {
		routes = append(routes, PathInfo{Method: http.MethodPost, Path: "/", Desc: "#Create"})
	}
	if h.Index != nil {
		routes = append(routes, PathInfo{Method: http.MethodGet, Path: "/", Desc: "#Index"})
	}
	if h.DestroyAll != nil {
		routes = append(routes, PathInfo{Method: http.MethodDelete, Path: "/", Desc: "#DestroyAll"})
	}
	if h.Show != nil {
		routes = append(routes, PathInfo{Method: http.MethodGet, Path: resourcePath, Desc: "#Show"})
	}
	if h.Update != nil {
		routes = append(routes, PathInfo{Method: http.MethodPut, Path: resourcePath, Desc: "#Update"})
	}
	if h.Destroy != nil {
		routes = append(routes, PathInfo{Method: http.MethodDelete, Path: resourcePath, Desc: "#Destroy"})
	}
	if h.ResourceRoutes != nil {
		routes = append(routes, GetRouteInfo(h.ResourceRoutes).WithMountPoint(resourcePath)...)
	}
	return routes
}

// RESTOwnershipCheck
//
// RESTOwnershipCheck checks if an entity (e.g., a note, attachment) belongs to the current REST request scope by verifying its association with the specified parent resource. This function ensures data isolation, allowing only relevant entities to be accessed or modified.
//
// # Key Concept
//
// In nested REST resources, RESTOwnershipCheck enforces that an entity is linked to its parent resource in the current request, helping maintain secure and isolated data access.
//
// # How It Works
//
// - Identify Parent and Entity:
//   - For a request path like /users/1/notes, 1 represents the parent resource (user), and notes are entities associated with that user.
//
// - Verify Association:
//   - The function checks if the entity’s foreign key (e.g., UserID in a note) matches the parent resource ID in the request.
//
// - Return Outcome:
//   - True if the entity is correctly associated (e.g., the note is owned by user 1).
//   - False if it isn’t, blocking access to unrelated data.
//
// # Examples
//
// - Basic: For /users/1/notes, RESTOwnershipCheck ensures each note is owned by user 1.
// - Nested: For /users/1/notes/2/attachments, it confirms that an attachment belongs to note 2 under user 1.
//
// This check maintains ownership boundaries and enhances security in nested REST resources.
func RESTOwnershipCheck(ctx context.Context, entity any) bool {
	parent, ok := internal.ContextRESTParentResourceValuePointer.Lookup(ctx)
	if !ok {
		return true
	}
	return relationship.Related(parent, entity)
}

type IDContextKey[ENT, ID any] struct{}
