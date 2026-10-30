package jsonkit

import (
	"bytes"
	"cmp"
	"encoding"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"slices"
)

// v1Options are the options of encoding/json, which the Codec writes and reads JSON with by encoding/json/v2,
// so a value is written and read the very same way as encoding/json writes and reads it.
var v1Options = json.DefaultOptionsV1()

// marshaler writes the JSON of a Codec#Marshal call.
//
// The value is written by encoding/json/v2, with the options of encoding/json, see v1Options,
// so it is written the very same way as encoding/json writes it.
// encoding/json/v2 calls the hook of the marshaler for each value before it writes it,
// and the hook writes the values which the Codec writes otherwise, see marshaler.hook,
// or it lets encoding/json/v2 write the value, as encoding/json would.
type marshaler struct {
	codec *Codec
	kw    keywords
	opts  jsonv2.Options
	// plain reports whether a value without a hook to call, see needsHooks, can be written by encoding/json/v2 without the hooks,
	// as the Codec has no custom codec, which could be called for any value.
	plain bool
	// typeMember is the name of the type member, the way encoding/json writes it.
	typeMember []byte
	// entries is the map whose entries are written as the map member of its type envelope,
	// where their keys are just data, so they are not checked against the type member, see marshaler.checkMapKeys.
	entries uintptr
	// err is the first error of the hook, which is returned as it is,
	// instead of how encoding/json/v2 reports the error of a hook.
	err error
}

// marshalCall is what a Marshal call keeps track of across the nested calls it makes while writing a value:
// the writing of a value in its type envelope, which needs the JSON of the value before it is written,
// and the calls of the custom codecs, which may write the values they reference with the Codec they receive.
//
// encoding/json/v2 reports a value which references back to itself as a cycle,
// but only within a single call, while a nested call is a call of its own.
// So the values written through a nested call are tracked here,
// and a value which is written through a nested call while it is being written already, is reported as a cycle,
// the same way as encoding/json reports it, instead of being written forever.
type marshalCall struct {
	writing map[reference]struct{}
}

// reference identifies a value which is being written through a nested call, see marshalCall.
// It is the address of the place of the value, like the one of an interface field,
// or the pointer, map or slice which the value is, like encoding/json/v2 identifies them.
type reference struct {
	typ reflect.Type
	ptr uintptr
	len int
}

func newMarshaler(c *Codec) *marshaler {
	if c.call == nil {
		c.call = &marshalCall{}
	}
	return &marshaler{codec: c, kw: c.keywords(), plain: !c.typeIDRegistry.hasCustomCodecs(false)}
}

func (m *marshaler) marshal(v any) ([]byte, error) {
	if m.plain && !needsHooks(reflect.TypeOf(v)) {
		if m.typeMember == nil {
			m.typeMember, _ = jsonv2.Marshal(m.kw.Type, v1Options)
		}
		// A member named like the type member may be the key of a map, which can't have one, see checkMapKeys,
		// so the value is written again with the hooks, which tell it.
		if data, err := jsonv2.Marshal(v, v1Options); err != nil || !bytes.Contains(data, m.typeMember) {
			return data, err
		}
	}
	if m.opts == nil {
		m.opts = jsonv2.JoinOptions(v1Options, jsonv2.WithMarshalers(jsonv2.MarshalToFunc(m.hook)))
	}
	data, err := jsonv2.Marshal(v, m.opts)
	if err != nil {
		return nil, cmp.Or(m.err, err)
	}
	return data, nil
}

// marshalEntries writes the entries of the map v, whatever their keys are.
func (m *marshaler) marshalEntries(v reflect.Value) ([]byte, error) {
	m.entries = v.Pointer()
	defer func() { m.entries = 0 }()
	return m.marshal(v.Interface())
}

func (m *marshaler) registry() *_TypeRegistry { return m.codec.typeIDRegistry }

// hook is called by encoding/json/v2 for each value it writes, with a pointer to the value.
// It returns errors.ErrUnsupported, when encoding/json/v2 writes the value, the way encoding/json writes it.
func (m *marshaler) hook(enc *jsontext.Encoder, p any) error {
	err := m.write(enc, reflect.ValueOf(p).Elem())
	if err != nil && m.err == nil && !errors.Is(err, errors.ErrUnsupported) {
		m.err = err
	}
	return err
}

