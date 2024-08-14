package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	. "go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/assert"
)

var (
	_ codec.Codec            = jsonkit.Codec{}
	_ codec.ListDecoderMaker = jsonkit.Codec{}
	_ codec.ListEncoderMaker = jsonkit.Codec{}
)

func TestSerializer_serializer(t *testing.T) {
	exp := Foo{
		ID:  "1",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	ser := jsonkit.Codec{}
	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.True(t, json.Valid(data))

	var got Foo
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestSerializer_list(t *testing.T) {
	var (
		exp1 = rnd.Make(Foo{}).(Foo)
		exp2 = rnd.Make(Foo{}).(Foo)
		exp3 = rnd.Make(Foo{}).(Foo)
	)

	ser := jsonkit.Codec{}

	var buf bytes.Buffer

	enc := ser.MakeListEncoder(&buf)
	assert.NoError(t, enc.Encode(exp1))
	assert.NoError(t, enc.Encode(exp2))
	assert.NoError(t, enc.Encode(exp3))
	assert.NoError(t, enc.Close())

	assert.True(t, json.Valid(buf.Bytes()),
		"expected that the final output after close is a valid json")

	stub := iokit.StubReader{Data: buf.Bytes()}
	dec := ser.MakeListDecoder(&stub)

	var got1, got2, got3 Foo
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got1))
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got2))
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got3))
	assert.False(t, dec.Next())
	assert.NoError(t, dec.Err())
	assert.NoError(t, dec.Close())
	assert.True(t, stub.IsClosed())

	assert.Equal(t, exp1, got1)
	assert.Equal(t, exp2, got2)
	assert.Equal(t, exp3, got3)
}

func TestJSONStream_serializer(t *testing.T) {
	exp := Foo{
		ID:  "1",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	ser := jsonkit.LinesCodec{}
	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var got Foo
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestJSONStream_list(t *testing.T) {
	var (
		exp1 = rnd.Make(Foo{}).(Foo)
		exp2 = rnd.Make(Foo{}).(Foo)
		exp3 = rnd.Make(Foo{}).(Foo)
	)

	ser := jsonkit.LinesCodec{}

	var buf bytes.Buffer

	enc := ser.MakeListEncoder(&buf)
	assert.NoError(t, enc.Encode(exp1))
	assert.NoError(t, enc.Encode(exp2))
	assert.NoError(t, enc.Encode(exp3))
	assert.NoError(t, enc.Close())

	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		assert.True(t, json.Valid(line),
			"expected that each line is a valid json in the stream+json")
	}

	stub := iokit.StubReader{Data: buf.Bytes()}
	dec := ser.NewListDecoder(&stub)

	var got1, got2, got3 Foo
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got1))
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got2))
	assert.True(t, dec.Next())
	assert.NoError(t, dec.Decode(&got3))
	assert.False(t, dec.Next())
	assert.NoError(t, dec.Err())
	assert.NoError(t, dec.Close())
	assert.True(t, stub.IsClosed())

	assert.Equal(t, exp1, got1)
	assert.Equal(t, exp2, got2)
	assert.Equal(t, exp3, got3)
}

func TestJSONSerializer_NewListDecoder(t *testing.T) {
	t.Run("E2E", func(t *testing.T) {
		foos := []Foo{
			{
				ID:  "id1",
				Foo: "foo1",
				Bar: "bar1",
				Baz: "baz1",
			},
			{
				ID:  "id2",
				Foo: "foo2",
				Bar: "bar2",
				Baz: "baz2",
			},
		}
		data, err := json.Marshal(foos)
		assert.NoError(t, err)

		dec := jsonkit.Codec{}.MakeListDecoder(io.NopCloser(bytes.NewReader(data)))

		var (
			gotFoos    []Foo
			iterations int
		)
		for dec.Next() {
			iterations++
			var got Foo
			assert.NoError(t, dec.Decode(&got))
			gotFoos = append(gotFoos, got)
		}
		assert.NoError(t, dec.Err())
		assert.NoError(t, dec.Close())
		assert.Equal(t, foos, gotFoos)
		assert.Equal(t, 2, iterations)
	})
}

func TestJSONSerializer_NewListEncoder(t *testing.T) {
	t.Run("E2E", func(t *testing.T) {
		foos := []Foo{
			{
				ID:  "id1",
				Foo: "foo1",
				Bar: "bar1",
				Baz: "baz1",
			},
			{
				ID:  "id2",
				Foo: "foo2",
				Bar: "bar2",
				Baz: "baz2",
			},
		}

		var buf bytes.Buffer
		enc := jsonkit.Codec{}.MakeListEncoder(&buf)
		for _, foo := range foos {
			assert.NoError(t, enc.Encode(foo))
		}

		assert.NoError(t, enc.Close())
		var gotFoos []Foo
		assert.NoError(t, json.Unmarshal(buf.Bytes(), &gotFoos))
		assert.Equal(t, foos, gotFoos)
	})
}
