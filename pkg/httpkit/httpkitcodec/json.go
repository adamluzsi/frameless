package httpkitcodec

import (
	"encoding/json"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
)

type JSON[T any] struct{}

var jsonMediaTypes = map[string]struct{}{
	"application/json":         {},
	"application/problem+json": {},
}

func (JSON[T]) SupporsMediaType(mediaType string) bool {
	_, ok := jsonMediaTypes[mediaType]
	return ok
}

func (JSON[T]) Marshal(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (JSON[T]) Unmarshal(data []byte, p *T) error {
	return json.Unmarshal(data, p)
}

func (JSON[T]) NewListEncoder(w io.Writer) codec.StreamEncoder[T] {
	return jsonkit.NewArrayStreamEncoder[T](w)
}

func (JSON[T]) NewListDecoder(r io.Reader) codec.StreamDecoder[T] {
	return jsonkit.NewArrayStreamDecoder[T](r)
}
