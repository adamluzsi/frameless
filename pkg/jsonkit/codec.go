package jsonkit

import (
	"context"
	"encoding/json"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/port/codec"
)

type Codec struct{}

func (Codec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (Codec) Unmarshal(data []byte, ptr any) error {
	return json.Unmarshal(data, ptr)
}

func (Codec) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return &ArrayEncoder[any]{W: w}
}

func (Codec) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator{
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

//////////////

type LinesCodec struct {
	// UseNumber causes the Decoder to unmarshal a number into an
	// interface value as a [Number] instead of as a float64.
	UseNumber             bool
	DisallowUnknownFields bool
}

func (LinesCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (LinesCodec) Unmarshal(data []byte, p any) error {
	return json.Unmarshal(data, p)
}

func (LinesCodec) NewStreamEncoder(w io.Writer) codec.StreamEncoder {
	return streamEncoder{Encoder: json.NewEncoder(w)}
}

type streamEncoder struct{ *json.Encoder }

func (streamEncoder) Close() error { return nil }

func (c LinesCodec) NewStreamDecoder(r io.Reader) codec.StreamDecoder {
	dec := json.NewDecoder(r)
	if c.UseNumber {
		dec.UseNumber()
	}
	if c.DisallowUnknownFields {
		dec.DisallowUnknownFields()
	}
	return func(yield func(dec codec.Decoder, err error) bool) {
		for dec.More() {
			if !yield(dec, nil) {
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

func (c *ArrayEncoder[T]) Encode(v T) error {
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

func NewArrayStreamDecoder(r io.Reader) codec.StreamDecoder {
	i := &jsontoken.ArrayIterator{
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

func NewEncoder[T any](w io.Writer) *Encoder[T] {
	return &Encoder[T]{Encoder: json.NewEncoder(w)}
}

type Encoder[T any] struct{ *json.Encoder }

func (e *Encoder[T]) Close() error {
	return nil
}

func (e *Encoder[T]) Encode(v T) error {
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
