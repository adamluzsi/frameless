package jsonkit

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// unmarshaler reads the JSON of a Codec#Unmarshal call.
//
// The JSON is read by encoding/json/v2, with the options of encoding/json, see v1Options,
// so it is read the very same way as encoding/json reads it.
// encoding/json/v2 calls the hook of the unmarshaler for each value before it reads it,
// and the hook reads the values which the Codec reads otherwise, see unmarshaler.hook,
// or it lets encoding/json/v2 read the value, as encoding/json would.
type unmarshaler struct {
	codec *Codec
	kw    keywords
	opts  jsonv2.Options
	// plain reports whether a value without a hook to call, see needsHooks, can be read by encoding/json/v2 without the hooks,
	// as the Codec has no custom codec or migration, which could be called for any value.
	plain bool
	// asIs is the destination of a nested call, which reads the JSON into it as it is,
	// as the Codec has already worked out what to read into it, see readAsIs.
	asIs any
	// clean reports whether the JSON of the current call has no type envelope anywhere in it, see mayHoldEnvelope,
	// so a JSON object read into a type which is not an interface is not looked into for one.
	clean bool
}

func newUnmarshaler(c *Codec) *unmarshaler {
	return &unmarshaler{codec: c, kw: c.keywords(), plain: !c.typeIDRegistry.hasCustomCodecs(true)}
}

func (u *unmarshaler) unmarshal(data []byte, ptr any) error {
	clean := !mayHoldEnvelope(data, u.kw)
	if u.plain && clean && !needsHooks(reflect.TypeOf(ptr)) {
		return jsonv2.Unmarshal(data, ptr, v1Options)
	}
	if u.opts == nil {
		u.opts = jsonv2.JoinOptions(v1Options, jsonv2.WithUnmarshalers(jsonv2.UnmarshalFromFunc(u.hook)))
	}
	defer func(outer bool) { u.clean = outer }(u.clean)
	u.clean = clean
	return jsonv2.Unmarshal(data, ptr, u.opts)
}

