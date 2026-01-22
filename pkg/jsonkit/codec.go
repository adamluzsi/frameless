package jsonkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/port/codec"
)

type Codec[T any] struct{}

func (Codec[T]) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (Codec[T]) MarshalT(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (Codec[T]) Unmarshal(data []byte, p any) error {
	return json.Unmarshal(data, p)
}

func (Codec[T]) UnmarshalT(data []byte, p *T) error {
	return json.Unmarshal(data, p)
}

func (Codec[T]) MarshalSlice(vs []T) ([]byte, error) {
	return json.Marshal(vs)
}

func (Codec[T]) UnmarshalSlice(data []byte, p *[]T) error {
	return json.Unmarshal(data, p)
}

func (Codec[T]) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return &ArrayEncoder[any]{W: w}
}

func (Codec[T]) NewStreamEncoderT(w io.Writer) codec.StreamEncoderT[T] {
	return &ArrayEncoder[T]{W: w}
}

func (Codec[T]) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator[any]{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.Decoder, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

func (Codec[T]) NewStreamDecoderT(r io.Reader) codec.StreamDecoderT[T] {
	i := &jsontoken.ArrayIterator[T]{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.DecoderT[T], error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

//////////////

type LinesCodec[T any] struct{}

func (LinesCodec[T]) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (LinesCodec[T]) MarshalT(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (LinesCodec[T]) Unmarshal(data []byte, p any) error {
	return json.Unmarshal(data, p)
}

func (LinesCodec[T]) UnmarshalT(data []byte, p *T) error {
	return json.Unmarshal(data, p)
}

func (c LinesCodec[T]) MarshalSlice(vs []T) ([]byte, error) {
	var buf bytes.Buffer
	enc := c.NewStreamEncoder(&buf)
	for i, v := range vs {
		if err := enc.Encode(v); err != nil {
			return nil, fmt.Errorf("[%d] %w", i, err)
		}
	}
	return buf.Bytes(), nil
}

func (c LinesCodec[T]) UnmarshalSlice(data []byte, p *[]T) error {
	*p = make([]T, 0)
	for dec, err := range c.NewStreamDecoder(bytes.NewReader(data)) {
		if err != nil {
			return err
		}
		var v T
		if err := dec.Decode(&v); err != nil {
			return err
		}
		*p = append(*p, v)
	}
	return nil
}

func (LinesCodec[T]) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return &ArrayEncoder[any]{W: w}
}

func (LinesCodec[T]) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator[any]{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.Decoder, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

func (LinesCodec[T]) NewStreamEncoderT(w io.Writer) codec.StreamEncoderT[T] {
	return &ArrayEncoder[T]{W: w}
}

func (LinesCodec[T]) NewStreamDecoderT(r io.Reader) codec.StreamDecoderT[T] {
	i := &jsontoken.ArrayIterator[T]{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.DecoderT[T], error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

//////////////

func NewArrayStreamEncoder[T any](w io.Writer) *ArrayEncoder[T] {
	return &ArrayEncoder[T]{W: w}
}

type ArrayEncoder[T any] struct {
	W io.Writer

	bracketOpen bool
	index       int
	err         error
	done        bool
}

func (c *ArrayEncoder[T]) Encode(v any) error {
	if c.err != nil {
		return c.err
	}

	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	if 0 < c.index {
		if _, err := c.W.Write([]byte(`,`)); err != nil {
			c.err = err
			return err
		}
	}

	if _, err := c.W.Write(data); err != nil {
		c.err = err
		return err
	}

	c.index++
	return nil
}

func (c *ArrayEncoder[T]) EncodeT(v T) error {
	return c.Encode(v)
}

func (c *ArrayEncoder[T]) Close() error {
	if c.done {
		return c.err
	}
	c.done = true
	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}
	if c.bracketOpen {
		if err := c.endList(); err != nil {
			return err
		}
	}
	return nil
}

func (c *ArrayEncoder[T]) endList() error {
	if _, err := c.W.Write([]byte(`]`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = false
	return nil
}

func (c *ArrayEncoder[T]) beginList() error {
	if _, err := c.W.Write([]byte(`[`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = true
	return nil
}

func NewArrayStreamDecoder[T any](r io.Reader) codec.StreamDecoderT[T] {
	i := &jsontoken.ArrayIterator[T]{
		Context: context.Background(),
		Input:   r,
	}
	return func(yield func(codec.DecoderT[T], error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i, nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	}
}

func NewEncoder[T any](w io.Writer) *Encoder[T] {
	return &Encoder[T]{Encoder: json.NewEncoder(w)}
}

type Encoder[T any] struct{ *json.Encoder }

var _ codec.StreamEncoderT[int] = (*Encoder[int])(nil)

func (e *Encoder[T]) Close() error {
	return nil
}

func (e *Encoder[T]) Encode(v any) error {
	return e.Encoder.Encode(v)
}

func (e *Encoder[T]) EncodeT(v T) error {
	return e.Encoder.Encode(v)
}

func NewDecoder[T any](r io.Reader) *Decoder[T] {
	var rc io.ReadCloser
	if v, ok := r.(io.ReadCloser); ok {
		rc = v
	} else {
		rc = io.NopCloser(r)
	}
	return &Decoder[T]{
		Decoder: json.NewDecoder(rc),
		Closer:  rc,
	}
}

type Decoder[T any] struct {
	*json.Decoder
	io.Closer
}

func (d *Decoder[T]) Decode(p *T) error {
	return d.Decoder.Decode(p)
}
