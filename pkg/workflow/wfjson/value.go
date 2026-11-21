package wfjson

import (
	"encoding/json"

	"go.llib.dev/frameless/pkg/jsonkit"
)

// Variables and recorded call arguments are read back into any, not a caller-supplied Go type.
func marshalValue(c *jsonkit.Codec, v any) (json.RawMessage, error) {
	return c.Marshal(&v)
}

func unmarshalValue(c *jsonkit.Codec, data json.RawMessage) (any, error) {
	if len(data) == 0 { // omitted field
		return nil, nil
	}
	var v any
	if err := c.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
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
