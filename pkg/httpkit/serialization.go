package httpkit

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/codec"
)

type RestResourceSerialization[Entity, ID any] struct {
	Serializers map[string]Serializer
	IDConverter idConverter[ID]
}

type Serializer interface {
	codec.Codec
}

var DefaultSerializers = map[string]Serializer{
	"application/json":                  jsonkit.Codec{},
	"application/problem+json":          jsonkit.Codec{},
	"application/x-ndjson":              jsonkit.LinesCodec{},
	"application/stream+json":           jsonkit.LinesCodec{},
	"application/json-stream":           jsonkit.LinesCodec{},
	"application/x-www-form-urlencoded": FormURLEncodedCodec{},
}

var DefaultSerializer = CodecDefault{
	Serializer: jsonkit.Codec{},
	MediaType:  mediatype.JSON,
}

type CodecDefault struct {
	Serializer interface {
		codec.Codec
		codec.ListDecoderMaker
	}
	MediaType string
}

func (m *RestResourceSerialization[Entity, ID]) getSerializer(mimeType string) (Serializer, string) {
	if ser, ok := m.lookupType(mimeType); ok {
		return ser, mimeType
	}
	return m.defaultSerializer()
}

func (m *RestResourceSerialization[Entity, ID]) requestBodySerializer(r *http.Request) (Serializer, string) {
	return m.contentTypeSerializer(r)
}

func (m *RestResourceSerialization[Entity, ID]) contentTypeSerializer(r *http.Request) (Serializer, string) {
	if mime, ok := m.getRequestBodyMimeType(r); ok { // TODO: TEST ME
		if serializer, ok := m.lookupType(mime); ok {
			return serializer, mime
		}
	}
	return m.defaultSerializer() // TODO: TEST ME
}

func (m *RestResourceSerialization[Entity, ID]) defaultSerializer() (Serializer, string) {
	return DefaultSerializer.Serializer, DefaultSerializer.MediaType
}

func (m *RestResourceSerialization[Entity, ID]) responseBodySerializer(r *http.Request) (Serializer, string) {
	var accept = r.Header.Get(headerKeyAccept)
	if accept == "" {
		return m.contentTypeSerializer(r)
	}
	var sers = mapkit.Merge(DefaultSerializers, m.Serializers)
	for _, mimeType := range strings.Fields(accept) {
		mimeType := string(mimeType)
		ser, ok := sers[mimeType]
		if ok {
			return ser, mimeType
		}
	}
	return m.contentTypeSerializer(r)
}

func (m *RestResourceSerialization[Entity, ID]) getRequestBodyMimeType(r *http.Request) (string, bool) {
	return getMIMETypeFrom(r.Header.Get(headerKeyContentType))
}

func getMIMETypeFrom(headerValue string) (string, bool) {
	if headerValue == "" {
		return *new(string), false
	}
	const parameterSeparatorSymbol = ";"
	if strings.Contains(headerValue, parameterSeparatorSymbol) {
		headerValue = strings.TrimSpace(strings.Split(headerValue, ";")[0])
	}
	mime := string(strings.Split(headerValue, ";")[0])
	return mime, true
}

func (m *RestResourceSerialization[Entity, ID]) lookupType(mimeType string) (Serializer, bool) {
	mimeType = getMediaType(mimeType) // TODO: TEST ME
	if m.Serializers != nil {
		if ser, ok := m.Serializers[mimeType]; ok {
			return ser, true
		}
	}
	if DefaultSerializers != nil {
		if ser, ok := DefaultSerializers[mimeType]; ok {
			return ser, true
		}
	}
	return nil, false
}

func (m *RestResourceSerialization[Entity, ID]) getIDConverter() idConverter[ID] {
	if m.IDConverter != nil {
		return m.IDConverter
	}
	return IDConverter[ID]{}
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
