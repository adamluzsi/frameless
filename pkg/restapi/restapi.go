package restapi

import (
	"context"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/frameless/pkg/units"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Resource is a HTTP Handler that allows you to expose a resource such as a repository as a Restful API resource.
// Depending on what CRUD operation is supported by the Handler.Resource, the Handler supports the following actions:
type Resource[Entity, ID any] struct {
	// Create will create a new entity in the restful resource.
	// 		POST /
	Create func(ctx context.Context, ptr *Entity) error
	// Index will return the entities, optionally filtered with the query argument.
	//		GET /
	Index func(ctx context.Context, query url.Values) (iterators.Iterator[Entity], error)
	// Show will return a single entity, looked up by its ID.
	// 		GET /:id
	Show func(ctx context.Context, id ID) (ent Entity, found bool, err error)
	// Update will update/replace an entity with the new state.
	// 		PUT   /:id - update/replace
	// 		PATCH /:id - partial update (WIP)
	Update func(ctx context.Context, id ID, ptr *Entity) error
	// Destroy will delete an entity, identified by its id.
	// 		 Delete /:id
	Destroy func(ctx context.Context, id ID) error

	// DestroyAll will delete all entity.
	// 		 Delete /
	DestroyAll func(ctx context.Context, query url.Values) error

	// Serialization is responsible to serialise a DTO into or out from the right serialisation format.
	// Most format is supported out of the box, but in case you want to configure your own,
	// you can do so using this config.
	Serialization ResourceSerialization[Entity, ID]

	// Mapping is responsible to map a given entity to a given DTO
	Mapping ResourceMapping[Entity]

	// ErrorHandler is used to handle errors from the request, by mapping the error value into an error DTOMapping.
	ErrorHandler ErrorHandler

	// IDContextKey is an optional field used to store the parsed ID from the URL in the context.
	//
	// Default: IDContextKey[Entity, ID]{}
	IDContextKey any

	// EntityRoutes is an http.Handler that will receive entity related requests.
	// The http.Request.Context will contain the parsed ID from the request path,
	// and can be accessed with the IDContextKey.
	//
	// Example paths
	// 		/plural-resource-identifier-name/:id/entity-routes
	// 		/users/42/status
	// 		/users/42/jobs/13
	//
	// Request paths will be stripped from their prefix.
	// For example, "/users/42/jobs" will end up as "/jobs".
	EntityRoutes http.Handler

	// BodyReadLimit is the max bytes that the handler is willing to read from the request body.
	//
	// The default value is DefaultBodyReadLimit, which is preset to 16MB.
	BodyReadLimit units.ByteSize
}

type idConverter[ID any] interface {
	FormatID(ID) (string, error)
	ParseID(string) (ID, error)
}

// ResourceMapping is responsible for map
type ResourceMapping[Entity any] struct {
	// Mapping is the primary entity to DTO mapping configuration.
	Mapping Mapping[Entity]
	// ForMIME defines a per MIMEType restapi.Mapping, that takes priority over Mapping
	ForMIME map[MIMEType]Mapping[Entity]
}

func (ms ResourceMapping[Entity]) get(mimeType MIMEType) Mapping[Entity] {
	mimeType = mimeType.Base() // TODO: TEST ME
	if ms.ForMIME != nil {
		if mapping, ok := ms.ForMIME[mimeType]; ok {
			return mapping
		}
	}
	if ms.Mapping != nil {
		return ms.Mapping
	}
	return passthroughMappingMode[Entity]()
}

// passthroughMappingMode enables a passthrough mapping mode where entity is used as the DTO itself.
// Since we can't rule out that they don't use restapi.Resource with a DTO type in the first place.
func passthroughMappingMode[Entity any]() DTOMapping[Entity, Entity] {
	return DTOMapping[Entity, Entity]{}
}

// Mapping is a generic interface used for representing a DTO-Entity mapping relationship.
// Its primary function is to allow Resource to list various mappings,
// each with its own DTO type, for different MIMEType values.
// This means we can use different DTO types within the same restful Resource handler based on different content types,
// making it more flexible and adaptable to support different Serialization formats.
//
// It is implemented by DTOMapping.
type Mapping[Entity any] interface {
	newDTO() (dtoPtr any)
	toEnt(ctx context.Context, dtoPtr any) (Entity, error)
	toDTO(ctx context.Context, ent Entity) (DTO any, _ error)
}

// DTOMapping is a type safe implementation for the generic Mapping interface.
// When using the frameless/pkg/dtos package, all you need to provide is the type arguments; nothing else is required.
type DTOMapping[Entity, DTO any] struct {
	// ToEnt is an optional function to describe how to map a DTO into an Entity.
	//
	// default: dtos.Map[Entity, DTO](...)
	ToEnt func(ctx context.Context, dto DTO) (Entity, error)
	// ToDTO is an optional function to describe how to map an Entity into a DTO.
	//
	// default: dtos.Map[DTO, Entity](...)
	ToDTO func(ctx context.Context, ent Entity) (DTO, error)
}

func (dto DTOMapping[Entity, DTO]) newDTO() any { return new(DTO) }

func (dto DTOMapping[Entity, DTO]) toEnt(ctx context.Context, dtoPtr any) (Entity, error) {
	if dtoPtr == nil {
		return *new(Entity), fmt.Errorf("nil dto ptr")
	}
	ptr, ok := dtoPtr.(*DTO)
	if !ok {
		return *new(Entity), fmt.Errorf("invalid %s type: %T", reflectkit.TypeOf[DTO]().String(), dtoPtr)
	}
	if ptr == nil {
		return *new(Entity), fmt.Errorf("nil %s pointer", reflectkit.TypeOf[DTO]().String())
	}
	if dto.ToEnt != nil { // TODO: testme
		return dto.ToEnt(ctx, *ptr)
	}
	return dtos.Map[Entity](ctx, *ptr)
}

func (dto DTOMapping[Entity, DTO]) toDTO(ctx context.Context, ent Entity) (any, error) {
	if dto.ToDTO != nil { // TODO: testme
		return dto.ToDTO(ctx, ent)
	}
	return dtos.Map[DTO](ctx, ent)
}

func (res Resource[Entity, ID]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = res.setupRequest(r)
	defer res.handlePanic(w, r)
	r, rc := internal.WithRoutingCountex(r)
	switch rc.Path {
	case `/`, ``:
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
		resourceID, rest := pathkit.Unshift(rc.Path)
		withMountPoint(rc, Path(resourceID))

		id, err := res.Serialization.getIDConverter().ParseID(resourceID)
		if err != nil {
			defaultErrorHandler.HandleError(w, r, ErrMalformedID.With().Detail(err.Error()))
			return
		}

		if res.IDContextKey != nil {
			r = r.WithContext(context.WithValue(r.Context(), res.IDContextKey, id))
		}

		if rest != "/" {
			if res.EntityRoutes == nil {
				res.getErrorHandler().HandleError(w, r, ErrPathNotFound)
				return
			}

			res.EntityRoutes.ServeHTTP(w, r)
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

func (res Resource[Entity, ID]) setupRequest(r *http.Request) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyHTTPRequest{}, r))
	return r
}

func (res Resource[Entity, ID]) handlePanic(w http.ResponseWriter, r *http.Request) {
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

func (res Resource[Entity, ID]) getErrorHandler() ErrorHandler {
	if res.ErrorHandler != nil {
		return res.ErrorHandler
	}
	return defaultErrorHandler
}

func (res Resource[Entity, ID]) errInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		fmt.Println("ERROR", err.Error())
	}
	res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
}

