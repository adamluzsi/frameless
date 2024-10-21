package httpkit

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/codec"
)

type MediaTypeCodecs map[mediatype.MediaType]codec.Codec

type MediaTypeMappings[ENT any] map[mediatype.MediaType]dtokit.Mapper[ENT]

var DefaultCodecs = map[string]codec.Codec{
	"application/json":                  jsonkit.Codec{},
	"application/problem+json":          jsonkit.Codec{},
	"application/x-ndjson":              jsonkit.LinesCodec{},
	"application/stream+json":           jsonkit.LinesCodec{},
	"application/json-stream":           jsonkit.LinesCodec{},
	"application/x-www-form-urlencoded": FormURLEncodedCodec{},
}

var DefaultCodec = CodecDefault{
	Codec:     jsonkit.Codec{},
	MediaType: mediatype.JSON,
}

type CodecDefault struct {
	Codec interface {
		codec.Codec
		codec.ListDecoderMaker
	}
	MediaType string
}

func (m MediaTypeCodecs) requestBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	return m.contentTypeCodec(r, fallbackMediaType)
}

func (m MediaTypeCodecs) contentTypeCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	if mediaType, ok := m.getRequestBodyMediaType(r); ok { // TODO: TEST ME
		if c, ok := m.lookup(mediaType); ok {
			return c, mediaType
		}
	}
	if c, ok := m.lookup(fallbackMediaType); ok {
		return c, fallbackMediaType

	}
	return DefaultCodec.Codec, DefaultCodec.MediaType
}

func (m MediaTypeCodecs) responseBodyCodec(r *http.Request, fallbackMediaType mediatype.MediaType) (codec.Codec, mediatype.MediaType) {
	var accept = r.Header.Get(headerKeyAccept)
	if accept == "" {
		return m.contentTypeCodec(r, fallbackMediaType)
	}
	for _, mediaType := range strings.Fields(accept) {
		if m != nil {
			c, ok := m[mediaType]
			if ok {
				return c, mediaType
			}
		}
		if DefaultCodecs != nil {
			c, ok := DefaultCodecs[mediaType]
			if ok {
				return c, mediaType
			}
		}
	}
	return m.contentTypeCodec(r, fallbackMediaType)
}

func (m MediaTypeCodecs) getRequestBodyMediaType(r *http.Request) (mediatype.MediaType, bool) {
	return getMediaTypeFrom(r.Header.Get(headerKeyContentType))
}

func getMediaTypeFrom(headerValue string) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(headerValue)
	if err != nil || mediaType == "" {
		return "", false
	}
	return mediaType, true
}

func (m MediaTypeCodecs) lookup(mediaType string) (codec.Codec, bool) {
	mediaType = getMediaType(mediaType) // TODO: TEST ME
	if m != nil {
		if c, ok := m[mediaType]; ok {
			return c, true
		}
	}
	if DefaultCodecs != nil {
		if c, ok := DefaultCodecs[mediaType]; ok {
			return c, true
		}
	}
	return nil, false
}

/////////////////////////////////////////////////////// MAPPING ///////////////////////////////////////////////////////

// IDInContext is an OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the context handling related methods.
type IDInContext[CtxKey, EntityIDType any] struct{}

func (cm IDInContext[CtxKey, EntityIDType]) ContextWithID(ctx context.Context, id EntityIDType) context.Context {
	return context.WithValue(ctx, *new(CtxKey), id)
}

func (cm IDInContext[CtxKey, EntityIDType]) ContextLookupID(ctx context.Context) (EntityIDType, bool) {
	v, ok := ctx.Value(*new(CtxKey)).(EntityIDType)
	return v, ok
}

// StringID is an OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type StringID[ID ~string] struct{}

func (m StringID[ID]) FormatID(id ID) (string, error) { return string(id), nil }
func (m StringID[ID]) ParseID(id string) (ID, error)  { return ID(id), nil }

// IntID is an OldMapping tool that you can embed in your OldMapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type IntID[ID ~int] struct{}

func (m IntID[ID]) FormatID(id ID) (string, error) {
	return strconv.Itoa(int(id)), nil
}

func (m IntID[ID]) ParseID(id string) (ID, error) {
	n, err := strconv.Atoi(id)
	return ID(n), err
}

// IDConverter is an OldMapping tool that you can embed in your OldMapping implementation,
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
