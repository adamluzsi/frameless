package jsonkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

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
	if v.Kind() == reflect.Slice && v.CanInterface() && v.Type().Implements(jsonMarshalerType) &&
		v.Type().PkgPath() == "go.llib.dev/frameless/pkg/jsonkit" &&
		strings.HasPrefix(v.Type().Name(), "Array[") &&
		v.Type().Elem().Kind() == reflect.Interface {
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
		fields := make([]reflect.StructField, 0, v.NumField())
		values := make([]reflect.Value, 0, v.NumField())
		for field, value := range reflectkit.IterStructFields(v) {
			if field.PkgPath != "" {
				continue
			}
			tagConfig, ok, err := jsonTag.Lookup(field)
			if err == nil && ok {
				if tagConfig.Omitzero && reflectkit.IsZero(value) {
					continue
				}
				if tagConfig.Omitempty && isEmptyForOmitEmpty(field.Type, value) {
					continue
				}
			}
			value, err = marshalPlaceholderWithReg(value, field.Type, reg, codec, false)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("%s: %w", field.Name, err)
			}
			field.Type = value.Type()
			if field.Anonymous && !validAnonymousFieldType(field.Type) {
				field.Anonymous = false
			}
			fields = append(fields, field)
			values = append(values, value)
		}
		shadowType := reflect.StructOf(fields)
		shadow := reflect.New(shadowType).Elem()
		for i, value := range values {
			shadow.Field(i).Set(value)
		}
		return shadow, nil

	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(reflect.SliceOf(rawMessageType)), nil
		}
		return marshalSequencePlaceholderWithReg(v, false, reg)

	case reflect.Array:
		return marshalSequencePlaceholderWithReg(v, true, reg)

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

func marshalSequencePlaceholder(v reflect.Value, array bool) (reflect.Value, error) {
	return marshalSequencePlaceholderWithReg(v, array, nil)
}

