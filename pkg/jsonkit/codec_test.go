package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	_ codec.Codec          = &jsonkit.Codec{}
	_ codec.Marshaler      = &jsonkit.Codec{}
	_ codec.Unmarshaler    = &jsonkit.Codec{}
	_ codec.StreamProducer = &jsonkit.Codec{}
	_ codec.StreamConsumer = &jsonkit.Codec{}
	_ codec.StreamEncoder  = (&jsonkit.Codec{}).NewStreamEncoder(nil)
	_ codec.StreamDecoder  = (&jsonkit.Codec{}).NewStreamDecoder(nil)
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

	ser := &jsonkit.Codec{}
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

	assert.True(t, json.Valid(buf.Bytes()), "expected that json bundle stream encoding produces a whole valid json value")

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

	var c = &jsonkit.Codec{}

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

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		return &jsonkit.Codec{}
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

func TestCodec_marshaling(t *testing.T) {
	s := testcase.NewSpec(t)

	c := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		var c = &jsonkit.Codec{}
		t.Cleanup(jsonkit.CodecRegisterTypeID[testent.Foo](c, "foo"))
		t.Cleanup(jsonkit.CodecRegisterTypeID[MT_PointerFoo](c, "pointer-foo"))
		t.Cleanup(jsonkit.CodecRegisterTypeID[MT_T14_E1](c, "e1"))
		t.Cleanup(jsonkit.CodecRegisterTypeID[MT_T14_E2](c, "e2"))
		t.Cleanup(jsonkit.CodecRegisterTypeID[MT_T14_E3](c, "e3"))
		t.Cleanup(jsonkit.CodecRegisterTypeID[MT_T14_E4](c, "e4"))
		return c
	})

	s.Test("interface", func(t *testcase.T) {
		var exp = MT_T1{
			Fooer: testent.MakeFoo(t),
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var got MT_T1
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("slice", func(t *testcase.T) {
		var exp = MT_T2{
			VS: random.Slice(t.Random.IntBetween(3, 7), func() testent.Fooer {
				return testent.MakeFoo(t)
			}),
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var got MT_T2
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("nested pointer, map and array", func(t *testcase.T) {
		exp := MT_T3{
			Nested: &MT_T1{Fooer: testent.MakeFoo(t)},
			ByName: map[string]testent.Fooer{
				t.Random.String(): testent.MakeFoo(t),
			},
			Array: [2]testent.Fooer{testent.MakeFoo(t), testent.MakeFoo(t)},
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T3
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("JSON field tags", func(t *testcase.T) {
		exp := MT_T4{Fooer: testent.MakeFoo(t), Ignored: testent.MakeFoo(t)}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"fooer"`)
		assert.NotContains(t, string(data), `"Ignored"`)

		var got MT_T4
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp.Fooer, got.Fooer)
		assert.Nil(t, got.Ignored)
	})

	s.Test("nil interface containers", func(t *testcase.T) {
		exp := MT_T3{}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T3
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("root pointer concrete value", func(t *testcase.T) {
		exp := &MT_PointerFoo{Value: t.Random.String()}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_PointerFoo
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, *exp, got)
	})

	s.Test("root slice of interfaces", func(t *testcase.T) {
		exp := random.Slice(t.Random.IntBetween(1, 7), func() testent.Fooer { return testent.MakeFoo(t) })
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got []testent.Fooer
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("pointer implementation in interface", func(t *testcase.T) {
		exp := MT_T1{Fooer: &MT_PointerFoo{Value: t.Random.String()}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T1
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("slice with nil interface elements", func(t *testcase.T) {
		exp := MT_T2{VS: []testent.Fooer{testent.MakeFoo(t), nil, testent.MakeFoo(t)}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T2
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("map with nil interface values", func(t *testcase.T) {
		exp := MT_T3{ByName: map[string]testent.Fooer{
			t.Random.String(): testent.MakeFoo(t),
			t.Random.String(): nil,
		}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T3
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("multi-dimensional slice of interfaces", func(t *testcase.T) {
		exp := MT_T5{Grid: [][]testent.Fooer{
			{testent.MakeFoo(t), testent.MakeFoo(t)},
			{},
			{nil, testent.MakeFoo(t)},
		}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T5
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("map of slices of interfaces", func(t *testcase.T) {
		exp := MT_T6{Groups: map[string][]testent.Fooer{
			t.Random.String(): {testent.MakeFoo(t), testent.MakeFoo(t)},
			t.Random.String(): {},
		}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T6
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("ordinary values coexist with polymorphic values", func(t *testcase.T) {
		exp := MT_T7{
			Name:    t.Random.String(),
			Count:   t.Random.Int(),
			Enabled: t.Random.Bool(),
			Labels:  random.Slice(t.Random.IntBetween(1, 5), t.Random.String),
			Meta:    map[string]int{t.Random.String(): t.Random.Int()},
			Fooer:   testent.MakeFoo(t),
		}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T7
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("named slice and map types", func(t *testcase.T) {
		exp := MT_T8{
			List: MT_Fooers{testent.MakeFoo(t), testent.MakeFoo(t)},
			Map:  MT_FooerMap{t.Random.String(): testent.MakeFoo(t)},
		}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T8
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("empty containers remain non-nil", func(t *testcase.T) {
		exp := MT_T8{List: MT_Fooers{}, Map: MT_FooerMap{}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T8
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
		assert.NotNil(t, got.List)
		assert.NotNil(t, got.Map)
	})

	s.Test("pointer to slice of interfaces", func(t *testcase.T) {
		list := MT_Fooers{testent.MakeFoo(t), testent.MakeFoo(t)}
		exp := MT_T9{List: &list}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T9
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("omitempty follows encoding json semantics", func(t *testcase.T) {
		data, err := c.Get(t).Marshal(MT_T10{})
		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(data))
	})

	s.Test("omitempty retains non-zero values", func(t *testcase.T) {
		exp := MT_T10{Name: t.Random.String(), List: MT_Fooers{testent.MakeFoo(t)}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T10
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("omitempty drops empty non-nil collections, omitzero keeps them", func(t *testcase.T) {
		emptyList := MT_Fooers{}
		// omitempty drops the empty (non-nil) slice; omitzero keeps it.
		expEmpty := MT_T11{List: emptyList}
		data, err := c.Get(t).Marshal(expEmpty)
		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(data))

		expKeep := MT_T12{List: emptyList}
		data, err = c.Get(t).Marshal(expKeep)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"list":[]`)

		// And both drop nil collections.
		expNilEmpty := MT_T11{}
		data, err = c.Get(t).Marshal(expNilEmpty)
		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(data))

		expNilZero := MT_T12{}
		data, err = c.Get(t).Marshal(expNilZero)
		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(data))
	})

	s.Test("omitzero omits zero values across supported kinds", func(t *testcase.T) {
		data, err := c.Get(t).Marshal(MT_T12{})
		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(data))

		exp := MT_T12{
			Name:    t.Random.String(),
			Count:   t.Random.Int(),
			Enabled: true,
			Labels:  []string{t.Random.String()},
			Fooer:   testent.MakeFoo(t),
		}
		data, err = c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T12
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("stringify quotes the value as a JSON string", func(t *testcase.T) {
		data, err := c.Get(t).Marshal(MT_T13{Count: 42, Enabled: true, Ratio: 0.5})
		assert.NoError(t, err)
		assert.Equal(t, `{"count":"42","enabled":"true","ratio":"0.5"}`, string(data))
	})

	s.Test("stringify round-trips through unmarshal", func(t *testcase.T) {
		exp := MT_T13{Count: 42, Enabled: true, Ratio: 0.5}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T13
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// s.Test("interface decode", func(t *testcase.T) {
	// 	// TODO
	// })

	s.Test("codec level type id registration is independent from other Codec instances", func(t *testcase.T) {
		var v = MT_T15_Reg{V: t.Random.String()}
		type T struct{ V MT_X_Interface }

		var exp T = T{V: v}

		_, err := c.Get(t).Marshal(exp)
		assert.Error(t, err, "expected that unregistered type will cause error on interface value marshaling")

		var c1, c2 = &jsonkit.Codec{}, &jsonkit.Codec{}
		defer jsonkit.CodecRegisterTypeID[MT_T15_Reg](c1, "t15")()

		data, err := c1.Marshal(exp)
		assert.NoError(t, err,
			"expected that globally registered type is recognised in the Codec")

		var got T
		assert.NoError(t, c1.Unmarshal(data, &got))

		assert.Equal(t, exp, got)

		_, err = c2.Marshal(exp)
		assert.Error(t, err,
			"expected that codec which is not aware of the registered type will fail to marshal the interface value")
	})

	s.Test("global type register integration", func(t *testcase.T) {
		var v = MT_T15_Reg{V: t.Random.String()}
		type T struct{ V MT_X_Interface }

		var exp T = T{V: v}

		_, err := c.Get(t).Marshal(exp)
		assert.Error(t, err, "expected that unregistered type will cause error on interface value marshaling")

		defer jsonkit.RegisterTypeID[MT_T15_Reg]("t15")()

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err,
			"expected that globally registered type is recognised in the Codec")

		var got T
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))

		assert.Equal(t, exp, got)
	})

	s.Test("nested", func(t *testcase.T) {
		var makeEI func() MT_X_Interface
		var makeE1 = func() MT_T14_E1 {
			return MT_T14_E1{
				A: t.Random.String(),
				B: random.Pick(t.Random, "", t.Random.String()),
				C: random.Pick(t.Random, "", t.Random.String()),
			}
		}
		var makeE2 = func() MT_T14_E2 {
			return MT_T14_E2{
				V: makeEI(),
			}
		}
		var makeE3 = func() MT_T14_E3 {
			return random.Slice(t.Random.IntBetween(0, 3), func() MT_X_Interface {
				return makeEI()
			})
		}
		var makeE4 = func() MT_T14_E4 {
			return random.Map(t.Random.IntB(0, 3), func() (string, MT_X_Interface) {
				return t.Random.HexN(5), makeEI()
			})
		}
		makeEI = func() MT_X_Interface {
			return random.Pick(t.Random,
				func() MT_X_Interface { return nil },
				func() MT_X_Interface { return makeE1() },
				func() MT_X_Interface { return makeE2() },
				func() MT_X_Interface { return makeE3() },
			)()
		}
		exp := MT_T14{List: []MT_X_Interface{
			nil, // nil intentionally
			makeE1(),
			makeE2(),
			makeE3(),
			makeE4(),
		}}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T14
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("race", func(t *testcase.T) {
		var c jsonkit.Codec
		t.Cleanup(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo"))
		testcase.Race(func() {
			_, _ = c.Marshal("hello")
		}, func() {
			_, _ = c.Marshal("world")
		}, func() {
			data, err := c.Marshal(MT_T1{Fooer: MakeFoo(t)})
			if err != nil {
				t.Error(err.Error())
				return
			}

			var got MT_T1
			assert.Should(t).NoError(c.Unmarshal(data, &got))
		})
	})
}

type MT_X_Interface interface{ X() }

type MT_T1 struct {
	Fooer testent.Fooer
}

type MT_T2 struct {
	VS []testent.Fooer
}

type MT_T3 struct {
	Nested *MT_T1
	ByName map[string]testent.Fooer
	Array  [2]testent.Fooer
}

type MT_T4 struct {
	Fooer   testent.Fooer `json:"fooer"`
	Ignored testent.Fooer `json:"-"`
}

type MT_T5 struct {
	Grid [][]testent.Fooer
}

type MT_T6 struct {
	Groups map[string][]testent.Fooer
}

type MT_T7 struct {
	Name    string
	Count   int
	Enabled bool
	Labels  []string
	Meta    map[string]int
	Fooer   testent.Fooer
}

type MT_Fooers []testent.Fooer
type MT_FooerMap map[string]testent.Fooer

type MT_T8 struct {
	List MT_Fooers
	Map  MT_FooerMap
}

type MT_T9 struct {
	List *MT_Fooers
}

type MT_T10 struct {
	Fooer testent.Fooer `json:"fooer,omitempty"`
	List  MT_Fooers     `json:"list,omitempty"`
	Name  string        `json:"name,omitempty"`
}

type MT_T11 struct {
	List MT_Fooers `json:"list,omitempty"`
}

type MT_T12 struct {
	Fooer   testent.Fooer `json:"fooer,omitzero"`
	List    MT_Fooers     `json:"list,omitzero"`
	Name    string        `json:"name,omitzero"`
	Count   int           `json:"count,omitzero"`
	Enabled bool          `json:"enabled,omitzero"`
	Labels  []string      `json:"labels,omitzero"`
}

type MT_T13 struct {
	Count   int     `json:"count,string"`
	Enabled bool    `json:"enabled,string"`
	Ratio   float64 `json:"ratio,string"`
}

type MT_PointerFoo struct{ Value string }

func (v *MT_PointerFoo) GetFoo() string { return v.Value }

type MT_T14 struct {
	IntVal Fooer
	List   []MT_X_Interface
}

type MT_T14_E1 struct {
	A string `json:"a"`
	B string `json:"b,omitempty"`
	C string `json:"c,omitzero"`
}

func (MT_T14_E1) X() {}

type MT_T14_E2 struct {
	V MT_X_Interface
}

func (MT_T14_E2) X() {}

type MT_T14_E3 []MT_X_Interface

func (MT_T14_E3) X() {}

type MT_T14_E4 map[string]MT_X_Interface

func (MT_T14_E4) X() {}

type MT_T15_Reg struct {
	V string `json:"v"`
}

var _ MT_X_Interface = MT_T15_Reg{}

func (MT_T15_Reg) X() {}

type MT_JM_Box struct {
	Inner MT_JM_Inner
}

type MT_JM_Inner struct {
	V string `json:"v"`
}

func (v MT_JM_Inner) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(map[string]string{"wrapped": v.V})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (v *MT_JM_Inner) UnmarshalJSON(data []byte) error {
	var wrap struct {
		Wrapped string `json:"wrapped"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return err
	}
	v.V = wrap.Wrapped
	return nil
}

type MT_JM_Slice struct {
	Items []MT_JM_Inner
}

type MT_JM_Map struct {
	Items map[string]MT_JM_Inner
}

type MT_JM_PointerStruct struct {
	Inner *MT_JM_Inner
}

type MT_JM_Interface struct {
	Fooer testent.Fooer
}

type MT_JM_PolyBox struct {
	V jsonkit.Interface[MT_X_Interface]
}

type MT_JM_PolyReg struct{}

func (MT_JM_PolyReg) X() {}

var _ MT_X_Interface = MT_JM_PolyReg{}

func (MT_JM_PolyReg) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"v": "poly"})
}

func (v *MT_JM_PolyReg) UnmarshalJSON(data []byte) error {
	var wrap struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return err
	}
	return nil
}

type MT_JM_PolyFooerReg struct {
	V string
}

func (v MT_JM_PolyFooerReg) GetFoo() string { return v.V }
func (MT_JM_PolyFooerReg) X()               {}

func (MT_JM_PolyFooerReg) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"v": "poly"})
}

func (v *MT_JM_PolyFooerReg) UnmarshalJSON(data []byte) error {
	var wrap struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return err
	}
	v.V = wrap.V
	return nil
}

func roundTrip[T any](t *testcase.T, c *jsonkit.Codec, exp T) {
	data, err := c.Marshal(exp)
	assert.NoError(t, err)
	assert.True(t, json.Valid(data))

	var got T
	assert.NoError(t, c.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestCodec_jsonMarshaler(t *testing.T) {
	s := testcase.NewSpec(t)
	var c jsonkit.Codec

	defer jsonkit.RegisterTypeID[MT_JM_PolyFooerReg]("jm_poly")()
	defer jsonkit.RegisterTypeID[MT_JM_PolyReg]("jm_poly_reg")()

	s.Test("struct field implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		exp := MT_JM_Box{Inner: MT_JM_Inner{V: t.Random.String()}}
		roundTrip(t, &c, exp)
	})

	s.Test("slice of json.Marshaler elements", func(t *testcase.T) {
		exp := MT_JM_Slice{Items: random.Slice(t.Random.IntBetween(1, 5), func() MT_JM_Inner {
			return MT_JM_Inner{V: t.Random.String()}
		})}
		roundTrip(t, &c, exp)
	})

	s.Test("map of json.Marshaler values", func(t *testcase.T) {
		exp := MT_JM_Map{Items: map[string]MT_JM_Inner{
			t.Random.String(): {V: t.Random.String()},
			t.Random.String(): {V: t.Random.String()},
		}}
		roundTrip(t, &c, exp)
	})

	s.Test("json.Marshaler returned wire format takes precedence over @type wrapping", func(t *testcase.T) {
		str := t.Random.String()

		exp := MT_JM_Box{Inner: MT_JM_Inner{V: str}}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		expStrBytes, err := json.Marshal(str)
		assert.NoError(t, err)
		assert.Contains(t, string(data), fmt.Sprintf(`"wrapped":%s`, string(expStrBytes)))
		assert.NotContains(t, string(data), `"@type"`)

		roundTrip(t, &c, exp)
	})

	s.Test("jsonkit.Interface wrapping a json.Marshaler implementation", func(t *testcase.T) {
		exp := MT_JM_PolyBox{V: jsonkit.Interface[MT_X_Interface]{V: MT_JM_PolyReg{}}}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"v":"poly"`)

		var got MT_JM_PolyBox
		assert.NoError(t, c.Unmarshal(data, &got))
		_, ok := got.V.V.(MT_JM_PolyReg)
		assert.True(t, ok)
	})

	s.Test("interface field holding a json.Marshaler implementation includes both @type and the marshaler's wire format", func(t *testcase.T) {
		exp := MT_PolyMarshalerBox{Inner: MT_JM_PolyReg{}}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		assert.Contains(t, string(data), `"@type":"jm_poly_reg"`,
			"jsonkit should add the @type discriminator for the interface element")
		assert.Contains(t, string(data), `"v":"poly"`,
			"the value's json.Marshaler should be honored for the wire format")

		var got MT_PolyMarshalerBox
		assert.NoError(t, c.Unmarshal(data, &got))
		_, ok := got.Inner.(MT_JM_PolyReg)
		assert.True(t, ok, "the unmarshaled value should be the original concrete type")
		assert.Equal[MT_JM_PolyReg](t, MT_JM_PolyReg{}, got.Inner.(MT_JM_PolyReg))
	})

	s.Test("typed envelope dispatches struct fields through the codec registry", func(t *testcase.T) {
		defer jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo")()
		defer jsonkit.CodecRegisterTypeID[MT_TypedContainer](&c, "typed_container")()
		inner := testent.MakeFoo(t)
		var iface testent.Fooer = inner
		container := MT_TypedContainer{Fooer: iface}
		data, err := c.Marshal(container)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"@type":"foo"`)
		assert.Contains(t, string(data), `"@type":"typed_container"`)

		var got MT_TypedContainer
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal[testent.Fooer](t, inner, got.Fooer)
	})
}

// TestCodec_marshalArrayInterface_describesWireFormat documents the
// public-API behavior of jsonkit.Codec when marshaling a slice of an
// interface type, expressed either as a plain []T or as a
// jsonkit.Array[T] wrapper. It doubles as a runnable example for users
// who serialize polymorphic collections (e.g. into a document store or
// message queue).
//
// What the user observes on the wire — the bytes they would store in
// their database — does not depend on which form they use. Both
// produce the same JSON: a top-level array, where each element carries
// its own @type envelope. There is no outer @type around the slice
// itself, because the polymorphic value lives one level inside, per
// element.
//
// What the user observes on the round-trip — what they get back when
// they read the bytes later — is also identical between the two forms.
// A value stored today as []Shape is read back today as []Shape; the
// same value stored as jsonkit.Array[Shape] is read back the same way.
// There is no obligation to pick one form over the other for storage
// compatibility.
//
// Per-codec isolation: each jsonkit.Codec instance carries its own
// per-instance type registry. The wire format is determined by the
// codec that produced it. Two codecs that disagree on how to name the
// same concrete type emit different @type values, and a wire from one
// codec cannot be decoded by the other. This is the property that
// lets a user migrate their storage layout gradually (or run multiple
// codec versions side by side) without clashes.
//
// End-user example (this whole test is runnable):
//
//	type Shape interface{ Area() float64 }
//	type Circle struct{ R float64 }
//	func (c Circle) Area() float64 { return 3.14 * c.R * c.R }
//
//	var c jsonkit.Codec
//	// Register what each Shape implementation looks like on the wire.
//	// This is the codec-local registry, so it does not leak across
//	// other codecs or other parts of the program.
//	jsonkit.CodecRegisterTypeID[Circle](&c, "circle")
//
//	// Marshal the polymorphic collection — works for []Shape or
//	// jsonkit.Array[Shape] with the same wire format.
//	shapes := []Shape{Circle{R: 1}}
//	data, _ := c.Marshal(shapes)
//	// data == `[{"@type":"circle","R":1}]`
//
//	// Read it back later, possibly from a different process or a
//	// different machine.
//	var got []Shape
//	_ = c.Unmarshal(data, &got)
//	// got == []Shape{Circle{R: 1}}
//
// Local types for the test, declared at package scope so Go's
// method-receiver declarations are legal. They are unexported so
// they don't leak beyond the test file. Keeping them at package scope
// is the standard way to attach methods to test-only structs.
type (
	wfShape  interface{ wfArea() float64 }
	wfCircle struct{ R float64 }
	wfSquare struct{ S float64 }
	wfTri    struct{ B, H float64 }
)

func (wfCircle) wfArea() float64 { return 3.14159 * 3 * 3 }
func (wfSquare) wfArea() float64 { return 4 * 4 }
func (wfTri) wfArea() float64    { return 0.5 * 3 * 4 }

func TestCodec_marshalArrayInterface_describesWireFormat(t *testing.T) {
	// A small polymorphic domain — the kind of thing an end user
	// would actually marshal: a list of shapes where the concrete
	// implementation varies per element.
	mkCircle := func(r float64) wfShape { return wfCircle{R: r} }
	mkSquare := func(s float64) wfShape { return wfSquare{S: s} }
	mkTriangle := func(b, h float64) wfShape { return wfTri{B: b, H: h} }

	// The user's natural shape: a heterogeneous collection, where each
	// element can be a different concrete implementation.
	database := jsonkit.Array[wfShape]{
		mkCircle(1),
		mkSquare(2),
		mkTriangle(3, 4),
	}

	// Two independent codecs, each with its own @type aliases for the
	// same concrete types. This is the "two storage layouts side by
	// side" scenario — a real concern when a user migrates their
	// schema incrementally.
	var legacyCodec, currentCodec jsonkit.Codec
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfCircle](&legacyCodec, "circle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfSquare](&legacyCodec, "rectangle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfTri](&legacyCodec, "polygon"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfCircle](&currentCodec, "shape.circle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfSquare](&currentCodec, "shape.square"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[wfTri](&currentCodec, "shape.triangle"))

	// Marshal through the current codec — the wire the user would
	// write to their database today.
	modernData, err := currentCodec.Marshal(database)
	assert.NoError(t, err)

	// The wire format is what an end user would inspect in their
	// database: a JSON array, each element tagged with its @type.
	// The exact bytes can be compared against a freshly-generated
	// value to ensure "what I store == what I get back".
	assert.Contains(t, modernData, []byte(`"@type":"shape.circle"`))
	assert.Contains(t, modernData, []byte(`"@type":"shape.square"`))
	assert.Contains(t, modernData, []byte(`"@type":"shape.triangle"`))
	// No outer @type around the array — the wire must start with the
	// JSON array bracket, not with an object envelope.
	if len(modernData) == 0 || modernData[0] != '[' {
		t.Fatalf("expected wire to be a top-level JSON array, got: %s", modernData)
	}

	// Round-trip through the same codec — what the user reads back
	// from the database tomorrow.
	var modernGot []wfShape
	assert.NoError(t, currentCodec.Unmarshal(modernData, &modernGot))
	assertShapeSliceEqual(t, []wfShape(database), modernGot)

	// The legacy codec produces a different wire — different @type
	// values for the same concrete types. The user can keep reading
	// old data through the legacy codec, and writing new data through
	// the current codec, without one stomping on the other.
	legacyData, err := legacyCodec.Marshal(database)
	assert.NoError(t, err)
	assert.Contains(t, legacyData, []byte(`"@type":"circle"`))
	assert.Contains(t, legacyData, []byte(`"@type":"rectangle"`))
	assert.Contains(t, legacyData, []byte(`"@type":"polygon"`))
	var legacyGot []wfShape
	assert.NoError(t, legacyCodec.Unmarshal(legacyData, &legacyGot))
	assertShapeSliceEqual(t, []wfShape(database), legacyGot)

	// Cross-codec isolation: the legacy codec cannot decode modern
	// wires (and vice versa). This is what protects the user from
	// accidental data corruption during a codec migration.
	if err := legacyCodec.Unmarshal(modernData, &legacyGot); err == nil {
		t.Fatalf("legacy codec should refuse to decode modern wire; got success")
	}
	if err := currentCodec.Unmarshal(legacyData, &modernGot); err == nil {
		t.Fatalf("current codec should refuse to decode legacy wire; got success")
	}

	// The wire format is identical between the two forms a Go
	// developer can write — a plain []Shape and the
	// jsonkit.Array[Shape] wrapper. This is the practical guarantee
	// the user cares about: they can pick either form, and the bytes
	// that hit the database are the same.
	plainSlice, err := currentCodec.Marshal([]wfShape(database))
	assert.NoError(t, err)
	if string(plainSlice) != string(modernData) {
		t.Fatalf("wire format mismatch between the two forms:\n  []Shape:        %s\n  Array[Shape]:   %s",
			plainSlice, modernData)
	}

	// And both forms are equally decodable through the same codec.
	var fromPlain []wfShape
	assert.NoError(t, currentCodec.Unmarshal(plainSlice, &fromPlain))
	assertShapeSliceEqual(t, []wfShape(database), fromPlain)
	var fromWrapped jsonkit.Array[wfShape]
	assert.NoError(t, currentCodec.Unmarshal(plainSlice, &fromWrapped))
	assertShapeSliceEqual(t, []wfShape(database), fromWrapped)
}

// The helpers below are local to this test to keep it self-contained —
// no shared test-utility surface, no internal state of the package is
// reached into. Anyone reading this file can see the full picture of
// the public-API behavior described above.

func assertShapeSliceEqual(t *testing.T, exp, got []wfShape) {
	t.Helper()
	if len(exp) != len(got) {
		t.Fatalf("length mismatch: exp=%d got=%d", len(exp), len(got))
	}
	for i, e := range exp {
		// Compare via the interface contract so the comparison works
		// for any concrete implementation.
		if e.wfArea() != got[i].wfArea() {
			t.Fatalf("element %d wfArea() mismatch: exp=%#v got=%#v", i, e, got[i])
		}
	}
}

type MT_TypedContainer struct{ Fooer testent.Fooer }

type MT_PolyMarshalerBox struct {
	Inner MT_X_Interface
}

type TestEntFooTypeCodec struct{}

var _ jsonkit.ITypeCodec[testent.Foo] = TestEntFooTypeCodec{}

func (TestEntFooTypeCodec) Marshal(c *jsonkit.Codec, v testent.Foo) ([]byte, error) {
	return json.Marshal(testent.FooDTO{
		ID:   v.ID.String(),
		FooV: v.Foo,
		BarV: v.Bar,
		BazV: v.Baz,
	})
}

func (TestEntFooTypeCodec) Unmarshal(c *jsonkit.Codec, data []byte, p *testent.Foo) error {
	var dto FooDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.ID = FooID(dto.ID)
	p.Foo = dto.FooV
	p.Bar = dto.BarV
	p.Baz = dto.BazV
	return nil
}

func TestCodecRegister(t *testing.T) {
	s := testcase.NewSpec(t)

	fooCodec := let.Var(s, func(t *testcase.T) jsonkit.TypeCodec[testent.Foo] {
		var fooTypeCodec TestEntFooTypeCodec
		return jsonkit.TypeCodec[testent.Foo]{
			MarshalFunc:   fooTypeCodec.Marshal,
			UnmarshalFunc: fooTypeCodec.Unmarshal,
		}
	})

	s.Test("1:1", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		t.Random.Repeat(3, 7, func() {
			var exp = MakeFoo(t)
			data, err := c.Marshal(exp)
			assert.NoError(t, err)
			var got testent.Foo
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.Equal(t, exp, got)
		})
	})

	s.Test("jsonkit.TypeCodec[T]", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		t.Random.Repeat(3, 7, func() {
			var exp = MakeFoo(t)
			data, err := c.Marshal(exp)
			assert.NoError(t, err)
			var got testent.Foo
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.Equal(t, exp, got)
		})
	})

	s.Test("impl of jsonkit.ITypeCodec[T]", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", TestEntFooTypeCodec{})()

		t.Random.Repeat(3, 7, func() {
			var exp = MakeFoo(t)
			data, err := c.Marshal(exp)
			assert.NoError(t, err)
			var got testent.Foo
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.Equal(t, exp, got)
		})
	})

	s.Test("nested-1", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct {
			Foo     testent.Foo
			Fooer   testent.Fooer
			FooerVS []testent.Fooer
		}

		var exp T = T{
			Foo:   testent.MakeFoo(t),
			Fooer: testent.MakeFoo(t),
			FooerVS: random.Slice(t.Random.IntBetween(0, 7), func() Fooer {
				return testent.MakeFoo(t)
			}),
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))

		assert.Equal(t, exp, got)
	})

	s.Test("nested-2", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		// Build a nested map[string]any where each level is a value map
		// (no pointer-to-map) and a randomly-placed leaf is a registered
		// testent.Foo. The wire must add @type only at the leaf so the
		// Foo can be reconstructed during unmarshal. JSON cannot
		// preserve Go pointer-vs-value distinctions, so the source uses
		// value maps only to keep the round-trip equality well-defined.
		var exp = map[string]any{}
		var current = exp
		t.Random.Repeat(3, 7, func() {
			sub := map[string]any{}
			current[t.Random.UUID()] = sub
			current = sub
		})

		var foo = testent.MakeFoo(t)
		current[t.Random.UUID()] = foo

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))

		assert.Equal(t, exp, got)
	})

	// Heterogeneous map[string]any with a mix of nested maps, slices,
	// and registered types at the leaves. The wire must dispatch
	// @type envelopes only for the registered types so the round-trip
	// preserves the concrete type at every registered leaf while
	// keeping the plain JSON container shapes intact.
	s.Test("nested-3 heterogeneous map with slices and registered types", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := map[string]any{
			t.Random.UUID(): testent.MakeFoo(t),
			t.Random.UUID(): map[string]any{
				t.Random.UUID(): []any{
					testent.MakeFoo(t),
					t.Random.String(),
					map[string]any{"inner": testent.MakeFoo(t)},
				},
			},
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))

		assert.Equal(t, exp, got)
	})

	// Pointer to a registered type held as `any` inside a nested
	// map[string]any. The marshaled wire must still dispatch the
	// @type envelope (the pointer wraps the concrete type). The
	// round-trip normalises the leaf back to the value-typed Foo:
	// the wire format doesn't preserve Go pointer-vs-value, but the
	// @type discriminator is enough to reconstruct the concrete
	// type either way.
	s.Test("nested-4 pointer to registered type in map[string]any", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := map[string]any{
			t.Random.UUID(): &testent.Foo{ID: testent.FooID(t.Random.String()), Foo: t.Random.String()},
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))

		for _, v := range got {
			_, ok := v.(testent.Foo)
			assert.True(t, ok,
				assert.MessageF("expected value-typed Foo at the registered leaf, got %T", v))
		}
	})

	// Top-level []any containing a registered type. The marshaled
	// wire must add @type envelopes per element so each registered
	// leaf is reconstructed during unmarshal.
	s.Test("nested-5 top-level []any with registered type", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := []any{
			testent.MakeFoo(t),
			testent.MakeFoo(t),
			t.Random.String(),
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got []any
		assert.NoError(t, c.Unmarshal(data, &got))

		assert.Equal(t, exp, got)
	})

	// Empty map[string]any with no registered leaves. Plain JSON,
	// no @type envelopes anywhere on the wire.
	s.Test("nested-6 empty map[string]any round-trips as map", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := map[string]any{}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Equal(t, "{}", string(data))

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// map[string]any where the registered type is held as a pointer
	// (*Foo) rather than a value (Foo). The codec must dispatch
	// through the same @type alias whether the concrete value is
	// reached as a value or as a pointer.
	s.Test("nested-7 map[string]any with mixed pointer and value leaves", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := map[string]any{
			t.Random.UUID(): testent.MakeFoo(t),                                                         // value
			t.Random.UUID(): &testent.Foo{ID: testent.FooID(t.Random.String()), Foo: t.Random.String()}, // pointer
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))

		// Both leaves are reconstituted as concrete testent.Foo values
		// (the unmarshal-side decodes pointer-wrapped and value-wrapped
		// leaves through the same typeID alias and produces the same
		// concrete type on the round-trip).
		for _, v := range got {
			_, ok := v.(testent.Foo)
			assert.True(t, ok,
				assert.MessageF("expected value-typed Foo at the registered leaf, got %T", v))
		}
	})

	s.Test("unregister", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			V string `json:"v"`
		}

		var c jsonkit.Codec

		dereg := jsonkit.CodecRegister[T](&c, "vT", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO(v))
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T(dto)
				return nil
			},
		})

		var exp = T{V: t.Random.String()}

		data1, err := c.Marshal(exp)
		assert.NoError(t, err)

		data2, err := c.Marshal(exp)
		assert.NoError(t, err)

		assert.Equal(t, data1, data2)

		dereg()

		data3, err := c.Marshal(exp)
		assert.NotEqual(t, data1, data3)
		assert.NoError(t, err)

		var gotT T
		assert.NoError(t, json.Unmarshal(data3, &gotT))
		assert.Equal(t, exp, gotT)
	})

	// Pointer types: registering *T should also accept T values via the
	// base() stripping, and produce identical output for both shapes.
	s.Test("pointer type registered as T also matches *T", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			V string `json:"v"`
		}

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[*T](&c, "ptr_t", jsonkit.TypeCodec[*T]{
			MarshalFunc: func(c *jsonkit.Codec, v *T) ([]byte, error) {
				return json.Marshal(DTO(*v))
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p **T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				if *p == nil {
					*p = &T{}
				}
				**p = T(dto)
				return nil
			},
		})()

		exp := T{V: t.Random.String()}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"@type":"ptr_t"`)

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// The user's Marshal closure can return a JSON null literal. The
	// codec must accept it and pass it through cleanly without trying
	// to wrap it with a @type envelope (which would produce invalid
	// JSON).
	s.Test("marshal returning null literal is preserved as null", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "null_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return []byte("null"), nil
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				*p = T{}
				return nil
			},
		})()

		data, err := c.Marshal(T{V: t.Random.String()})
		assert.NoError(t, err)
		assert.Equal(t, "null", string(data))

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, T{}, got)
	})

	// Marshal errors from the user's closure must bubble up verbatim
	// through the codec. The codec must not swallow them or replace
	// them with a generic "missing @type id" message.
	s.Test("marshal error from user closure propagates", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		sentinel := errors.New("user marshal failure")
		defer jsonkit.CodecRegister[T](&c, "err_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return nil, sentinel
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				return nil
			},
		})()

		_, err := c.Marshal(T{V: t.Random.String()})
		assert.ErrorIs(t, err, sentinel)
	})

	// Unmarshal errors from the user's closure must bubble up verbatim
	// through the codec. This guards against future refactors that
	// might wrap the closure call in a typed-envelope decode path.
	s.Test("unmarshal error from user closure propagates", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "unerr_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(map[string]string{"v": v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				return errors.New("user unmarshal failure")
			},
		})()

		data, err := c.Marshal(T{V: t.Random.String()})
		assert.NoError(t, err)

		var got T
		unmarshalErr := c.Unmarshal(data, &got)
		assert.Error(t, unmarshalErr)
		assert.Contains(t, unmarshalErr.Error(), "user unmarshal failure")
	})

	// Re-registering the same concrete type with a different TypeID on
	// the same codec must panic with a clear message, mirroring how
	// CodecRegisterTypeID panics when a type is double-registered.
	// Silent overwrites would let a typo'd test registration corrupt
	// a prior registration's @type id.
	s.Test("re-registering the same type with a different TypeID panics", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "first_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(v)
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				return json.Unmarshal(data, p)
			},
		})()

		out := assert.Panic(t, func() {
			jsonkit.CodecRegister[T](&c, "second_t", jsonkit.TypeCodec[T]{
				MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
					return json.Marshal(v)
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
					return json.Unmarshal(data, p)
				},
			})
		})
		s, ok := out.(string)
		assert.True(t, ok, "expected panic value to be a string")
		assert.Contains(t, s, "first_t")
		assert.Contains(t, s, "second_t")
	})

	// A struct field whose concrete type is custom-codec-registered
	// should marshal and unmarshal through the custom wire format,
	// even when the field is reached through a slice (where each
	// element gets its own @type envelope).
	s.Test("slice field of custom-codec type round-trips", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			V string `json:"v"`
		}

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "slice_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO(v))
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T(dto)
				return nil
			},
		})()

		type Container struct {
			List []T `json:"list"`
		}

		exp := Container{List: []T{
			{V: t.Random.String()},
			{V: t.Random.String()},
			{V: t.Random.String()},
		}}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"@type":"slice_t"`)

		var got Container
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// A map field whose value type is custom-codec-registered should
	// marshal each value through the custom format and unmarshal back
	// to the same map. Per-key @type envelopes are emitted by the
	// codec's shadow build for map values.
	s.Test("map field with custom-codec value type round-trips", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			V string `json:"v"`
		}

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "map_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO(v))
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T(dto)
				return nil
			},
		})()

		type Container struct {
			Items map[string]T `json:"items"`
		}

		exp := Container{Items: map[string]T{
			"alpha": {V: t.Random.String()},
			"beta":  {V: t.Random.String()},
			"gamma": {V: t.Random.String()},
		}}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"@type":"map_t"`)

		var got Container
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// A type's own json.Marshaler takes precedence over the registered
	// custom codec. If a user adds json.Marshaler to a type that was
	// also CodecRegister'd, the type's own MarshalJSON wins. This
	// matches the existing behaviour for plain @type registration
	// and ensures that retrofitting a type with a custom MarshalJSON
	// doesn't silently bypass the type's intended wire format.
	s.Test("json.Marshaler on the type wins over the custom codec", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "marshaler_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(map[string]string{"via": "codec", "v": v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var raw struct {
					V string `json:"v"`
				}
				if err := json.Unmarshal(data, &raw); err != nil {
					return err
				}
				*p = T{V: raw.V}
				return nil
			},
		})()

		// T also brings its own json.Marshaler. The type's own
		// MarshalJSON wins; the registered custom codec must NOT
		// run, so the wire lacks the "via":"codec" key the custom
		// closure would emit.
		exp := MT_TWithMarshaler{V: t.Random.String()}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.NotContains(t, string(data), `"via":"codec"`,
			"the type's own MarshalJSON should take precedence over the custom codec")
	})

	// Two codecs each registering the same concrete type with their
	// own TypeID must not bleed into each other. Marshaling through
	// codec A produces codec-A's wire, and codec B refuses to decode
	// that wire (because codec B has its own ID for the same type).
	s.Test("two codecs with different TypeIDs for the same type are isolated", func(t *testcase.T) {
		type T struct{ V string }

		// Helper closure constructors so both codecs share the same
		// wire-format mapping but resolve to a different @type id.
		makeCodec := func(side string) jsonkit.TypeCodec[T] {
			return jsonkit.TypeCodec[T]{
				MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
					return json.Marshal(map[string]string{"side": side, "v": v.V})
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
					var raw struct {
						V string `json:"v"`
					}
					if err := json.Unmarshal(data, &raw); err != nil {
						return err
					}
					*p = T{V: raw.V}
					return nil
				},
			}
		}

		var cA, cB jsonkit.Codec
		defer jsonkit.CodecRegister[T](&cA, "a_t", makeCodec("a"))()
		defer jsonkit.CodecRegister[T](&cB, "b_t", makeCodec("b"))()

		exp := T{V: t.Random.String()}
		dataA, err := cA.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(dataA), `"@type":"a_t"`)

		dataB, err := cB.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(dataB), `"@type":"b_t"`)

		assert.NotEqual(t, dataA, dataB)

		var got T
		assert.NoError(t, cA.Unmarshal(dataA, &got))
		assert.Equal(t, exp, got)
	})

	// The custom Unmarshal closure must tolerate a nil *Codec. The codec
	// dispatches unmarshal through several paths (typed integration via
	// jsonkit.Interface/WithType, direct Codec.Unmarshal, polymorphic
	// recursion). Some of those paths do not have a *Codec reference
	// available; the closure must not panic if it ignores the codec
	// parameter. A correctly-written closure either ignores the codec
	// parameter or nil-guards before dereferencing it.
	s.Test("custom unmarshal closure tolerates nil codec parameter", func(t *testcase.T) {
		type T struct{ V string }
		var codecSeen *jsonkit.Codec
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "nil_c_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(map[string]string{"v": v.V})
			},
			UnmarshalFunc: func(codec *jsonkit.Codec, data []byte, p *T) error {
				codecSeen = codec
				var raw struct {
					V string `json:"v"`
				}
				if err := json.Unmarshal(data, &raw); err != nil {
					return err
				}
				*p = T{V: raw.V}
				return nil
			},
		})()

		// Codec.Marshal/Unmarshal propagate the codec to the closure,
		// so it must be non-nil here.
		exp := T{V: t.Random.String()}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
		assert.NotNil(t, codecSeen,
			"the codec's own Unmarshal path must thread the *Codec through to the closure")
	})

	// Deregister must clean up the per-codec entry AND the package-wide
	// marker. If the marker leaks, future unmarshal plans for parent
	// structs that contain this type would still be built with the
	// custom-codec-aware DTO shape, even though the type is no longer
	// custom-codec-registered on any codec.
	s.Test("deregister removes the package-wide marker for fresh plans", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			V string `json:"v"`
		}

		var c jsonkit.Codec
		dereg := jsonkit.CodecRegister[T](&c, "marker_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO(v))
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T(dto)
				return nil
			},
		})

		// Warm the plan cache for the parent type so we can prove
		// deregister forces a rebuild.
		type Container struct {
			Val T `json:"val"`
		}
		exp := Container{Val: T{V: t.Random.String()}}
		data1, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.Contains(t, string(data1), `"@type":"marker_t"`)

		dereg()

		// After deregister, marshal should fall back to the default
		// wire (no @type envelope, no DTO mapping).
		data2, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.NotEqual(t, data1, data2)
		assert.NotContains(t, string(data2), `"@type"`)

		// And the parent struct should still round-trip through
		// default decode.
		var got Container
		assert.NoError(t, c.Unmarshal(data2, &got))
		assert.Equal(t, exp, got)
	})
}

