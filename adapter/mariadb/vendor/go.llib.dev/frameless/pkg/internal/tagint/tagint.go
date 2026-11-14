// Package tagint holds the struct tag handlers that more than one frameless package understands,
// so a tag means the same thing everywhere it is used.
package tagint

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
)

// TimeLayout is the "layout" tag, which sets the layout used to parse a time.Time field from text.
//
// The layout follows the format of time.Parse, e.g. `layout:"2006-01-02"`.
var TimeLayout = reflectkit.TagHandler[string]{
	Name: "layout",
	Parse: func(field reflect.StructField, tagName, tagValue string) (string, error) {
		return tagValue, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, layout string) error {
		return nil
	},
}

// DefaultSeparator separates the list elements of a slice field's text value when the field has no separator tag.
const DefaultSeparator = ","

// Separator returns the convkit.Options#Separator to parse a slice field's raw text value with.
//
// It is the field's separator tag value, or DefaultSeparator when the field has none.
// When the raw value is a JSON array, it returns an empty separator, which makes convkit parse the value as JSON,
// so list elements can contain the separator.
// Only a JSON array can hold list elements, so any other JSON value,
// like the "42" in "APP_INTS=42", is a separator joined list with a single element.
func Separator(tagValue, raw string) string {
	if isJSONArray(raw) {
		return ""
	}
	return zerokit.Coalesce(tagValue, DefaultSeparator)
}

func isJSONArray(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "[") && json.Valid([]byte(raw))
}

var typeTextUnmarshaler = reflectkit.TypeOf[encoding.TextUnmarshaler]()

// IsJSONObject reports whether a field of the type takes its text value as a JSON object.
//
// That is the case for a struct, or a pointer to one, unless it has a text format of its own,
// like the types registered to convkit (e.g. time.Time), or an encoding.TextUnmarshaler.
func IsJSONObject(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return false
	}
	if convkit.IsRegistered[any](reflect.Zero(typ).Interface()) {
		return false
	}
	return !reflect.PointerTo(typ).Implements(typeTextUnmarshaler)
}

// ParseOnto parses a field's raw text value as the type of base, the field's current value,
// the same way convkit.ParseReflect does, except for a JSON object (see IsJSONObject).
//
// A JSON object is unmarshalled onto base, rather than onto a zero value,
// so it is merged onto base: the fields that the JSON object doesn't mention keep their value from base.
// It follows json.Unmarshal, so a non-nil pointer and a non-nil map of base are decoded into in place.
func ParseOnto(base reflect.Value, raw string, opts convkit.Options) (reflect.Value, error) {
	typ := base.Type()
	if opts.ParseFunc != nil || !IsJSONObject(typ) {
		return convkit.ParseReflect(typ, raw, opts)
	}
	ptr := reflect.New(typ)
	ptr.Elem().Set(base)
	if err := json.Unmarshal([]byte(raw), ptr.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

// opaqueTag is the "opaque" tag, with "norecurse" as its alias.
var opaqueTag = reflectkit.TagHandler[bool]{
	Name:  "opaque",
	Alias: []string{"norecurse"},
	Parse: func(field reflect.StructField, tagName, tagValue string) (bool, error) {
		if len(tagValue) == 0 {
			return true, nil
		}
		v, err := strconv.ParseBool(tagValue)
		if err != nil {
			return false, fmt.Errorf("%s field's %s tag has an invalid value: %q (expected a bool, or an empty value for true)",
				field.Name, tagName, tagValue)
		}
		return v, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, v bool) error {
		return nil
	},
	PanicOnParseError: true,
}

// IsOpaque reports whether a struct field is tagged as opaque with `opaque:""`, or with its alias `norecurse:""`.
//
// The value of an opaque field is not looked into for the tags of the frameless packages,
// like flag, arg, env, default or required.
// It is not an ignore: the field itself can still be an input through its own tags,
// e.g. a struct field taking a JSON value.
//
// Like other bool tags, an empty value means true, and `opaque:"false"` turns it off.
// An invalid value panics, as a struct tag can only be fixed in the source code.
func IsOpaque(field reflect.StructField) bool {
	v, _, _ := opaqueTag.Lookup(field)
	return v
}
