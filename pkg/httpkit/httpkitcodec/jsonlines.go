package httpkitcodec

import (
	"encoding/json"
	"errors"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
)

// MediaType: application/stream+json

type JSONLines[T any] struct{}

var jsonLinesMediaTypes = map[string]struct{}{
	"application/x-ndjson":    {},
	"application/stream+json": {},
	"application/json-stream": {},
}

func (JSONLines[T]) SupporsMediaType(mediaType string) bool {
	_, ok := jsonLinesMediaTypes[mediaType]
	return ok
}

func (JSONLines[T]) Marshal(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONLines[T]) Unmarshal(data []byte, p *T) error {
	return json.Unmarshal(data, p)
}

func (JSONLines[T]) NewListEncoder(w io.Writer) codec.StreamEncoder[T] {
	return jsonkit.NewEncoder[T](w)
}

func (JSONLines[T]) NewListDecoder(r io.Reader) codec.StreamDecoder[T] {
	return func(yield func(codec.Decoder[T], error) bool) {
		dec := json.NewDecoder(r)

		for {
			var raw json.RawMessage
			err := dec.Decode(&raw)

			if err != nil {
				if errors.Is(err, io.EOF) {
					return // done
				}
				yield(nil, err)
				return
			}

			var elem = codec.DecoderFunc[T](func(p *T) error {
				return json.Unmarshal(raw, p)
			})

			if !yield(elem, nil) {
				return
			}
		}
	}
}