// MT_TWithMarshaler is a test type that implements its own json.Marshaler
// and json.Unmarshaler. Used by the "json.Marshaler on the type wins over
// the custom codec" test to assert the codec defers to the type's own
// wire format when both a json.Marshaler and a CodecRegister entry exist.
type MT_TWithMarshaler struct{ V string }

func (MT_TWithMarshaler) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"own": "marshaler"})
}

func (p *MT_TWithMarshaler) UnmarshalJSON(data []byte) error {
	var raw struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.V = raw.V
	return nil
}

// TestCodec_customCodecEnvelopeShape pins the rule that the polymorphic
// wrapper around a custom codec's wire output must match the wire's shape:
//
//   - If the closure produced a JSON object, the @type discriminator is
//     prepended as a field of that same object.
//   - If the closure produced a non-object value (e.g. a JSON array for a
//     registered slice kind), the value is wrapped in a {"@type":...,"@value":…}
//     envelope so the wire remains valid.
//
// Historically the wrapper called wrapWithTypeIDValue unconditionally,
// which corrupted non-object closures by stripping a leading `{` and
// prepending the discriminator without a corresponding closing brace. The
// fix routes through wrapPolymorphicWithReg, which picks the right
// envelope shape based on the closure's first non-whitespace byte.
func TestCodec_customCodecEnvelopeShape(t *testing.T) {
	s := testcase.NewSpec(t)

	// MT_ObjectClosure is a struct whose custom closure emits a JSON object.
	// The wire should carry @type as a field of that object.
	type MT_ObjectClosure struct{ V string }
	type MT_ObjectClosureDTO struct {
		V string `json:"v"`
	}

	// MT_SliceClosure is `[]int`-shaped whose custom closure emits a JSON
	// array. The wire should carry @type in a wrapping {"@type":...,"@value":[…]}
	// envelope because a bare-array wire cannot host a discriminator field.
	type MT_SliceClosure []int

	s.Test("object closure: @type is prepended as a field of the closure output", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_ObjectClosure](&c, "object-closure",
			jsonkit.TypeCodec[MT_ObjectClosure]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_ObjectClosure) ([]byte, error) {
					return json.Marshal(MT_ObjectClosureDTO{V: v.V})
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_ObjectClosure) error {
					var dto MT_ObjectClosureDTO
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = MT_ObjectClosure{V: dto.V}
					return nil
				},
			},
		)()

		exp := MT_ObjectClosure{V: "hello"}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		// The wire must carry @type alongside the DTO's `v` field, with no
		// enclosing envelope. The discriminator belongs to the object.
		var probe struct {
			Type string `json:"@type"`
			V    string `json:"v"`
		}
		assert.NoError(t, json.Unmarshal(data, &probe))
		assert.Equal(t, "object-closure", probe.Type)
		assert.Equal(t, "hello", probe.V)

		var got MT_ObjectClosure
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("non-object closure: @type is wrapped in a full envelope with @value", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_SliceClosure](&c, "slice-closure",
			jsonkit.TypeCodec[MT_SliceClosure]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_SliceClosure) ([]byte, error) {
					// Closure intentionally emits a bare JSON array —
					// the registered type is a slice, so the wire shape
					// is naturally array-shaped.
					return json.Marshal([]int(v))
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_SliceClosure) error {
					var arr []int
					if err := json.Unmarshal(data, &arr); err != nil {
						return err
					}
					*p = MT_SliceClosure(arr)
					return nil
				},
			},
		)()

		exp := MT_SliceClosure{1, 2, 3}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		// The wire must be {"@type":"slice-closure","@value":[1,2,3]}.
		// Anything else (e.g. {"@type":"slice-closure","@value":[1,2,3]) or
		// a corrupted prefix) means the wrapper assumed the closure output
		// was an object and prepended @type as a field of an array.
		var probe struct {
			Type  string          `json:"@type"`
			Value json.RawMessage `json:"@value"`
		}
		assert.NoError(t, json.Unmarshal(data, &probe))
		assert.Equal(t, "slice-closure", probe.Type)
		assert.Equal(t, "[1,2,3]", string(probe.Value))

		var got MT_SliceClosure
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