func (res Resource[Entity, ID]) errMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrMethodNotAllowed)
}

func (res Resource[Entity, ID]) errEntityNotFound(w http.ResponseWriter, r *http.Request) {
	res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
}

// DefaultBodyReadLimit is the maximum number of bytes that a restapi.Handler will read from the requester,
// if the Handler.BodyReadLimit is not provided.
var DefaultBodyReadLimit int = 16 * units.Megabyte

func (res Resource[Entity, ID]) getBodyReadLimit() int {
	if res.BodyReadLimit != 0 {
		return res.BodyReadLimit
	}
	return DefaultBodyReadLimit
}

func (res Resource[Entity, ID]) index(w http.ResponseWriter, r *http.Request) {
	if res.Index == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	ctx := r.Context()

	index, err := res.Index(ctx, r.URL.Query())
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	defer func() {
		if err := index.Close(); err != nil {
			logger.Warn(ctx, "error during closing the index result resource",
				logger.ErrField(err))
		}
	}()
	if err := index.Err(); err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	resSer, resMIMEType := res.Serialization.responseBodySerializer(r) // TODO:TEST_ME
	resMapping := res.Mapping.get(resMIMEType)

	serMaker, ok := resSer.(serializers.ListEncoderMaker)
	if !ok {
		const code = http.StatusNotAcceptable
		http.Error(w, http.StatusText(code), code)
		return
	}

	w.Header().Set(headerKeyContentType, resMIMEType.String())
	listEncoder := serMaker.MakeListEncoder(w)

	defer func() {
		if err := listEncoder.Close(); err != nil {
			logger.Warn(ctx, "finishing the index list encoding encountered an error",
				logger.ErrField(err))
			return
		}
	}()

	var n int
	for ; index.Next(); n++ {
		ent := index.Value()

		dto, err := resMapping.toDTO(ctx, ent)
		if err != nil {
			logger.Warn(ctx, "error during index element DTO Mapping", logger.ErrField(err))
			break
		}

		if err := listEncoder.Encode(dto); err != nil {
			logger.Warn(ctx, "error during DTO value encoding", logger.ErrField(err))
			break
		}
	}

	if err := index.Err(); err != nil {
		logger.Error(ctx, "error during iterating index result",
			logger.Field("entity_type", reflectkit.TypeOf[Entity]().String()),
			logger.ErrField(err))

		if n == 0 { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrInternalServerError)
			return
		}
		return
	}
}

