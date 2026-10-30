package jsonkit

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"reflect"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/port/codec"
)

type Codec struct {
	// TypeKeyword names the member of the type envelope which holds the type ID of a value, see Marshal.
	//
	// Default: "$type"
	TypeKeyword string
	// ValueKeyword names the member of the type envelope which holds a value which is not written next to the type member,
	// like a slice or a string, see Marshal.
	//
	// Default: "$value"
	ValueKeyword string
	// MapKeyword names the member of the type envelope which holds the entries of a map, see Marshal.
	// The keys of its entries are just data, so they can be named like anything, like a "$type".
	//
	// Default: "$map"
	MapKeyword string

	typeIDRegistry *_TypeRegistry
	// call is the Marshal call in progress, when the Codec is the one a custom codec received,
	// so the values it writes with it are part of that call, see marshalCall.
	call *marshalCall
}

// CodecRegisterTypeID registers T with the type ID id on the Codec,
// so a value of T held by an interface is written with id as its $type, and read back as T.
// The aliases are further type IDs which are read back as T, like the former type IDs of T.
//
// When T is registered with id already, like with CodecRegister, the call links the aliases to id,
// so the data written under a former type ID of a type with a custom codec is read back through its custom codec too:
//
//	jsonkit.CodecRegister[Invoice](&c, "app:invoice", InvoiceCodec{})
//	jsonkit.CodecRegisterTypeID[Invoice](&c, "app:invoice", "invoice")
//
// It panics when T is registered with another type ID, or when an alias is in use already.
//
// T can be an interface type too, which names the interface as the element type of a slice or a map,
// so a []T or a map[string]T held by an interface is read back as the very same type, see Codec.Marshal.
// It names no value, as no value is of an interface type.
//
// The returned function removes this registration, and only this one.
// Every registration, made with CodecRegisterTypeID, CodecRegister or CodecRegisterMigration,
// stays until the function it returned is called, whichever order the registrations are made or removed in.
// So the type ID of T stays registered as long as one of the registrations holding it is not removed,
// and removing a link drops its aliases, while the registration they were linked to stays.
// To remove a registration, call its function, for example with defer or t.Cleanup:
//
//	defer jsonkit.CodecRegisterTypeID[Invoice](&c, "app:invoice", "invoice")()
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
// so $type envelopes continue to be emitted and the value can be reconstructed
// during unmarshal via the same type ID lookup.
//
// T can be of any kind, not only a struct.
// Like with encoding/json/v2, where the marshalers given as options take precedence over the type's own methods,
// the closures win over the own methods of T, which encoding/json calls, like its json.Marshaler and json.Unmarshaler,
// wherever the value sits, see Codec.Marshal and Codec.Unmarshal.
// Codecs without the registration keep using T's own methods.
//
// Like a json.Marshaler, the Marshal closure writes a single JSON value, and writes null when there is no value.
// Anything else, including writing nothing, makes the Codec's Marshal fail.
//
// The Codec writes the $type next to the members of the JSON object the Marshal closure returns,
// or as {"$type":"...","$value":...} around any other JSON value.
// Either way, the Unmarshal closure gets back what the Marshal closure wrote, without the $type.
// So what the Marshal closure writes can't have a $type member of its own, see Codec.Marshal.
//
// The closure's T can be either the value type or a pointer to it. The codec
// takes care of the matching between an incoming value's runtime kind and the
// closure's expected T so a registration like CodecRegister[*T] still accepts
// value-typed T (we allocate a fresh *T and copy) and a registration like
// CodecRegister[T] still accepts *T (we dereference it).
//
// Registering, like with CodecRegisterTypeID and CodecRegisterMigration, is part of setting up the Codec,
// so it is done before the Codec is used, as it is not safe to do while the same Codec is in use.
//
// The Codec the closures receive belongs to the Marshal or Unmarshal call in progress,
// so it is meant to be used within the closure's call, from the same goroutine.
// A value written with it is part of the value being written,
// so a value which references back to it is reported as a cycle, like with encoding/json, instead of being written forever.
//
// The returned function removes this registration, and only this one, see CodecRegisterTypeID.
// When T is registered with CodecRegister more than once, the one registered last, of those not removed yet, is in effect.
//
// The former type IDs of T are linked to id with CodecRegisterTypeID.
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
	marshal := func(c *Codec, v any) ([]byte, error) {
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
	}
	entry := &customCodecEntry{
		TypeID: id,
		Marshal: func(c *Codec, v any) ([]byte, error) {
			data, err := marshal(c, v)
			if err != nil {
				return nil, err
			}
			return compactCustomCodecOutput(baseT, data)
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

// compactCustomCodecOutput validates and compacts the JSON a custom codec of the type t wrote,
// like encoding/json does with the JSON of a json.Marshaler,
// so the Codec can add the $type to it as it is.
// Like with a json.Marshaler, it has to be a single JSON value, so nothing, or only whitespace, is an error.
// It is compacted in a copy, as the custom codec may keep what it wrote, like a JSON value it writes each time.
func compactCustomCodecOutput(t reflect.Type, data []byte) ([]byte, error) {
	out := jsontext.Value(data).Clone()
	if err := out.Compact(); err != nil {
		return nil, fmt.Errorf("jsonkit: the custom codec of %s wrote invalid JSON: %w", t, err)
	}
	return out, nil
}

// CodecRegisterMigration migrates data stored under a retired $type into T when it is read.
// Values tagged with fromID are decoded by migrate, which receives the payload of the $type envelope:
// the members of a {"$type":"...",...} JSON object without the $type, or the $value of a {"$type":"...","$value":...} one,
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

// Marshal encodes v into JSON, like json.Marshal does,
// except that a value held by an interface, like an `any` field, is written with its $type,
// so Unmarshal can decode it back into the same type.
//
// A value whose type is known from where it sits, like a struct field or a slice element of a concrete type,
// is written without a $type, even if its type is registered, the same as with encoding/json.
// This goes for v itself too, as the caller reads it back into the type it knows.
// To have v written with its $type, say so explicitly by holding it in a jsonkit.Interface, like Interface[any]{V: v},
// the way an interface field would hold it.
// A pointer to an interface which holds v, like a *any, writes the very same JSON,
// the same way as Unmarshal is given a pointer to an interface, to read a value back by its $type.
//
// A value which plain JSON is read back as, into an `any`, needs no $type, so like with encoding/json,
// a nil, bool, int, float64 or string held by an interface is written as plain JSON, and so is a []any or a map[string]any of them.
// A float64 held by an interface is written with a fraction when it is a whole number, like 42.0 instead of 42,
// so it is read back as a float64, and not as an int, see Unmarshal.
// The other built-in types, like an int64 or a uint8, keep their $type,
// as their plain JSON would be read back as an int or a float64.
// A built-in type registered on the Codec itself is written with the $type of the registration.
//
// A slice or a map with string keys, held by an interface, whose type is not registered,
// has a $type derived from the type IDs of its elements, the way Go spells its type,
// like "[]item" for a []Item, or "map[string][]int" for a map[string][]int,
// and it is written as {"$type":"[]item","$value":[...]}, or {"$type":"map[string][]int","$map":{...}}, so it is read back as the very same type.
// The element type is either registered, including an interface type, like for a []Greeter, or such a slice or map itself.
// A slice or a map of the empty interface needs none, so a []any or a map[string]any is written as plain JSON,
// and so is a slice or a map of an element type which has no type ID, like an unregistered struct or a pointer.
// An array, whose type is not registered, is written as plain JSON too, like with encoding/json, and read back as a []any,
// as its type ID would hold its length, which would let any JSON make Unmarshal build a new type, and allocate an array of any size.
//
// The $type of a value is written next to the members of its JSON object, like with a struct: {"$type":"item","Name":"a"},
// or as {"$type":"...","$value":...} around any other JSON value.
// The entries of a map are written as the $map member of the envelope, like {"$type":"counts","$map":{"a":1}},
// where their keys are just data, so they may be named like anything, like a "$type".
// A map whose custom codec or own marshaler writes another JSON value, like an array, is written as the $value, like any other value.
//
// Like with encoding/json, a value with its own json.Marshaler, or json.MarshalerTo of encoding/json/v2, is written with it,
// and a value with its own encoding.TextMarshaler or encoding.TextAppender, but neither of those,
// is written as the JSON string of its text.
// A method with a pointer receiver is used only for an addressable value,
// like a field of a struct reached through a pointer, or an element of a slice,
// while a value which isn't addressable, like v itself, a map value or a value held by an interface, is written with reflection.
// A custom codec registered with CodecRegister wins over both.
// What it writes is written as it is, and like with any other value, it gets its $type only where an interface holds the value.
//
// Like with encoding/json, a value which references back to itself, like a pointer, a map or a slice which holds itself,
// is an error, a *json.UnsupportedValueError, while a value referenced from more places is written at each of them.
//
// The members of the type envelope are named by the TypeKeyword, the ValueKeyword and the MapKeyword of the Codec,
// $type, $value and $map by default.
// Unmarshal reads a JSON object with a type member as a type envelope, so a value can't have a member of its own named like it
// where it would be read back as a member of the envelope, instead of as a member of the value:
//   - a map written as plain JSON, without an envelope, with a key named like the type member,
//     like a map in a struct field of its own type, or a map without a type ID, like a map[string]any, held by an interface,
//   - a value held by an interface, whose JSON object has a member named like the type member,
//     or, when its type member is written next to its own members, like the value or the map member,
//   - and what a custom codec writes, when it has a member named like the type member.
//
// Such a value is not written, but Marshal fails with ErrKeywordCollision.
// A map with a type ID, held by an interface, has no such limit, as its entries are the $map member of its envelope.
// A struct with a member named like the type member, like a field with a json:"$type" tag, is written,
// where the type of the place it sits in tells its type, like a struct field of its own type.
// So is what a value writes with its own marshaler, like a json.RawMessage, which may have a type member on purpose.
// To have such a value written, name the members of the envelope otherwise, with the keywords of the Codec.
func (c Codec) Marshal(v any) ([]byte, error) {
	if err := c.checkKeywords(); err != nil {
		return nil, err
	}
	return newMarshaler(&c).marshal(v)
}

// Unmarshal decodes data into the value ptr points to, like json.Unmarshal does:
// a struct keeps the fields without a member in the JSON, a pointer keeps pointing to the value it points to,
// a map keeps its other entries, and the elements of a slice or an array are decoded in place.
//
// Unlike with encoding/json, an interface value is decoded by its $type,
// and it replaces the value the interface holds, instead of being decoded into it.
//
// Unlike the members of a struct, which are matched case-insensitively, like with encoding/json,
// the members of the type envelope are matched by their exact names, the keywords of the Codec,
// so a member named otherwise, like an @type, or a $TYPE, is just a member of the value,
// like the @type of a JSON-LD document, which a struct can have a json:"@type" field for.
//
// The $map of an envelope is read as the entries of the map the $type names, whatever their keys are, like a "$type".
// Like the JSON of any other value, the JSON of a map is read from the $value too, when the envelope has no $map,
// whatever the destination is, while an envelope with both has the entries read from the $map, where Marshal writes them.
//
// A JSON number without a $type is read into an empty interface as an int, when it is only digits, like 42 or -7, and fits an int,
// and as a float64 otherwise, like 4.2, 42.0, 1e3, or a number too big for an int.
// The number is parsed from its text, so an int is read exactly, even when a float64 can't hold it.
// A quoted number, like "42", is a string, like with encoding/json.
//
// Like with encoding/json, JSON can be read into another type of the same shape, the way a value is remapped into a DTO.
// Into a type which is not an interface, the destination already tells the type,
// so the $type envelope of a type the Codec knows is read by its payload, wherever it is in the JSON.
// The members of a JSON object are unordered, so its $type is found wherever it is among them, like {"$map":{...},"$type":"..."},
// which is how a store that reorders the members, like a PostgreSQL jsonb column, gives the JSON back.
// For example, a registered `type Counts map[string]int`, which Marshal writes as {"$type":"counts","$map":{"a":1}},
// is read into a map[string]int as {"a":1}, like when it is held by an `any` field, which the DTO has a map[string]int field for.
// A "$type" which names no type the Codec knows is just a member of the JSON object, like with encoding/json.
// A value with its own JSON unmarshaler, like a json.RawMessage, gets the JSON as it is, with the $type,
// unless the $type names its very own type, as then it gets what its own marshaler wrote, see below.
// A struct with a member for the $map or the $value of the envelope, like a DTO of the envelope itself, gets the JSON as it is too.
//
// Like with encoding/json, a value with its own json.Unmarshaler, or json.UnmarshalerFrom of encoding/json/v2, is read with it,
// and a value with its own encoding.TextUnmarshaler, but neither of those, is read from a JSON string with it.
// A registered type with its own JSON unmarshaler gets what its own marshaler wrote,
// without the $type envelope the Codec wrote around it, like when an interface held it.
// A custom codec registered with CodecRegister wins over both.
func (c Codec) Unmarshal(data []byte, ptr any) error {
	if ptr == nil {
		return fmt.Errorf("Unmarshal: nil destination")
	}
	dst := reflect.ValueOf(ptr)
	if dst.Kind() != reflect.Pointer || dst.IsNil() {
		return fmt.Errorf("Unmarshal: destination must be a non-nil pointer")
	}
	if err := c.checkKeywords(); err != nil {
		return err
	}
	return newUnmarshaler(&c).unmarshal(data, ptr)
}

func (c Codec) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return &ArrayEncoder[any]{W: w, C: c}
}

// NewStreamDecoder reads the JSON array which NewStreamEncoder writes,
// and decodes each element with the Codec, the way Unmarshal reads what Marshal writes.
func (c Codec) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator{
		Context: context.Background(),
		Input:   r,
	}
	dec := arrayElementDecoder{Codec: c, Iterator: i}
	return func(yield func(codec.Decoder, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(dec, nil) {
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

// arrayElementDecoder decodes the current element of a JSON array with the Codec.
type arrayElementDecoder struct {
	Codec    Codec
	Iterator *jsontoken.ArrayIterator
}

func (d arrayElementDecoder) Decode(ptr any) error {
	return d.Codec.Unmarshal(d.Iterator.Value(), ptr)
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
	// The json.Decoder is made when the stream is first read, and not when the stream is made,
	// as json.NewDecoder panics on a nil io.Reader since Go 1.27,
	// while a stream can be made without being read, like the one of Codec#NewStreamDecoder.
	// It is made only once, so reading the stream again continues where the last read stopped.
	var dec *json.Decoder
	return func(yield func(dec codec.Decoder, err error) bool) {
		if dec == nil {
			dec = json.NewDecoder(r)
			if c.UseNumber {
				dec.UseNumber()
			}
			if c.DisallowUnknownFields {
				dec.DisallowUnknownFields()
			}
		}
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

// NewArrayStreamDecoder reads the JSON array which NewArrayStreamEncoder writes,
// and decodes each element the way a zero Codec does, which is what NewArrayStreamEncoder writes with.
func NewArrayStreamDecoder(r io.Reader) codec.StreamDecoder {
	return Codec{}.NewStreamDecoder(r)
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

type DataTransferObjectMapping[ENT, DTO any] struct {
	ToENT func(ctx context.Context, dto DTO) (ENT, error)
	ToDTO func(ctx context.Context, ent ENT) (DTO, error)
}

func (m DataTransferObjectMapping[ENT, DTO]) mapping() dtokit.Mapping[ENT, DTO] {
	return dtokit.Mapping[ENT, DTO]{ToENT: m.ToENT, ToDTO: m.ToDTO}
}

func (m DataTransferObjectMapping[ENT, DTO]) Marshal(c *Codec, v ENT) ([]byte, error) {
	var dto, err = m.mapping().MapToDTO(context.Background(), v)
	if err != nil {
		return nil, err
	}
	return c.Marshal(dto)
}

func (m DataTransferObjectMapping[ENT, DTO]) Unmarshal(c *Codec, data []byte, p *ENT) error {
	var dto DTO
	if err := c.Unmarshal(data, &dto); err != nil {
		return err
	}
	var ent, err = m.mapping().MapToENT(context.Background(), dto)
	if err != nil {
		return err
	}
	*p = ent
	return nil
}