func (m *marshaler) write(enc *jsontext.Encoder, v reflect.Value) error {
	t := v.Type()
	switch t.Kind() {
	case reflect.Interface:
		return m.writeHeld(enc, v)
	case reflect.Pointer:
		// encoding/json/v2 writes a nil pointer as null, and the value of any other with this hook.
		return errors.ErrUnsupported
	}
	// Like with encoding/json/v2, where the marshalers given as options take precedence over the type's own methods,
	// a custom codec wins over the type's own marshaler.
	// What it writes is the JSON of the value, so like any other value which is not held by an interface,
	// it is written without a $type, as the type it is read back into is told by the place of the value.
	if entry := m.registry().LookupCustomCodec(t); entry != nil {
		var data []byte
		err := m.nested(func() (err error) {
			data, err = entry.Marshal(m.codec, v.Addr().Interface())
			return err
		}, v.Addr())
		if err != nil {
			return err
		}
		// Unmarshal reads the type member of what it wrote as the type member of an envelope,
		// and leaves it out of what it gives back to the custom codec, so it can't have one.
		if hasObjectMember(data, m.kw.Type) {
			return errKeywordCollision(t, m.kw.Type, m.kw)
		}
		return enc.WriteValue(data)
	}
	switch {
	case t.Kind() == reflect.Struct && hasReadOnlyMember(t):
		return jsonv2.MarshalEncode(enc, v.Addr().Interface(), jsonv2.WithMarshalers(nil))
	// The own json.Marshaler of a jsonkit wrapper can only use the package level registry,
	// so the Codec writes the value it wraps, with its own registry.
	case isArrayWrapper(t):
		return jsonv2.MarshalEncode(enc, v.Addr().Convert(reflect.PointerTo(reflect.SliceOf(t.Elem()))).Interface())
	case isInterfaceWrapper(t):
		return jsonv2.MarshalEncode(enc, v.Field(0).Addr().Interface())
	case t.Kind() == reflect.Map:
		return m.checkMapKeys(v)
	}
	return errors.ErrUnsupported
}

// checkMapKeys fails with ErrKeywordCollision when the map v, written as plain JSON, has a key named like the type member,
// as Unmarshal would read the JSON object as a type envelope.
// The entries of a map in its type envelope are its map member, where their keys are just data, see marshalEntries.
func (m *marshaler) checkMapKeys(v reflect.Value) error {
	if v.Pointer() == m.entries {
		m.entries = 0
		return errors.ErrUnsupported
	}
	t := v.Type()
	if v.Len() == 0 || t.Key().Kind() != reflect.String || hasOwnMarshaler(t) || hasOwnMarshaler(reflect.PointerTo(t)) {
		return errors.ErrUnsupported
	}
	if v.MapIndex(reflect.ValueOf(m.kw.Type).Convert(t.Key())).IsValid() {
		return errKeywordCollision(t, m.kw.Type, m.kw)
	}
	return errors.ErrUnsupported
}

// writeHeld writes the value which the interface iv holds, see Codec.Marshal.
func (m *marshaler) writeHeld(enc *jsontext.Encoder, iv reflect.Value) error {
	if iv.IsNil() {
		return errors.ErrUnsupported
	}
	value := iv.Elem().Interface()
	if needsNoTypeID(value, m.registry()) {
		if f, ok := value.(float64); ok {
			data, err := marshalFloat64(f)
			if err != nil {
				return err
			}
			return enc.WriteValue(data)
		}
		return errors.ErrUnsupported
	}
	typeID, ok := lookupTypeIDForHeldValue(m.registry(), value)
	if !ok {
		return m.writePlain(enc, iv)
	}
	data, err := m.envelope(iv, typeID)
	if err != nil {
		return err
	}
	return enc.WriteValue(data)
}

// writePlain writes the value without a type ID, which the interface iv holds, as plain JSON.
//
// Like with encoding/json, an empty interface can hold any value, and it reads plain JSON back as the generic shape of it,
// like a time.Time as a string, or a struct as a map[string]any, so the value is written as plain JSON, as a best effort.
// A non-empty interface can't be read back without the $type, except from the generic shape of a JSON array or object,
// so a value which isn't a slice, an array, a map, or a pointer to them, needs a type ID.
//
// A member of the JSON object named like the type member would be read back as the type member of an envelope,
// unless the value writes it with its own marshaler on purpose, like a json.RawMessage.
// The keys of a map are checked when the map is written, see checkMapKeys.
func (m *marshaler) writePlain(enc *jsontext.Encoder, iv reflect.Value) error {
	concrete := iv.Elem()
	kind := base(concrete.Type()).Kind()
	if iv.Type().NumMethod() != 0 {
		switch kind {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.Interface:
		default:
			return errMissingTypeID(concrete.Interface(), m.kw)
		}
	}
	if kind != reflect.Struct || hasOwnMarshaler(concrete.Type()) || isJSONKitWrapper(base(concrete.Type())) ||
		!hasJSONMember(base(concrete.Type()), m.kw.Type) {
		return errors.ErrUnsupported
	}
	var data []byte
	err := m.nested(func() (err error) {
		data, err = m.marshal(concrete.Interface())
		return err
	}, iv.Addr(), concrete)
	if err != nil {
		return err
	}
	if hasObjectMember(data, m.kw.Type) {
		return errKeywordCollision(concrete.Type(), m.kw.Type, m.kw)
	}
	return enc.WriteValue(data)
}