// mayHoldEnvelope reports whether the JSON value data may have a type envelope, at its top or nested in it,
// which is read into a value whose type is not an interface by its payload, see readConcrete.
//
// It looks for a JSON string which starts with the first character of the type member,
// like the $ of a $type, or with an escaped character, as that may be the same as the character itself.
// So it may report a JSON value which has no envelope, like one with a "$name" string, but it never misses one.
func mayHoldEnvelope(data []byte, kw keywords) bool {
	return bytes.Contains(data, []byte{'"', kw.Type[0]}) || bytes.Contains(data, []byte(`"\`))
}

// readAsIs reads data into the value ptr points to, without a custom codec or a type envelope of its own,
// as the Codec has already worked out what data is, and how to read it.
// The values nested in it are read by the Codec, the same way as anywhere else.
func (u *unmarshaler) readAsIs(data []byte, ptr any) error {
	u.asIs = ptr
	defer func() { u.asIs = nil }()
	return u.unmarshal(data, ptr)
}

func (u *unmarshaler) registry() *_TypeRegistry { return u.codec.typeIDRegistry }

// hook is called by encoding/json/v2 for each value it reads, with a pointer to the value.
// It returns errors.ErrUnsupported, when encoding/json/v2 reads the value, the way encoding/json reads it.
func (u *unmarshaler) hook(dec *jsontext.Decoder, p any) error {
	asIs := u.asIs != nil && p == u.asIs
	if asIs {
		u.asIs = nil
	}
	v := reflect.ValueOf(p).Elem()
	t := v.Type()
	switch t.Kind() {
	case reflect.Interface:
		return u.readHeld(dec, v)
	case reflect.Pointer:
		// encoding/json/v2 reads null as a nil pointer, and anything else into the value it points to, with this hook.
		return errors.ErrUnsupported
	}
	// Like with encoding/json/v2, where the unmarshalers given as options take precedence over the type's own methods,
	// a custom codec wins over the type's own unmarshaler.
	if !asIs && u.registry().hasDecoder(t) {
		return u.readCustom(dec, v)
	}
	switch {
	case t.Kind() == reflect.Struct && hasReadOnlyMember(t):
		return readReadOnlyMembers(dec, v)
	// The own json.Unmarshaler of a jsonkit wrapper can only use the package level registry,
	// so the Codec reads the value it wraps, with its own registry.
	case isInterfaceWrapper(t):
		return jsonv2.UnmarshalDecode(dec, v.Field(0).Addr().Interface())
	case isArrayWrapper(t):
		return jsonv2.UnmarshalDecode(dec, v.Addr().Convert(reflect.PointerTo(reflect.SliceOf(t.Elem()))).Interface())
	case asIs:
		return errors.ErrUnsupported
	}
	// The type envelope of a value is read into a type which is not an interface by its payload, see readConcrete.
	// The members of a JSON object are unordered, so its type member can be anywhere among them,
	// and the whole object is read to find it, unless the JSON of the call has no envelope at all.
	if u.clean || dec.PeekKind() != '{' {
		return errors.ErrUnsupported
	}
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	return u.readConcrete(raw, v)
}

// readReadOnlyMembers reads the struct v, which has a member whose value can't be handed to the Codec, see hasReadOnlyMember,
// the encoding/json way.
//
// encoding/json panics on reading into a nil pointer to an unexported struct type, embedded with a name in its json tag,
// as it can't allocate it, and so does the encoding/json of Go 1.27 on reading null into it, which it used to leave as it is.
// The Codec reports these as an error instead.
func readReadOnlyMembers(dec *jsontext.Decoder, v reflect.Value) (err error) {
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	defer func() {
		r := recover()
		if msg, ok := r.(string); ok && strings.Contains(msg, "using value obtained using unexported field") {
			err = fmt.Errorf("json: cannot read into the embedded pointer to an unexported struct type of %v", v.Type())
			return
		}
		if r != nil {
			panic(r)
		}
	}()
	return jsonv2.Unmarshal(raw, v.Addr().Interface(), v1Options)
}

// readCustom reads the value v of a type with a custom codec or a migration,
// which gets the payload of the type envelope, when the JSON is one, or else the JSON as it is.
func (u *unmarshaler) readCustom(dec *jsontext.Decoder, v reflect.Value) error {
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	var (
		env, _  = readEnvelope(raw, u.kw)
		payload = raw
	)
	if env.TypeID != "" {
		payload = env.payload(raw, u.kw)
	}
	// A type that only has migrations has no decoder for its current $type,
	// so such data is read the default way.
	decode := u.registry().lookupDecoder(v.Type(), env.TypeID)
	if decode == nil {
		return u.readConcrete(raw, v)
	}
	// The closure gets the value v already holds,
	// like the UnmarshalJSON method of a value, which encoding/json decodes into.
	dest := reflect.New(v.Type())
	dest.Elem().Set(v)
	if err := decode(u.codec, payload, dest.Interface()); err != nil {
		return err
	}
	v.Set(dest.Elem())
	return nil
}

// readConcrete reads raw, a JSON value, into v, whose type is not an interface,
// so the type of the value is already told by the destination,
// and the type envelope of a type the Codec knows has done its job, so it is read by its payload, see Codec.Unmarshal.
// The type member of a type the Codec doesn't know is not a type member of the Codec, so like with encoding/json, it is just a member.
//
// A struct reads the JSON object by the names of its members, so for a struct,
//   - a flat envelope is read as it is, as its type member is just another member, which the struct reads, if it has a member for it,
//   - and the map or the value member form is read as it is, when the struct has a member for the member it has, like a DTO of the envelope itself.
//
// A value with its own JSON unmarshaler gets the JSON as it is, as it may keep it to be read later, like a json.RawMessage,
// unless the envelope names its very own type, as then it gets what its own marshaler wrote.
func (u *unmarshaler) readConcrete(raw []byte, v reflect.Value) error {
	data := raw
	if env, _ := readEnvelope(raw, u.kw); env.TypeID != "" {
		if rType, known := lookupTypeByIDForCodec(u.registry(), env.TypeID); known {
			switch t := v.Type(); {
			case hasOwnJSONUnmarshaler(t):
				if base(rType) == base(t) {
					data = env.payload(raw, u.kw)
				}
			case t.Kind() == reflect.Struct:
				if env.ValueMember != "" && !hasJSONMember(t, env.ValueMember) {
					data = env.Value
				}
			default:
				data = env.payload(raw, u.kw)
			}
		}
	}
	return u.readAsIs(data, v.Addr().Interface())
}

// readHeld reads the value of the interface iv, which replaces the value it holds, instead of being read into it.
// A JSON object is read by its type member, see readObject, and any other JSON value as plain JSON, see readPlain.
func (u *unmarshaler) readHeld(dec *jsontext.Decoder, iv reflect.Value) error {
	var value reflect.Value
	if dec.PeekKind() == '{' {
		raw, err := dec.ReadValue()
		if err != nil {
			return err
		}
		if value, err = u.readObject(raw, iv.Type()); err != nil {
			return err
		}
	} else {
		v, err := u.readPlain(dec)
		if err != nil {
			return err
		}
		value = reflect.ValueOf(v)
	}
	return setDecoded(iv, value)
}

// readPlain reads a JSON value, which carries no type information, as it is not an object, into an `any`, the way encoding/json does,
// except that a number of only digits, like 42 or -7, is read as an int, when it fits one, instead of as a float64,
// and the values with a type member nested in a JSON array are read by it.
func (u *unmarshaler) readPlain(dec *jsontext.Decoder) (any, error) {
	switch dec.PeekKind() {
	case '[':
		var vs []any
		if err := jsonv2.UnmarshalDecode(dec, &vs); err != nil {
			return nil, err
		}
		return vs, nil
	case '0':
		raw, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		return parseNumber(raw)
	default:
		var v any
		err := jsonv2.UnmarshalDecode(dec, &v, jsonv2.WithUnmarshalers(nil))
		return v, err
	}
}

// parseNumber reads raw, a JSON number, as an int, when it is only digits, like 42 or -7, and fits an int,
// and as a float64 otherwise, like 4.2, 42.0, 1e3, or a number too big for an int.
// The number is parsed from its text, so an int is read exactly, even when a float64 can't hold it.
func parseNumber(raw []byte) (any, error) {
	if i, err := strconv.ParseInt(string(raw), 10, strconv.IntSize); err == nil {
		return int(i), nil
	}
	f, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		// A number out of the range of a float64, like 1e400, fails the way encoding/json fails on it.
		return nil, jsonv2.Unmarshal(raw, new(float64), v1Options)
	}
	return f, nil
}

// readObject reads raw, a JSON object, as a value of the interface type target, by its type member.
// An empty interface reads a JSON object without a type member as a map[string]any, like encoding/json does,
// while a non-empty interface needs the type member to tell the type of the value.
func (u *unmarshaler) readObject(raw []byte, target reflect.Type) (reflect.Value, error) {
	env, err := readEnvelope(raw, u.kw)
	switch {
	case err != nil:
		return reflect.Value{}, err
	case !env.Found && target.NumMethod() == 0:
		var object map[string]any
		err := u.readAsIs(raw, &object)
		return reflect.ValueOf(object), err
	case !env.Found, env.Null:
		return reflect.Value{}, fmt.Errorf("%s is not set", u.kw.Type)
	case env.TypeID == "":
		return reflect.Value{}, fmt.Errorf("%s is empty", u.kw.Type)
	}
	return u.readTyped(raw, env, target)
}

// readTyped reads raw, the type envelope env, as a value of the type it names, which is held by the target type.
// The value is of a type the target type is, or of a pointer to it, when only the pointer implements the target interface.
//
// The type of the target can name the value it holds too, with its own name, like an `any` holding a []any,
// which WithType[any] writes as {"$type":"interface {}","$value":[...]}.
func (u *unmarshaler) readTyped(raw []byte, env envelope, target reflect.Type) (reflect.Value, error) {
	rType, ok := lookupTypeByIDForCodec(u.registry(), env.TypeID)
	if !ok {
		if TypeID(target.String()) != env.TypeID {
			return reflect.Value{}, errUnknownTypeID(u.registry(), env.TypeID)
		}
		data := raw
		if env.ValueMember != "" {
			data = env.Value
		} else if target.Kind() == reflect.Interface {
			// Reading the members of a flat envelope into the interface would read the very same envelope again, without an end.
			return reflect.Value{}, fmt.Errorf("%s is the type of no value, so it needs a %s", env.TypeID, u.kw.Value)
		}
		ptr := reflect.New(target)
		err := u.readAsIs(data, ptr.Interface())
		return ptr.Elem(), err
	}
	ptr, err := newImpl(target, rType)
	if err != nil {
		return reflect.Value{}, err
	}
	value := ptr
	for value.Type().Elem() != rType {
		value = value.Elem()
	}
	if decode := u.registry().lookupDecoder(rType, env.TypeID); decode != nil {
		err = decode(u.codec, env.payload(raw, u.kw), value.Interface())
	} else {
		data := raw
		switch {
		case env.ValueMember != "":
			data = env.Value
		case rType.Kind() != reflect.Struct || hasOwnJSONUnmarshaler(rType):
			// A struct reads its members from the flat envelope, where the type member is just another member,
			// while any other value gets what was written for it, like a map its entries, or an own JSON unmarshaler its own JSON.
			data = env.payload(raw, u.kw)
		}
		err = u.readAsIs(data, value.Interface())
	}
	if err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

// memberNameIs reports whether quoted, the JSON string of a member name, is name.
func memberNameIs(quoted []byte, name string, escaped bool) bool {
	if !escaped {
		return string(quoted[1:len(quoted)-1]) == name
	}
	unquoted, err := jsontext.AppendUnquote(nil, quoted)
	return err == nil && string(unquoted) == name
}

// ownJSONUnmarshalers are the interfaces of the methods which encoding/json reads a value from its JSON with, as it is:
// a json.Unmarshaler, or a json.UnmarshalerFrom of encoding/json/v2, which encoding/json calls too.
var ownJSONUnmarshalers = []reflect.Type{
	reflect.TypeFor[jsonv2.Unmarshaler](),
	reflect.TypeFor[jsonv2.UnmarshalerFrom](),
}

// hasOwnJSONUnmarshaler reports whether a value of the type t is read with its own JSON unmarshaler, see ownJSONUnmarshalers,
// like with encoding/json, unless it is a jsonkit wrapper, which the Codec reads as the value it wraps.
// A pointer has the unmarshaler of the type it points to.
func hasOwnJSONUnmarshaler(t reflect.Type) bool {
	return (slices.ContainsFunc(ownJSONUnmarshalers, t.Implements) ||
		slices.ContainsFunc(ownJSONUnmarshalers, reflect.PointerTo(t).Implements)) &&
		!isJSONKitWrapper(base(t))
}

// jsonMembers caches whether a struct type has a member of a given name, see hasJSONMember.
var jsonMembers sync.Map // map[jsonMember]bool

type jsonMember struct {
	typ  reflect.Type
	name string
}

// hasJSONMember reports whether the struct type t may have a member with the given name,
// which encoding/json matches the way it matches the names of the members, case-insensitively.
// The fields promoted from the embedded structs are members too.
// A field which encoding/json leaves out, as another one with the same name hides it, is reported as well.
func hasJSONMember(t reflect.Type, name string) bool {
	key := jsonMember{typ: t, name: name}
	if has, ok := jsonMembers.Load(key); ok {
		return has.(bool)
	}
	has := scanJSONMember(t, name, map[reflect.Type]struct{}{})
	jsonMembers.Store(key, has)
	return has
}

func scanJSONMember(t reflect.Type, name string, seen map[reflect.Type]struct{}) bool {
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		tagName, _, _ := strings.Cut(tag, ",")
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		embeddedStruct := field.Anonymous && fieldType.Kind() == reflect.Struct
		switch {
		case embeddedStruct && tagName == "":
			if scanJSONMember(fieldType, name, seen) {
				return true
			}
		case !field.IsExported() && !embeddedStruct:
		case strings.EqualFold(cmp.Or(tagName, field.Name), name):
			return true
		}
	}
	return false
}

// readEnvelope reads the type envelope of raw, a JSON object, see envelope.
// A JSON value which is not an object has none.
func readEnvelope(raw []byte, kw keywords) (env envelope, err error) {
	if !isObject(bytes.TrimSpace(raw)) {
		return env, nil
	}
	var typeMember []byte
	err = eachObjectMember(raw, func(name, value []byte, escaped bool) {
		switch {
		case memberNameIs(name, kw.Type, escaped):
			env.Found, typeMember = true, value
		case memberNameIs(name, kw.Map, escaped):
			env.Value, env.ValueMember = value, kw.Map
		case env.ValueMember != kw.Map && memberNameIs(name, kw.Value, escaped):
			env.Value, env.ValueMember = value, kw.Value
		}
	})
	if err != nil || !env.Found {
		return envelope{}, err
	}
	if env.Null = isNull(typeMember); env.Null {
		return env, nil
	}
	if err := jsonv2.Unmarshal(typeMember, &env.TypeID, v1Options); err != nil {
		return envelope{}, fmt.Errorf("unable to unmarshal %s field for:\n%s", kw.Type, string(raw))
	}
	return env, nil
}

// eachObjectMember calls fn with each member of raw, a JSON object, in their order, duplicates included:
// the JSON string of its name, which is escaped, when it has a backslash in it, and its JSON value.
func eachObjectMember(raw []byte, fn func(name, value []byte, escaped bool)) error {
	dec := jsontext.NewDecoder(bytes.NewBuffer(raw), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	for dec.PeekKind() == '"' {
		name, err := dec.ReadValue()
		if err != nil {
			return err
		}
		value, err := dec.ReadValue()
		if err != nil {
			return err
		}
		fn(name, value, bytes.IndexByte(name, '\\') >= 0)
	}
	return nil
}

// hasObjectMember reports whether data is a JSON object with a member of the given name,
// which is matched exactly, the way the members of the type envelope are read, see readEnvelope.
func hasObjectMember(data []byte, name string) bool {
	// The name of a member is in the JSON as it is, unless it is escaped in it.
	if !isObject(data) || (!bytes.Contains(data, []byte(name)) && !bytes.Contains(data, []byte(`\`))) {
		return false
	}
	var has bool
	_ = eachObjectMember(data, func(member, _ []byte, escaped bool) {
		has = has || memberNameIs(member, name, escaped)
	})
	return has
}

// stripMember returns raw, a JSON object, without its members of the given name,
// and keeps its other members as they are, in their order, duplicates included,
// so a member with another name, like an "@type" or an "@context", stays as well.
func stripMember(raw []byte, name string) []byte {
	out := []byte{'{'}
	err := eachObjectMember(raw, func(member, value []byte, escaped bool) {
		if memberNameIs(member, name, escaped) {
			return
		}
		if 1 < len(out) {
			out = append(out, ',')
		}
		out = append(out, member...)
		out = append(out, ':')
		out = append(out, value...)
	})
	if err != nil {
		return raw
	}
	return append(out, '}')
}
