package restapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

type ResourceSerialization[Entity, ID any] struct {
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
	NewListDecoder(w io.ReadCloser) ListDecoder
}

type ListEncoder interface {
	// Encode will encode an Entity in the underlying io writer.
	Encode(v any) error
	// Closer represent the finishing of the List encoding process.
	io.Closer
}

type ListDecoder interface {
	Decode(ptr any) error
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Err return the error cause.
	Err() error
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
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

func (m *ResourceSerialization[Entity, ID]) getSerializer(mimeType MIMEType) (Serializer, MIMEType) {
	if ser, ok := m.lookupType(mimeType); ok {
		return ser, mimeType
	}
	return m.defaultSerializer()
}

func (m *ResourceSerialization[Entity, ID]) requestBodySerializer(r *http.Request) (serializerUnmarshaller, MIMEType) {
	return m.contentTypeSerializer(r)
}

func (m *ResourceSerialization[Entity, ID]) contentTypeSerializer(r *http.Request) (Serializer, MIMEType) {
	if mime, ok := m.getRequestBodyMimeType(r); ok { // TODO: TEST ME
		if serializer, ok := m.lookupType(mime); ok {
			return serializer, mime
		}
	}
	return m.defaultSerializer() // TODO: TEST ME
}

func (m *ResourceSerialization[Entity, ID]) defaultSerializer() (Serializer, MIMEType) {
	return DefaultSerializer.Serializer, DefaultSerializer.MIMEType
}

func (m *ResourceSerialization[Entity, ID]) responseBodySerializer(r *http.Request) (serializerMarshaller, MIMEType) {
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

func (m *ResourceSerialization[Entity, ID]) getRequestBodyMimeType(r *http.Request) (MIMEType, bool) {
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

func (m *ResourceSerialization[Entity, ID]) lookupType(mimeType MIMEType) (Serializer, bool) {
	mimeType = mimeType.Base() // TODO: TEST ME
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

func (m *ResourceSerialization[Entity, ID]) getIDConverter() idConverter[ID] {
	if m.IDConverter != nil {
		return m.IDConverter
	}
	return IDConverter[ID]{}
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

func (s JSONSerializer) NewListDecoder(r io.ReadCloser) ListDecoder {
	return &jsonListDecoder{R: r}
}

type jsonListEncoder struct {
	W io.Writer

	bracketOpen bool
	index       int
	err         error
	done        bool
}

func (c *jsonListEncoder) Encode(dto any) error {
	if c.err != nil {
		return c.err
	}

	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	if 0 < c.index {
		if _, err := c.W.Write([]byte(`,`)); err != nil {
			c.err = err
			return err
		}
	}

	if _, err := c.W.Write(data); err != nil {
		c.err = err
		return err
	}

	c.index++
	return nil
}

func (c *jsonListEncoder) Close() error {
	if c.done {
		return c.err
	}
	c.done = true
	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}
	if c.bracketOpen {
		if err := c.endList(); err != nil {
			return err
		}
	}
	return nil
}

func (c *jsonListEncoder) endList() error {
	if _, err := c.W.Write([]byte(`]`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = false
	return nil
}

func (c *jsonListEncoder) beginList() error {
	if _, err := c.W.Write([]byte(`[`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = true
	return nil
}

type jsonListDecoder struct {
	R io.ReadCloser

	br  *bufio.Reader
	dec *json.Decoder

	inList      bool
	bracketOpen bool
	index       int
	err         error
	done        bool
	data        []byte
}

func (c *jsonListDecoder) Next() bool {
	if c.done {
		return false
	}
	if c.err != nil {
		return false
	}
	if !c.inList {
		char, err := c.readRune()
		if err != nil {
			c.err = err
			return false
		}
		if char != '[' {
			c.err = fmt.Errorf("unexpected character, got %s, but expected %s", string(char), "[")
			return false
		}
		c.inList = true
	}

	data, ok := c.prepareForNextListItem()
	if !ok {
		return false
	}

	for !json.Valid(data) {
		char, err := c.readRune()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			c.err = err
			return false
		}
		data = append(data, []byte(string(char))...)
	}

	if !json.Valid(data) {
		c.err = fmt.Errorf("invalid json received: %s", string(data))
		return false
	}

	c.data = data
	return true
}

func (c *jsonListDecoder) prepareForNextListItem() ([]byte, bool) {
	var data []byte
	char, err := c.readRune()
	if errors.Is(err, io.EOF) {
		return data, false
	}
	if err != nil {
		c.err = err
		return data, false
	}
	if c.inList {
		if char == ']' { // we are done
			c.done = true
			return data, false
		}
		if char != ',' {
			data = append(data, []byte(string(char))...)
		}
	}
	return data, true
}

func (c *jsonListDecoder) readRune() (rune, error) {
	rn, _, err := c.reader().ReadRune()
	return rn, err
}

func (c *jsonListDecoder) reader() *bufio.Reader {
	if c.br == nil {
		c.br = bufio.NewReader(c.R)
	}
	return c.br
}

func (c *jsonListDecoder) Err() error {
	return c.err
}

func (c *jsonListDecoder) Decode(ptr any) error {
	return json.Unmarshal(c.data, ptr)
}

func (c *jsonListDecoder) Close() error {
	return c.R.Close()
}

type JSONStreamSerializer struct{}

func (s JSONStreamSerializer) NewListDecoder(w io.ReadCloser) ListDecoder {
	//TODO implement me
	panic("implement me")
}

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