// TestCodec_customUnmarshalReceivesOriginatingCodec pins the rule that when
// the codec dispatches to a user-registered custom Unmarshal closure via the
// polymorphic (`typed`) path, the closure receives the same *Codec instance
// that initiated the dispatch — not a freshly-zero Codec that has lost its
// per-instance registry.
//
// Without that propagation, a nested custom codec that calls back into the
// supplied codec (e.g. to round-trip a recursive polymorphic field) would see
// an empty registry and fail to dispatch the inner value's @type tag.
func TestCodec_customUnmarshalReceivesOriginatingCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	type MT_Inner struct{ Marker string }
	type MT_Outer struct {
		Name  string
		Inner MT_Inner
	}

	// MT_OuterCodec's Unmarshal captures the codec it was handed and asserts
	// it can resolve MT_Inner via that same codec. This catches the regression
	// where the dispatch path lost the codec between unmarshalReflectWithReg
	// and the custom Unmarshal closure.
	s.Test("nested custom codec round-trip resolves the inner type via the supplied codec", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_Inner](&c, "inner",
			jsonkit.TypeCodec[MT_Inner]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_Inner) ([]byte, error) {
					return json.Marshal(map[string]string{"marker": v.Marker})
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_Inner) error {
					var dto struct {
						Marker string `json:"marker"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					p.Marker = dto.Marker
					return nil
				},
			},
		)()
		defer jsonkit.CodecRegister[MT_Outer](&c, "outer",
			jsonkit.TypeCodec[MT_Outer]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_Outer) ([]byte, error) {
					// Marshal the inner field by delegating through the codec
					// supplied to the closure, so this round-trip only works
					// when the supplied codec is the originating codec with
					// the inner registration.
					innerBytes, err := _internalMarshal(c, v.Inner)
					if err != nil {
						return nil, err
					}
					return json.Marshal(struct {
						Name  string          `json:"name"`
						Inner json.RawMessage `json:"inner"`
					}{Name: v.Name, Inner: innerBytes})
				},
				UnmarshalFunc: func(supplied *jsonkit.Codec, data []byte, p *MT_Outer) error {
					var dto struct {
						Name  string          `json:"name"`
						Inner json.RawMessage `json:"inner"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					var inner MT_Inner
					if err := supplied.Unmarshal(dto.Inner, &inner); err != nil {
						// The closure was handed a codec whose registry
						// cannot resolve MT_Inner's @type tag. That means
						// the originating codec was lost during dispatch.
						return fmt.Errorf("supplied codec could not resolve inner: %w", err)
					}
					p.Name = dto.Name
					p.Inner = inner
					return nil
				},
			},
		)()

		exp := MT_Outer{Name: "outer-name", Inner: MT_Inner{Marker: "inner-marker"}}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		// Sanity: the marshalled wire carries the @type envelope on the
		// outer, plus an inner envelope for the nested value.
		var probe struct {
			Type  string          `json:"@type"`
			Inner json.RawMessage `json:"inner"`
		}
		assert.NoError(t, json.Unmarshal(data, &probe))
		assert.Equal(t, "outer", probe.Type)
		assert.Contains(t, string(probe.Inner), `"@type":"inner"`)

		var got MT_Outer
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// Companion scenario for the slice dispatch path: the per-element
	// `typed` produced inside jsonArray.UnmarshalJSON must also receive
	// the codec. The assertion below drives the same regression class via
	// a heterogeneous slice round-trip.
	s.Test("slice dispatch propagates the codec to per-element custom unmarshals", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_Inner](&c, "inner",
			jsonkit.TypeCodec[MT_Inner]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_Inner) ([]byte, error) {
					return json.Marshal(map[string]string{"marker": v.Marker})
				},
				UnmarshalFunc: func(supplied *jsonkit.Codec, data []byte, p *MT_Inner) error {
					// The supplied codec must carry the per-instance
					// registration; if the dispatch lost it, json.Unmarshal
					// on the raw bytes succeeds but supplied-driven lookups
					// would not. We use supplied for symmetry with the
					// marshal path so the codec instance is provably the
					// one that initiated dispatch.
					if supplied == nil {
						return fmt.Errorf("supplied codec is nil during dispatch")
					}
					var dto struct {
						Marker string `json:"marker"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					p.Marker = dto.Marker
					return nil
				},
			},
		)()

		exp := []MT_Inner{{Marker: "a"}, {Marker: "b"}, {Marker: "c"}}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got []MT_Inner
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

// _internalMarshal is a small helper that lets the test type's Marshal
// closure round-trip nested polymorphic values through the supplied codec
// without depending on the package-internal `c.Marshal` (which is the
// function we're testing). The codec exposes Marshal via the public Codec
// type.
func _internalMarshal(c jsonkit.Codec, v any) ([]byte, error) {
	return c.Marshal(v)
}
