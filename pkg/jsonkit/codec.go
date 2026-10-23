package jsonkit

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/codec"
)

type Codec struct {
	typeIDRegistry *_TypeRegistry
}

func CodecRegisterTypeID[T any](codec *Codec, id TypeID, aliases ...TypeID) func() {
	if codec == nil {
		panic("CodecRegisterTypeID: codec must not be nil")
	}
	if codec.typeIDRegistry == nil {
		codec.typeIDRegistry = &_TypeRegistry{}
	}
	var rType = reflect.TypeOf((*T)(nil)).Elem()
	return codec.typeIDRegistry.Register(rType, id, aliases...)
}

// CodecRegister binds a custom Marshal/Unmarshal pair to a concrete type for
// this codec instance. After registration, c.Marshal(v) and c.Unmarshal(data,
// &v) for values of (or behind an interface field pointing to) the registered
// type will route through the supplied closures instead of the default
// reflect-based encoding. The codec-local registry is updated with the type ID
// so @type envelopes continue to be emitted and the value can be reconstructed
// during unmarshal via the same type ID lookup.
//
// The closure's T can be either the value type or a pointer to it. The codec
// takes care of the matching between an incoming value's runtime kind and the
// closure's expected T so a registration like CodecRegister[*T] still accepts
// value-typed T (we allocate a fresh *T and copy) and a registration like
// CodecRegister[T] still accepts *T (we dereference it).
//
// The returned function restores the previous registration (typically a
// no-op), so test setups can register types in a t.Cleanup-friendly way.
func CodecRegister[T any](codec *Codec, id TypeID, tc ITypeCodec[T]) func() {
	if codec == nil {
		panic("CodecRegister: codec must not be nil")
	}
	if codec.typeIDRegistry == nil {
		codec.typeIDRegistry = &_TypeRegistry{}
	}
	var userT = reflect.TypeOf((*T)(nil)).Elem()
	wantPtr := userT.Kind() == reflect.Pointer
	baseT := userT
	for baseT.Kind() == reflect.Pointer {
		baseT = baseT.Elem()
	}
	// Closure adapters bridge between the codec's runtime shape and the
	// user's registered T. The codec marshals/unmarshals the BASE type
	// (e.g. Foo), but the user's closure may be parameterised on either
	// the base (Foo) or a pointer to it (*Foo). The adapter normalises
	// so a registration with either shape accepts values of either shape.
	entry := &customCodecEntry{
		TypeID: id,
		Marshal: func(c *Codec, v any) ([]byte, error) {
			if !wantPtr {
				// userT is Foo (value type); closure wants Foo.
				rv := reflect.ValueOf(v)
				if rv.Kind() == reflect.Pointer {
					// A *Foo reached via an interface field — the
					// codec may unmarshal a pointer-wrapped leaf back
					// to a value Foo via the type alias, so we
					// dereference here and let the closure produce a
					// Foo wire.
					if rv.IsNil() {
						return []byte("null"), nil
					}
					if rv.Elem().Type() == baseT {
						return tc.Marshal(c, rv.Elem().Interface().(T))
					}
					return tc.Marshal(c, rv.Convert(baseT).Interface().(T))
				}
				return tc.Marshal(c, v.(T))
			}
			// userT is *Foo; closure wants *Foo.
			rv := reflect.ValueOf(v)
			if rv.Type() == userT {
				return tc.Marshal(c, v.(T))
			}
			// v is value-typed Foo; allocate *Foo and copy.
			ptr := reflect.New(baseT)
			if rv.Type() == baseT {
				ptr.Elem().Set(rv)
			} else {
				ptr.Elem().Set(rv.Convert(baseT))
			}
			var arg T = ptr.Interface().(T)
			return tc.Marshal(c, arg)
		},
		Unmarshal: func(c *Codec, data []byte, p any) error {
			if !wantPtr {
				return tc.Unmarshal(c, data, p.(*T))
			}
			// userT is *Foo; closure wants **Foo. The codec gave us a
			// *Foo (a freshly allocated T). Wrap in **Foo for the
			// closure, run it, then copy the populated inner back to
			// the *Foo so dst.Set (called by the codec after we
			// return) observes the result.
			pp := reflect.New(userT) // **Foo
			pp.Elem().Set(reflect.ValueOf(p))
			var arg T = pp.Elem().Interface().(T)
			if err := tc.Unmarshal(c, data, &arg); err != nil {
				return err
			}
			// Copy *pp (the populated *Foo) back to the *Foo the
			// codec gave us.
			reflect.ValueOf(p).Elem().Set(pp.Elem().Elem())
			return nil
		},
	}
	return codec.typeIDRegistry.SetCustomCodec(baseT, entry)
}

// CodecRegisterMigration migrates data stored under a retired @type into T when it is read.
// Values tagged with fromID are decoded by migrate, which receives the payload with the @type envelope stripped,
// and maps the legacy shape onto T.
// Marshalling keeps emitting T's own type ID, so migrated data is written back in the current format.
//
// Unlike the aliases of CodecRegisterTypeID, which are alternative names for the same wire shape,
// a migration owns its own decoding, so the retired shape may differ from T's.
//
// T must be a concrete (non-pointer) type. The returned function removes the migration.
func CodecRegisterMigration[T any](codec *Codec, fromID TypeID, migrate func(c *Codec, data []byte, p *T) error) func() {
	if codec == nil {
		panic("CodecRegisterMigration: codec must not be nil")
	}
	if migrate == nil {
		panic("CodecRegisterMigration: migrate must not be nil")
	}
	if codec.typeIDRegistry == nil {
		codec.typeIDRegistry = &_TypeRegistry{}
	}
	rType := reflect.TypeOf((*T)(nil)).Elem()
	if rType.Kind() == reflect.Pointer {
		panic("CodecRegisterMigration: T must not be a pointer type")
	}
	return codec.typeIDRegistry.SetMigration(fromID, &migrationEntry{
		Type: rType,
		Unmarshal: func(c *Codec, data []byte, p any) error {
			return migrate(c, data, p.(*T))
		},
	})
}

type ITypeCodec[T any] interface {
	Marshal(c *Codec, v T) ([]byte, error)
	Unmarshal(c *Codec, data []byte, p *T) error
}

type TypeCodec[T any] struct {
	MarshalFunc   func(c *Codec, v T) ([]byte, error)
	UnmarshalFunc func(c *Codec, data []byte, p *T) error
}

func (tc TypeCodec[T]) Marshal(c *Codec, v T) ([]byte, error) {
	return tc.MarshalFunc(c, v)
}

func (tc TypeCodec[T]) Unmarshal(c *Codec, data []byte, p *T) error {
	return tc.UnmarshalFunc(c, data, p)
}

func (c Codec) Marshal(v any) ([]byte, error) {
	placeholder, err := marshalPlaceholderWithReg(reflect.ValueOf(v), nil, c.typeIDRegistry, &c, true)
	if err != nil {
		return nil, err
	}
	if !placeholder.IsValid() {
		return json.Marshal(nil)
	}
	return json.Marshal(placeholder.Interface())
}

// Unmarshal decodes data into the value ptr points to, like json.Unmarshal does:
// a struct keeps the fields without a member in the JSON, a pointer keeps pointing to the value it points to,
// a map keeps its other entries, and the elements of a slice or an array are decoded in place.
//
// Unlike with encoding/json, an interface value is decoded by its @type,
// and it replaces the value the interface holds, instead of being decoded into it.
func (c Codec) Unmarshal(data []byte, ptr any) error {
	if ptr == nil {
		return fmt.Errorf("Unmarshal: nil destination")
	}
	dst := reflect.ValueOf(ptr)
	if dst.Kind() != reflect.Pointer || dst.IsNil() {
		return fmt.Errorf("Unmarshal: destination must be a non-nil pointer")
	}
	return unmarshalReflectWithReg(data, dst.Elem(), c.typeIDRegistry, &c)
}

