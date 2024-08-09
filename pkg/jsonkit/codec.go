package jsonkit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/ports/codec"
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

	br *bufio.Reader

	inList bool
	err    error
	done   bool
	data   []byte
}

func (c *jsonListDecoder) Next() bool {
	if c.done {
		return false
	}
	if c.err != nil {
		return false
	}
	if !c.inList {
		char, err := c.readRune()
		if err != nil {
			c.err = err
			return false
		}
		if char != '[' {
			c.err = fmt.Errorf("unexpected character, got %s, but expected %s", string(char), "[")
			return false
		}
		c.inList = true
	}

	data, ok := c.prepareForNextListItem()
	if !ok {
		return false
	}

	for !json.Valid(data) {
		char, err := c.readRune()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			c.err = err
			return false
		}
		data = append(data, []byte(string(char))...)
	}

	if !json.Valid(data) {
		c.err = fmt.Errorf("invalid json received: %s", string(data))
		return false
	}

	c.data = data
	return true
}

func (c *jsonListDecoder) prepareForNextListItem() ([]byte, bool) {
	var data []byte
	char, err := c.readRune()
	if errors.Is(err, io.EOF) {
		return data, false
	}
	if err != nil {
		c.err = err
		return data, false
	}
	if c.inList {
		if char == ']' { // we are done
			c.done = true
			return data, false
		}
		if char != ',' {
			data = append(data, []byte(string(char))...)
		}
	}
	return data, true
}

func (c *jsonListDecoder) readRune() (rune, error) {
	rn, _, err := c.reader().ReadRune()
	return rn, err
}

func (c *jsonListDecoder) reader() *bufio.Reader {
	if c.br == nil {
		c.br = bufio.NewReader(c.R)
	}
	return c.br
}

func (c *jsonListDecoder) Err() error {
	return c.err
}

func (c *jsonListDecoder) Decode(ptr any) error {
	return json.Unmarshal(c.data, ptr)
}

func (c *jsonListDecoder) Close() error {
	return c.R.Close()
}

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