func marshalSequencePlaceholderWithReg(v reflect.Value, array bool, reg *_TypeRegistry) (reflect.Value, error) {
	values := make([]reflect.Value, v.Len())
	var elemType reflect.Type
	for i := 0; i < v.Len(); i++ {
		value, err := marshalPlaceholderWithReg(v.Index(i), v.Type().Elem(), reg, nil, false)
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

func validAnonymousFieldType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name() != ""
}

// unmarshalPlan describes how to unmarshal JSON into a target struct value
// using a generated DTO and stdlib json.Unmarshal.
type unmarshalPlan struct {
	dtoType reflect.Type
	copies  []fieldCopy
}

type fieldCopy struct {
	dtoIndex  int
	targetIdx []int
	copy      func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error
}

// unmarshalPlanCache memoizes unmarshalPlan per struct reflect.Type.
// We use a plain map guarded by a mutex; the cache is read-only after
// population so building plans (which may recurse via dtoFieldFor) does
// not need to re-acquire the cache lock.
var unmarshalPlanCache synckit.Map[reflect.Type, *unmarshalPlan]

// getUnmarshalPlan returns a cached or freshly built unmarshalPlan for rt.
func getUnmarshalPlan(rt reflect.Type) (*unmarshalPlan, error) {
	if plan := unmarshalPlanCache.Get(rt); plan != nil {
		return plan, nil
	}
	plan, err := buildUnmarshalPlan(rt)
	if err != nil {
		return nil, err
	}
	unmarshalPlanCache.Set(rt, plan)
	return plan, nil
}

// invalidateUnmarshalPlanCache removes any cached unmarshal plan whose
// struct type might transitively contain a now-custom-codec-registered
// type as a field. Without this, a plan cached before CodecRegister was
// called would silently route the field through the default decode path
// and produce zero values for any concrete-type field whose wire was
// encoded with a custom Marshal closure.
//
// We invalidate the whole cache rather than tracking reverse
// dependencies. The cache is built lazily, plans are cheap to rebuild,
// and CodecRegister is typically called once during setup. The cost is
// that subsequent unmarshals rebuild plans they would otherwise reuse,
// which is negligible for typical payloads.
func invalidateUnmarshalPlanCache() {
	unmarshalPlanCache.Do(func(entries map[reflect.Type]*unmarshalPlan) error {
		for k := range entries {
			delete(entries, k)
		}
		return nil
	})
}

// buildUnmarshalPlan constructs the DTO type and per-field copy closures
// that copy values from a populated DTO into the target struct.
func buildUnmarshalPlan(rt reflect.Type) (*unmarshalPlan, error) {
	dtoFields := make([]reflect.StructField, 0, rt.NumField())
	copies := make([]fieldCopy, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag, _, _ := jsonTag.Lookup(field)
		if tag.Ignore {
			continue
		}
		dtoFieldType, copyFn, ok := dtoFieldFor(field, tag)
		if !ok {
			continue
		}
		name := field.Name
		if tag.Name != "" {
			name = tag.Name
		}
		dtoTag := field.Tag
		if tag.Stringify {
			dtoTag = stripStringifyTag(field.Tag)
		}
		dtoFields = append(dtoFields, reflect.StructField{
			Name:      dtoFieldName(name),
			Type:      dtoFieldType,
			Tag:       reflect.StructTag(dtoTag),
			Anonymous: field.Anonymous && validAnonymousFieldType(dtoFieldType),
		})
		copies = append(copies, fieldCopy{
			dtoIndex:  len(dtoFields) - 1,
			targetIdx: field.Index,
			copy:      copyFn,
		})
	}
	dtoType := reflect.StructOf(dtoFields)
	return &unmarshalPlan{dtoType: dtoType, copies: copies}, nil
}

// dtoFieldName returns a struct field name suitable for reflect.StructOf.
// reflect.StructOf requires that any lowercase (unexported) field name carry
// a non-empty PkgPath, which would complicate reflection; we simply ensure
// DTO field names are exported. The actual JSON matching uses the field tag.
func dtoFieldName(name string) string {
	if name == "" {
		return "Field"
	}
	if r := []rune(name)[0]; r >= 'a' && r <= 'z' {
		return string(r-'a'+'A') + name[1:]
	}
	return name
}

// stripStringifyTag removes the ",string" option from a struct tag's value.
func stripStringifyTag(tag reflect.StructTag) reflect.StructTag {
	value, hasName := tag.Lookup("json")
	if !hasName {
		return tag
	}
	parts := strings.Split(value, ",")
	if len(parts) < 2 {
		return tag
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "string" {
			continue
		}
		out = append(out, p)
	}
	newValue := strings.Join(out, ",")
	if newValue == value {
		return tag
	}
	return reflect.StructTag(`json:"` + newValue + `"`)
}

// copyTypedFromRaw unmarshals a polymorphic JSON value into target, which
// must be an interface-typed reflect.Value. It uses typed.UnmarshalJSON
// with the correct target interface type so that the @type/@value envelope
// is dispatched correctly. For struct implementations that have interface
// fields (or other nested polymorphic data), it routes through the DTO
// path so stdlib isn't asked to unmarshal directly into the original
// struct type. Slice/map/array implementations that contain interface
// elements are dispatched element-by-element so each element gets its own
// typed dispatch.
func copyTypedFromRaw(raw []byte, target reflect.Value) error {
	return copyTypedFromRawWithReg(raw, target, nil, nil)
}

func copyTypedFromRawWithReg(raw []byte, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, null) {
		target.SetZero()
		return nil
	}
	envTypeID, inner, ok := splitTypedEnvelope(raw)
	if !ok {
		// Not a @type/@value envelope; fall back to typed.UnmarshalJSON.
		return applyTypedValueWithReg(raw, target, reg)
	}
	rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
	if !registered {
		return applyTypedValueWithReg(raw, target, reg)
	}
	switch rType.Kind() {
	case reflect.Struct:
		return copyTypedStructWithReg(inner, rType, target, reg, codec)
	case reflect.Slice, reflect.Array, reflect.Map:
		return copyTypedContainerWithReg(inner, rType, target, reg, codec)
	}
	return applyTypedValueWithReg(raw, target, reg)
}

