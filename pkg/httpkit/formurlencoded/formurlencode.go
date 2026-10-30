package formurlencoded

import (
	"encoding"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/port/codec"
)

// Codec marshals values into the application/x-www-form-urlencoded format, and unmarshals them from it.
//
// A form is a list of key-value pairs, so its root value is a struct or a map:
//   - a single value, like a string, a number, a bool or an encoding.TextMarshaler, is its key with its text: `name=foo`;
//   - a nested struct or map puts its keys under its own key, separated by a dot: `foo.name=bar`;
//   - a slice of single values repeats its key for each element: `tag=a&tag=b`;
//   - any other slice puts each element under its index: `items.0.name=foo&items.1.name=bar`.
//
// A field's key is its name in snake case, or the name in its `form` tag, or else in its `url` tag.
// Like with encoding/json, the tag `form:"-"` leaves the field out, `form:"-,"` names it "-",
// and the omitempty option leaves it out when it's empty: `form:",omitempty"`.
//
// Like encoding/json, it ignores unexported fields, leaves out nil pointers,
// and keeps the fields of the destination which have no key in the data.
type Codec struct {
	// Collection is the id/name of the collection type.
	// The default value is the short type name of T in snake case with an "s" suffix.
	// For example if T is
	// - `Item` -> `items`
	// - `User` -> `users`
	// - `UserEmail` -> `user_emails`
	// Collection string

	// StringCase is the formatter used to format the url keys.
	// Default: stringkit.ToSnake
	StringCase func(string) string
}

func (c *Codec) fmtKey(key string) string {
	if c.StringCase != nil {
		return c.StringCase(key)
	}
	return stringkit.ToSnake(key)
}

var pkgPrefix = regexp.MustCompile(`^[\w\d]+\.`)

func (c *Codec) typeName(typ reflect.Type) string {
	if name := typ.Name(); 0 < len(name) {
		return c.fmtKey(name)
	}
	raw := typ.String()
	raw = pkgPrefix.ReplaceAllString(raw, "")
	return c.fmtKey(raw)
}

var mediaTypeFormURLEncoded = map[string]struct{}{
	mediatype.FormUrlencoded: {},
}

func (c Codec) Marshal(v any) ([]byte, error) {
	val := deref(reflect.ValueOf(v))
	if !val.IsValid() {
		return []byte{}, nil
	}
	if err := checkRoot(val.Type()); err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := c.marshalAppend(q, "", val); err != nil {
		return nil, err
	}
	return []byte(q.Encode()), nil
}

func (c Codec) marshalAppend(q url.Values, key string, val reflect.Value) error {
	val = deref(val)
	if !val.IsValid() { // a nil pointer or interface has nothing to write, like an omitted field
		return nil
	}
	typ := val.Type()
	if isText(typ) {
		text, err := convkit.FormatReflect(val, textOptions)
		if err != nil {
			return fmt.Errorf("error while formatting %q: %w", key, err)
		}
		q.Add(key, text)
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		for i := range typ.NumField() {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			prop := c.getFormProperties(field)
			if prop.Skip {
				continue
			}
			value := val.Field(i)
			if prop.OmitEmpty && reflectkit.IsEmpty(value) {
				continue
			}
			if err := c.marshalAppend(q, joinKey(key, prop.Name), value); err != nil {
				return err
			}
		}
		return nil

	case reflect.Map:
		entries := val.MapRange()
		for entries.Next() {
			mKey, err := convkit.FormatReflect(entries.Key(), textOptions)
			if err != nil {
				return fmt.Errorf("error while formatting %#v key: %w", entries.Key().Interface(), err)
			}
			if err := c.marshalAppend(q, joinKey(key, mKey), entries.Value()); err != nil {
				return err
			}
		}
		return nil

	case reflect.Slice, reflect.Array:
		repeated := isSingle(typ.Elem())
		for i := range val.Len() {
			elemKey := key
			if !repeated {
				elemKey = joinKey(key, strconv.Itoa(i))
			}
			if err := c.marshalAppend(q, elemKey, val.Index(i)); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("[%w] %s is not supported by the form url encoded format", codec.ErrNotSupported, typ)
	}
}

func (c Codec) Unmarshal(data []byte, ptr any) error {
	if ptr == nil {
		return fmt.Errorf("nil pointer received")
	}
	p := reflect.ValueOf(ptr)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("non pointer type received as ptr argument for Form URL Encoded Unmarshal")
	}
	if p.IsNil() {
		return fmt.Errorf("nil %s received as ptr argument for Form URL Encoded Unmarshal", p.Type())
	}
	if typ, _ := reflectkit.DerefType(p.Type().Elem()); typ.Kind() != reflect.Interface {
		if err := checkRoot(typ); err != nil {
			return err
		}
	}
	q, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	return c.unmarshalValue(q, "", p.Elem())
}