func (res Resource[Entity, ID]) create(w http.ResponseWriter, r *http.Request) {
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
		reqMapping          = res.Mapping.get(reqMIMEType)
	)

	dtoPtr := reqMapping.newDTO()
	if err := reqSer.Unmarshal(data, dtoPtr); err != nil {
		logger.Debug(ctx, "invalid request body", logger.ErrField(err))
		res.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody)
		return
	}

	ent, err := reqMapping.toEnt(ctx, dtoPtr)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.Create(ctx, &ent); err != nil {
		if errors.Is(err, crud.ErrAlreadyExists) { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrEntityAlreadyExist.With().Wrap(err))
			return
		}
		logger.Error(ctx, "error during restapi.Resource#Create operation", logger.ErrField(err))
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	var (
		resSer, resMIMEType = res.Serialization.responseBodySerializer(r)
		resMapping          = res.Mapping.get(resMIMEType)
	)

	dto, err := resMapping.toDTO(ctx, ent)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	data, err = resSer.Marshal(dto) // TODO:TEST_ME
	if err != nil {
		logger.Error(ctx, "error during Marshaling entity operation",
			logger.Field("type", reflectkit.TypeOf[Entity]().String()),
			logger.ErrField(err))
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.Header().Set(headerKeyContentType, resMIMEType.String())
	w.WriteHeader(http.StatusCreated)

	if _, err := w.Write(data); err != nil {
		logger.Debug(ctx, "error during writing response to the caller",
			logger.ErrField(err))
	}
}

func (res Resource[Entity, ID]) show(w http.ResponseWriter, r *http.Request, id ID) {
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
	mapping := res.Mapping.get(resMIMEType)

	w.Header().Set(headerKeyContentType, resMIMEType.String())

	dto, err := mapping.toDTO(ctx, entity)
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
		logger.Debug(ctx, "error while writing back the response", logger.ErrField(err))
		return
	}
}