func (c Codec) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return &ArrayEncoder[any]{W: w, C: c}
}

func (c Codec) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.Decoder, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

//////////////

type LinesCodec struct {
	// UseNumber causes the Decoder to unmarshal a number into an
	// interface value as a [Number] instead of as a float64.
	UseNumber             bool
	DisallowUnknownFields bool
}

func (LinesCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (LinesCodec) Unmarshal(data []byte, p any) error {
	return json.Unmarshal(data, p)
}

func (LinesCodec) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return streamEncoder{Encoder: json.NewEncoder(w)}
}

type streamEncoder struct{ *json.Encoder }

func (streamEncoder) Close() error { return nil }

func (c LinesCodec) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	dec := json.NewDecoder(r)
	if c.UseNumber {
		dec.UseNumber()
	}
	if c.DisallowUnknownFields {
		dec.DisallowUnknownFields()
	}
	return func(yield func(dec codec.Decoder, err error) bool) {
		for dec.More() {
			if !yield(dec, nil) {
				return
			}
		}
	}
}

//////////////

func NewArrayStreamEncoder[T any](w io.Writer) *ArrayEncoder[T] {
	return &ArrayEncoder[T]{W: w, C: Codec{}}
}

type ArrayEncoder[T any] struct {
	W io.Writer
	C Codec

	bracketOpen bool
	index       int
	err         error
	done        bool
}

func (c *ArrayEncoder[T]) Encode(v T) error {
	if c.err != nil {
		return c.err
	}

	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}

	data, err := c.C.Marshal(v)
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