// unmarshalValue unmarshals the value under the key into dst.
// When the data has no key for it, dst is kept as it is,
// like encoding/json keeps a field which the JSON doesn't have.
func (c Codec) unmarshalValue(q url.Values, key string, dst reflect.Value) error {
	if !hasKey(q, key) {
		return nil
	}
	if raws, ok := q[key]; ok && key != "" && isSingle(dst.Type()) {
		if len(raws) == 0 {
			return nil
		}
		return unmarshalText(raws[0], dst)
	}
	switch dst.Kind() {
	case reflect.Pointer:
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return c.unmarshalValue(q, key, dst.Elem())

	case reflect.Interface:
		if dst.NumMethod() != 0 {
			return errUnknownImplementation(dst.Type())
		}
		// Like encoding/json reads a JSON object into an any, the keys under the key make a map[string]any.
		m := reflect.ValueOf(map[string]any{})
		if err := c.unmarshalMap(q, key, m); err != nil {
			return err
		}
		dst.Set(m)
		return nil

	case reflect.Struct:
		typ := dst.Type()
		for i := range typ.NumField() {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			prop := c.getFormProperties(field)
			if prop.Skip {
				continue
			}
			if err := c.unmarshalValue(q, joinKey(key, prop.Name), dst.Field(i)); err != nil {
				return err
			}
		}
		return nil

	case reflect.Map:
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		return c.unmarshalMap(q, key, dst)

	case reflect.Slice, reflect.Array:
		return c.unmarshalList(q, key, dst)

	default:
		return fmt.Errorf("[%w] %s is not supported by the form url encoded format", codec.ErrNotSupported, dst.Type())
	}
}

func (c Codec) unmarshalMap(q url.Values, key string, dst reflect.Value) error {
	typ := dst.Type()
	for _, mKey := range mapKeys(q, key, isFlat(typ.Elem())) {
		k := reflect.New(typ.Key()).Elem()
		if err := unmarshalText(mKey, k); err != nil {
			return fmt.Errorf("failed to parse key value for type %s: %w", typ.Key(), err)
		}
		v := reflect.New(typ.Elem()).Elem()
		if err := c.unmarshalValue(q, joinKey(key, mKey), v); err != nil {
			return err
		}
		dst.SetMapIndex(k, v)
	}
	return nil
}

