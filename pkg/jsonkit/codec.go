package jsonkit

import (
	"context"
	"encoding/json"
	"io"

	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/port/codec"
)

// type Register struct{}
//
// func (Register) Marshal(v any) ([]byte, error) {
// 	return json.Marshal(v)
// }
//
// func (Register) Unmarshal(data []byte, dtoPtr any) error {
// 	return json.Unmarshal(data, &dtoPtr)
// }

type Codec[T any] struct{}

func (s Codec[T]) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s Codec[T]) Unmarshal(data []byte, dtoPtr any) error {
	return json.Unmarshal(data, &dtoPtr)
}

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

func (c *ArrayEncoder[T]) Encode(dto T) error {
	if c.err != nil {
		return c.err
	}

	if !c.bracketOpen {
		if err := c.beginList(); err != nil {
			return err
		}
	}

	data, err := json.Marshal(dto)
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

// LinesCodec is a json codec that uses the application/jsonlines format
type LinesCodec[T any] struct{}

func (s LinesCodec[T]) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s LinesCodec[T]) Unmarshal(data []byte, ptr any) error {
	return json.Unmarshal(data, ptr)
}

func (s LinesCodec[T]) MakeListEncoder(w io.Writer) *Encoder[T] {
	return NewEncoder[T](w)
}

func NewEncoder[T any](w io.Writer) *Encoder[T] {
	return &Encoder[T]{Encoder: json.NewEncoder(w)}
}

type Encoder[T any] struct{ *json.Encoder }

func (e *Encoder[T]) Encode(v T) error {
	return e.Encoder.Encode(v)
}

func (s LinesCodec[T]) NewListDecoder(rc io.ReadCloser) *Decoder[T] {
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