func (c *ArrayEncoder[T]) Close() error {
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

func (c *ArrayEncoder[T]) endList() error {
	if _, err := c.W.Write([]byte(`]`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = false
	return nil
}

func (c *ArrayEncoder[T]) beginList() error {
	if _, err := c.W.Write([]byte(`[`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = true
	return nil
}

func NewArrayStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.Decoder, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

func NewEncoder[T any](w io.Writer) *Encoder[T] {
	return &Encoder[T]{Encoder: json.NewEncoder(w)}
}

type Encoder[T any] struct{ *json.Encoder }

func (e *Encoder[T]) Close() error {
	return nil
}

func (e *Encoder[T]) Encode(v T) error {
	return e.Encoder.Encode(v)
}

func NewDecoder[T any](r io.Reader) *Decoder[T] {
	var rc io.ReadCloser
	if v, ok := r.(io.ReadCloser); ok {
		rc = v
	} else {
		rc = io.NopCloser(r)
	}
	return &Decoder[T]{
		Decoder: json.NewDecoder(rc),
		Closer:  rc,
	}
}

type Decoder[T any] struct {
	*json.Decoder
	io.Closer
}

func (d *Decoder[T]) Decode(p *T) error {
	return d.Decoder.Decode(p)
}

var rawMessageType = reflect.TypeOf(json.RawMessage{})

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

// isAutoWrappableKind reports whether a reflect.Value should be auto-wrapped
// with the @type envelope when its type is registered. We skip the unnamed
// primitive kinds (string/int/bool/float) because they are registered globally
// for the explicit jsonkit.WithType[T] use case, but they must continue to
// encode as bare JSON primitives when used as plain fields.
//
// Named types that happen to have a primitive kind (e.g. wftemplate.Condition
// is `type Condition string`) DO get auto-wrapped: their user intent is to be
// a polymorphic type, not a plain string.
func isAutoWrappableKind(v reflect.Value) bool {
	if !isJSONPrimitiveKind(v.Kind()) {
		return true
	}
	// If the value's reflect.Type is a named type (not the unnamed
	// `string`/`int`/...), it is a user-defined type and gets auto-wrapped.
	// Unnamed primitive kinds have Name() == "".
	return v.Type().Name() != ""
}

// isJSONPrimitiveKind reports whether a reflect.Kind is one that JSON encodes
// as a primitive literal (string, number, bool). These kinds must NOT be
// auto-wrapped with the @type envelope when they appear as plain field values,
// even if their concrete type happens to be globally registered via
// jsonkit.RegisterTypeID (e.g. for the explicit jsonkit.WithType[T] use case).
func isJSONPrimitiveKind(k reflect.Kind) bool {
	switch k {
	case reflect.String,
		reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// marshalWithMarshaler delegates to a value's own json.Marshaler
// implementation, returning a json.RawMessage shadow so the caller can keep
// treating the result as a placeholder. This lets types that bring their own
// MarshalJSON (e.g. workflow.If, workflow.Suspend, ...) be marshaled by the
// codec without losing the per-type envelope semantics those methods already
// implement.
func marshalWithMarshaler(v reflect.Value, reg *_TypeRegistry) (reflect.Value, error) {
	data, err := v.Interface().(json.Marshaler).MarshalJSON()
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(json.RawMessage(data)), nil
}

func marshalPlaceholder(v reflect.Value, declared reflect.Type) (reflect.Value, error) {
	return marshalPlaceholderWithReg(v, declared, nil, nil, true)
}

// marshalPlaceholderWithReg marshals v into a shadow reflect.Value suitable
// for json.Marshal. When allowEnvelopeWrap is true and the value's type is
// registered but has no json.Marshaler, the shadow is wrapped with a typed
// envelope so the JSON output carries a @type discriminator.
//
// allowEnvelopeWrap is false when the call originates from the polymorphic
// recursion in the declared-interface branch (line below). That branch
// already adds the @type envelope via wrapPolymorphicWithReg, so a second
// envelope would duplicate the @type field.
//
// codec is the originating *Codec if any, used to thread the codec reference
// through to user-registered custom Marshal closures. It may be nil when the
// call originates from a context that has no codec (e.g. typed.MarshalJSON
// triggered by an explicit jsonkit.WithType[T]).
func marshalPlaceholderWithReg(v reflect.Value, declared reflect.Type, reg *_TypeRegistry, codec *Codec, allowEnvelopeWrap bool) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.ValueOf(json.RawMessage("null")), nil
	}
	if declared != nil && declared.Kind() == reflect.Interface {
		if v.Kind() == reflect.Interface && v.IsNil() {
			return reflect.ValueOf(json.RawMessage("null")), nil
		}
		// For values that contain interface elements (structs with interface
		// fields, slices/maps/arrays of interfaces, etc.), recurse through the
		// shadow logic so nested polymorphic values are dispatched correctly,
		// then wrap the resulting JSON with a @type envelope.
		if v.Kind() == reflect.Struct ||
			(v.Kind() == reflect.Interface && v.Elem().Kind() == reflect.Struct) ||
			containsInterfaceElement(v.Type()) {
			concrete := v
			if v.Kind() == reflect.Interface {
				concrete = v.Elem()
			}
			// When the concrete type has a CodecRegister-style custom
			// Marshal closure, invoke it directly so this interface
			// branch is the single source of the @type envelope. A
			// naive recursion through marshalPlaceholderWithReg
			// would also add @type via the custom-codec branch,
			// producing a duplicate @type field on the wire.
			var innerData []byte
			if entry := reg.LookupCustomCodec(concrete.Type()); entry != nil {
				data, err := entry.Marshal(codec, concrete.Interface())
				if err != nil {
					return reflect.Value{}, err
				}
				if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), null) {
					return reflect.ValueOf(json.RawMessage("null")), nil
				}
				innerData = data
			} else {
				shadow, err := marshalPlaceholderWithReg(concrete, nil, reg, codec, false)
				if err != nil {
					return reflect.Value{}, err
				}
				innerData, err = json.Marshal(shadow.Interface())
				if err != nil {
					return reflect.Value{}, err
				}
			}
			// Wrap with the @type envelope based on the concrete type's
			// registry lookup, so the round-trip can dispatch the
			// polymorphic value back to its concrete type on the unmarshal
			// side. For a registered concrete type (e.g. workflow.Sequence
			// — which is itself a slice but is registered with a TypeID)
			// the envelope is added; for an unregistered plain container
			// (e.g. map[string]any, []any, *[]any) the wrapper returns the
			// inner data unchanged so the caller does not have to register
			// the container type.
			wrapped, err := wrapPolymorphicWithReg(concrete.Interface(), innerData, reg)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(json.RawMessage(wrapped)), nil
		}
		t := typed{Type: declared, Value: v.Interface(), Force: true, reg: reg}
		data, err := t.MarshalJSON()
		return reflect.ValueOf(json.RawMessage(data)), err
	}
	if reflectkit.IsNilable(v.Kind()) && v.IsNil() {
		return reflect.Zero(rawMessageType), nil
	}
	if v.Kind() == reflect.Interface {
		return marshalPlaceholderWithReg(v.Elem(), v.Type(), reg, codec, allowEnvelopeWrap)
	}
	// If the value is one of the jsonkit polymorphic wrapper types
	// (jsonkit.Array[T] / jsonkit.Interface[I]), bypass their custom
	// MarshalJSON and process them via the codec's reflection path so the
	// codec's per-instance registry is honoured. The wrappers' MarshalJSON
	// is still useful for direct stdlib usage (json.Marshal(wrapper)),
	// but it has no way to receive a per-instance registry, so when the
	// codec owns the marshaling, the reflection path is what produces a
	// codec-isolated wire.
	if v.CanInterface() && isArrayOfInterfaces(v.Type()) {
		// Recurse on the underlying []T (the slice that Array[T] is
		// defined as), keeping the registry. allowEnvelopeWrap=false so
		// the inner reflection path picks the right marshaling branch.
		underlying := v.Convert(reflect.SliceOf(v.Type().Elem()))
		return marshalPlaceholderWithReg(
			underlying,
			v.Type().Elem(),
			reg,
			codec,
			false,
		)
	}

	// If the value's concrete type brings its own json.Marshaler, prefer it
	// over the reflect-based shadow build. Types like workflow.If,
	// workflow.Sequence, workflow.Suspend, etc. already implement MarshalJSON
	// to inject the @type envelope and the per-field envelope for interface
	// fields; re-deriving that from reflect would lose both. The shadow build
	// only kicks in for types without a custom MarshalJSON.
	if v.CanInterface() && v.Type().Implements(jsonMarshalerType) {
		return marshalWithMarshaler(v, reg)
	}
	// If the value's concrete type was registered via CodecRegister with a
	// user-supplied Marshal closure, prefer that over the reflect-based shadow
	// build AND the generic typed envelope. The custom closure encodes the
	// If the value's concrete type was registered via CodecRegister with a
	// user-supplied Marshal closure, prefer that over the reflect-based shadow
	// build AND the generic typed envelope. The custom closure encodes the
	// value into the wire format the user wants (often a DTO shape), and the
	// @type envelope is added on top so unmarshal can route the bytes back
	// through the matching custom Unmarshal. Without this branch, types
	// registered with CodecRegister would fall back to default field
	// marshaling, losing the DTO mapping the user registered for.
	if v.CanInterface() {
		if entry := reg.LookupCustomCodec(v.Type()); entry != nil {
			data, err := entry.Marshal(codec, v.Interface())
			if err != nil {
				return reflect.Value{}, err
			}
			if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), null) {
				return reflect.Zero(rawMessageType), nil
			}
			if entry.TypeID != "" {
				// wrapPolymorphicWithReg picks the right envelope shape
				// based on whether the custom Marshal closure produced a
				// JSON object (prepend @type as a field) or a non-object
				// value (full envelope with @value). Calling
				// wrapWithTypeIDValue directly would corrupt any non-object
				// wire format — e.g. workflow.Sequence whose closure emits
				// a bare JSON array.
				wrapped, wErr := wrapPolymorphicWithReg(v.Interface(), data, reg)
				if wErr != nil {
					return reflect.Value{}, wErr
				}
				return reflect.ValueOf(json.RawMessage(wrapped)), nil
			}
			return reflect.ValueOf(json.RawMessage(data)), nil
		}
	}
	// If the value's concrete type is registered (in the codec or in the
	// package-level registry) but has no json.Marshaler, route it through the
	// typed envelope so the JSON output carries the @type discriminator. This
	// mirrors what jsonkit.WithType[T] does for explicit callers, and keeps
	// codec.Marshal(workflow.SetVar{...}) round-trippable through the
	// Definition / Condition interfaces.
	//
	// Skip primitive kinds: string/int/bool/float are registered globally for
	// the explicit jsonkit.WithType[T] use case, but they must continue to
	// encode as bare JSON primitives when used as plain fields (e.g. inside a
	// parent struct). Only composite / pointer kinds get auto-wrapped.
	if allowEnvelopeWrap && v.CanInterface() && isAutoWrappableKind(v) {
		if _, ok := lookupTypeIDForType(reg, v.Type()); ok {
			data, err := typed{Type: v.Type(), Value: v.Interface(), Force: true, reg: reg}.MarshalJSON()
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(json.RawMessage(data)), nil
		}
	}

	switch v.Kind() {
	case reflect.Pointer:
		value, err := marshalPlaceholderWithReg(v.Elem(), v.Type().Elem(), reg, codec, allowEnvelopeWrap)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(value.Type())
		ptr.Elem().Set(value)
		return ptr, nil

	case reflect.Struct:
		fields := jsonFields(v.Type())
		shadowFields := make([]reflect.StructField, 0, len(fields))
		values := make([]reflect.Value, 0, len(fields))
		for _, field := range fields {
			value, err := v.FieldByIndexErr(field.index)
			if err != nil {
				// The field is in a struct embedded through a nil pointer, which encoding/json leaves out too.
				continue
			}
			if field.tag.Omitzero && reflectkit.IsZero(value) {
				continue
			}
			if field.tag.Omitempty && isEmptyForOmitEmpty(field.field.Type, value) {
				continue
			}
			value, err = marshalPlaceholderWithReg(value, field.field.Type, reg, codec, false)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("%s: %w", field.field.Name, err)
			}
			shadowFields = append(shadowFields, reflect.StructField{
				Name: "F" + strconv.Itoa(len(shadowFields)),
				Type: value.Type(),
				Tag:  jsonStructTag(field.name, field.tag.Stringify),
				// A shadow field is never embedded, as the methods of an embedded type,
				// like the MarshalJSON of a json.RawMessage, would be promoted to the shadow struct.
			})
			values = append(values, value)
		}
		shadow := reflect.New(reflect.StructOf(shadowFields)).Elem()
		for i, value := range values {
			shadow.Field(i).Set(value)
		}
		return shadow, nil

	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(reflect.SliceOf(rawMessageType)), nil
		}
		return marshalSequencePlaceholderWithReg(v, false, reg, codec)

	case reflect.Array:
		return marshalSequencePlaceholderWithReg(v, true, reg, codec)

	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(reflect.MapOf(v.Type().Key(), rawMessageType)), nil
		}
		values := make([]struct {
			key   reflect.Value
			value reflect.Value
		}, 0, v.Len())
		var elemType reflect.Type
		iter := v.MapRange()
		for iter.Next() {
			value, err := marshalPlaceholderWithReg(iter.Value(), v.Type().Elem(), reg, codec, false)
			if err != nil {
				return reflect.Value{}, err
			}
			if elemType == nil {
				elemType = value.Type()
			} else if value.Type() != elemType {
				data, err := json.Marshal(value.Interface())
				if err != nil {
					return reflect.Value{}, err
				}
				value = reflect.ValueOf(json.RawMessage(data))
				elemType = rawMessageType
			}
			values = append(values, struct {
				key   reflect.Value
				value reflect.Value
			}{iter.Key(), value})
		}
		if elemType == nil {
			elemType = rawMessageType
		}
		shadow := reflect.MakeMapWithSize(reflect.MapOf(v.Type().Key(), elemType), len(values))
		for _, entry := range values {
			value := entry.value
			if value.Type() != elemType {
				data, err := json.Marshal(value.Interface())
				if err != nil {
					return reflect.Value{}, err
				}
				value = reflect.ValueOf(json.RawMessage(data))
			}
			shadow.SetMapIndex(entry.key, value)
		}
		return shadow, nil

	default:
		return v, nil
	}
}