func (c Codec) unmarshalList(q url.Values, key string, dst reflect.Value) error {
	if isSingle(dst.Type().Elem()) {
		raws := q[key]
		resetList(dst, len(raws))
		for i := range min(len(raws), dst.Len()) {
			if err := unmarshalText(raws[i], dst.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}
	length, err := listLength(q, key)
	if err != nil {
		return err
	}
	resetList(dst, length)
	for i := range min(length, dst.Len()) {
		if err := c.unmarshalValue(q, joinKey(key, strconv.Itoa(i)), dst.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

// resetList makes dst a list of zero values with the given length,
// or for an array, which has a length of its own, it zeroes it,
// like encoding/json zeroes the elements of an array which the JSON doesn't have.
func resetList(dst reflect.Value, length int) {
	if dst.Kind() == reflect.Array {
		dst.SetZero()
		return
	}
	dst.Set(reflect.MakeSlice(dst.Type(), length, length))
}

// listLength tells the length of the list under the key, from the highest index the data has under it.
//
// An index is expected to be lower than the number of keys in the form,
// so a form can't make the codec allocate a list much larger than the form itself.
func listLength(q url.Values, key string) (int, error) {
	var length int
	for k := range q {
		rest, ok := cutKey(k, key)
		if !ok {
			continue
		}
		raw, _, _ := strings.Cut(rest, ".")
		index, err := strconv.Atoi(raw)
		if err != nil || index < 0 {
			continue
		}
		if len(q) <= index {
			return 0, fmt.Errorf("the index of %q is out of the range of the form, which has only %d keys", k, len(q))
		}
		length = max(length, index+1)
	}
	return length, nil
}

// unmarshalText unmarshals a single value from its text into dst.
func unmarshalText(raw string, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.Pointer:
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return unmarshalText(raw, dst.Elem())

	case reflect.Interface:
		if dst.NumMethod() != 0 {
			return errUnknownImplementation(dst.Type())
		}
		v, err := convkit.DuckParse(raw)
		if err != nil {
			return err
		}
		if v == nil {
			dst.SetZero()
			return nil
		}
		dst.Set(reflect.ValueOf(v))
		return nil

	default:
		return convkit.UnmarshalReflect(dst.Type(), []byte(raw), dst.Addr(), textOptions)
	}
}

func errUnknownImplementation(typ reflect.Type) error {
	return fmt.Errorf("unable to unmarshal into %s, as the form doesn't tell which implementation to make", typ)
}

// textOptions makes a time.Time written the way its MarshalText writes it, like encoding/json does.
var textOptions = convkit.Options{TimeLayout: time.RFC3339Nano}

var typeTextMarshaler = reflect.TypeFor[encoding.TextMarshaler]()

// isText tells whether a value of typ is written as a single text.
func isText(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Interface:
		return false
	}
	// convkit has the text codecs registered to it, and time.Time is one of them.
	if convkit.IsRegistered[any](reflect.Zero(typ).Interface()) {
		return true
	}
	return typ.Implements(typeTextMarshaler) || reflect.PointerTo(typ).Implements(typeTextMarshaler)
}

// isSingle tells whether a value of typ is a single value, which is written as a text under its key.
// An interface counts as one, since its text is read like encoding/json reads a JSON value into an any.
func isSingle(typ reflect.Type) bool {
	typ, _ = reflectkit.DerefType(typ)
	return typ.Kind() == reflect.Interface || isText(typ)
}

// isFlat tells whether a value of typ is written right at its own key, without keys under it,
// like a single value, or a list of them, which repeats its key for each.
func isFlat(typ reflect.Type) bool {
	if isSingle(typ) {
		return true
	}
	typ, _ = reflectkit.DerefType(typ)
	return (typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array) && isSingle(typ.Elem())
}

// checkRoot tells whether a value of typ can be a form on its own, which is a list of key-value pairs.
func checkRoot(typ reflect.Type) error {
	if !isText(typ) && (typ.Kind() == reflect.Struct || typ.Kind() == reflect.Map) {
		return nil
	}
	return fmt.Errorf("[%w] the form url encoded format is a list of key-value pairs, so it can't hold a %s on its own",
		codec.ErrNotSupported, typ)
}

// mapKeys returns the keys of the map under the key.
//
// A flat value is right at its map key, so the rest of the key is the map key, dots included.
// Other values have their keys under the map key, so the map key ends at the next dot,
// which is why only a map of flat values supports a map key with a dot in it.
func mapKeys(q url.Values, key string, flat bool) []string {
	keys := map[string]struct{}{}
	for k := range q {
		rest, ok := cutKey(k, key)
		if !ok || rest == "" {
			continue
		}
		if !flat {
			rest, _, _ = strings.Cut(rest, ".")
		}
		keys[rest] = struct{}{}
	}
	return slices.Sorted(maps.Keys(keys))
}

// hasKey tells whether the data has the key, or a key under it.
func hasKey(q url.Values, key string) bool {
	if key == "" {
		return 0 < len(q)
	}
	if _, ok := q[key]; ok {
		return true
	}
	for k := range q {
		if _, ok := cutKey(k, key); ok {
			return true
		}
	}
	return false
}

// cutKey returns the part of k which is under the key.
func cutKey(k, key string) (string, bool) {
	if key == "" {
		return k, true
	}
	return strings.CutPrefix(k, key+".")
}

func joinKey(key, name string) string {
	if key == "" {
		return name
	}
	return key + "." + name
}

// deref returns the value which v holds behind its pointers and interfaces,
// or the zero reflect.Value, when one of them is nil.
func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

type formProperties struct {
	Name      string
	OmitEmpty bool
	// Skip is set by the "-" tag: the field is neither written nor read.
	Skip bool
}

func (c Codec) getFormProperties(typ reflect.StructField) formProperties {
	var prop formProperties
	prop.Name = c.fmtKey(typ.Name)
	c.lookupTag(typ, "url", &prop)
	c.lookupTag(typ, "form", &prop)
	return prop
}

func (c Codec) lookupTag(typ reflect.StructField, tagKey string, prop *formProperties) {
	tag, ok := typ.Tag.Lookup(tagKey)
	if !ok || len(tag) == 0 {
		return
	}
	// Like encoding/json, only "-" on its own leaves the field out; "-," names it "-".
	prop.Skip = tag == "-"
	name, opts, _ := strings.Cut(tag, ",")
	if name != "" {
		prop.Name = name
	}
	for _, opt := range strings.Split(opts, ",") {
		switch strings.TrimSpace(opt) {
		case "omitempty":
			prop.OmitEmpty = true
		}
	}
}
