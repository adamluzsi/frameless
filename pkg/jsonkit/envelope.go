package jsonkit

import (
	"cmp"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// keywords are the names of the members of the type envelope, which tells the type of a value where an interface holds it,
// see Codec#TypeKeyword, Codec#ValueKeyword and Codec#MapKeyword.
// The type member names the type, the map member holds the entries of a map,
// and the value member holds any other value which is not written with its own members next to the type member, like a struct is.
type keywords struct{ Type, Value, Map string }

var defaultKeywords = keywords{
	Type:  "$type",
	Value: "$value",
	Map:   "$map",
}

// keywords returns the names of the members of the type envelope, which the Codec writes and reads.
func (c *Codec) keywords() keywords {
	return keywords{
		Type:  cmp.Or(c.TypeKeyword, defaultKeywords.Type),
		Value: cmp.Or(c.ValueKeyword, defaultKeywords.Value),
		Map:   cmp.Or(c.MapKeyword, defaultKeywords.Map),
	}
}

// checkKeywords reports a Codec where two of the TypeKeyword, the ValueKeyword and the MapKeyword name the same member,
// as the members of the envelope couldn't be told apart.
func (c *Codec) checkKeywords() error {
	var kw = c.keywords()
	switch {
	case kw.Type == kw.Value:
		return c.assertSameKeyword("TypeKeyword", "ValueKeyword", kw.Type)
	case kw.Type == kw.Map:
		return c.assertSameKeyword("TypeKeyword", "MapKeyword", kw.Type)
	case kw.Value == kw.Map:
		return c.assertSameKeyword("ValueKeyword", "MapKeyword", kw.Value)
	}
	return nil
}

func (c *Codec) assertSameKeyword(a, b, name string) error {
	return fmt.Errorf("%w: the %s and the %s of the Codec are both %q", ErrKeywordCollision, a, b, name)
}

// errKeywordCollision is the error of writing a value of the type t, whose JSON object has a member of its own named like a keyword,
// where it would be read back as a member of the type envelope, instead of as a member of the value.
func errKeywordCollision(t reflect.Type, member string, kw keywords) error {
	keyword, role := "TypeKeyword", "type"
	switch member {
	case kw.Value:
		keyword, role = "ValueKeyword", "value"
	case kw.Map:
		keyword, role = "MapKeyword", "map"
	}
	return fmt.Errorf("%w: %s has a member %q of its own, which would be read back as the %s member of the type envelope; set the %s of the Codec to a name its members don't have",
		ErrKeywordCollision, t, member, role, keyword)
}

// errMissingTypeID is the error of writing a value without a type ID, where it needs one for its $type.
func errMissingTypeID(value any, kw keywords) error {
	return fmt.Errorf("missing %s id alias for %T", kw.Type, value)
}

// errUnknownTypeID tells why the $type of the data names no type of a value.
func errUnknownTypeID(reg *_TypeRegistry, id TypeID) error {
	if rType, ok := lookupTypeByID(reg, id, 0); ok && rType.Kind() == reflect.Interface {
		return fmt.Errorf("%w %q: it names the interface type %s, which no value is of", ErrUnknownTypeID, id, rType)
	}
	return fmt.Errorf("%w %q", ErrUnknownTypeID, id)
}

// envelope is the type envelope of a JSON object, see readEnvelope.
//
// The type member names the type, and the map member or the value member holds the value, when it is not a flat envelope,
// where the type member is next to the members of the value.
// The map member holds the entries of a map, but like any other value, the JSON of a map can be the value member too,
// which is read when the envelope has no map member, as the map member wins over it.
// Unlike the members of a struct, which encoding/json matches case-insensitively,
// the members of the envelope are matched by their exact names, the keywords of the Codec,
// so a member named otherwise, like an @type, or a $TYPE, is just a member of the value.
type envelope struct {
	// Found reports whether the JSON object has a type member.
	Found bool
	// Null reports whether the type member is null, which is like no type member.
	Null bool
	// TypeID is what the type member names.
	TypeID TypeID
	// Value is the JSON of the member which holds the value, when the envelope has one: the map member or the value member.
	Value []byte
	// ValueMember is the name of the member which Value is the JSON of.
	ValueMember string
}

// payload returns the payload of raw, the type envelope env:
// the map or the value member of a {"$type":"...","$map":{...}} or {"$type":"...","$value":...} envelope,
// or the members of a flat {"$type":"...",...} envelope, without the type member,
// which has done its job by telling the type of the value.
func (env envelope) payload(raw []byte, kw keywords) []byte {
	if env.ValueMember != "" {
		return env.Value
	}
	return stripMember(raw, kw.Type)
}

const pkgPath = "go.llib.dev/frameless/pkg/jsonkit"

// isArrayWrapper reports whether t is a jsonkit.Array[T].
// Its own MarshalJSON and UnmarshalJSON can only use the package level registry,
// which matters also when T is not an interface, as T can still hold interface values,
// so the Codec processes it as the []T it is defined as, which keeps the Codec's own registry in use.
func isArrayWrapper(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.PkgPath() == pkgPath && strings.HasPrefix(t.Name(), "Array[")
}

// isInterfaceWrapper reports whether t is a jsonkit.Interface[I] with an interface I.
// Like with jsonkit.Array, its own MarshalJSON and UnmarshalJSON can only use the package level registry,
// so the Codec processes it as the interface value it holds, which keeps the Codec's own registry in use.
func isInterfaceWrapper(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.PkgPath() == pkgPath && strings.HasPrefix(t.Name(), "Interface[") &&
		t.NumField() == 1 && t.Field(0).Type.Kind() == reflect.Interface
}

// isJSONKitWrapper reports whether t is a jsonkit wrapper type, which the Codec processes as the value it wraps,
// instead of with its own json.Marshaler and json.Unmarshaler.
func isJSONKitWrapper(t reflect.Type) bool {
	return isArrayWrapper(t) || isInterfaceWrapper(t)
}

// hasReadOnlyMember reports whether the struct type t has a member whose value can't be handed to the Codec,
// as it is the value of an unexported field: a struct of an unexported type, or a pointer to it,
// embedded with a name in its json tag, which makes it a member of its own, like with encoding/json,
// instead of having its fields promoted.
//
// encoding/json/v2 calls the Codec for each value it writes or reads, like for the value of a struct field,
// and it panics on doing so for such a value, as reflection can't hand out the value of an unexported field.
// So a struct with such a member is written and read as a whole the encoding/json way, see marshaler.write and unmarshaler.hook,
// which means that the values held by its interface fields are written and read without their $type too.
func hasReadOnlyMember(t reflect.Type) bool {
	if has, ok := readOnlyMembers.Load(t); ok {
		return has.(bool)
	}
	has := scanReadOnlyMember(t, map[reflect.Type]struct{}{})
	readOnlyMembers.Store(t, has)
	return has
}

var readOnlyMembers sync.Map // map[reflect.Type]bool

// needsHooks reports whether a value of the type t may hold a value which the Codec writes or reads otherwise than encoding/json:
// a value held by an interface, a jsonkit wrapper, or a struct with a member whose value can't be handed to the Codec.
// Only the hooks, which encoding/json/v2 calls for each value, can tell these apart, see marshaler.hook and unmarshaler.hook,
// while calling them for each value costs about as much as writing or reading the value itself.
// So the value of a type which needs none is written and read by encoding/json/v2 without them,
// unless the Codec has a custom codec, or the JSON may have a type envelope in it, see Codec.Marshal and Codec.Unmarshal.
func needsHooks(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if needs, ok := typesNeedingHooks.Load(t); ok {
		return needs.(bool)
	}
	needs := scanNeedsHooks(t, map[reflect.Type]struct{}{})
	typesNeedingHooks.Store(t, needs)
	return needs
}

var typesNeedingHooks sync.Map // map[reflect.Type]bool

func scanNeedsHooks(t reflect.Type, seen map[reflect.Type]struct{}) bool {
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	if t.Kind() == reflect.Interface || isJSONKitWrapper(t) {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return scanNeedsHooks(t.Elem(), seen)
	case reflect.Map:
		return scanNeedsHooks(t.Key(), seen) || scanNeedsHooks(t.Elem(), seen)
	case reflect.Struct:
		if hasReadOnlyMember(t) {
			return true
		}
		for i := range t.NumField() {
			if field := t.Field(i); (field.IsExported() || field.Anonymous) && scanNeedsHooks(field.Type, seen) {
				return true
			}
		}
	}
	return false
}

func scanReadOnlyMember(t reflect.Type, seen map[reflect.Type]struct{}) bool {
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	for field := range t.Fields() {
		var embedded = field.Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if !field.Anonymous || embedded.Kind() != reflect.Struct {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		if name, _, _ := strings.Cut(tag, ","); name != "" {
			if !field.IsExported() {
				return true
			}
			continue
		}
		// The fields of an embedded struct without a name are promoted, so its members are members of t.
		if scanReadOnlyMember(embedded, seen) {
			return true
		}
	}
	return false
}