func marshalSequencePlaceholderWithReg(v reflect.Value, array bool, reg *_TypeRegistry, codec *Codec) (reflect.Value, error) {
	values := make([]reflect.Value, v.Len())
	var elemType reflect.Type
	for i := 0; i < v.Len(); i++ {
		value, err := marshalPlaceholderWithReg(v.Index(i), v.Type().Elem(), reg, codec, false)
		if err != nil {
			return reflect.Value{}, err
		}
		values[i] = value
		if elemType == nil {
			elemType = value.Type()
		} else if elemType != value.Type() {
			elemType = rawMessageType
		}
	}
	if elemType == nil {
		elemType = rawMessageType
	}
	var shadow reflect.Value
	if array {
		shadow = reflect.New(reflect.ArrayOf(v.Len(), elemType)).Elem()
	} else {
		shadow = reflect.MakeSlice(reflect.SliceOf(elemType), v.Len(), v.Len())
	}
	for i, value := range values {
		if value.Type() != elemType {
			data, err := json.Marshal(value.Interface())
			if err != nil {
				return reflect.Value{}, err
			}
			value = reflect.ValueOf(json.RawMessage(data))
		}
		shadow.Index(i).Set(value)
	}
	return shadow, nil
}

// isArrayOfInterfaces reports whether t is a jsonkit.Array[T] with an interface T.
// Its own MarshalJSON and UnmarshalJSON can only use the package level registry,
// so the Codec processes it as the []T it is defined as, which keeps the Codec's own registry in use.
func isArrayOfInterfaces(t reflect.Type) bool {
	return t.Kind() == reflect.Slice &&
		t.PkgPath() == "go.llib.dev/frameless/pkg/jsonkit" &&
		strings.HasPrefix(t.Name(), "Array[") &&
		t.Elem().Kind() == reflect.Interface
}