// copyTypedStruct dispatches a struct polymorphic value through the DTO path.
func copyTypedStruct(inner []byte, rType reflect.Type, target reflect.Value) error {
	return copyTypedStructWithReg(inner, rType, target, nil, nil)
}

func copyTypedStructWithReg(inner []byte, rType reflect.Type, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	value := reflect.New(rType)
	// If the registered type has a user-supplied Unmarshal closure, prefer
	// it over the DTO path. The closure expects the inner JSON bytes (with
	// the @type envelope stripped), and is responsible for populating the
	// concrete type however the user wired it (often via a DTO shape that
	// does not match the concrete type's own fields).
	if entry := reg.LookupCustomCodec(rType); entry != nil {
		if err := entry.Unmarshal(codec, inner, value.Interface()); err != nil {
			return err
		}
	} else if err := unmarshalStructWithReg(inner, value.Elem(), reg, codec); err != nil {
		return err
	}
	concrete := value.Elem()
	if !concrete.Type().Implements(target.Type()) && reflect.PointerTo(concrete.Type()).Implements(target.Type()) {
		pv := reflect.New(concrete.Type())
		pv.Elem().Set(concrete)
		concrete = pv
	}
	target.Set(concrete)
	return nil
}

// copyTypedContainer dispatches a slice/array/map polymorphic value element
// by element. Each element of the JSON array/object is itself a polymorphic
// value and is routed through copyTypedFromRaw so it gets typed dispatch.
func copyTypedContainer(inner []byte, rType reflect.Type, target reflect.Value) error {
	return copyTypedContainerWithReg(inner, rType, target, nil, nil)
}

func copyTypedContainerWithReg(inner []byte, rType reflect.Type, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	trimmed := bytes.TrimSpace(inner)
	if len(trimmed) == 0 || bytes.Equal(trimmed, null) {
		// Preserve the typed-nil of the registered type so a nil polymorphic
		// value stays distinguishable from a plain nil interface.
		target.Set(reflect.Zero(rType))
		return nil
	}
	// Strip the leading "@type" field if the inner still carries it
	// (splitTypedEnvelope returns the whole raw for flat envelopes).
	stripped := stripTypeField(inner)
	// Build a destination of the registered type. For maps, use the registered
	// map type's key/value; for slices/arrays, the registered element type.
	dest := reflect.New(rType)
	if err := decodeContainerElementsWithReg(stripped, dest, reg, codec); err != nil {
		return err
	}
	concrete := dest.Elem()
	if !concrete.Type().Implements(target.Type()) && reflect.PointerTo(concrete.Type()).Implements(target.Type()) {
		pv := reflect.New(concrete.Type())
		pv.Elem().Set(concrete)
		concrete = pv
	}
	target.Set(concrete)
	return nil
}

// decodeContainerElements populates the registered container type's elements
// from a JSON array (for slices/arrays) or object (for maps), routing each
// element through copyTypedFromRaw.
func decodeContainerElements(jsonBytes []byte, dest reflect.Value) error {
	return decodeContainerElementsWithReg(jsonBytes, dest, nil, nil)
}

func decodeContainerElementsWithReg(jsonBytes []byte, dest reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	switch dest.Kind() {
	case reflect.Pointer:
		return decodeContainerElementsWithReg(jsonBytes, dest.Elem(), reg, codec)
	case reflect.Slice:
		var raws []json.RawMessage
		if err := json.Unmarshal(jsonBytes, &raws); err != nil {
			return err
		}
		out := reflect.MakeSlice(dest.Type(), len(raws), len(raws))
		elemType := dest.Type().Elem()
		for i, raw := range raws {
			elem := reflect.New(elemType).Elem()
			if err := copyTypedFromRawWithReg(raw, elem, reg, codec); err != nil {
				return err
			}
			out.Index(i).Set(elem)
		}
		dest.Set(out)
		return nil
	case reflect.Array:
		var raws []json.RawMessage
		if err := json.Unmarshal(jsonBytes, &raws); err != nil {
			return err
		}
		if len(raws) != dest.Len() {
			return fmt.Errorf("cannot unmarshal array of length %d into %s", len(raws), dest.Type())
		}
		elemType := dest.Type().Elem()
		for i, raw := range raws {
			elem := reflect.New(elemType).Elem()
			if err := copyTypedFromRawWithReg(raw, elem, reg, codec); err != nil {
				return err
			}
			dest.Index(i).Set(elem)
		}
		return nil
	case reflect.Map:
		if dest.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("non-string-keyed maps not supported for typed dispatch: %s", dest.Type())
		}
		var raws map[string]json.RawMessage
		if err := json.Unmarshal(jsonBytes, &raws); err != nil {
			return err
		}
		out := reflect.MakeMapWithSize(dest.Type(), len(raws))
		elemType := dest.Type().Elem()
		for k, raw := range raws {
			elem := reflect.New(elemType).Elem()
			if err := copyTypedFromRawWithReg(raw, elem, reg, codec); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k), elem)
		}
		dest.Set(out)
		return nil
	}
	return fmt.Errorf("unsupported container kind: %s", dest.Kind())
}

