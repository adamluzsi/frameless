package wfjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
)

// The workflow holds the variable values and the arguments of participants and conditions as `any`,
// so their JSON has to tell their type, for them to come back as the same type.
//
// A nil, bool or string, and a []any or map[string]any of values, is written as plain JSON, the way encoding/json writes it.
// Any other value is written by the Codec, the way it writes a value held by an interface:
// an int is written as a plain JSON number, like 42, and a float64 too, but with a fraction when it is a whole number, like 42.0,
// so the Codec reads them back as an int and a float64.
// A value of a type registered with the Codec carries its $type,
// and so does a number of another built-in type, like {"$type":"int64","$value":42},
// and a slice or a map of such a type, like a []workflow.Definition,
// and it is read back as that type, instead of as the generic shape of its JSON, like a number or a map[string]any.
// A value of a type without a type ID, like a time.Time or an unregistered struct, is written the way encoding/json writes it,
// so it is read back as the generic shape of its JSON, like a string or a map[string]any,
// while the values it holds in its interface fields still carry their $type.
//
// A value with a member of its own named like the $type, like a map[string]any with a "$type" key,
// would be read back as the value its $type names, so it is not written, but fails with jsonkit.ErrKeywordCollision.
// A map with a type ID, like a map[string]string, can have such a key, as its entries are the $map member of its envelope.
//
// History recorded before a whole float64 was written with a fraction holds it without one, like 42,
// and so does history recorded before the other built-in numbers carried their type ID, for every number,
// so there a whole number is read back as an int, and any other as a float64.

func marshalValue(c *jsonkit.Codec, v any) (json.RawMessage, error) {
	return marshalValueOnPath(c, v, &reftree.RecursionGuard{})
}

// marshalValueOnPath writes v, which is referenced from the []any and map[string]any values on the path,
// so a []any or map[string]any which leads back to one of them is reported as a cycle, like encoding/json does.
// The values written by the Codec are looked after by the Codec.
func marshalValueOnPath(c *jsonkit.Codec, v any, path *reftree.RecursionGuard) (json.RawMessage, error) {
	switch v := v.(type) {
	case nil, bool, string:
		return json.Marshal(v)

	case []any:
		if v == nil {
			return json.Marshal(v)
		}
		leave, err := enterValue(path, v)
		if err != nil {
			return nil, err
		}
		defer leave()
		elems := make([]json.RawMessage, len(v))
		for i, elem := range v {
			data, err := marshalValueOnPath(c, elem, path)
			if err != nil {
				return nil, err
			}
			elems[i] = data
		}
		return json.Marshal(elems)

	case map[string]any:
		if v == nil {
			return json.Marshal(v)
		}
		leave, err := enterValue(path, v)
		if err != nil {
			return nil, err
		}
		defer leave()
		members := make(map[string]json.RawMessage, len(v))
		for key, member := range v {
			data, err := marshalValueOnPath(c, member, path)
			if err != nil {
				return nil, err
			}
			members[key] = data
		}
		// The Codec writes the members the way encoding/json does,
		// and it reports a key named like its type member, which would be read back as the type of the value.
		return c.Marshal(members)

	default:
		// The Codec writes the value it is given to Marshal the way the caller reads it back,
		// into a type of the caller's choice, so it would be written without its $type.
		// The value is held by an `any`, so it is given to Marshal held by a jsonkit.Interface[any],
		// which has it written with its $type, the way an `any` field holds it.
		held, err := c.Marshal(jsonkit.Interface[any]{V: v})
		if err == nil {
			return held, nil
		}
		// encoding/json would write it with its member named like the $type, which would be read back as the type of the value.
		if errors.Is(err, jsonkit.ErrKeywordCollision) {
			return nil, err
		}
		// The Codec can't write every value encoding/json can,
		// like a struct whose non-empty interface field holds a value of a type which is not registered.
		// A value which encoding/json can't write either, like a cyclic one, fails the encoding/json way.
		return json.Marshal(v)
	}
}

// enterValue marks the []any or map[string]any v as being written on the path, until leave is called,
// and reports a cycle, with the error of encoding/json, when v is on the path already.
func enterValue(path *reftree.RecursionGuard, v any) (leave func(), _ error) {
	rv := reflect.ValueOf(v)
	if path.Seen(rv) {
		return nil, &json.UnsupportedValueError{Value: rv, Str: fmt.Sprintf("encountered a cycle via %s", rv.Type())}
	}
	return func() { path.Forget(rv) }, nil
}

// unmarshalValue reads a value written by marshalValue.
// A missing value, like an omitted field, is nil.
func unmarshalValue(c *jsonkit.Codec, data json.RawMessage) (any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var v any
	if err := c.Unmarshal(data, &v); err == nil {
		return v, nil
	}
	// The Codec reads plain JSON the way encoding/json does, except for an object with a $type member,
	// which it reads as the type the member names.
	// An object whose $type the Codec doesn't know, which an earlier version may have written, or a value the Codec can't write,
	// is read member by member, so its $type stays a member, the way encoding/json reads it,
	// while the values in it are still read by the Codec.
	//
	// data is a json.RawMessage cut out by encoding/json, so it starts with the first byte of the value.
	switch {
	case data[0] == '[':
		var elems []json.RawMessage
		if err := json.Unmarshal(data, &elems); err != nil {
			return nil, err
		}
		values := make([]any, len(elems))
		for i, elem := range elems {
			value, err := unmarshalValue(c, elem)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil

	case data[0] == '{':
		var members map[string]json.RawMessage
		if err := json.Unmarshal(data, &members); err != nil {
			return nil, err
		}
		values := make(map[string]any, len(members))
		for key, member := range members {
			value, err := unmarshalValue(c, member)
			if err != nil {
				return nil, err
			}
			values[key] = value
		}
		return values, nil
	}
	var plain any
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	return plain, nil
}

// marshalValues is marshalValue for a list of values, like the input of a participant.
// A nil list stays nil, and an empty one stays empty.
func marshalValues(c *jsonkit.Codec, vs []any) ([]json.RawMessage, error) {
	if vs == nil {
		return nil, nil
	}
	out := make([]json.RawMessage, len(vs))
	for i, v := range vs {
		data, err := marshalValue(c, v)
		if err != nil {
			return nil, err
		}
		out[i] = data
	}
	return out, nil
}

// unmarshalValues reads a list written by marshalValues.
func unmarshalValues(c *jsonkit.Codec, raws []json.RawMessage) ([]any, error) {
	if raws == nil {
		return nil, nil
	}
	out := make([]any, len(raws))
	for i, raw := range raws {
		v, err := unmarshalValue(c, raw)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