// envelope writes the value which the interface iv holds in its type envelope, see wrapInEnvelope.
//
// The value is written as it is held by the interface, like with encoding/json, so not as an addressable value,
// and its JSON is written in the envelope, as Unmarshal reads it back.
// A value with a custom codec is written by it, and the null it writes is written as it is, without an envelope.
// The entries of a map are the map member of the envelope, where their keys are just data, so they may be named like anything.
func (m *marshaler) envelope(iv reflect.Value, typeID TypeID) ([]byte, error) {
	concrete := iv.Elem()
	var (
		data   []byte
		custom bool
	)
	err := m.nested(func() (err error) {
		if entry := m.registry().LookupCustomCodec(concrete.Type()); entry != nil {
			custom = true
			data, err = entry.Marshal(m.codec, concrete.Interface())
			return err
		}
		if entries, ok := entriesOfHeldMap(concrete, m.registry()); ok {
			data, err = m.marshalEntries(entries)
			return err
		}
		data, err = m.marshal(concrete.Interface())
		return err
	}, iv.Addr(), concrete)
	if err != nil {
		return nil, err
	}
	if custom && isNull(data) {
		return data, nil
	}
	return wrapInEnvelope(m.kw, typeID, concrete.Type(), data)
}

// nested calls fn, which writes the values refs refer to through a nested call, see marshalCall.
// A value which is being written through a nested call already, references back to itself,
// so it is reported as a cycle, with the error of encoding/json.
func (m *marshaler) nested(fn func() error, refs ...reflect.Value) error {
	call := m.codec.call
	var entered [2]reference
	n := 0
	defer func() {
		for _, ref := range entered[:n] {
			delete(call.writing, ref)
		}
	}()
	for _, v := range refs {
		ref, ok := referenceOf(v)
		if !ok {
			continue
		}
		if _, ok := call.writing[ref]; ok {
			return &json.UnsupportedValueError{Value: v, Str: "encountered a cycle via " + v.Type().String()}
		}
		if call.writing == nil {
			call.writing = make(map[reference]struct{})
		}
		call.writing[ref] = struct{}{}
		entered[n] = ref
		n++
	}
	return fn()
}

// referenceOf returns the reference which v is, when it is a pointer, a map or a slice, which is not nil.
func referenceOf(v reflect.Value) (reference, bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Map:
		if v.IsNil() {
			return reference{}, false
		}
		return reference{typ: v.Type(), ptr: v.Pointer()}, true
	case reflect.Slice:
		if v.IsNil() {
			return reference{}, false
		}
		return reference{typ: v.Type(), ptr: v.Pointer(), len: v.Len()}, true
	default:
		return reference{}, false
	}
}

// wrapInEnvelope writes data, the JSON of a value of the type t, in the type envelope of typeID.
//
// The type member is written next to the members of a JSON object, like with a struct: {"$type":"item","Name":"a"},
// while any other JSON value is the value member of the envelope: {"$type":"...","$value":...}.
// The JSON object of a map, or its null, is the map member of the envelope: {"$type":"counts","$map":{"a":1}},
// as its members are its entries, whose keys are just data, which may be named like anything.
func wrapInEnvelope(kw keywords, typeID TypeID, t reflect.Type, data []byte) ([]byte, error) {
	object := isObject(data)
	switch {
	case base(t).Kind() == reflect.Map && (object || isNull(data)):
		return wrapWithEnvelopeMember(kw, typeID, kw.Map, data)
	case object:
		return wrapWithTypeIDValue(kw, typeID, data, t)
	default:
		return wrapWithEnvelopeMember(kw, typeID, kw.Value, data)
	}
}

// wrapWithTypeIDValue writes the type member next to the members of data, the JSON object of a value of the type t:
// {"$type":"...",<members>}.
// It fails with ErrKeywordCollision, when the value has a member of its own named like a member of the envelope,
// as it would be read back as a member of the envelope.
func wrapWithTypeIDValue(kw keywords, typeID TypeID, data []byte, t reflect.Type) ([]byte, error) {
	for _, keyword := range [...]string{kw.Type, kw.Value, kw.Map} {
		if hasObjectMember(data, keyword) {
			return nil, errKeywordCollision(t, keyword, kw)
		}
	}
	out, err := appendTypeMember([]byte{'{'}, kw, typeID)
	if err != nil {
		return nil, err
	}
	members := bytes.TrimSpace(data)[1:]
	if members = bytes.TrimSpace(members); members[0] != '}' {
		out = append(out, ',')
	}
	return append(out, members...), nil
}