// applyTypedValue delegates to typed.UnmarshalJSON for non-struct or unknown
// polymorphic values.
func applyTypedValue(raw []byte, target reflect.Value) error {
	return applyTypedValueWithReg(raw, target, nil)
}

func applyTypedValueWithReg(raw []byte, target reflect.Value, reg *_TypeRegistry) error {
	var t typed
	t.Type = target.Type()
	t.Force = true
	t.reg = reg
	if err := t.UnmarshalJSON(raw); err != nil {
		return err
	}
	if t.Value == nil {
		target.SetZero()
		return nil
	}
	target.Set(reflect.ValueOf(t.Value))
	return nil
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

// dtoTypeFor returns the corresponding DTO type for a target container type
// whose element chain contains an interface. Each container level is
// replaced by its RawMessage counterpart.
func dtoTypeFor(t reflect.Type) reflect.Type {
	switch t.Kind() {
	case reflect.Interface:
		return rawMessageType
	case reflect.Pointer:
		return reflect.PointerTo(dtoTypeFor(t.Elem()))
	case reflect.Slice:
		return reflect.SliceOf(dtoTypeFor(t.Elem()))
	case reflect.Array:
		return reflect.ArrayOf(t.Len(), dtoTypeFor(t.Elem()))
	case reflect.Map:
		return reflect.MapOf(t.Key(), dtoTypeFor(t.Elem()))
	case reflect.Chan:
		return reflect.ChanOf(t.ChanDir(), dtoTypeFor(t.Elem()))
	}
	return t
}

// copyContainer copies a DTO container value (whose leaves are json.RawMessage)
// back into the target container, recursively decoding each leaf with
// copyTypedFromRaw.
func copyContainer(dto, target reflect.Value) error {
	return copyContainerWithReg(dto, target, nil, nil)
}

func copyContainerWithReg(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	if dto.IsValid() && dto.Kind() == reflect.Interface && dto.IsNil() {
		target.SetZero()
		return nil
	}
	switch target.Kind() {
	case reflect.Pointer:
		if dto.IsNil() {
			target.SetZero()
			return nil
		}
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return copyContainerWithReg(dto.Elem(), target.Elem(), reg, codec)
	case reflect.Slice:
		if dto.IsNil() {
			target.SetZero()
			return nil
		}
		out := reflect.MakeSlice(target.Type(), dto.Len(), dto.Len())
		for i := 0; i < dto.Len(); i++ {
			elem := reflect.New(target.Type().Elem()).Elem()
			if err := copyContainerWithReg(dto.Index(i), elem, reg, codec); err != nil {
				return err
			}
			out.Index(i).Set(elem)
		}
		target.Set(out)
		return nil
	case reflect.Array:
		for i := 0; i < target.Len(); i++ {
			elem := reflect.New(target.Type().Elem()).Elem()
			if err := copyContainerWithReg(dto.Index(i), elem, reg, codec); err != nil {
				return err
			}
			target.Index(i).Set(elem)
		}
		return nil
	case reflect.Map:
		if dto.IsNil() {
			target.SetZero()
			return nil
		}
		out := reflect.MakeMapWithSize(target.Type(), dto.Len())
		iter := dto.MapRange()
		for iter.Next() {
			elem := reflect.New(target.Type().Elem()).Elem()
			if err := copyContainerWithReg(iter.Value(), elem, reg, codec); err != nil {
				return err
			}
			out.SetMapIndex(iter.Key(), elem)
		}
		target.Set(out)
		return nil
	case reflect.Interface:
		return copyTypedFromRawWithReg(dto.Bytes(), target, reg, codec)
	}
	return fmt.Errorf("copyContainer: unsupported kind %s", target.Kind())
}

// dtoFieldFor returns the DTO field type and copy-back closure for a target field.
// The bool result reports whether the field should be included in the DTO.
func dtoFieldFor(field reflect.StructField, tag jsonTagConfig) (reflect.Type, func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error, bool) {
	if field.Type.Kind() == reflect.Interface {
		copyFn := func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
			return copyTypedFromRawWithReg(dto.Bytes(), target, reg, codec)
		}
		return rawMessageType, copyFn, true
	}
	if containsInterfaceElement(field.Type) {
		dtoType := dtoTypeFor(field.Type)
		copyFn := func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
			return copyContainerWithReg(dto, target, reg, codec)
		}
		return dtoType, copyFn, true
	}
	// A concrete struct field whose type was registered via CodecRegister
	// needs to be decoded through the user's custom Unmarshal closure,
	// not through the default reflect-based copy. The default path
	// would do `target.Set(dto)` where `dto` is a freshly json.Unmarshal-ed
	// value of the concrete type, but the wire format the user encoded
	// with the custom Marshal usually does NOT match the concrete type's
	// own field names (a typical CodecRegister use case is wiring a DTO
	// shape that differs from the entity shape).
	//
	// The wire at this point includes the @type envelope added by the
	// marshal side. Strip the envelope via splitTypedEnvelope and hand
	// the inner JSON to the user's Unmarshal closure. If no envelope is
	// present (e.g. a top-level marshal where allowEnvelopeWrap was true
	// but the @type alias was set to the empty string), hand the raw
	// bytes unchanged.
	if isCustomCodecField(field.Type) {
		copyFn := func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
			raw := dto.Bytes()
			inner := raw
			if _, stripped, ok := splitTypedEnvelope(raw); ok {
				inner = stripped
			}
			entry := reg.LookupCustomCodec(target.Type())
			if entry == nil {
				entry = typeIDRegistry.LookupCustomCodec(target.Type())
			}
			if entry == nil {
				return fmt.Errorf("missing custom codec entry for %s during unmarshal", target.Type())
			}
			// Allocate a fresh pointer of the field's concrete type so the
			// user's Unmarshal closure writes into it. If the field type is
			// a pointer (e.g. *Foo), reflect.New yields the pointer the user
			// expects.
			dest := reflect.New(target.Type())
			if err := entry.Unmarshal(codec, inner, dest.Interface()); err != nil {
				return err
			}
			target.Set(dest.Elem())
			return nil
		}
		return rawMessageType, copyFn, true
	}
	if field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct {
		// Use the nested struct's DTO type so json.Unmarshal can populate it
		// using the same rules (interface fields as RawMessage, etc.).
		nestedPlan, err := getUnmarshalPlan(field.Type.Elem())
		if err != nil {
			return nil, nil, false
		}
		dtoPtrType := reflect.PointerTo(nestedPlan.dtoType)
		copyFn := func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
			if dto.IsNil() {
				target.SetZero()
				return nil
			}
			ptr := reflect.New(target.Type().Elem())
			dtoElem := dto.Elem()
			for _, fc := range nestedPlan.copies {
				if err := fc.copy(dtoElem.Field(fc.dtoIndex), ptr.Elem().FieldByIndex(fc.targetIdx), reg, codec); err != nil {
					return err
				}
			}
			target.Set(ptr)
			return nil
		}
		return dtoPtrType, copyFn, true
	}
	if tag.Stringify && isStringifyKind(field.Type.Kind()) {
		copyFn := func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
			s := dto.String()
			return setStringifiedPrimitive(target, s)
		}
		return reflect.TypeOf(""), copyFn, true
	}
	return field.Type, func(dto, target reflect.Value, reg *_TypeRegistry, codec *Codec) error {
		target.Set(dto)
		return nil
	}, true
}