// hasOwnJSONUnmarshaler reports whether the Codec decodes a value of type t with t's own json.Unmarshaler.
// It mirrors the marshal side, where a value with its own json.Marshaler is encoded with it.
// A pointer has the json.Unmarshaler of the type it points to.
func hasOwnJSONUnmarshaler(t reflect.Type) bool {
	if !implementsJSONUnmarshaler(t) {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return !isArrayOfInterfaces(t)
}

// unmarshalPlan describes how to decode JSON into a struct which needs the Codec, see requiresCodec.
// The JSON is first decoded by encoding/json into a DTO struct, which has a field for each JSON member of the struct,
// then each member is decoded into the struct field it belongs to.
type unmarshalPlan struct {
	dtoType reflect.Type
	fields  []fieldPlan
}

type fieldPlan struct {
	dtoIndex  int
	targetIdx []int
	// plain reports whether encoding/json decodes the member on its own.
	// Then the DTO field has the type and the ,string option of the struct field,
	// and it is set from the struct field before the decoding,
	// so the member is decoded into the value the struct field already holds, like with encoding/json.
	plain bool
	// decode decodes the member into the struct field, when it is not plain.
	// Then the DTO field is a json.RawMessage, which is empty when the JSON has no member for the field.
	decode func(data []byte, target reflect.Value, reg *_TypeRegistry, codec *Codec) error
}

// unmarshalPlanCache memoizes the unmarshalPlan of the struct types.
var unmarshalPlanCache synckit.Map[reflect.Type, *unmarshalPlan]

// getUnmarshalPlan returns a cached or freshly built unmarshalPlan for rt.
func getUnmarshalPlan(rt reflect.Type) *unmarshalPlan {
	if plan := unmarshalPlanCache.Get(rt); plan != nil {
		return plan
	}
	plan := buildUnmarshalPlan(rt)
	unmarshalPlanCache.Set(rt, plan)
	return plan
}

// invalidateUnmarshalPlanCache drops what is cached about how to decode the types,
// as registering a custom codec for a type changes how the types holding it are decoded.
//
// We invalidate the whole cache rather than tracking reverse
// dependencies. The cache is built lazily, plans are cheap to rebuild,
// and CodecRegister is typically called once during setup. The cost is
// that subsequent unmarshals rebuild plans they would otherwise reuse,
// which is negligible for typical payloads.
func invalidateUnmarshalPlanCache() {
	unmarshalPlanCache.Reset()
	requiresCodecCache.Reset()
}

func buildUnmarshalPlan(rt reflect.Type) *unmarshalPlan {
	var (
		fields    = jsonFields(rt)
		dtoFields = make([]reflect.StructField, 0, len(fields))
		plans     = make([]fieldPlan, 0, len(fields))
	)
	for _, field := range fields {
		var (
			fp       = fieldPlan{dtoIndex: len(dtoFields), targetIdx: field.index}
			dtoField = reflect.StructField{
				Name: "F" + strconv.Itoa(len(dtoFields)),
				Type: rawMessageType,
				Tag:  jsonStructTag(field.name, false),
				// A DTO field is never embedded, as the methods of an embedded type would be promoted to the DTO.
			}
			// An unexported field is an embedded struct with a name in its json tag,
			// which can't be set as a whole, so it is decoded in place, like with encoding/json.
			codecFree = field.field.IsExported() && !requiresCodec(field.field.Type)
		)
		switch {
		case codecFree && !field.throughPointer:
			fp.plain = true
			dtoField.Type = field.field.Type
			dtoField.Tag = jsonStructTag(field.name, field.tag.Stringify)
		case codecFree:
			// The field is in a struct embedded through a pointer, which is allocated only when the JSON has a member for the field,
			// so the member is captured as raw JSON, which tells whether the JSON has it.
			fp.decode = decodeWithEncodingJSON(field.field.Type, field.tag.Stringify)
		default:
			fp.decode = unmarshalReflectWithReg
		}
		dtoFields = append(dtoFields, dtoField)
		plans = append(plans, fp)
	}
	return &unmarshalPlan{dtoType: reflect.StructOf(dtoFields), fields: plans}
}

// decodeWithEncodingJSON returns a decode function of a fieldPlan, which decodes the member with encoding/json,
// with the ,string option of the field when stringify is set.
func decodeWithEncodingJSON(t reflect.Type, stringify bool) func(data []byte, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	// The member is decoded into the field of a wrapper struct, as that is where encoding/json applies the ,string option.
	wrapperType := reflect.StructOf([]reflect.StructField{{Name: "V", Type: t, Tag: jsonStructTag("V", stringify)}})
	return func(data []byte, target reflect.Value, _ *_TypeRegistry, _ *Codec) error {
		wrapper := reflect.New(wrapperType).Elem()
		wrapper.Field(0).Set(target)
		err := json.Unmarshal(slices.Concat([]byte(`{"V":`), data, []byte(`}`)), wrapper.Addr().Interface())
		// What encoding/json decoded before an error is kept, like when it decodes the field itself.
		target.Set(wrapper.Field(0))
		return err
	}
}

// fieldByIndexAlloc is like reflect.Value#FieldByIndex,
// but it allocates the nil embedded struct pointers on the way, like encoding/json does.
func fieldByIndexAlloc(v reflect.Value, index []int) (reflect.Value, error) {
	for i, x := range index {
		if 0 < i && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, errCannotSetEmbeddedPointer(v.Type())
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, nil
}

func errCannotSetEmbeddedPointer(ptrType reflect.Type) error {
	return fmt.Errorf("cannot set embedded pointer to unexported struct: %v", ptrType.Elem())
}

// jsonStructTag returns the json struct tag of a field in a struct built by reflection,
// which is the JSON member with the given name.
func jsonStructTag(name string, stringify bool) reflect.StructTag {
	value := name
	if stringify {
		value += ",string"
	} else if value == "-" {
		value += "," // a json tag of only "-" would make encoding/json ignore the field
	}
	return reflect.StructTag("json:" + strconv.Quote(value))
}

// copyTypedFromRawWithReg decodes a JSON value into target, which is an interface-typed reflect.Value.
// Unlike encoding/json, the value the interface holds is replaced instead of decoded into,
// as the @type envelope of the JSON tells what the new value is.
// A registered struct type is decoded with its custom codec or migration, if it has one,
// and a registered slice, array or map type is decoded with the Codec,
// so the interface values nested in them are decoded by their @type too.
func copyTypedFromRawWithReg(raw []byte, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, null) {
		target.SetZero()
		return nil
	}
	// A JSON object without a @type is not a polymorphic value, but a generic JSON object,
	// which only an empty interface can hold, as a map[string]any.
	// Its values are still decoded by their @type, when they have one.
	if target.NumMethod() == 0 && isNestedObjectWithoutType(trimmed) {
		value, err := unmarshalGenericJSON(trimmed, reg, codec)
		if err != nil {
			return err
		}
		return setDecoded(target, reflect.ValueOf(value))
	}
	envTypeID, inner, ok := splitTypedEnvelope(raw)
	if !ok {
		// Not a @type/@value envelope; fall back to typed.UnmarshalJSON.
		return applyTypedValueWithReg(raw, target, reg, codec)
	}
	rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
	if !registered {
		return applyTypedValueWithReg(raw, target, reg, codec)
	}
	switch rType.Kind() {
	case reflect.Struct:
		return copyTypedStructWithReg(inner, rType, target, reg, codec, TypeID(envTypeID))
	case reflect.Slice, reflect.Array, reflect.Map:
		return copyTypedContainerWithReg(raw, inner, rType, target, reg, codec)
	}
	return applyTypedValueWithReg(raw, target, reg, codec)
}

// copyTypedStruct dispatches a struct polymorphic value through the DTO path.
func copyTypedStruct(inner []byte, rType reflect.Type, target reflect.Value) error {
	return copyTypedStructWithReg(inner, rType, target, nil, nil, "")
}

func copyTypedStructWithReg(inner []byte, rType reflect.Type, target reflect.Value, reg *_TypeRegistry, codec *Codec, envTypeID TypeID) error {
	value := reflect.New(rType)
	// If the registered type has a user-supplied Unmarshal closure, prefer
	// it over the DTO path. The closure expects the inner JSON bytes (with
	// the @type envelope stripped), and is responsible for populating the
	// concrete type however the user wired it (often via a DTO shape that
	// does not match the concrete type's own fields).
	if decode := reg.lookupDecoder(rType, envTypeID); decode != nil {
		if err := decode(codec, inner, value.Interface()); err != nil {
			return err
		}
	} else if err := unmarshalReflectWithReg(inner, value.Elem(), reg, codec); err != nil {
		return err
	}
	concrete := value.Elem()
	if !concrete.Type().Implements(target.Type()) && reflect.PointerTo(concrete.Type()).Implements(target.Type()) {
		pv := reflect.New(concrete.Type())
		pv.Elem().Set(concrete)
		concrete = pv
	}
	return setDecoded(target, concrete)
}

// copyTypedContainerWithReg decodes the registered slice, array or map type rType from its @type envelope in raw,
// whose payload is inner, and sets it to target, which is an interface-typed reflect.Value.
func copyTypedContainerWithReg(raw, inner []byte, rType reflect.Type, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	trimmed := bytes.TrimSpace(inner)
	if len(trimmed) == 0 || bytes.Equal(trimmed, null) {
		// Preserve the typed-nil of the registered type so a nil polymorphic
		// value stays distinguishable from a plain nil interface.
		return setDecoded(target, reflect.Zero(rType))
	}
	if len(inner) == len(raw) {
		// splitTypedEnvelope returns the whole object for a flat envelope,
		// which keeps the @type next to the entries of a map: {"@type":"...","key":...}.
		inner = stripTypeField(inner)
	}
	dest := reflect.New(rType)
	if err := unmarshalReflectWithReg(inner, dest.Elem(), reg, codec); err != nil {
		return err
	}
	concrete := dest.Elem()
	if !concrete.Type().Implements(target.Type()) && reflect.PointerTo(concrete.Type()).Implements(target.Type()) {
		concrete = dest
	}
	return setDecoded(target, concrete)
}

// applyTypedValue delegates to typed.UnmarshalJSON for non-struct or unknown
// polymorphic values.
func applyTypedValue(raw []byte, target reflect.Value) error {
	return applyTypedValueWithReg(raw, target, nil, nil)
}

func applyTypedValueWithReg(raw []byte, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	var t typed
	t.Type = target.Type()
	t.Force = true
	t.reg = reg
	t.codec = codec
	if err := t.UnmarshalJSON(raw); err != nil {
		return err
	}
	return setDecoded(target, reflect.ValueOf(t.Value))
}

// stripTypeField removes the "@type" field from a JSON object so that the
// remaining bytes can be parsed as if it were a plain container. Used when
// splitTypedEnvelope returned the whole raw for a flat envelope and we
// still need to deserialize the inner container. Returns the input bytes
// when stripping is not applicable.
func stripTypeField(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return raw
	}
	// Walk the object and emit a copy with the @type field omitted.
	var buf bytes.Buffer
	buf.WriteByte('{')
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}
	first := true
	for k, v := range probe {
		if strings.HasPrefix(k, "@") {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return raw
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// splitTypedEnvelope parses a JSON object that may carry a "@type" field
// and returns the typeID, the bytes to feed back into unmarshalStruct for
// the registered struct type, and a bool indicating success. Two envelope
// shapes are supported:
//   - flat:     {"@type":"x", "field":..., ...}        (struct fields inlined)
//   - wrapped:  {"@type":"x", "@value": {...}}         (value wrapped)
func splitTypedEnvelope(raw []byte) (typeID string, inner []byte, ok bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", nil, false
	}
	var tc struct {
		Type  string          `json:"@type"`
		Value json.RawMessage `json:"@value"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return "", nil, false
	}
	if tc.Type == "" {
		return "", nil, false
	}
	if len(tc.Value) > 0 {
		return tc.Type, tc.Value, true
	}
	// Flat envelope: the registered struct should consume the whole object.
	return tc.Type, raw, true
}

// containsInterfaceElement reports whether t is (or recursively contains) an
// interface-typed element. Recognizes pointers, slices, arrays, maps, and
// channels; structs are not descended into (they are handled by the nested
// DTO path).
func containsInterfaceElement(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
		return containsInterfaceElement(t.Elem())
	case reflect.Map:
		return containsInterfaceElement(t.Elem())
	}
	return false
}

// requiresCodec reports whether decoding a value of type t needs the Codec, because encoding/json can't decode it on its own.
// A value which doesn't need the Codec is handed to encoding/json, which decodes it the encoding/json way, as it is.
//
// It needs the Codec, when it holds somewhere inside
//   - an interface, which is decoded by its @type,
//   - a type with a custom codec or migration,
//   - or a struct embedded through a pointer to an unexported struct type, with a name in its json tag,
//     as encoding/json panics on decoding into it while the pointer is nil.
func requiresCodec(t reflect.Type) bool {
	if v, ok := requiresCodecCache.Lookup(t); ok {
		return v
	}
	// Not computed under the lock of the cache,
	// as invalidateUnmarshalPlanCache resets the cache while holding the lock of the custom codec markers.
	v := requiresCodecSeen(t, map[reflect.Type]struct{}{})
	requiresCodecCache.Set(t, v)
	return v
}

var requiresCodecCache synckit.Map[reflect.Type, bool]

func requiresCodecSeen(t reflect.Type, seen map[reflect.Type]struct{}) bool {
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	if t.Kind() == reflect.Interface {
		return true
	}
	if hasOwnJSONUnmarshaler(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return requiresCodecSeen(t.Elem(), seen)
	case reflect.Struct:
		if isCustomCodecField(t) {
			return true
		}
		for _, field := range jsonFields(t) {
			if !field.field.IsExported() && field.field.Type.Kind() == reflect.Pointer {
				return true
			}
			if requiresCodecSeen(field.field.Type, seen) {
				return true
			}
		}
	}
	return false
}

// isCustomCodecField reports whether a struct type, or the struct type a pointer points to,
// has a custom codec or a migration registered on any codec in the process.
//
// We use a process-wide marker (not a per-codec registry) because what
// requiresCodec and the unmarshal plans tell about a type is cached once per reflect.Type,
// and reused across codecs.
func isCustomCodecField(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	return typeIDRegistry.LookupCustomCodec(t) != nil || hasCustomCodecMarker(t)
}

// wrapPolymorphic wraps an inner JSON value with a @type envelope when the
// value has a registered type ID. Unregistered polymorphic values produce
// an error so that missing registrations don't silently produce JSON that
// can't be round-tripped through unmarshal.
func wrapPolymorphic(value any, innerData []byte) ([]byte, error) {
	return wrapPolymorphicWithReg(value, innerData, nil)
}

// wrapPolymorphicWithReg wraps innerData with the @type discriminator for
// the given value if it has a registered type ID. Plain JSON container
// kinds (slice/map/chan) and pointers to them are returned unchanged
// even when unregistered: the codec cannot reconstruct their concrete
// type without a discriminator, but the natural Go type for a JSON
// object/array is `map[string]any` / `[]any`, which is good enough for
// the round-trip. For STRUCT and PTR-TO-STRUCT values, registration is
// required because the concrete type info is otherwise lost; in that
// case an error is returned so the caller knows to register the type.
func wrapPolymorphicWithReg(value any, innerData []byte, reg *_TypeRegistry) ([]byte, error) {
	typeID, gotTypeID := lookupTypeIDForValue(reg, value)
	if !gotTypeID {
		rv := reflect.ValueOf(value)
		for rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Slice, reflect.Map, reflect.Chan, reflect.Interface:
			return innerData, nil
		default:
			return nil, fmt.Errorf("missing @type id alias for %T", value)
		}
	}
	trimmed := bytes.TrimSpace(innerData)
	if len(trimmed) == 0 || trimmed[0] == '{' {
		return wrapWithTypeIDValue(typeID, innerData)
	}
	return wrapWithValueEnvelopeType(typeID, innerData)
}

// wrapWithTypeIDValue prepends a `@type` field to an object JSON byte slice.
func wrapWithTypeIDValue(typeID TypeID, innerData []byte) ([]byte, error) {
	typeIDPart, err := json.Marshal(map[string]TypeID{"@type": typeID})
	if err != nil {
		return nil, err
	}
	typeIDPart = bytes.TrimSuffix(typeIDPart, curlyBracketClose)
	innerData = bytes.TrimPrefix(innerData, curlyBracketOpen)
	out := append([]byte{}, typeIDPart...)
	if !bytes.HasPrefix(innerData, curlyBracketClose) {
		out = append(out, fieldSep...)
	}
	out = append(out, innerData...)
	return out, nil
}

// wrapWithValueEnvelopeType wraps a non-object JSON value in a typedContainer
// envelope: {"@type":"...","@value":<inner>}.
func wrapWithValueEnvelopeType(typeID TypeID, innerData []byte) ([]byte, error) {
	tc := typedContainer{Type: typeID, Value: innerData}
	return json.Marshal(tc)
}

func unmarshalStruct(data []byte, dst reflect.Value) error {
	return unmarshalStructWithReg(data, dst, nil, nil)
}

// unmarshalStructWithReg decodes data into the struct dst, which keeps the fields without a member in the JSON,
// like with encoding/json.
func unmarshalStructWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	// If the destination type itself has a custom codec registered on the
	// current codec, invoke it instead of going through the DTO path. The
	// DTO path assumes the wire's field names match the struct's field
	// names, which is exactly what a custom codec exists to avoid: the
	// user wires a DTO shape that does not match the concrete type's
	// fields. Stripping the @type envelope (if present) and handing the
	// inner JSON to the user's Unmarshal closure is what makes the
	// round-trip work.
	//
	// Like encoding/json, which doesn't use the UnmarshalJSON of a value it can't interface,
	// such a value is decoded field by field.
	if dst.CanInterface() && reg.hasDecoder(dst.Type()) {
		raw := data
		var envTypeID TypeID
		if id, stripped, ok := splitTypedEnvelope(raw); ok {
			raw = stripped
			envTypeID = TypeID(id)
		}
		// A type that only has migrations has no decoder for its current @type,
		// so such data falls through to the default decoding below.
		if decode := reg.lookupDecoder(dst.Type(), envTypeID); decode != nil {
			// The closure gets the value dst already holds,
			// like the UnmarshalJSON method of a value, which encoding/json decodes into.
			dest := reflect.New(dst.Type())
			dest.Elem().Set(dst)
			if err := decode(codec, raw, dest.Interface()); err != nil {
				return err
			}
			dst.Set(dest.Elem())
			return nil
		}
	}
	if dst.CanInterface() && !requiresCodec(dst.Type()) {
		return json.Unmarshal(data, dst.Addr().Interface())
	}
	var (
		plan = getUnmarshalPlan(dst.Type())
		dto  = reflect.New(plan.dtoType).Elem()
	)
	for _, fp := range plan.fields {
		if fp.plain {
			dto.Field(fp.dtoIndex).Set(dst.FieldByIndex(fp.targetIdx))
		}
	}
	// Like with encoding/json, an error doesn't undo what is already decoded,
	// and the error of a field doesn't stop the decoding of the other fields.
	// The first error is returned.
	decodeErr := json.Unmarshal(data, dto.Addr().Interface())
	for _, fp := range plan.fields {
		dtoField := dto.Field(fp.dtoIndex)
		if fp.plain {
			dst.FieldByIndex(fp.targetIdx).Set(dtoField)
			continue
		}
		if dtoField.Len() == 0 {
			// The JSON has no member for the field, so the field keeps its value,
			// and the embedded pointers on the way to it are not allocated for it.
			continue
		}
		target, err := fieldByIndexAlloc(dst, fp.targetIdx)
		if err == nil {
			err = fp.decode(dtoField.Bytes(), target, reg, codec)
		}
		if decodeErr == nil {
			decodeErr = err
		}
	}
	return decodeErr
}

func unmarshalReflect(data []byte, dst reflect.Value) error {
	return unmarshalReflectWithReg(data, dst, nil, nil)
}

// isNestedObjectWithoutType reports whether data is a JSON object that
// lacks a top-level @type discriminator. Used by unmarshalReflectWithReg
// to decide between typed.UnmarshalJSON (which expects a @type envelope)
// and the recursive generic unmarshaller (which preserves a nested
// map[string]any shape while still dispatching @type envelopes that
// appear deeper in the structure).
func isNestedObjectWithoutType(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return false
	}
	_, ok := probe["@type"]
	return !ok
}

// unmarshalGenericJSON unmarshals a JSON value into an `any`, recursing
// through nested objects and arrays so that any @type envelopes deeper
// in the structure are still dispatched through the codec's registry.
// This matches what the marshal side produces for nested map[string]any
// values: the outermost layer is a plain JSON object (no @type), but
// values stored inside it might be registered types wrapped in @type
// envelopes. Plain json.Unmarshal into `any` would lose the @type
// dispatch because it produces map[string]any / []any for nested
// objects/arrays without consulting the codec.
func unmarshalGenericJSON(data []byte, reg *_TypeRegistry, codec *Codec) (any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.Equal(trimmed, null) {
		return nil, nil
	}
	if trimmed[0] == '{' {
		// If the object carries a @type discriminator, dispatch through
		// the codec's typed.UnmarshalJSON so the registered concrete
		// type is reconstructed (not a generic map[string]any).
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, err
		}
		if _, hasType := probe["@type"]; hasType {
			var v typed
			v.Type = reflect.TypeOf((*any)(nil)).Elem()
			v.Force = true
			v.reg = reg
			if err := v.UnmarshalJSON(trimmed); err != nil {
				return nil, err
			}
			return v.Value, nil
		}
		result := make(map[string]any, len(probe))
		for k, raw := range probe {
			v, err := unmarshalGenericJSON(raw, reg, codec)
			if err != nil {
				return nil, err
			}
			result[k] = v
		}
		return result, nil
	}
	if trimmed[0] == '[' {
		var rawArray []json.RawMessage
		if err := json.Unmarshal(trimmed, &rawArray); err != nil {
			return nil, err
		}
		result := make([]any, len(rawArray))
		for i, raw := range rawArray {
			v, err := unmarshalGenericJSON(raw, reg, codec)
			if err != nil {
				return nil, err
			}
			result[i] = v
		}
		return result, nil
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// unmarshalReflectWithReg decodes data into dst, which keeps what the JSON doesn't change, like with encoding/json:
// a struct keeps the fields without a member in the JSON, a pointer keeps pointing to the value it points to,
// a map keeps its other entries, and the elements of a slice or an array are decoded in place.
// An interface is the exception, as its value is replaced by the one its @type tells.
func unmarshalReflectWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	switch dst.Kind() {
	case reflect.Interface:
		return copyTypedFromRawWithReg(data, dst, reg, codec)
	case reflect.Pointer:
		if isJSONNull(data) {
			// A pointer which can't be set is an embedded pointer to an unexported struct type,
			// which encoding/json leaves as it is too, unless it is nil, where encoding/json panics.
			if dst.CanSet() {
				dst.SetZero()
			}
			return nil
		}
		if dst.IsNil() {
			if !dst.CanSet() {
				return errCannotSetEmbeddedPointer(dst.Type())
			}
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return unmarshalReflectWithReg(data, dst.Elem(), reg, codec)
	}
	// Like encoding/json, which doesn't use the UnmarshalJSON of a value it can't interface.
	if dst.CanAddr() && dst.CanInterface() && hasOwnJSONUnmarshaler(dst.Type()) {
		return json.Unmarshal(data, dst.Addr().Interface())
	}
	if dst.Kind() == reflect.Struct {
		return unmarshalStructWithReg(data, dst, reg, codec)
	}
	data = stripOwnTypedEnvelope(data, dst.Type(), reg)
	if !requiresCodec(dst.Type()) {
		return json.Unmarshal(data, dst.Addr().Interface())
	}
	switch dst.Kind() {
	case reflect.Slice:
		return unmarshalSliceWithReg(data, dst, reg, codec)
	case reflect.Array:
		return unmarshalArrayWithReg(data, dst, reg, codec)
	case reflect.Map:
		return unmarshalMapWithReg(data, dst, reg, codec)
	}
	return json.Unmarshal(data, dst.Addr().Interface())
}

// stripOwnTypedEnvelope returns the payload of data, when data is the @type envelope of t itself,
// like a registered slice type, which the Codec writes with its @type, when it is the top-level value.
func stripOwnTypedEnvelope(data []byte, t reflect.Type, reg *_TypeRegistry) []byte {
	envTypeID, inner, ok := splitTypedEnvelope(data)
	if !ok {
		return data
	}
	if rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID)); !registered || rType != t {
		return data
	}
	if t.Kind() == reflect.Map && len(inner) == len(data) {
		// splitTypedEnvelope returns the whole object for a flat envelope,
		// which keeps the @type next to the entries: {"@type":"...","key":...}.
		return stripTypeField(inner)
	}
	return inner
}

// unmarshalSliceWithReg decodes a JSON array into the slice dst, like encoding/json does:
// the elements dst already has are decoded into, and dst is grown or cut to the length of the JSON array.
func unmarshalSliceWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	if isJSONNull(data) {
		dst.SetZero()
		return nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(data, &elems); err != nil {
		return err
	}
	if len(elems) == 0 {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
		return nil
	}
	var decodeErr error
	for i, elem := range elems {
		if dst.Cap() <= i {
			dst.Grow(1)
		}
		if dst.Len() <= i {
			dst.SetLen(i + 1)
		}
		if err := unmarshalReflectWithReg(elem, dst.Index(i), reg, codec); decodeErr == nil {
			decodeErr = err
		}
	}
	dst.SetLen(len(elems))
	return decodeErr
}

// unmarshalArrayWithReg decodes a JSON array into the array dst, like encoding/json does:
// the elements of dst are decoded into, the rest of them are zeroed, and the extra JSON elements are skipped.
func unmarshalArrayWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	if isJSONNull(data) {
		return nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(data, &elems); err != nil {
		return err
	}
	var decodeErr error
	for i := 0; i < dst.Len(); i++ {
		if len(elems) <= i {
			dst.Index(i).SetZero()
			continue
		}
		if err := unmarshalReflectWithReg(elems[i], dst.Index(i), reg, codec); decodeErr == nil {
			decodeErr = err
		}
	}
	return decodeErr
}

// unmarshalMapWithReg decodes a JSON object into the map dst, like encoding/json does:
// dst keeps its other entries, and each entry in the JSON gets a newly decoded value.
func unmarshalMapWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	if isJSONNull(data) {
		dst.SetZero()
		return nil
	}
	// The keys are decoded by encoding/json, which knows the kinds of keys a JSON object can have.
	entries := reflect.New(reflect.MapOf(dst.Type().Key(), rawMessageType))
	if err := json.Unmarshal(data, entries.Interface()); err != nil {
		return err
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMapWithSize(dst.Type(), entries.Elem().Len()))
	}
	var decodeErr error
	for iter := entries.Elem().MapRange(); iter.Next(); {
		value := reflect.New(dst.Type().Elem()).Elem()
		if err := unmarshalReflectWithReg(iter.Value().Bytes(), value, reg, codec); decodeErr == nil {
			decodeErr = err
		}
		dst.SetMapIndex(iter.Key(), value)
	}
	return decodeErr
}

func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), null)
}

const errMalformedJSONTag errorkitlite.Error = "malformed JSON struct tag value"

var jsonTag = reflectkit.TagHandler[jsonTagConfig]{
	Name: "json",
	Parse: func(field reflect.StructField, tagName, tagValue string) (jsonTagConfig, error) {
		var c jsonTagConfig
		if tagValue == "-" {
			c.Ignore = true
			return c, nil
		}
		var parts = strings.Split(tagValue, ",")
		if len(parts) == 0 {
			return c, errMalformedJSONTag
		}
		c.Name = parts[0]
		for _, option := range parts[1:] {
			switch option {
			case "omitempty":
				c.Omitempty = true
			case "omitzero":
				c.Omitzero = true
			case "string":
				c.Stringify = true
			default:
				c.Unknown = append(c.Unknown, option)
			}
		}
		return c, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, v jsonTagConfig) error {
		return nil
	},
}

type jsonTagConfig struct {
	// Name is the JSON member name for the tagged field.
	// Per RFC 8259, a JSON object's member is composed of a name and a value,
	// where the name is defined as a string.
	Name string
	// Ignore flags that the field should be ignored form json encoding
	Ignore bool
	// Omitempty instructs the encoder to omit the member when the field
	// has an empty value as defined by encoding/json: false, 0, a nil
	// pointer or interface, or any array, slice, map, or string of length zero.
	Omitempty bool
	// Omitzero instructs the encoder to omit the member when the field
	// is the zero value for its type, or when the type implements
	// "IsZero() bool" and reports true. Added in encoding/json/v2 (Go 1.24+).
	Omitzero bool
	// Stringify instructs the encoder to encode the field's value as a JSON string.
	// This option only applies to fields of string, floating point, integer, or boolean type.
	Stringify bool
	// Unknown holds the options which are not known, and which are ignored, like with encoding/json.
	Unknown []string
}

// jsonField is a member of the JSON object of a struct type.
type jsonField struct {
	// name is the name of the JSON member.
	name string
	// index is the index sequence of the struct field, see reflect.Value#FieldByIndex.
	index []int
	// field is the struct field, which is in an embedded struct when the index sequence has more than one index.
	field reflect.StructField
	tag   jsonTagConfig
	// tagged reports whether the name is from the json tag of the struct field.
	tagged bool
	// throughPointer reports whether the struct field is in a struct embedded through a pointer.
	throughPointer bool
}

var jsonFieldsCache synckit.Map[reflect.Type, []jsonField]

// jsonFields returns the members of the JSON object of the struct type t, in the order of their struct fields.
//
// It follows the rules of encoding/json.
// The fields of an embedded struct are promoted into the object, unless the embedded field has a name in its json tag.
// Of the fields with the same name, the least nested one is used.
// If several of them are equally nested, the only one with its name from its json tag is used,
// and if there is no such single field, then none of them is.
func jsonFields(t reflect.Type) []jsonField {
	return jsonFieldsCache.GetOrInit(t, func() []jsonField {
		return dominantJSONFields(collectJSONFields(t))
	})
}

// collectJSONFields returns all the fields of the struct type t which could be a member of its JSON object.
// It walks the embedded structs breadth first, like encoding/json does.
func collectJSONFields(t reflect.Type) []jsonField {
	type embedded struct {
		typ            reflect.Type
		index          []int
		throughPointer bool
	}
	var (
		fields  []jsonField
		visited = map[reflect.Type]bool{}
		current []embedded
		next    = []embedded{{typ: t}}
		// count and nextCount are how many times a struct type is embedded at the current and the next depth.
		count, nextCount map[reflect.Type]int
	)
	for 0 < len(next) {
		current, next = next, nil
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, e := range current {
			// The fields of a struct type which is already embedded at a lower depth are hidden by those.
			if visited[e.typ] {
				continue
			}
			visited[e.typ] = true
			for i := 0; i < e.typ.NumField(); i++ {
				sf := e.typ.Field(i)
				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				// An embedded struct of an unexported type still has its exported fields promoted.
				if !sf.IsExported() && !(sf.Anonymous && ft.Kind() == reflect.Struct) {
					continue
				}
				tag, _, _ := jsonTag.Lookup(sf)
				if tag.Ignore {
					continue
				}
				name := tag.Name
				if !isValidJSONTagName(name) {
					name = ""
				}
				index := append(slices.Clone(e.index), i)
				if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
					nextCount[ft]++
					if nextCount[ft] == 1 {
						next = append(next, embedded{
							typ:            ft,
							index:          index,
							throughPointer: e.throughPointer || sf.Type.Kind() == reflect.Pointer,
						})
					}
					continue
				}
				field := jsonField{
					name:           cmp.Or(name, sf.Name),
					index:          index,
					field:          sf,
					tag:            tag,
					tagged:         name != "",
					throughPointer: e.throughPointer,
				}
				fields = append(fields, field)
				if 1 < count[e.typ] {
					// The struct type is embedded more than once at this depth,
					// and the duplicate makes its fields conflict with each other.
					fields = append(fields, field)
				}
			}
		}
	}
	return fields
}

// dominantJSONFields keeps only the dominant field of each name, see jsonFields, in the order of their struct fields.
func dominantJSONFields(fields []jsonField) []jsonField {
	// Sorted by name, then by depth, then the ones with their name from their json tag first.
	slices.SortFunc(fields, func(a, b jsonField) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if c := cmp.Compare(len(a.index), len(b.index)); c != 0 {
			return c
		}
		if a.tagged != b.tagged {
			if a.tagged {
				return -1
			}
			return 1
		}
		return slices.Compare(a.index, b.index)
	})
	out := make([]jsonField, 0, len(fields))
	for i, n := 0, 0; i < len(fields); i += n {
		for n = 1; i+n < len(fields) && fields[i+n].name == fields[i].name; n++ {
		}
		if 1 < n && len(fields[i].index) == len(fields[i+1].index) && fields[i].tagged == fields[i+1].tagged {
			continue
		}
		out = append(out, fields[i])
	}
	slices.SortFunc(out, func(a, b jsonField) int {
		return slices.Compare(a.index, b.index)
	})
	return out
}

// isValidJSONTagName reports whether encoding/json uses name from a json tag as the name of a JSON member.
func isValidJSONTagName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

func hasJSONOption(tag, option string) bool {
	var parts = strings.Split(tag, ",")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts[1:] {
		if part == option {
			return true
		}
	}
	return false
}

// isEmptyForOmitEmpty matches the encoding/json omitempty semantics for
// interface and pointer kinds: an interface field is "empty" only when
// it is a nil interface value, not when it holds a typed non-nil value
// (even if the underlying value is itself zero). This matches what
// stdlib json.Marshal does: `var x any = Foo{}` produces `{"x":{}}`,
// not `{}`.
//
// Without this special case, a polymorphic field like
// `Then Definition `json:"then,omitempty"“ would be silently dropped
// whenever the user assigned a typed zero value to it (e.g.
// `If{Then: workflow.Join{}}`), breaking the round-trip.
func isEmptyForOmitEmpty(fieldType reflect.Type, value reflect.Value) bool {
	switch fieldType.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return true
		}
		// A non-nil interface holding a struct/slice/map/etc. is not
		// empty for omitempty purposes. The codec must preserve the
		// assignment so unmarshal can reconstruct the typed value.
		return false
	case reflect.Pointer:
		return value.IsNil()
	}
	return reflectkit.IsEmpty(value)
}
