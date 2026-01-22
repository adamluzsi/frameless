package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	. "go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase/assert"
)

var (
	_ codec.Bundle         = jsonkit.Codec[any]{}
	_ codec.Marshaler      = jsonkit.Codec[any]{}
	_ codec.Unmarshaler    = jsonkit.Codec[any]{}
	_ codec.StreamProducer = jsonkit.Codec[any]{}
	_ codec.StreamConsumer = jsonkit.Codec[any]{}
	_ codec.StreamEncoder  = jsonkit.Codec[any]{}.NewStreamEncoder(nil)
	_ codec.StreamDecoder  = jsonkit.Codec[any]{}.NewStreamDecoder(nil)

	_ codec.Codec[Foo]             = jsonkit.Codec[Foo]{}
	_ codec.MarshalerT[Foo]        = jsonkit.Codec[Foo]{}
	_ codec.UnmarshalerT[Foo]      = jsonkit.Codec[Foo]{}
	_ codec.StreamProducerT[Foo]   = jsonkit.Codec[Foo]{}
	_ codec.StreamConsumerT[Foo]   = jsonkit.Codec[Foo]{}
	_ codec.StreamEncoderT[Foo]    = jsonkit.Codec[Foo]{}.NewStreamEncoderT(nil)
	_ codec.StreamDecoderT[Foo]    = jsonkit.Codec[Foo]{}.NewStreamDecoderT(nil)
	_ codec.SliceMarshalerT[Foo]   = jsonkit.Codec[Foo]{}
	_ codec.SliceUnmarshalerT[Foo] = jsonkit.Codec[Foo]{}
)

var (
	_ codec.Bundle         = jsonkit.LinesCodec[any]{}
	_ codec.Marshaler      = jsonkit.LinesCodec[any]{}
	_ codec.Unmarshaler    = jsonkit.LinesCodec[any]{}
	_ codec.StreamProducer = jsonkit.LinesCodec[any]{}
	_ codec.StreamConsumer = jsonkit.LinesCodec[any]{}
	_ codec.StreamEncoder  = jsonkit.LinesCodec[any]{}.NewStreamEncoder(nil)
	_ codec.StreamDecoder  = jsonkit.LinesCodec[any]{}.NewStreamDecoder(nil)

	_ codec.Codec[Foo]             = jsonkit.LinesCodec[Foo]{}
	_ codec.MarshalerT[Foo]        = jsonkit.LinesCodec[Foo]{}
	_ codec.UnmarshalerT[Foo]      = jsonkit.LinesCodec[Foo]{}
	_ codec.StreamProducerT[Foo]   = jsonkit.LinesCodec[Foo]{}
	_ codec.StreamConsumerT[Foo]   = jsonkit.LinesCodec[Foo]{}
	_ codec.StreamEncoderT[Foo]    = jsonkit.LinesCodec[Foo]{}.NewStreamEncoderT(nil)
	_ codec.StreamDecoderT[Foo]    = jsonkit.LinesCodec[Foo]{}.NewStreamDecoderT(nil)
	_ codec.SliceMarshalerT[Foo]   = jsonkit.LinesCodec[Foo]{}
	_ codec.SliceUnmarshalerT[Foo] = jsonkit.LinesCodec[Foo]{}
)

func TestSerializer_serializer(t *testing.T) {
	exp := Foo{
		ID:  "1",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	ser := jsonkit.Codec[Foo]{}
	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.True(t, json.Valid(data))

	var got Foo
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func Test_arrayStream(t *testing.T) {
	var (
		exp1 = rnd.Make(Foo{}).(Foo)
		exp2 = rnd.Make(Foo{}).(Foo)
		exp3 = rnd.Make(Foo{}).(Foo)
	)

	var buf bytes.Buffer
	enc := jsonkit.NewArrayStreamEncoder[Foo](&buf)
	assert.NoError(t, enc.Encode(exp1))
	assert.NoError(t, enc.Encode(exp2))
	assert.NoError(t, enc.Encode(exp3))
	assert.NoError(t, enc.Close())

	assert.True(t, json.Valid(buf.Bytes()),
		"expected that the final output after close is a valid json")

	stub := iokit.Stub{Data: buf.Bytes()}

	stream := jsonkit.NewArrayStreamDecoder[Foo](&stub)

	next, stop := iter.Pull2(stream)
	defer stop()

	var got1, got2, got3 Foo

	dec, err, ok := next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.DecodeT(&got1))

	dec, err, ok = next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.DecodeT(&got2))

	dec, err, ok = next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.DecodeT(&got3))

	_, _, ok = next()
	assert.False(t, ok)

	stop()
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

		stream := jsonkit.NewArrayStreamDecoder[Foo](bytes.NewReader(data))

		var (
			gotFoos    []Foo
			iterations int
		)
		for dec, err := range stream {
			assert.NoError(t, err)
			iterations++
			var got Foo
			assert.NoError(t, dec.DecodeT(&got))
			gotFoos = append(gotFoos, got)
		}
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
		enc := jsonkit.NewArrayStreamEncoder[Foo](&buf)
		for _, foo := range foos {
			assert.NoError(t, enc.Encode(foo))
		}

		assert.NoError(t, enc.Close())
		var gotFoos []Foo
		assert.NoError(t, json.Unmarshal(buf.Bytes(), &gotFoos))
		assert.Equal(t, foos, gotFoos)
	})
}