// isCustomCodecField reports whether a struct field's type was ever
// registered through CodecRegister on any codec in the process. The check is performed at
// plan-build time using the package-wide marker set populated by CodecRegister,
// so the plan can route the field through the custom Unmarshal closure rather
// than the default reflect-based copy.
//
// We use a process-wide marker (not a per-codec registry) because the unmarshal
// plan is built once per reflect.Type and cached for reuse across codecs. If
// a type was CodecRegister'd on any codec, every plan for that type must
// produce a DTO shape compatible with custom Unmarshal (i.e. RawMessage for
// the field), so the runtime copyFn can decide per-codec whether to invoke
// the custom closure or fall back to a clear error.
func isCustomCodecField(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	return typeIDRegistry.LookupCustomCodec(t) != nil || hasCustomCodecMarker(t)
}

func isStringifyKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func setStringifiedPrimitive(dst reflect.Value, s string) error {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(s)
		return nil
	case reflect.Bool:
		v, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		dst.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(s, 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetInt(v)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(s, 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetUint(v)
		return nil
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(s, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetFloat(v)
		return nil
	}
	return fmt.Errorf("unsupported stringify kind: %s", dst.Kind())
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

func unmarshalStructWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	// If the destination type itself has a custom codec registered on the
	// current codec, invoke it instead of going through the DTO path. The
	// DTO path assumes the wire's field names match the struct's field
	// names, which is exactly what a custom codec exists to avoid: the
	// user wires a DTO shape that does not match the concrete type's
	// fields. Stripping the @type envelope (if present) and handing the
	// inner JSON to the user's Unmarshal closure is what makes the
	// round-trip work.
	if entry := reg.LookupCustomCodec(dst.Type()); entry != nil {
		raw := data
		if _, stripped, ok := splitTypedEnvelope(raw); ok {
			raw = stripped
		}
		dest := reflect.New(dst.Type())
		if err := entry.Unmarshal(codec, raw, dest.Interface()); err != nil {
			return err
		}
		dst.Set(dest.Elem())
		return nil
	}
	plan, err := getUnmarshalPlan(dst.Type())
	if err != nil {
		return err
	}
	dto := reflect.New(plan.dtoType)
	if err := json.Unmarshal(data, dto.Interface()); err != nil {
		return err
	}
	dtoElem := dto.Elem()
	for _, fc := range plan.copies {
		if err := fc.copy(dtoElem.Field(fc.dtoIndex), dst.FieldByIndex(fc.targetIdx), reg, codec); err != nil {
			return err
		}
	}
	return nil
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

func unmarshalReflectWithReg(data []byte, dst reflect.Value, reg *_TypeRegistry, codec *Codec) error {
	if dst.Kind() == reflect.Interface {
		// If the JSON is a nested object without a @type discriminator,
		// the value is not a polymorphic registered type but a generic
		// JSON object (e.g. a nested map produced by a parent map's
		// iteration). Build the result recursively so that deeper
		// @type envelopes are still dispatched correctly: each level
		// either gets @type-dispatched to the registered concrete type,
		// or stays as a map[string]any / []any / primitive when no
		// @type is present.
		if isNestedObjectWithoutType(data) {
			value, err := unmarshalGenericJSON(data, reg, codec)
			if err != nil {
				return err
			}
			if value == nil {
				dst.SetZero()
			} else {
				dst.Set(reflect.ValueOf(value))
			}
			return nil
		}
		var value typed
		value.Type = dst.Type()
		value.Force = true
		value.reg = reg
		value.codec = codec
		if err := value.UnmarshalJSON(data); err != nil {
			return err
		}
		if value.Value == nil {
			dst.SetZero()
		} else {
			dst.Set(reflect.ValueOf(value.Value))
		}
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(data), null) {
			dst.SetZero()
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return unmarshalReflectWithReg(data, dst.Elem(), reg, codec)
	}

	switch dst.Kind() {
	case reflect.Struct:
		return unmarshalStructWithReg(data, dst, reg, codec)
	case reflect.Slice:
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			dst.SetZero()
			return nil
		}
		// The wire for a registered slice type may carry a typed envelope
		// ({"@type":"...","@value":[…]}) rather than a bare array, e.g.
		// when an empty workflow.Sequence round-trips into its concrete
		// type. Strip the envelope so the slice unmarshaller sees a bare
		// array payload.
		if envTypeID, inner, ok := splitTypedEnvelope(data); ok && len(envTypeID) > 0 {
			rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
			if registered && rType == dst.Type() {
				data = inner
			}
		}
		var array []json.RawMessage
		if err := json.Unmarshal(data, &array); err != nil {
			return err
		}
		value := reflect.MakeSlice(dst.Type(), len(array), len(array))
		for i, raw := range array {
			if err := unmarshalReflectWithReg(raw, value.Index(i), reg, codec); err != nil {
				return err
			}
		}
		dst.Set(value)
		return nil
	case reflect.Array:
		if envTypeID, inner, ok := splitTypedEnvelope(data); ok && len(envTypeID) > 0 {
			rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
			if registered && rType == dst.Type() {
				data = inner
			}
		}
		var array []json.RawMessage
		if err := json.Unmarshal(data, &array); err != nil {
			return err
		}
		if len(array) != dst.Len() {
			return fmt.Errorf("cannot unmarshal array of length %d into %s", len(array), dst.Type())
		}
		for i, raw := range array {
			if err := unmarshalReflectWithReg(raw, dst.Index(i), reg, codec); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			dst.SetZero()
			return nil
		}
		if envTypeID, inner, ok := splitTypedEnvelope(data); ok && len(envTypeID) > 0 {
			rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
			if registered && rType == dst.Type() {
				data = inner
			}
		}
		if dst.Type().Key().Kind() != reflect.String {
			return json.Unmarshal(data, dst.Addr().Interface())
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		value := reflect.MakeMapWithSize(dst.Type(), len(object))
		for key, raw := range object {
			elem := reflect.New(dst.Type().Elem()).Elem()
			if err := unmarshalReflectWithReg(raw, elem, reg, codec); err != nil {
				return err
			}
			value.SetMapIndex(reflect.ValueOf(key).Convert(dst.Type().Key()), elem)
		}
		dst.Set(value)
		return nil
	default:
		// Concrete named primitive kinds (e.g. wftemplate.Condition
		// which is `type Condition string`) are registered globally so
		// their marshalled wire is a typed envelope. Strip the envelope
		// before delegating to stdlib json.Unmarshal so the named
		// primitive is populated from the inner @value (or, for a flat
		// envelope, from the same object minus @type).
		if envTypeID, inner, ok := splitTypedEnvelope(data); ok && len(envTypeID) > 0 {
			rType, registered := lookupTypeByIDForCodec(reg, TypeID(envTypeID))
			if registered && rType == dst.Type() {
				data = inner
			}
		}
		return json.Unmarshal(data, dst.Addr().Interface())
	}
}

const errMalformedJSONTag errorkitlite.Error = "malformed JSON struct tag value"

var jsonTag = reflectkit.TagHandler[jsonTagConfig]{
	Name: "json",
	Parse: func(field reflect.StructField, tagName, tagValue string) (jsonTagConfig, error) {
		var c jsonTagConfig
		var parts = strings.Split(tagValue, ",")
		if len(parts) == 0 {
			return c, errMalformedJSONTag
		}
		c.Name = parts[0]
		if c.Name == "-" {
			c.Name = ""
			c.Ignore = true
		}
		for _, option := range parts[1:] {
			switch option {
			case "omitempty":
				c.Omitempty = true
			case "omitzero":
				c.Omitzero = true
			case "string":
				c.Stringify = true
			default:
				return c, fmt.Errorf("%w: %q", errMalformedJSONTag, option)
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
	// Unknown
	Unknown []string
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
