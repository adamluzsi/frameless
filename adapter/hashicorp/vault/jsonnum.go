package vault

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
)

// jsonNumbersToStrings rewrites every JSON number found in data into a JSON string,
// leaving every other value untouched.
//
// HashiCorp Vault decodes the JSON request body into a map[string]interface{} on the
// server side, where the encoding/json package turns every JSON number into a float64.
// Integers that don't fit into float64's 53-bit mantissa are therefore silently rounded
// before they are ever stored. Encoding numbers as strings keeps Vault from parsing (and
// rounding) them; jsonStringsToNumbers performs the inverse transformation on read.
func jsonNumbersToStrings(data []byte) ([]byte, error) {
	v, err := decodeJSONUsingNumber(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(numbersToStrings(v))
}

func numbersToStrings(v any) any {
	switch v := v.(type) {
	case json.Number:
		return v.String()
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, elem := range v {
			out[key] = numbersToStrings(elem)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = numbersToStrings(elem)
		}
		return out
	default:
		return v
	}
}

// jsonStringsToNumbers is the inverse of jsonNumbersToStrings. Guided by dtoType,
// it rewrites JSON strings back into JSON numbers wherever the destination type is
// numeric, so the result can be unmarshaled into the DTO without loss of precision.
//
// Conversion is strictly type-directed: a JSON string is only turned back into a number
// when the matching DTO field (or map/slice element) is of a numeric kind. Genuine string
// fields are therefore never affected, regardless of their content. Fields typed as
// interface{} are left untouched, since their concrete type cannot be inferred.
func jsonStringsToNumbers(data []byte, dtoType reflect.Type) ([]byte, error) {
	v, err := decodeJSONUsingNumber(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(stringsToNumbers(v, dtoType))
}

func stringsToNumbers(v any, t reflect.Type) any {
	if t == nil {
		return v
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		if s, ok := v.(string); ok {
			if num, ok := toJSONNumber(s); ok {
				return num
			}
		}
		return v

	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		fields := jsonFieldTypesOf(t)
		out := make(map[string]any, len(m))
		for key, elem := range m {
			if ft, ok := lookupJSONFieldType(fields, key); ok {
				out[key] = stringsToNumbers(elem, ft)
			} else {
				out[key] = elem
			}
		}
		return out

	case reflect.Map:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		elemType := t.Elem()
		out := make(map[string]any, len(m))
		for key, elem := range m {
			out[key] = stringsToNumbers(elem, elemType)
		}
		return out

	case reflect.Slice, reflect.Array:
		s, ok := v.([]any)
		if !ok {
			return v
		}
		elemType := t.Elem()
		out := make([]any, len(s))
		for i, elem := range s {
			out[i] = stringsToNumbers(elem, elemType)
		}
		return out

	default:
		return v
	}
}

func decodeJSONUsingNumber(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// toJSONNumber reports whether s is a single, valid JSON number and, if so, returns it
// as a json.Number so it is re-encoded as a number rather than a quoted string.
func toJSONNumber(s string) (json.Number, bool) {
	if s == "" {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err != nil {
		return "", false
	}
	if dec.More() {
		return "", false
	}
	if n.String() != s {
		return "", false
	}
	return n, true
}

// jsonFieldTypesOf maps the JSON object keys of struct type t to their field types.
// Embedded (anonymous) fields are treated as regular fields; their promoted fields are
// not flattened, which is acceptable because such fields don't participate in the
// numeric precision round trip handled here.
func jsonFieldTypesOf(t reflect.Type) map[string]reflect.Type {
	out := make(map[string]reflect.Type, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, ok := jsonFieldName(f)
		if !ok {
			continue
		}
		out[name] = f.Type
	}
	return out
}

func jsonFieldName(f reflect.StructField) (string, bool) {
	tag, ok := f.Tag.Lookup("json")
	if !ok {
		return f.Name, true
	}
	name := tag
	if i := strings.IndexByte(tag, ','); i >= 0 {
		name = tag[:i]
	}
	if name == "-" {
		return "", false
	}
	if name == "" {
		return f.Name, true
	}
	return name, true
}

func lookupJSONFieldType(fields map[string]reflect.Type, key string) (reflect.Type, bool) {
	if t, ok := fields[key]; ok {
		return t, true
	}
	for name, t := range fields {
		if strings.EqualFold(name, key) {
			return t, true
		}
	}
	return nil, false
}
