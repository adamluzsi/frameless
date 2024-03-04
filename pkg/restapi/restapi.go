package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/restapi/internal"
	"go.llib.dev/frameless/pkg/units"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/iterators"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
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

	// Serialization is responsible to serialise a DTO into or out from the right serialisation format.
	// Most format is supported out of the box, but in case you want to configure your own,
	// you can do so using this config.
	Serialization Serialization[Entity, ID]

	// Mapping
	Mapping Mapping[Entity]

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

	// BodyReadLimitByteSize is the max bytes that the handler is willing to read from the request body.
	//
	// The default value is DefaultBodyReadLimit, which is preset to 16MB.
	BodyReadLimitByteSize int
}

type IDContextKey[Entity, ID any] struct{}

func (ick IDContextKey[Entity, ID]) ContextWithID(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, ick, id)
}

func (ick IDContextKey[Entity, ID]) LookupID(ctx context.Context) (ID, bool) {
	if ctx == nil {
		return *new(ID), false
	}
	id, ok := ctx.Value(ick).(ID)
	return id, ok
}

type idConverter[ID any] interface {
	FormatID(ID) (string, error)
	ParseID(string) (ID, error)
}

// Mapping is responsible for map
type Mapping[Entity any] map[MIMEType]dtoMapping[Entity]

func (ms Mapping[Entity]) mappingFor(mimeType MIMEType) dtoMapping[Entity] {
	if ms == nil {
		return ms.passthroughMappingMode()
	}
	mapping, ok := ms[mimeType]
	if !ok {
		return ms.passthroughMappingMode()
	}
	return mapping
}

// passthroughMappingMode enables a passthrough mapping mode where entity is used as the DTO itself.
// Since we can't rule out that they don't use restapi.Resource with a DTO type in the first place.
func (ms Mapping[Entity]) passthroughMappingMode() DTOMapping[Entity, Entity] {
	return DTOMapping[Entity, Entity]{M: &dtos.M{}}
}

type dtoMapping[Entity any] interface {
	dto()

	newDTO() (dtoPtr any)
	dtoToEnt(dtoPtr any) (Entity, error)
	entToDTO(ent Entity) (DTO any, _ error)
}

type DTOMapping[Entity, DTO any] struct {
	// M is the type mapping register.
	// It contains the knowledge how to map an Entity into a Data Transfer Object
	M *dtos.M
}

func (dto DTOMapping[Entity, DTO]) dto() {}

func (dto DTOMapping[Entity, DTO]) newDTO() any { return new(DTO) }

func (dto DTOMapping[Entity, DTO]) dtoToEnt(dtoPtr any) (Entity, error) {
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
	return dtos.Map[Entity](dto.M, *ptr)
}

func (dto DTOMapping[Entity, DTO]) entToDTO(ent Entity) (any, error) {
	return dtos.Map[DTO](dto.M, ent)
}