// wrapWithEnvelopeMember writes the type envelope which holds data, a JSON value, as its member with the given name:
// {"$type":"...","<member>":<data>}.
func wrapWithEnvelopeMember(kw keywords, typeID TypeID, member string, data []byte) ([]byte, error) {
	out, err := appendTypeMember([]byte{'{'}, kw, typeID)
	if err != nil {
		return nil, err
	}
	out = append(out, ',')
	if out, err = jsontext.AppendQuote(out, member); err != nil {
		return nil, err
	}
	out = append(out, ':')
	out = append(out, bytes.TrimSpace(data)...)
	return append(out, '}'), nil
}

// appendTypeMember appends the type member of an envelope, which names typeID: "$type":"...".
func appendTypeMember(out []byte, kw keywords, typeID TypeID) ([]byte, error) {
	out, err := jsontext.AppendQuote(out, kw.Type)
	if err != nil {
		return nil, err
	}
	out = append(out, ':')
	return jsontext.AppendQuote(out, typeID)
}

// needsNoTypeID reports whether v, held by an interface, is written as plain JSON, without a $type.
//
// The Codec reads plain JSON into an empty interface as a nil, bool, int, float64, string, []any or map[string]any,
// see unmarshaler.readPlain, so a nil, bool, int, float64 or string is read back as the very same value without a $type.
// A float64 is written with a fraction, see marshalFloat64, so a whole one is not read back as an int.
// Apart from the nil, these types have no methods, so only an empty interface can hold them.
// The other built-in types, like an int64, keep their $type, as their plain JSON would be read back as an int or a float64.
// A []any and a map[string]any are written as a plain JSON array and object anyway.
//
// A registration of the type on the Codec itself wins, so the value is written with the $type of the registration.
//
// A value of a type without a type ID, held by an empty interface, is written as plain JSON too, see marshaler.writePlain,
// but unlike these, it is read back as the generic shape of its JSON, instead of as the very same value.
func needsNoTypeID(v any, reg *_TypeRegistry) bool {
	switch v.(type) {
	case nil, bool, int, float64, string:
	default:
		return false
	}
	if reg == nil {
		return true
	}
	_, registered := reg.TypeIDFor(v)
	return !registered
}

// marshalFloat64 writes f, a float64 held by an interface, the way encoding/json writes it,
// except that a whole number gets a ".0" fraction, like 42.0 or -0.0, instead of 42 or -0,
// as a JSON number of only digits is read back into an empty interface as an int, see unmarshaler.readPlain.
// A number which encoding/json writes with an exponent, like 1e+21, is read back as a float64 already.
func marshalFloat64(f float64) ([]byte, error) {
	data, err := jsonv2.Marshal(f, v1Options)
	if err != nil {
		return nil, err
	}
	if !bytes.ContainsAny(data, ".eE") {
		data = append(data, ".0"...)
	}
	return data, nil
}

// entriesOfHeldMap returns the map which v, a value held by an interface, is, or points to,
// when the Codec writes its entries itself, see marshaler.marshalEntries.
// ok is false for any other value, for a nil map or a nil pointer,
// and for a map, or a pointer to it, which is written by its custom codec or its own marshaler,
// as what they write is written as it is.
func entriesOfHeldMap(v reflect.Value, reg *_TypeRegistry) (_ reflect.Value, ok bool) {
	for v.IsValid() && reg.LookupCustomCodec(v.Type()) == nil && !hasOwnMarshaler(v.Type()) {
		switch {
		case v.Kind() == reflect.Map && !v.IsNil():
			return v, true
		case v.Kind() == reflect.Pointer && !v.IsNil():
			v = v.Elem()
		default:
			return reflect.Value{}, false
		}
	}
	return reflect.Value{}, false
}

// ownMarshalers are the interfaces of the methods which encoding/json writes a value with, instead of by its kind:
// a json.Marshaler, or a json.MarshalerTo of encoding/json/v2, which encoding/json calls too,
// and an encoding.TextMarshaler or an encoding.TextAppender, whose text is written as a JSON string.
var ownMarshalers = []reflect.Type{
	reflect.TypeFor[jsonv2.Marshaler](),
	reflect.TypeFor[jsonv2.MarshalerTo](),
	reflect.TypeFor[encoding.TextMarshaler](),
	reflect.TypeFor[encoding.TextAppender](),
}

// hasOwnMarshaler reports whether a value of the type t is written with its own marshaler, see ownMarshalers,
// like with encoding/json, unless it is a jsonkit wrapper, which the Codec writes as the value it wraps.
func hasOwnMarshaler(t reflect.Type) bool {
	return slices.ContainsFunc(ownMarshalers, t.Implements) && !isJSONKitWrapper(base(t))
}
