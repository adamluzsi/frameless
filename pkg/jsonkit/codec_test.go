package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/testing/testent"
	. "go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var (
	_ codec.Codec          = jsonkit.Codec{}
	_ codec.Marshaler      = jsonkit.Codec{}
	_ codec.Unmarshaler    = jsonkit.Codec{}
	_ codec.StreamProducer = jsonkit.Codec{}
	_ codec.StreamConsumer = jsonkit.Codec{}
	_ codec.StreamEncoder  = jsonkit.Codec{}.NewStreamEncoder(nil)
	_ codec.StreamDecoder  = jsonkit.Codec{}.NewStreamDecoder(nil)
)

var (
	_ codec.Codec          = jsonkit.LinesCodec{}
	_ codec.Marshaler      = jsonkit.LinesCodec{}
	_ codec.Unmarshaler    = jsonkit.LinesCodec{}
	_ codec.StreamProducer = jsonkit.LinesCodec{}
	_ codec.StreamConsumer = jsonkit.LinesCodec{}
	_ codec.StreamEncoder  = jsonkit.LinesCodec{}.NewStreamEncoder(nil)
	_ codec.StreamDecoder  = jsonkit.LinesCodec{}.NewStreamDecoder(nil)
)

func TestCodec_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := testent.MakeFoo(t)

	ser := jsonkit.Codec{}
	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.True(t, json.Valid(data))

	var got Foo
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)

	vs := random.Slice(t.Random.IntBetween(3, 7), func() Foo {
		return testent.MakeFoo(t)
	}, random.UniqueValues)

	var buf bytes.Buffer
	enc := ser.NewStreamEncoder(&buf)

	for _, v := range vs {
		assert.NoError(t, enc.Encode(v))
	}
	assert.NoError(t, enc.Close())

	assert.True(t, json.Valid(buf.Bytes()), "expcted that json Budnle stream encoding produces a whole valid json value")

	var vsGOT []Foo
	assert.NoError(t, ser.Unmarshal(buf.Bytes(), &vsGOT))
	assert.Equal(t, vs, vsGOT)

	stream := ser.NewStreamDecoder(&buf)

	vsGOT = nil
	for elem, err := range stream {
		assert.NoError(t, err)

		var v Foo
		assert.NoError(t, elem.Decode(&v))
		vsGOT = append(vsGOT, v)
	}

	assert.Equal(t, vs, vsGOT)
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

	stream := jsonkit.NewArrayStreamDecoder(&stub)

	next, stop := iter.Pull2(stream)
	defer stop()

	var got1, got2, got3 Foo

	dec, err, ok := next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.Decode(&got1))

	dec, err, ok = next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.Decode(&got2))

	dec, err, ok = next()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.NoError(t, dec.Decode(&got3))

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

		stream := jsonkit.NewArrayStreamDecoder(bytes.NewReader(data))

		var (
			gotFoos    []Foo
			iterations int
		)
		for dec, err := range stream {
			assert.NoError(t, err)
			iterations++
			var got Foo
			assert.NoError(t, dec.Decode(&got))
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

var dataSmokeFoos = []byte(`[
	{
		"ID": "3",
		"Foo": "0 or 1=1",
		"Bar": "+++ATH0",
		"Baz": "ABC\u003cdiv style=\"x:exp\\x5Cression(javascript:alert(38)\"\u003eDEF"
	},
	{
		"ID": "2",
		"Foo": " ORDER BY 17# ",
		"Bar": "\u003cIMG SRC=\"jav\u0026#x0D;ascript:alert('217');\"\u003e",
		"Baz": " or '1'='1"
	}
]`)

func TestCodec_NewStreamDecoder_smoke(t *testing.T) {
	var exp []testent.Foo
	assert.NoError(t, json.Unmarshal(dataSmokeFoos, &exp))

	var c jsonkit.Codec

	stream := c.NewStreamDecoder(bytes.NewReader(dataSmokeFoos))

	var got []testent.Foo
	for elem, err := range stream {
		assert.NoError(t, err)

		var v testent.Foo
		assert.NoError(t, elem.Decode(&v))
		got = append(got, v)
	}

	assert.Equal(t, exp, got)
}

func TestCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) jsonkit.Codec {
		return jsonkit.Codec{}
	})

	s.Context("stream", func(s *testcase.Spec) {
		exp := let.Var(s, func(t *testcase.T) []testent.Foo {
			return random.Slice(t.Random.IntBetween(3, 7), testent.MakeFooFunc(t))
		})

		s.Test("Encode", func(t *testcase.T) {
			var buf bytes.Buffer
			enc := subject.Get(t).NewStreamEncoder(&buf)
			for _, v := range exp.Get(t) {
				assert.NoError(t, enc.Encode(v))
			}
			assert.NoError(t, enc.Close())

			var got []testent.Foo
			assert.NoError(t, json.Unmarshal(buf.Bytes(), &got))
			assert.Equal(t, exp.Get(t), got)
		})

		s.Test("Decode", func(t *testcase.T) {
			data, err := json.Marshal(exp.Get(t))
			assert.NoError(t, err)

			streamDec := subject.Get(t).NewStreamDecoder(bytes.NewReader(data))

			var got []testent.Foo
			for streamElem, err := range streamDec {
				assert.NoError(t, err)

				var v testent.Foo
				assert.NoError(t, streamElem.Decode(&v))
				got = append(got, v)
			}

			assert.Equal(t, exp.Get(t), got)
		})
	})
}

func TestLinesCodec_smoke(tt *testing.T) {
	s := testcase.NewSpec(tt)

	subject := let.Var(s, func(t *testcase.T) jsonkit.LinesCodec {
		return jsonkit.LinesCodec{}
	})

	s.Context("stream", func(s *testcase.Spec) {
		exp := let.Var(s, func(t *testcase.T) []testent.Foo {
			return random.Slice(t.Random.IntBetween(3, 7), testent.MakeFooFunc(t))
		})

		s.Test("Encode", func(t *testcase.T) {
			var buf bytes.Buffer
			enc := subject.Get(t).NewStreamEncoder(&buf)
			for _, v := range exp.Get(t) {
				assert.NoError(t, enc.Encode(v))
			}
			assert.NoError(t, enc.Close())

			var got []testent.Foo
			dec := json.NewDecoder(&buf)
			for dec.More() {
				var v testent.Foo
				assert.NoError(t, dec.Decode(&v))
				got = append(got, v)
			}
			assert.Equal(t, exp.Get(t), got)
		})
		s.Test("Decode", func(t *testcase.T) {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			for _, v := range exp.Get(t) {
				assert.NoError(t, enc.Encode(v))
			}

			streamDec := subject.Get(t).NewStreamDecoder(&buf)

			var got []testent.Foo
			for streamElem, err := range streamDec {
				assert.NoError(t, err)

				var v testent.Foo
				assert.NoError(t, streamElem.Decode(&v))
				got = append(got, v)
			}

			assert.Equal(t, exp.Get(t), got)
		})
	})
}
