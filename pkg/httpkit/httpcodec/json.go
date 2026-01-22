package httpcodec

import (
	"encoding/json"
	"errors"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
)

type JSON[T any] struct {
	jsonkit.Codec[T]
}

type JSONLines[T any] struct {
	jsonkit.LinesCodec[T]
	jsonLines[T]
}

func (JSONLines[T]) NewStreamEncoder(w io.Writer) codec.StreamEncoder {

}

func JSONLines[T any]() Codec[T] {
	var jsonlnc jsonLines[T]
	return Codec[T]{
		Marshal:   jsonlnc.Marshal,
		Unmarshal: jsonlnc.Unmarshal,
		List: ListCodec[T]{
			MarshalTFunc:   jsonlnc.MarshalList,
			UnmarshalTFunc: jsonlnc.UnmarshalList,

			NewEncoder: jsonlnc.NewListEncoder,
			NewDecoder: jsonlnc.NewListDecoder,
		},
	}
}

type jsonLines[T any] struct{}

func (jsonLines[T]) Marshal(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonLines[T]) Unmarshal(data []byte, p *T) error {
	return json.Unmarshal(data, p)
}

func (jl jsonLines[T]) UnmarshalList(data []byte, p *[]T) error {

}

func (jsonLines[T]) NewListEncoder(w io.Writer) codec.StreamEncoderT[T] {
	return jsonkit.NewEncoder[T](w)
}

func (jsonLines[T]) NewListDecoder(r io.Reader) codec.StreamDecoderT[T] {
	return func(yield func(codec.DecoderT[T], error) bool) {
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
