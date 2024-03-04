package restapi

import (
	"go.llib.dev/frameless/pkg/mapkit"
	"io"
	"net/http"
	"strings"
)

type Serialization[Entity, ID any] struct {
	Serializers map[MIMEType]Serializer
	IDConverter idConverter[ID]
}

type Serializer interface {
	serializerMarshaller
	serializerUnmarshaller
}

type serializerMarshaller interface {
	Marshal(v any) ([]byte, error)
	NewListEncoder(w io.Writer) ListEncoder
}

type serializerUnmarshaller interface {
	Unmarshal(data []byte, ptr any) error
}

type ListEncoder interface {
	// Encode will encode an Entity in the underlying io writer.
	Encode(v any) error
	// Closer represent the finishing of the List encoding process.
	io.Closer
}

var DefaultSerializers = map[MIMEType]Serializer{
	JSON:                       JSONSerializer{},
	"application/problem+json": JSONSerializer{},
	"application/x-ndjson":     JSONStreamSerializer{},
	"application/stream+json":  JSONStreamSerializer{},
	"application/json-stream":  JSONStreamSerializer{},
}

var DefaultSerializer = SerializerDefault{
	Serializer: JSONSerializer{},
	MIMEType:   JSON,
}

type SerializerDefault struct {
	Serializer Serializer
	MIMEType   MIMEType
}

func (m *Serialization[Entity, ID]) requestBodySerializer(r *http.Request) (serializerUnmarshaller, MIMEType) {
	return m.contentTypeSerializer(r)
}

func (m *Serialization[Entity, ID]) contentTypeSerializer(r *http.Request) (Serializer, MIMEType) {
	if mime, ok := m.getRequestBodyMimeType(r); ok { // TODO: TEST ME
		if serializer, ok := m.lookupType(mime); ok {
			return serializer, mime
		}
	}
	return DefaultSerializer.Serializer, DefaultSerializer.MIMEType // TODO: TEST ME
}

func (m *Serialization[Entity, ID]) responseBodySerializer(r *http.Request) (serializerMarshaller, MIMEType) {
	var accept = r.Header.Get(headerKeyAccept)
	if accept == "" {
		return m.contentTypeSerializer(r)
	}
	var serializers = mapkit.Merge(DefaultSerializers, m.Serializers)
	for _, mimeType := range strings.Fields(accept) {
		mimeType := MIMEType(mimeType)
		ser, ok := serializers[mimeType]
		if ok {
			return ser, mimeType
		}
	}
	return m.contentTypeSerializer(r)
}

func (m *Serialization[Entity, ID]) getRequestBodyMimeType(r *http.Request) (MIMEType, bool) {
	return getMIMETypeFrom(r.Header.Get(headerKeyContentType))
}

func getMIMETypeFrom(headerValue string) (MIMEType, bool) {
	if headerValue == "" {
		return *new(MIMEType), false
	}
	const parameterSeparatorSymbol = ";"
	if strings.Contains(headerValue, parameterSeparatorSymbol) {
		headerValue = strings.TrimSpace(strings.Split(headerValue, ";")[0])
	}
	mime := MIMEType(strings.Split(headerValue, ";")[0])
	return mime, true
}

func (m *Serialization[Entity, ID]) lookupType(mimeType MIMEType) (Serializer, bool) {
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

func (m *Serialization[Entity, ID]) getIDConverter() idConverter[ID] {
	if m.IDConverter != nil {
		return m.IDConverter
	}
	return IDConverter[ID]{}
}