func (res Resource[Entity, ID]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer res.handlePanic(w, r)
	r, rc := internal.WithRoutingCountex(r)
	switch rc.Path {
	case `/`, ``:
		switch r.Method {
		case http.MethodGet:
			res.index(w, r)
		case http.MethodPost:
			res.create(w, r)
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

		r = r.WithContext(context.WithValue(r.Context(), res.getIDContextKey(), id))

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
var DefaultBodyReadLimit int64 = 16 * units.Megabyte

func (res Resource[Entity, ID]) getBodyReadLimit() int64 {
	if res.BodyReadLimitByteSize != 0 {
		return int64(res.BodyReadLimitByteSize)
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

	w.Header().Set(headerKeyContentType, resMIMEType.String())
	listEncoder := resSer.NewListEncoder(w)

	defer func() {
		if err := listEncoder.Close(); err != nil {
			logger.Warn(ctx, "finishing the index list encoding encountered an error",
				logger.ErrField(err))
			return
		}
	}()

	var n int
	for index.Next() {
		n++
		if err := listEncoder.Encode(index.Value()); err != nil {
			logger.Warn(ctx, "error during index element value encoding", logger.ErrField(err))
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
		reqMapping          = res.Mapping.mappingFor(reqMIMEType)
	)

	dtoPtr := reqMapping.newDTO()
	if err := reqSer.Unmarshal(data, dtoPtr); err != nil {
		logger.Debug(ctx, "invalid request body", logger.ErrField(err))
		res.getErrorHandler().HandleError(w, r, ErrInvalidRequestBody)
		return
	}

	ent, err := reqMapping.dtoToEnt(dtoPtr)
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
		resMapping          = res.Mapping.mappingFor(resMIMEType)
	)

	dto, err := resMapping.entToDTO(ent)
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
	mapping := res.Mapping.mappingFor(resMIMEType)

	w.Header().Set(headerKeyContentType, resMIMEType.String())

	dto, err := mapping.entToDTO(entity)
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
		reqMapping          = res.Mapping.mappingFor(reqMIMEType)
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

	entity, err := reqMapping.dtoToEnt(dtoPtr)
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

func (res Resource[Entity, ID]) readAllBody(r *http.Request) (_ []byte, returnErr error) {
	if r.Body == nil { // TODO:TEST_ME
		return []byte{}, nil
	}
	defer errorkit.Finish(&returnErr, r.Body.Close) // TODO:TEST_ME
	bodyReadLimit := res.getBodyReadLimit()
	data, err := io.ReadAll(io.LimitReader(r.Body, bodyReadLimit))
	if err != nil {
		return nil, err
	}
	if _, err := r.Body.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		return nil, ErrRequestEntityTooLarge
	}
	return data, nil
}

func (res Resource[Entity, ID]) getIDContextKey() any {
	if res.IDContextKey != nil {
		return res.IDContextKey
	}
	return IDContextKey[Entity, ID]{}
}

func (res Resource[Entity, ID]) ContextLookupID(ctx context.Context) (ID, bool) {
	if ctx == nil {
		return *new(ID), false
	}
	id, ok := ctx.Value(res.getIDContextKey()).(ID)
	return id, ok
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
	PlainText   MIMEType = "text/plain"
	JSON        MIMEType = "application/json"
	XML         MIMEType = "application/xml"
	HTML        MIMEType = "text/html"
	OctetStream MIMEType = "application/octet-stream"
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

func (ct MIMEType) String() string { return string(ct) }

// SERIALIZERS

type JSONSerializer struct{}

func (s JSONSerializer) MIMEType() MIMEType { return JSON }

func (s JSONSerializer) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s JSONSerializer) Unmarshal(data []byte, dtoPtr any) error {
	return json.Unmarshal(data, &dtoPtr)
}

func (s JSONSerializer) NewListEncoder(w io.Writer) ListEncoder {
	return &jsonListEncoder{W: w}
}

type jsonListEncoder struct {
	W io.Writer
	M *dtos.M

	bracketOpen bool
	index       int
	err         error
	done        bool
}

func (le *jsonListEncoder) Encode(dto any) error {
	if le.err != nil {
		return le.err
	}

	if !le.bracketOpen {
		if err := le.beginList(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	if 0 < le.index {
		if _, err := le.W.Write([]byte(`,`)); err != nil {
			le.err = err
			return err
		}
	}

	if _, err := le.W.Write(data); err != nil {
		le.err = err
		return err
	}

	le.index++
	return nil
}

func (le *jsonListEncoder) Close() error {
	if le.done {
		return le.err
	}
	le.done = true
	if !le.bracketOpen {
		if err := le.beginList(); err != nil {
			return err
		}
	}
	if le.bracketOpen {
		if err := le.endList(); err != nil {
			return err
		}
	}
	return nil
}

func (le *jsonListEncoder) endList() error {
	if _, err := le.W.Write([]byte(`]`)); err != nil {
		le.err = err
		return err
	}
	le.bracketOpen = false
	return nil
}

func (le *jsonListEncoder) beginList() error {
	if _, err := le.W.Write([]byte(`[`)); err != nil {
		le.err = err
		return err
	}
	le.bracketOpen = true
	return nil
}

type JSONStreamSerializer struct{}

func (s JSONStreamSerializer) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s JSONStreamSerializer) Unmarshal(data []byte, ptr any) error {
	return json.Unmarshal(data, ptr)
}

func (s JSONStreamSerializer) NewListEncoder(w io.Writer) ListEncoder {
	return closerEncoder{Encoder: json.NewEncoder(w)}
}

type closerEncoder struct {
	Encoder interface {
		Encode(v any) error
	}
}

func (e closerEncoder) Encode(v any) error {
	return e.Encoder.Encode(v)
}

func (closerEncoder) Close() error { return nil }

type GenericListEncoder[T any] struct {
	W       io.Writer
	Marshal func(v []T) ([]byte, error)

	vs     []T
	closed bool
}

func (enc *GenericListEncoder[T]) Encode(v T) error {
	if enc.closed {
		return fmt.Errorf("list encoder is already closed")
	}
	enc.vs = append(enc.vs, v)
	return nil
}

func (enc *GenericListEncoder[T]) Close() error {
	if enc.closed {
		return nil
	}
	data, err := enc.Marshal(enc.vs)
	if err != nil {
		return err
	}
	if _, err := enc.W.Write(data); err != nil {
		return err
	}
	enc.closed = true
	return nil
}

const (
	headerKeyContentType = "Content-Type"
	headerKeyAccept      = "Accept"
)

func NewRouter(configure ...func(*Router)) *Router {
	router := &Router{}
	for _, c := range configure {
		c(router)
	}
	return router
}

func RouterFrom[V Routes](v V) *Router {
	switch v := any(v).(type) {
	case Routes:
		r := NewRouter()
		r.MountRoutes(v)
		return r

	default:
		panic("not implemented")
	}
}

type Router struct {
	route *route
	mutex sync.RWMutex
}

type route struct {
	Handler http.Handler
	Routes  map[string]*route
}

func (r *route) GetRoutes() map[string]*route {
	if r.Routes == nil {
		r.Routes = make(map[string]*route)
	}
	return r.Routes
}

func (r *route) Ensure(part string) *route {
	if _, ok := r.GetRoutes()[part]; !ok {
		r.Routes[part] = &route{}
	}
	return r.Routes[part]
}

func (r *route) Lookup(part string) (*route, bool) {
	if r == nil {
		return nil, false
	}
	if r.Routes == nil {
		return nil, false
	}
	sr, ok := r.Routes[part]
	return sr, ok
}

func (router *Router) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	router.mutex.RLock()
	defer router.mutex.RUnlock()
	if router.route == nil {
		defaultErrorHandler.HandleError(responseWriter, request, ErrPathNotFound)
		return
	}
	request, rc := internal.WithRoutingCountex(request)
	var (
		mount   []string
		route   = router.route
		handler = route.Handler
	)
	for _, part := range pathkit.Split(rc.Path) {
		var ok bool
		route, ok = route.Lookup(part)
		if !ok {
			break
		}
		mount = append(mount, part)
		handler = route.Handler
	}
	if handler == nil {
		defaultErrorHandler.HandleError(responseWriter, request, ErrPathNotFound)
		return
	}
	withMountPoint(rc, pathkit.Join(mount...))
	handler.ServeHTTP(responseWriter, request)
}

func (router *Router) Mount(path Path, handler http.Handler) {
	router.mutex.Lock()
	defer router.mutex.Unlock()

	if router.route == nil {
		router.route = &route{}
	}

	var ro *route
	ro = router.route
	for _, part := range pathkit.Split(path) {
		ro = ro.Ensure(part)
	}

	ro.Handler = handler
}

type (
	Path   = string
	Routes map[Path]http.Handler
)

func (router *Router) MountRoutes(routes Routes) {
	for path, handler := range routes {
		router.Mount(path, handler)
	}
}

/////////////////////////////////////////////////////// MAPPING ///////////////////////////////////////////////////////

// IDInContext is a OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the context handling related methods.
type IDInContext[CtxKey, EntityIDType any] struct{}

func (cm IDInContext[CtxKey, EntityIDType]) ContextWithID(ctx context.Context, id EntityIDType) context.Context {
	return context.WithValue(ctx, *new(CtxKey), id)
}

func (cm IDInContext[CtxKey, EntityIDType]) ContextLookupID(ctx context.Context) (EntityIDType, bool) {
	v, ok := ctx.Value(*new(CtxKey)).(EntityIDType)
	return v, ok
}

// StringID is a OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type StringID[ID ~string] struct{}

func (m StringID[ID]) FormatID(id ID) (string, error) { return string(id), nil }
func (m StringID[ID]) ParseID(id string) (ID, error)  { return ID(id), nil }

// IntID is a OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type IntID[ID ~int] struct{}

func (m IntID[ID]) FormatID(id ID) (string, error) {
	return strconv.Itoa(int(id)), nil
}

func (m IntID[ID]) ParseID(id string) (ID, error) {
	n, err := strconv.Atoi(id)
	return ID(n), err
}

// IDConverter is a OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type IDConverter[ID any] struct {
	Format func(ID) (string, error)
	Parse  func(string) (ID, error)
}

func (m IDConverter[ID]) FormatID(id ID) (string, error) {
	return m.getFormatter()(id)
}

var (
	stringType = reflectkit.TypeOf[string]()
	intType    = reflectkit.TypeOf[int]()
)

func (m IDConverter[ID]) getFormatter() func(ID) (string, error) {
	if m.Format != nil {
		return m.Format
	}
	rtype := reflectkit.TypeOf[ID]()
	switch rtype.Kind() {
	case reflect.String:
		return func(id ID) (_ string, returnErr error) {
			defer errorkit.Recover(&returnErr)
			return reflect.ValueOf(id).Convert(stringType).String(), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(id ID) (string, error) {
			return strconv.Itoa(int(reflect.ValueOf(id).Convert(intType).Int())), nil
		}
	default:
		return func(id ID) (string, error) {
			return "", fmt.Errorf("not implemented")
		}
	}
}

func (m IDConverter[ID]) ParseID(data string) (ID, error) {
	return m.getParser()(data)
}

func (m IDConverter[ID]) getParser() func(string) (ID, error) {
	if m.Parse != nil {
		return m.Parse
	}
	rtype := reflectkit.TypeOf[ID]()
	switch rtype.Kind() {
	case reflect.String:
		return func(s string) (_ ID, returnErr error) {
			defer errorkit.Recover(&returnErr)
			return reflect.ValueOf(s).Convert(rtype).Interface().(ID), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(s string) (_ ID, returnErr error) {
			defer errorkit.Recover(&returnErr)
			n, err := strconv.Atoi(s)
			if err != nil {
				return *new(ID), err
			}
			return reflect.ValueOf(n).Convert(rtype).Interface().(ID), nil
		}
	default:
		return func(s string) (ID, error) {
			return *new(ID), fmt.Errorf("not implemented")
		}
	}
}
