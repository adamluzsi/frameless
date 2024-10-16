package jsonkit

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/port/codec"
)

type Codec struct{}

func (s Codec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s Codec) Unmarshal(data []byte, dtoPtr any) error {
	return json.Unmarshal(data, &dtoPtr)
}

func (s Codec) MakeListEncoder(w io.Writer) codec.ListEncoder {
	return &jsonListEncoder{W: w}
}

func (s Codec) MakeListDecoder(r io.Reader) codec.ListDecoder {
	return &jsonListDecoder{R: iokit.NewKeepAliveReader(r, 5*time.Second)}
}

type jsonListEncoder struct {
	W io.Writer

	bracketOpen bool
	index       int
	err         error
	done        bool
}

func (c *jsonListEncoder) Encode(dto any) error {
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

func (c *jsonListEncoder) Close() error {
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

func (c *jsonListEncoder) endList() error {
	if _, err := c.W.Write([]byte(`]`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = false
	return nil
}

func (c *jsonListEncoder) beginList() error {
	if _, err := c.W.Write([]byte(`[`)); err != nil {
		c.err = err
		return err
	}
	c.bracketOpen = true
	return nil
}

type jsonListDecoder struct {
	R io.ReadCloser

	dec *json.Decoder

	inList bool

	err  error
	done bool

	data []byte
}

func (c *jsonListDecoder) Next() bool {
	if c.done {
		return false
	}
	if c.err != nil {
		return false
	}
	if c.dec == nil {
		c.dec = json.NewDecoder(c.R)
	}

	if !c.inList {
		tkn, err := c.dec.Token()
		if err != nil {
			c.err = err
			return false
		}
		delim, ok := tkn.(json.Delim)
		if !ok {
			c.err = jsontoken.ErrMalformedF("unexpecte json token: %v", tkn)
			return false
		}
		if delim != '[' {
			c.err = jsontoken.ErrMalformedF(`unexpecte json token delimiter, expected "%c" but got "%s"`, '[', delim)
			return false
		}
		c.inList = true
	}

	if !c.dec.More() {
		tkn, err := c.dec.Token()
		if err != nil {
			c.err = err
		}

		delim, ok := tkn.(json.Delim)
		if !ok {
			c.err = jsontoken.ErrMalformedF("unexpecte json token: %v", tkn)
			return false
		}
		if delim != ']' {
			c.err = jsontoken.ErrMalformedF(`unexpecte json token delimiter, expected "%c" but got "%s"`, ']', delim)
			return false
		}

		c.done = true
		return false
	}

	var raw json.RawMessage
	if err := c.dec.Decode(&raw); err != nil {
		c.err = err
		return false
	}

	c.data = raw
	return true
}

func (c *jsonListDecoder) Err() error           { return c.err }
func (c *jsonListDecoder) Decode(ptr any) error { return json.Unmarshal(c.data, ptr) }
func (c *jsonListDecoder) Close() error         { return c.R.Close() }

// LinesCodec is a json codec that uses the application/jsonlines
type LinesCodec struct{}

func (s LinesCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s LinesCodec) Unmarshal(data []byte, ptr any) error {
	return json.Unmarshal(data, ptr)
}

func (s LinesCodec) MakeListEncoder(w io.Writer) codec.ListEncoder {
	return jsonEncoder{Encoder: json.NewEncoder(w)}
}

type jsonEncoder struct {
	Encoder interface{ Encode(v any) error }
}

func (e jsonEncoder) Encode(v any) error {
	return e.Encoder.Encode(v)
}

func (jsonEncoder) Close() error { return nil }

func (s LinesCodec) NewListDecoder(w io.ReadCloser) codec.ListDecoder {
	return &jsonDecoder{Decoder: json.NewDecoder(w), Closer: w}
}

type jsonDecoder struct {
	Decoder interface{ Decode(v any) error }
	Closer  io.Closer

	err error
	val json.RawMessage
}

func (i *jsonDecoder) Next() bool {
	if i.err != nil {
		return false
	}
	var next json.RawMessage
	err := i.Decoder.Decode(&next)
	if errors.Is(err, io.EOF) {
		return false
	}
	if err != nil {
		i.err = err
		return false
	}
	i.val = next
	return true
}

func (i *jsonDecoder) Err() error {
	return i.err
}

func (i jsonDecoder) Close() error {
	return i.Closer.Close()
}

func (i jsonDecoder) Decode(v any) error {
	return json.Unmarshal(i.val, v)
}