func (res Resource[Entity, ID]) update(w http.ResponseWriter, r *http.Request, id ID) {
	if res.Update == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	var (
		ctx                 = r.Context()
		reqSer, reqMIMEType = res.Serialization.requestBodySerializer(r)
		reqMapping          = res.Mapping.get(reqMIMEType)
	)

	data, err := res.readAllBody(r)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	dtoPtr := reqMapping.newDTO()

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

	entity, err := reqMapping.toEnt(ctx, dtoPtr)
	if err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := res.Update(ctx, id, &entity); err != nil {
		if errors.Is(err, crud.ErrNotFound) { // TODO:TEST_ME
			res.getErrorHandler().HandleError(w, r, ErrEntityNotFound)
			return
		}
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res Resource[Entity, ID]) destroy(w http.ResponseWriter, r *http.Request, id ID) {
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

func (res Resource[Entity, ID]) destroyAll(w http.ResponseWriter, r *http.Request) {
	if res.DestroyAll == nil {
		res.errMethodNotAllowed(w, r)
		return
	}

	if err := res.DestroyAll(r.Context(), r.URL.Query()); err != nil {
		res.getErrorHandler().HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (res Resource[Entity, ID]) readAllBody(r *http.Request) (_ []byte, returnErr error) {
	return bodyReadAll(r.Body, res.getBodyReadLimit())
}

type ctxKeyHTTPRequest struct{}

func (res Resource[Entity, ID]) HTTPRequest(ctx context.Context) (*http.Request, bool) {
	req, ok := ctx.Value(ctxKeyHTTPRequest{}).(*http.Request)
	return req, ok
}

func (res Resource[Entity, ID]) WithCRUD(repo crud.ByIDFinder[Entity, ID]) Resource[Entity, ID] {
	// TODO: add support to exclude certain operations from being mapped

	if repo, ok := repo.(crud.Creator[Entity]); ok && res.Create == nil {
		res.Create = repo.Create
	}
	if repo, ok := repo.(crud.AllFinder[Entity]); ok && res.Index == nil {
		res.Index = func(ctx context.Context, query url.Values) (iterators.Iterator[Entity], error) {
			return repo.FindAll(ctx), nil // TODO: handle query
		}
	}
	if repo, ok := repo.(crud.ByIDFinder[Entity, ID]); ok && res.Show == nil {
		res.Show = repo.FindByID
	}
	if repo, ok := repo.(crud.Updater[Entity]); ok && res.Update == nil {
		res.Update = func(ctx context.Context, id ID, ptr *Entity) error {
			if repo, ok := repo.(crud.ByIDFinder[Entity, ID]); ok {
				_, found, err := repo.FindByID(ctx, id)
				if err != nil {
					return err
				}
				if !found {
					return ErrEntityNotFound
				}
			}
			if err := extid.Set[ID](ptr, id); err != nil {
				return err
			}
			return repo.Update(ctx, ptr)
		}
	}
	if repo, ok := repo.(crud.AllDeleter); ok && res.DestroyAll == nil {
		res.DestroyAll = func(ctx context.Context, query url.Values) error {
			return repo.DeleteAll(ctx) // TODO: handle query
		}
	}
	if repo, ok := repo.(crud.ByIDDeleter[ID]); ok && res.Destroy == nil {
		res.Destroy = repo.DeleteByID
	}
	return res
}

func bodyReadAll(body io.ReadCloser, bodyReadLimit units.ByteSize) (_ []byte, returnErr error) {
	data, err := iokit.ReadAllWithLimit(body, bodyReadLimit)
	if errors.Is(err, iokit.ErrReadLimitReached) {
		return nil, ErrRequestEntityTooLarge
	}
	return data, err
}

// MIMEType or Multipurpose Internet Mail Extensions is an internet standard
// that extends the original email protocol to support non-textual content,
// such as images, audio files, and binary data.
//
// It was first defined in RFC 1341 and later updated in RFC 2045.
// MIMEType allows for the encoding different types of data using a standardised format
// that can be transmitted over email or other internet protocols.
// This makes it possible to send and receive messages with a variety of content,
// such as text, images, audio, and video, in a consistent way across different mail clients and servers.
//
// The MIMEType type is an essential component of this system, as it specifies the format of the data being transmitted.
// A MIMEType type consists of two parts: the type and the subtype, separated by a forward slash (`/`).
// The type indicates the general category of the data, such as `text`, `image`, or `audio`.
// The subtype provides more information about the specific format of the data,
// such as `plain` for plain text or `jpeg` for JPEG images.
// Today MIMEType is not only used for email but also for other internet protocols, such as HTTP,
// where it is used to specify the format of data in web requests and responses.
//
// MIMEType type is commonly used in RESTful APIs as well.
// In an HTTP request or response header, the Content-Type field specifies the MIMEType type of the entity body.
type MIMEType string

const (
	PlainText      MIMEType = "text/plain"
	JSON           MIMEType = "application/json"
	XML            MIMEType = "application/xml"
	HTML           MIMEType = "text/html"
	OctetStream    MIMEType = "application/octet-stream"
	FormUrlencoded MIMEType = "application/x-www-form-urlencoded"
)

func (ct MIMEType) WithCharset(charset string) MIMEType {
	const attrKey = "charset"
	if strings.Contains(string(ct), attrKey) {
		var parts []string
		for _, pt := range strings.Split(string(ct), ";") {
			if !strings.Contains(pt, attrKey) {
				parts = append(parts, pt)
			}
		}
		ct = MIMEType(strings.Join(parts, ";"))
	}
	return MIMEType(fmt.Sprintf("%s; %s=%s", ct, attrKey, charset))
}

func (ct MIMEType) Base() MIMEType {
	for _, pt := range strings.Split(string(ct), ";") {
		return MIMEType(pt)
	}
	return ct
}

func (ct MIMEType) String() string { return string(ct) }

const (
	headerKeyContentType = "Content-Type"
	headerKeyAccept      = "Accept"
)

type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}
