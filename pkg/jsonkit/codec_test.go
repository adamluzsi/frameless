package jsonkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/uuid"
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

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			// value is the JSON value under test.
			// Each destination context decides where it sits within data.
			value = let.Var(s, func(t *testcase.T) string {
				return makeUntypedJSONValue(t)
			})
			data = let.Var(s, func(t *testcase.T) []byte {
				return []byte(value.Get(t))
			})
			ptr = let.Var(s, func(t *testcase.T) any {
				return new(any)
			})
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Unmarshal(data.Get(t), ptr.Get(t))
		}

		s.Then("JSON without @type is decoded into `any` the same way as encoding/json does", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var exp any
			assert.NoError(t, json.Unmarshal(data.Get(t), &exp))
			assert.Equal(t, exp, *ptr.Get(t).(*any))
		})

		// JSON allows insignificant whitespace around a value, like the trailing newline of a JSON file.
		// It must not change how the value is decoded.
		itIgnoresTheWhitespaceAroundTheJSON := func(s *testcase.Spec) {
			s.When("the JSON is surrounded by whitespace", func(s *testcase.Spec) {
				withoutWhitespace := let.Var(s, func(t *testcase.T) []byte {
					return data.Super(t)
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(makeJSONWhitespace(t) + string(withoutWhitespace.Get(t)) + makeJSONWhitespace(t))
				})

				s.Then("it is decoded the same way as without the whitespace", func(t *testcase.T) {
					exp := reflect.New(reflect.TypeOf(ptr.Get(t)).Elem())
					assert.Must(t).NoError(subject.Get(t).Unmarshal(withoutWhitespace.Get(t), exp.Interface()))

					assert.NoError(t, act(t))
					assert.Equal(t, exp.Elem().Interface(), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
				})
			})
		}

		itIgnoresTheWhitespaceAroundTheJSON(s)

		// JSON without @type carries no type information,
		// so wherever the `any` is, the Codec decodes it the same way as encoding/json does.
		thenItIsDecodedLikeWithEncodingJSON := func(t *testcase.T) {
			exp := reflect.New(reflect.TypeOf(ptr.Get(t)).Elem()).Interface()
			assert.Must(t).NoError(json.Unmarshal(data.Get(t), exp))

			assert.NoError(t, act(t))
			assert.Equal(t, exp, ptr.Get(t))
		}

		s.When("the destination is a struct field with an `any` type", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V any })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":` + value.Get(t) + `}`)
			})

			s.Then("JSON without @type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a struct field with a map of `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V map[string]any })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"k":` + value.Get(t) + `}}`)
			})

			s.Then("JSON without @type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a struct field with a slice of `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V []any })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":[` + value.Get(t) + `]}`)
			})

			s.Then("JSON without @type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a map of `any` with integer keys", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[int]any)
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"` + strconv.Itoa(t.Random.Int()) + `":` + value.Get(t) + `}`)
			})

			s.Then("JSON without @type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the JSON value is a @type envelope of a registered type", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[testent.FooerT1](c, "fooer-t1"))
				return c
			})
			fooer := let.Var(s, func(t *testcase.T) testent.FooerT1 {
				return testent.FooerT1{V: t.Random.String()}
			})
			value.Let(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, map[string]any{"@type": "fooer-t1", "V": fooer.Get(t).V})
			})

			s.Then("it is decoded into the registered type", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, fooer.Get(t), *ptr.Get(t).(*any))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			s.And("the destination is an interface which the registered type implements", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(testent.Fooer)
				})

				s.Then("it is decoded into the registered type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[testent.Fooer](t, fooer.Get(t), *ptr.Get(t).(*testent.Fooer))
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)
			})

			s.And("the destination is a pointer to an interface which the registered type implements", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(*testent.Fooer)
				})

				s.Then("it is decoded into the registered type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					got := *ptr.Get(t).(**testent.Fooer)
					assert.NotNil(t, got)
					assert.Equal[testent.Fooer](t, fooer.Get(t), *got)
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)
			})

			s.And("the destination is a struct field with an interface type which the registered type implements", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ V testent.Fooer })
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"V":` + value.Get(t) + `}`)
				})

				s.Then("it is decoded into the registered type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[testent.Fooer](t, fooer.Get(t), ptr.Get(t).(*struct{ V testent.Fooer }).V)
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)
			})
		})

		s.When("the JSON value is a @type envelope of a registered container type", func(s *testcase.Spec) {
			container := let.Var(s, func(t *testcase.T) any {
				return random.Pick[any](t.Random,
					withTypeSlice{t.Random.Int(), t.Random.Int()},
					map[string]int{t.Random.HexN(4): t.Random.Int()},
				)
			})
			value.Let(s, func(t *testcase.T) string {
				v := container.Get(t)
				data, err := subject.Get(t).Marshal(&v)
				assert.Must(t).NoError(err)
				return string(data)
			})

			s.Then("it is decoded into the registered container type", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, container.Get(t), *ptr.Get(t).(*any))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			s.And("the destination is the registered container type itself", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return reflect.New(reflect.TypeOf(container.Get(t))).Interface()
				})

				s.Then("it is decoded into the registered container type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, container.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)
			})

			s.And("the destination is a struct field with an `any` type", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ V any })
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"V":` + value.Get(t) + `}`)
				})

				s.Then("it is decoded into the registered container type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, container.Get(t), ptr.Get(t).(*struct{ V any }).V)
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)
			})
		})

		s.When("the JSON value is a @value envelope of a registered map which has an entry with a @ prefixed key", func(s *testcase.Spec) {
			entries := let.Var(s, func(t *testcase.T) map[string]int {
				return map[string]int{"@" + t.Random.HexN(4): t.Random.Int()}
			})
			value.Let(s, func(t *testcase.T) string {
				return `{"@type":"with_type_map","@value":` + mustMarshalJSON(t, entries.Get(t)) + `}`
			})

			s.Then("the entry is kept, as only a flat envelope has a @type next to the entries", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, entries.Get(t), *ptr.Get(t).(*any))
			})

			s.And("the destination is the registered map type itself", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(map[string]int)
				})

				s.Then("the entry is kept, as only a flat envelope has a @type next to the entries", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, entries.Get(t), *ptr.Get(t).(*map[string]int))
				})
			})
		})

		s.When("the JSON value is a @type envelope of a registered map with interface values", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_GreeterMap](c, "greeter-map"))
				return c
			})
			greeters := let.Var(s, func(t *testcase.T) MT_GreeterMap {
				return random.Map(t.Random.IntBetween(1, 3), func() (string, Greeter) {
					return t.Random.HexN(4), random.Pick[Greeter](t.Random,
						TypeA{V: t.Random.String()},
						TypeB{V: t.Random.Int()},
					)
				})
			})
			value.Let(s, func(t *testcase.T) string {
				var v any = greeters.Get(t)
				data, err := subject.Get(t).Marshal(&v)
				assert.Must(t).NoError(err)
				return string(data)
			})

			s.Then("its values are decoded into their registered types", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, greeters.Get(t), *ptr.Get(t).(*any))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			s.And("the destination is an interface which the registered map implements", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(Greeter)
				})

				s.Then("its values are decoded into their registered types", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[Greeter](t, greeters.Get(t), *ptr.Get(t).(*Greeter))
				})
			})

			s.And("the destination is the registered map type itself", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(MT_GreeterMap)
				})

				s.Then("its values are decoded into their registered types", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, greeters.Get(t), *ptr.Get(t).(*MT_GreeterMap))
				})
			})
		})

		s.When("the JSON value is a registered slice type which holds a nil element", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(Greeter)
			})
			greeters := let.Var(s, func(t *testcase.T) TypeE {
				return TypeE{TypeA{V: t.Random.String()}, nil}
			})
			value.Let(s, func(t *testcase.T) string {
				var g Greeter = greeters.Get(t)
				data, err := subject.Get(t).Marshal(&g)
				assert.Must(t).NoError(err)
				return string(data)
			})

			s.Then("the nil element is decoded back as a nil interface value", func(t *testcase.T) {
				var err error
				assert.NotPanic(t, func() { err = act(t) })
				assert.NoError(t, err)
				assert.Equal[Greeter](t, greeters.Get(t), *ptr.Get(t).(*Greeter))
			})
		})

		// A non-empty interface can only hold a value that implements it.
		// When the JSON describes anything else, Unmarshal must report it as an error,
		// and must not let the reflect based assignment panic.
		itRejectsValuesTheInterfaceCannotHold := func(s *testcase.Spec) {
			thenItFails := func(t *testcase.T) {
				var err error
				assert.NotPanic(t, func() { err = act(t) })
				assert.Error(t, err)
			}

			s.Then("JSON without @type is rejected with an error", thenItFails)

			s.And("the JSON value is a bare array", func(s *testcase.Spec) {
				value.Let(s, makeUntypedJSONArray)

				s.Then("it is rejected with an error", thenItFails)
			})

			s.And("the @type names a registered struct type which does not implement the interface", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](c, "foo"))
					return c
				})
				value.LetValue(s, `{"@type":"foo","@value":{}}`)

				s.Then("it is rejected with an error", thenItFails)
			})

			s.And("the @type names a registered slice type which does not implement the interface", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[MT_Fooers](c, "fooers"))
					return c
				})
				value.LetValue(s, `{"@type":"fooers","@value":[]}`)

				s.Then("it is rejected with an error", thenItFails)
			})
		}

		s.When("the destination is a non-empty interface", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(MT_X_Interface)
			})

			itRejectsValuesTheInterfaceCannotHold(s)
		})

		s.When("the destination is a non-empty interface struct field", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V MT_X_Interface })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":` + value.Get(t) + `}`)
			})

			itRejectsValuesTheInterfaceCannotHold(s)
		})

		s.When("the destination is a slice of non-empty interfaces", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new([]MT_X_Interface)
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`[` + value.Get(t) + `]`)
			})

			itRejectsValuesTheInterfaceCannotHold(s)
		})

		s.When("the destination is a map of non-empty interfaces", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[string]MT_X_Interface)
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"k":` + value.Get(t) + `}`)
			})

			itRejectsValuesTheInterfaceCannotHold(s)
		})

		// Like encoding/json, Unmarshal decodes the JSON into the value the destination already holds,
		// instead of replacing that value as a whole.
		s.When("the destination already holds a value", func(s *testcase.Spec) {
			// existing makes a pointer to the value, which the destination holds before the decoding.
			// Each call makes a new copy of the same value,
			// so the Codec and encoding/json can each decode into a copy of their own.
			existing := let.Var(s, func(t *testcase.T) func() any {
				v := makeMTMerge(t)
				return func() any {
					c := v.clone()
					return &c
				}
			})
			ptr.Let(s, func(t *testcase.T) any {
				return existing.Get(t)()
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(mustMarshalJSON(t, map[string]any{
					"Text":   t.Random.String(),
					"Quoted": strconv.Itoa(t.Random.Int()),
				}))
			})

			thenItDecodesLikeEncodingJSON := func(t *testcase.T) {
				exp := existing.Get(t)()
				assert.Must(t).NoError(json.Unmarshal(data.Get(t), exp))

				assert.NoError(t, act(t))
				assert.Equal(t, exp, ptr.Get(t))
			}

			s.Then("the fields without a member in the JSON keep their value, like with encoding/json", thenItDecodesLikeEncodingJSON)

			s.And("the JSON has null members", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					members := map[string]any{}
					for _, field := range reflect.VisibleFields(reflect.TypeOf(MT_Merge{})) {
						members[field.Name] = nil
					}
					return []byte(mustMarshalJSON(t, members))
				})

				s.Then("null makes the interfaces, pointers, maps and slices nil, and leaves the other fields as they are, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("the JSON has objects with only some of the members for the struct fields", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, map[string]any{
						"Pointer":     map[string]any{"A": t.Random.String()},
						"Nested":      map[string]any{"B": t.Random.Int()},
						"PlainNested": map[string]any{"A": t.Random.String()},
					}))
				})

				s.Then("the members are decoded into the structs the fields already hold, like with encoding/json", thenItDecodesLikeEncodingJSON)

				s.Then("the pointer field keeps pointing to the struct it pointed to, like with encoding/json", func(t *testcase.T) {
					pointer := ptr.Get(t).(*MT_Merge).Pointer

					assert.NoError(t, act(t))
					assert.True(t, pointer == ptr.Get(t).(*MT_Merge).Pointer)
				})
			})

			s.And("the JSON has members for the container fields", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					v := existing.Get(t)().(*MT_Merge)
					return []byte(mustMarshalJSON(t, map[string]any{
						// a new entry, and one for a key the map already has
						"Map":         map[string]int{t.Random.HexN(8): t.Random.Int(), firstKey(v.Map): t.Random.Int()},
						"EmbeddedMap": map[string]int{t.Random.HexN(8): t.Random.Int(), firstKey(v.EmbeddedMap): t.Random.Int()},
						"Greeters":    map[string]any{t.Random.HexN(8): nil, firstKey(v.Greeters): nil},
						"Leaves":      map[string]any{t.Random.HexN(8): makePartialLeafJSON(t), firstKey(v.Leaves): makePartialLeafJSON(t)},
						// fewer or more elements than the slice and the array have
						"Slice": random.Slice(t.Random.IntBetween(1, len(v.Slice)+2), func() any { return makePartialLeafJSON(t) }),
						"Array": random.Slice(t.Random.IntBetween(1, len(v.Array)+1), func() any { return makePartialLeafJSON(t) }),
					}))
				})

				s.Then("the maps keep their other entries, and the elements of the slices and arrays are decoded in place, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("the JSON has a member for the interface field", func(s *testcase.Spec) {
				greeter := let.Var(s, makeGreeter)
				data.Let(s, func(t *testcase.T) []byte {
					g := greeter.Get(t)
					value, err := subject.Get(t).Marshal(&g)
					assert.Must(t).NoError(err)
					return []byte(`{"Greeter":` + string(value) + `}`)
				})

				s.Then("the interface field gets the decoded value in place of the one it held", func(t *testcase.T) {
					exp := existing.Get(t)().(*MT_Merge)
					exp.Greeter = greeter.Get(t)

					assert.NoError(t, act(t))
					assert.Equal[any](t, exp, ptr.Get(t))
				})
			})

			s.And("it is a slice, and the JSON array has fewer elements", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := random.Slice(t.Random.IntBetween(2, 3), func() MT_MergeLeaf { return makeMTMergeLeaf(t) })
					return func() any {
						c := slices.Clone(v)
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, []any{makePartialLeafJSON(t)}))
				})

				s.Then("its elements are decoded in place, and it is cut to the length of the JSON array, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is a slice, and the JSON array has more elements", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := random.Slice(t.Random.IntBetween(1, 2), func() MT_MergeLeaf { return makeMTMergeLeaf(t) })
					return func() any {
						c := slices.Clone(v)
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, random.Slice(3, func() any { return makePartialLeafJSON(t) })))
				})

				s.Then("its elements are decoded in place, and it grows to the length of the JSON array, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is a nil slice, and the JSON array is empty", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					return func() any { return new([]MT_MergeLeaf) }
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`[]`)
				})

				s.Then("it becomes an empty slice, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is an array, and the JSON array has fewer elements", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := [2]MT_MergeLeaf{makeMTMergeLeaf(t), makeMTMergeLeaf(t)}
					return func() any {
						c := v
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, []any{makePartialLeafJSON(t)}))
				})

				s.Then("its elements are decoded in place, and the rest of them are zeroed, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is an array, and the JSON array has more elements", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := [2]MT_MergeLeaf{makeMTMergeLeaf(t), makeMTMergeLeaf(t)}
					return func() any {
						c := v
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, random.Slice(3, func() any { return makePartialLeafJSON(t) })))
				})

				s.Then("its elements are decoded in place, and the extra JSON elements are skipped, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is a map", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := random.Map(t.Random.IntBetween(1, 3), func() (string, MT_MergeLeaf) {
						return t.Random.HexN(8), makeMTMergeLeaf(t)
					})
					return func() any {
						c := maps.Clone(v)
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					v := *existing.Get(t)().(*map[string]MT_MergeLeaf)
					return []byte(mustMarshalJSON(t, map[string]any{
						t.Random.HexN(8): makePartialLeafJSON(t),
						firstKey(v):      makePartialLeafJSON(t),
					}))
				})

				s.Then("it keeps its other entries, and the entries in the JSON get a newly decoded value, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is a struct which needs no Codec", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := MT_PlainLeaf{A: t.Random.String(), B: t.Random.Int()}
					return func() any {
						c := v
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, makePartialLeafJSON(t)))
				})

				s.Then("the fields without a member in the JSON keep their value, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is a map of values which need no Codec", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := random.Map(t.Random.IntBetween(1, 3), func() (string, int) {
						return t.Random.HexN(8), t.Random.Int()
					})
					return func() any {
						c := maps.Clone(v)
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, map[string]int{t.Random.HexN(8): t.Random.Int()}))
				})

				s.Then("it keeps its other entries, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})

			s.And("it is an array of values which need no Codec, and the JSON array has fewer elements", func(s *testcase.Spec) {
				existing.Let(s, func(t *testcase.T) func() any {
					v := [3]int{t.Random.Int(), t.Random.Int(), t.Random.Int()}
					return func() any {
						c := v
						return &c
					}
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(mustMarshalJSON(t, []int{t.Random.Int()}))
				})

				s.Then("the rest of its elements are zeroed, like with encoding/json", thenItDecodesLikeEncodingJSON)
			})
		})
	})
}

// makeUntypedJSONValue makes a random JSON value without a @type discriminator:
// an array, an object or a primitive.
func makeUntypedJSONValue(t *testcase.T) string {
	return random.Pick(t.Random,
		makeUntypedJSONArray,
		func(t *testcase.T) string {
			return mustMarshalJSON(t, random.Map(t.Random.IntBetween(0, 3), func() (string, int) {
				return t.Random.HexN(4), t.Random.Int()
			}))
		},
		func(t *testcase.T) string { return mustMarshalJSON(t, t.Random.String()) },
		func(t *testcase.T) string { return mustMarshalJSON(t, t.Random.Int()) },
		func(t *testcase.T) string { return mustMarshalJSON(t, t.Random.Bool()) },
	)(t)
}

// makeUntypedJSONArray makes a random, possibly empty, JSON array.
func makeUntypedJSONArray(t *testcase.T) string {
	// start from a non-nil slice, so an empty array encodes as [] instead of null.
	vs := append([]string{}, random.Slice(t.Random.IntBetween(0, 3), func() string { return t.Random.String() })...)
	return mustMarshalJSON(t, vs)
}

// makeJSONWhitespace makes a random, non-empty run of the insignificant whitespace JSON allows around a value.
func makeJSONWhitespace(t *testcase.T) string {
	return t.Random.StringNC(t.Random.IntBetween(1, 3), " \t\r\n")
}

func mustMarshalJSON(t *testcase.T, v any) string {
	data, err := json.Marshal(v)
	assert.Must(t).NoError(err)
	return string(data)
}

// assertEncodesLikeEncodingJSON asserts that the Codec writes v as the same JSON as encoding/json does,
// and that it reads that JSON back the same way as encoding/json does.
func assertEncodesLikeEncodingJSON[T any](t *testcase.T, c *jsonkit.Codec, v T) {
	t.Helper()
	exp, err := json.Marshal(v)
	assert.Must(t).NoError(err)

	got, err := c.Marshal(v)
	assert.Must(t).NoError(err)
	assert.Equal(t, string(exp), string(got))

	assertDecodesLikeEncodingJSON[T](t, c, exp)
}

// assertDecodesLikeEncodingJSON asserts that the Codec reads data into a T the same way as encoding/json does.
func assertDecodesLikeEncodingJSON[T any](t *testcase.T, c *jsonkit.Codec, data []byte) {
	t.Helper()
	assertDecodesIntoLikeEncodingJSON(t, c, data, func() T {
		var zero T
		return zero
	})
}

// assertDecodesIntoLikeEncodingJSON asserts that the Codec reads data into the value made by makeDestination
// the same way as encoding/json does.
// makeDestination is called for both of them, so each of them decodes into a value of its own.
func assertDecodesIntoLikeEncodingJSON[T any](t *testcase.T, c *jsonkit.Codec, data []byte, makeDestination func() T) {
	t.Helper()
	exp, got := makeDestination(), makeDestination()
	assert.Must(t).NoError(json.Unmarshal(data, &exp))
	assert.Must(t).NoError(c.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

// jsonMemberNames returns the names of the members of the JSON object in data.
func jsonMemberNames(t *testcase.T, data []byte) []string {
	t.Helper()
	var object map[string]json.RawMessage
	assert.Must(t).NoError(json.Unmarshal(data, &object))
	return slices.Collect(maps.Keys(object))
}

// MT_Merge has a field of each kind, which encoding/json decodes into the value it already holds in its own way.
// Its Greeter field makes the Codec decode it with its own reflection based decoding, instead of handing it to encoding/json.
type MT_Merge struct {
	Greeter     Greeter
	Text        string
	Number      int
	Quoted      int `json:",string"`
	Pointer     *MT_MergeLeaf
	Map         map[string]int
	Greeters    map[string]Greeter
	Leaves      map[string]MT_MergeLeaf
	Slice       []MT_MergeLeaf
	Array       [2]MT_MergeLeaf
	Nested      MT_MergeLeaf
	PlainNested MT_PlainLeaf
	*MT_MergeEmbedded
}

// MT_MergeEmbedded is embedded through a pointer in MT_Merge, so its fields are promoted through the pointer.
type MT_MergeEmbedded struct {
	EmbeddedText string
	EmbeddedMap  map[string]int
}

// clone makes a deep copy of v, so decoding into the copy leaves v as it is.
func (v MT_Merge) clone() MT_Merge {
	if v.Pointer != nil {
		p := *v.Pointer
		v.Pointer = &p
	}
	if v.MT_MergeEmbedded != nil {
		e := *v.MT_MergeEmbedded
		e.EmbeddedMap = maps.Clone(e.EmbeddedMap)
		v.MT_MergeEmbedded = &e
	}
	v.Map = maps.Clone(v.Map)
	v.Greeters = maps.Clone(v.Greeters)
	v.Leaves = maps.Clone(v.Leaves)
	v.Slice = slices.Clone(v.Slice)
	return v
}

// MT_MergeLeaf has a Greeter field, which makes the Codec decode it with its own reflection based decoding.
type MT_MergeLeaf struct {
	Greeter Greeter
	A       string
	B       int
}

// MT_PlainLeaf needs no Codec, so the Codec can hand it to encoding/json.
type MT_PlainLeaf struct {
	A string
	B int
}

func makeMTMerge(t *testcase.T) MT_Merge {
	leaf := makeMTMergeLeaf(t)
	return MT_Merge{
		Greeter: makeGreeter(t),
		Text:    t.Random.String(),
		Number:  t.Random.Int(),
		Quoted:  t.Random.Int(),
		Pointer: &leaf,
		Map: random.Map(t.Random.IntBetween(1, 3), func() (string, int) {
			return t.Random.HexN(8), t.Random.Int()
		}),
		Greeters: random.Map(t.Random.IntBetween(1, 3), func() (string, Greeter) {
			return t.Random.HexN(8), makeGreeter(t)
		}),
		Leaves: random.Map(t.Random.IntBetween(1, 3), func() (string, MT_MergeLeaf) {
			return t.Random.HexN(8), makeMTMergeLeaf(t)
		}),
		Slice:       random.Slice(t.Random.IntBetween(1, 3), func() MT_MergeLeaf { return makeMTMergeLeaf(t) }),
		Array:       [2]MT_MergeLeaf{makeMTMergeLeaf(t), makeMTMergeLeaf(t)},
		Nested:      makeMTMergeLeaf(t),
		PlainNested: MT_PlainLeaf{A: t.Random.String(), B: t.Random.Int()},
		MT_MergeEmbedded: &MT_MergeEmbedded{
			EmbeddedText: t.Random.String(),
			EmbeddedMap: random.Map(t.Random.IntBetween(1, 3), func() (string, int) {
				return t.Random.HexN(8), t.Random.Int()
			}),
		},
	}
}

func makeMTMergeLeaf(t *testcase.T) MT_MergeLeaf {
	return MT_MergeLeaf{Greeter: makeGreeter(t), A: t.Random.String(), B: t.Random.Int()}
}

// makePartialLeafJSON makes the JSON object of an MT_MergeLeaf or an MT_PlainLeaf with only some of their members.
// It has no Greeter member, which encoding/json could not decode.
func makePartialLeafJSON(t *testcase.T) map[string]any {
	return random.Pick(t.Random,
		map[string]any{},
		map[string]any{"A": t.Random.String()},
		map[string]any{"B": t.Random.Int()},
	)
}

func makeGreeter(t *testcase.T) Greeter {
	return random.Pick[Greeter](t.Random,
		TypeA{V: t.Random.String()},
		TypeB{V: t.Random.Int()},
	)
}

// firstKey returns the smallest key of the non-empty map m.
func firstKey[V any](m map[string]V) string {
	return slices.Min(slices.Collect(maps.Keys(m)))
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

	s.Test("nested struct value with an interface field", func(t *testcase.T) {
		exp := MT_T16{Inner: MT_T1{Fooer: testent.MakeFoo(t)}}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T16
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("struct values with an interface field, nested in containers and deeper structs", func(t *testcase.T) {
		makeT1 := func() MT_T1 { return MT_T1{Fooer: testent.MakeFoo(t)} }
		exp := MT_T17{
			Deep:     MT_T16{Inner: makeT1()},
			List:     []MT_T1{makeT1(), makeT1()},
			ByName:   map[string]MT_T1{t.Random.HexN(8): makeT1()},
			Pointers: []*MT_T1{{Fooer: testent.MakeFoo(t)}},
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T17
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("recursive type", func(t *testcase.T) {
		exp := MT_PlainNode{
			V:    t.Random.Int(),
			Next: &MT_PlainNode{V: t.Random.Int()},
			Kids: []MT_PlainNode{{V: t.Random.Int()}},
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_PlainNode
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("recursive type with an interface field", func(t *testcase.T) {
		exp := MT_Node{
			Fooer: testent.MakeFoo(t),
			Next:  &MT_Node{Fooer: testent.MakeFoo(t)},
			Kids:  []MT_Node{{Fooer: testent.MakeFoo(t)}},
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_Node
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("absent nested fields decode as zero values", func(t *testcase.T) {
		var got MT_T17
		assert.NoError(t, c.Get(t).Unmarshal([]byte(`{}`), &got))
		assert.Equal(t, MT_T17{}, got)
	})

	s.Test("embedded struct pointers", func(t *testcase.T) {
		exp := MT_T18{
			Name:        t.Random.String(),
			MT_T1:       &MT_T1{Fooer: testent.MakeFoo(t)},
			MT_T18_Base: &MT_T18_Base{ID: t.Random.UUID()},
		}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T18
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("embedded struct value with an interface field", func(t *testcase.T) {
		exp := MT_T19{Name: t.Random.String(), MT_T1: MT_T1{Fooer: testent.MakeFoo(t)}}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T19
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// The Codec follows the rules of encoding/json for embedded fields,
	// so a struct with embedded fields has the same JSON as with encoding/json.
	s.Test("embedded structs have their fields promoted, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_Embedding{
			Name:                 t.Random.String(),
			MT_EmbeddedValue:     MT_EmbeddedValue{ID: t.Random.UUID(), Note: t.Random.String()},
			MT_EmbeddedPointer:   &MT_EmbeddedPointer{Count: t.Random.Int()},
			mtEmbeddedUnexported: mtEmbeddedUnexported{Hidden: true},
			MT_Labels:            MT_Labels{t.Random.String()},
		})
	})

	s.Test("a nil embedded struct pointer is left out, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_Embedding{Name: t.Random.String()})

		// a member of the embedded struct allocates it, even when its value is null
		assertDecodesLikeEncodingJSON[MT_Embedding](t, c.Get(t), []byte(`{"Count":null}`))

		// the same for a struct which needs the Codec, as its MT_T1 has an interface field
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_T18{Name: t.Random.String()})
		assertDecodesLikeEncodingJSON[MT_T18](t, c.Get(t), []byte(`{"ID":null}`))
	})

	s.Test("conflicting field names are resolved like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_Shadowing{
			MT_EmbeddedValue: MT_EmbeddedValue{ID: t.Random.UUID(), Note: t.Random.String()},
			MT_ConflictA: MT_ConflictA{
				Dup:       t.Random.String(),
				Winner:    t.Random.String(),
				MT_Shared: MT_Shared{Shared: t.Random.String()},
			},
			MT_ConflictB: MT_ConflictB{
				Dup:       t.Random.String(),
				Tagged:    t.Random.String(),
				MT_Shared: MT_Shared{Shared: t.Random.String()},
			},
			ID: t.Random.UUID(),
		})

		assertDecodesLikeEncodingJSON[MT_Shadowing](t, c.Get(t), []byte(mustMarshalJSON(t, map[string]string{
			"ID":     t.Random.UUID(),
			"note":   t.Random.String(),
			"Dup":    t.Random.String(),
			"Winner": t.Random.String(),
			"Shared": t.Random.String(),
		})))
	})

	s.Test("an embedded pointer to the struct itself is not followed, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_SelfEmbedding{
			MT_SelfEmbedding: &MT_SelfEmbedding{V: t.Random.Int()},
			V:                t.Random.Int(),
		})
	})

	s.Test("embedded structs with a name in their json tag are fields with that name, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_TaggedEmbedding{
			MT_EmbeddedValue:     MT_EmbeddedValue{ID: t.Random.UUID(), Note: t.Random.String()},
			mtEmbeddedUnexported: mtEmbeddedUnexported{Hidden: true},
		})
	})

	s.Test("an embedded pointer to an unexported struct type can't be allocated, like with encoding/json", func(t *testcase.T) {
		data := []byte(`{"Hidden":true}`)
		assert.Must(t).Error(json.Unmarshal(data, new(MT_UnexportedEmbeddedPointer)))

		assert.Error(t, c.Get(t).Unmarshal(data, new(MT_UnexportedEmbeddedPointer)))
	})

	s.Test("the members after a field in a nil embedded pointer to an unexported struct type are still decoded, like with encoding/json", func(t *testcase.T) {
		data := []byte(mustMarshalJSON(t, map[string]any{"Hidden": true, "Name": t.Random.String()}))

		var exp, got MT_UnexportedEmbeddedPointerAndName
		assert.Must(t).Error(json.Unmarshal(data, &exp))
		assert.Error(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// encoding/json panics on decoding such a field while it is nil.
	s.Test("a tagged embedded pointer to an unexported struct type is decoded only when it is already allocated", func(t *testcase.T) {
		data := []byte(`{"hidden":{"Hidden":true}}`)
		assert.Error(t, c.Get(t).Unmarshal(data, new(MT_TaggedEmbeddedPointer)))

		secret := t.Random.String()
		allocated := func() MT_TaggedEmbeddedPointer {
			return MT_TaggedEmbeddedPointer{mtEmbeddedUnexported: &mtEmbeddedUnexported{secret: secret}}
		}
		// like with encoding/json, only the exported fields are set, and null leaves it as it is
		for _, data := range [][]byte{data, []byte(`{"hidden":null}`)} {
			exp, got := allocated(), allocated()
			assert.Must(t).NoError(json.Unmarshal(data, &exp))
			assert.NoError(t, c.Get(t).Unmarshal(data, &got))
			// assert.Equal can't compare structs with an unexported pointer field, so the structs they point to are compared.
			assert.Equal(t, *exp.mtEmbeddedUnexported, *got.mtEmbeddedUnexported)
		}
	})

	s.Test("an embedded time.Time has its MarshalJSON and UnmarshalJSON promoted, which take over the whole struct, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_EmbeddedTime{Time: t.Random.Time(), Note: t.Random.String()})
	})

	// An unexported embedded field can't be interfaced, so encoding/json can't call the UnmarshalJSON of its value.
	s.Test("a tagged embedded struct of an unexported type is decoded field by field, even when it has its own UnmarshalJSON, like with encoding/json", func(t *testcase.T) {
		var _ json.Unmarshaler = &mtEmbeddedUnmarshalerA{}
		assert.False(t, reflect.TypeFor[*MT_TaggedEmbeddedUnmarshalers]().Implements(reflect.TypeFor[json.Unmarshaler]()),
			"the UnmarshalJSON of the embedded structs are not promoted")

		assertDecodesLikeEncodingJSON[MT_TaggedEmbeddedUnmarshalers](t, c.Get(t), []byte(mustMarshalJSON(t, map[string]any{
			"a": map[string]string{"A": t.Random.String()},
			"b": map[string]string{"B": t.Random.String()},
		})))
	})

	s.Test("an interface field promoted from an embedded struct of an unexported type, in a nested struct", func(t *testcase.T) {
		exp := MT_T21{Inner: MT_T20{mtEmbeddedFooer{Fooer: testent.MakeFoo(t)}}}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got MT_T21
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("an embedded interface is a field named after its type, like with encoding/json", func(t *testcase.T) {
		exp := MT_EmbeddedInterface{Name: t.Random.String(), Fooer: testent.MakeFoo(t)}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.ContainsExactly(t, []string{"Name", "Fooer"}, jsonMemberNames(t, data))

		var got MT_EmbeddedInterface
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// Only a type embedded as the first field gets its methods promoted to the struct type built by reflection,
	// which is where a promoted MarshalJSON could take over writing the whole struct.
	s.Test("an embedded interface as the first field is a field named after its type, like with encoding/json", func(t *testcase.T) {
		exp := MT_EmbeddedInterfaceFirst{Fooer: testent.MakeFoo(t), Name: t.Random.String()}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.ContainsExactly(t, []string{"Name", "Fooer"}, jsonMemberNames(t, data))

		var got MT_EmbeddedInterfaceFirst
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

	s.Test("JSON member names from field tags are like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_TagNames{
			Dash:    t.Random.String(),
			Invalid: t.Random.String(),
			Unknown: t.Random.String(),
		})
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

	s.Test("the ,string option applies to strings and to the pointers of the types it applies to, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), makeMTStringified(t))
		// the Greeter field makes the Codec decode it with its own reflection based decoding
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_StringifiedWithCodec{MT_Stringified: makeMTStringified(t)})
		stringified := makeMTStringified(t)
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_StringifiedThroughPointer{MT_Stringified: &stringified})
	})

	s.Test("a quoted null makes a pointer with the ,string option nil, like with encoding/json", func(t *testcase.T) {
		data := []byte(`{"P":"null","F":null,"S":"null"}`)
		allocated := func() MT_Stringified {
			p, f, s := t.Random.Int(), t.Random.Float64(), t.Random.String()
			return MT_Stringified{P: &p, F: &f, S: &s}
		}

		assertDecodesIntoLikeEncodingJSON(t, c.Get(t), data, allocated)
		assertDecodesIntoLikeEncodingJSON(t, c.Get(t), data, func() MT_StringifiedWithCodec {
			return MT_StringifiedWithCodec{MT_Stringified: allocated()}
		})
		assertDecodesIntoLikeEncodingJSON(t, c.Get(t), data, func() MT_StringifiedThroughPointer {
			stringified := allocated()
			return MT_StringifiedThroughPointer{MT_Stringified: &stringified}
		})
	})

	s.Test("a byte slice is a base64 encoded string, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), []byte(t.Random.String()))
		assertEncodesLikeEncodingJSON(t, c.Get(t), MT_Bytes{
			Data: []byte(t.Random.String()),
			List: [][]byte{[]byte(t.Random.String())},
		})
	})

	s.Test("a map with integer keys and interface values", func(t *testcase.T) {
		exp := map[int]Greeter{t.Random.Int(): makeGreeter(t)}

		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got map[int]Greeter
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

type MT_T16 struct {
	Inner MT_T1
}

type MT_T17 struct {
	Deep     MT_T16
	List     []MT_T1
	ByName   map[string]MT_T1
	Pointers []*MT_T1
}

type MT_PlainNode struct {
	V    int
	Next *MT_PlainNode
	Kids []MT_PlainNode
}

type MT_Node struct {
	Fooer testent.Fooer
	Next  *MT_Node
	Kids  []MT_Node
}

type MT_T18 struct {
	Name string
	*MT_T1
	*MT_T18_Base
}

type MT_T18_Base struct{ ID string }

type MT_T19 struct {
	Name string
	MT_T1
}

// MT_Embedding embeds structs in the ways whose fields encoding/json promotes into the outer JSON object,
// and a named non-struct type, which encoding/json treats as a field named after its type.
type MT_Embedding struct {
	Name string
	MT_EmbeddedValue
	*MT_EmbeddedPointer
	mtEmbeddedUnexported
	MT_Labels
}

type MT_EmbeddedValue struct {
	ID   string
	Note string `json:"note,omitempty"`
}

type MT_EmbeddedPointer struct{ Count int }

type mtEmbeddedUnexported struct {
	Hidden bool
	secret string
}

type MT_Labels []string

// MT_Shadowing has conflicting field names, which encoding/json resolves by their depth and json tags.
type MT_Shadowing struct {
	MT_EmbeddedValue
	MT_ConflictA
	MT_ConflictB
	ID string // hides MT_EmbeddedValue.ID, as it is less nested, even though it comes later
}

type MT_ConflictA struct {
	Dup    string // conflicts with MT_ConflictB.Dup at the same depth, so neither of them is used
	Winner string
	MT_Shared
}

type MT_ConflictB struct {
	Dup    string
	Tagged string `json:"Winner"` // wins over MT_ConflictA.Winner, as only it has its name from a json tag
	MT_Shared
}

// MT_Shared is embedded twice at the same depth in MT_Shadowing, so its fields conflict with each other.
type MT_Shared struct{ Shared string }

// MT_SelfEmbedding embeds a pointer to itself, which makes its embedded fields a cycle.
type MT_SelfEmbedding struct {
	*MT_SelfEmbedding
	V int
}

// MT_TaggedEmbedding embeds structs with a name in their json tag,
// which makes them fields with that name, instead of having their fields promoted.
type MT_TaggedEmbedding struct {
	MT_EmbeddedValue     `json:"value"`
	mtEmbeddedUnexported `json:"hidden"`
}

type MT_UnexportedEmbeddedPointer struct {
	*mtEmbeddedUnexported
}

// MT_UnexportedEmbeddedPointerAndName has a field after the fields of its embedded pointer to an unexported struct type,
// and its Greeter field makes the Codec decode it with its own reflection based decoding.
type MT_UnexportedEmbeddedPointerAndName struct {
	*mtEmbeddedUnexported
	Name    string
	Greeter Greeter
}

// MT_EmbeddedTime has the MarshalJSON and UnmarshalJSON methods of time.Time promoted.
type MT_EmbeddedTime struct {
	time.Time
	Note string
}

// MT_TaggedEmbeddedUnmarshalers embeds structs of unexported types, which have their own UnmarshalJSON,
// and its Greeter field makes the Codec decode it with its own reflection based decoding.
// They are embedded at the same depth, so their UnmarshalJSON methods are not promoted,
// as neither of them would take over the whole struct then.
type MT_TaggedEmbeddedUnmarshalers struct {
	mtEmbeddedUnmarshalerA `json:"a"`
	mtEmbeddedUnmarshalerB `json:"b"`
	Greeter                Greeter
}

type mtEmbeddedUnmarshalerA struct{ A string }

func (v *mtEmbeddedUnmarshalerA) UnmarshalJSON(data []byte) error {
	v.A = "decoded by its own UnmarshalJSON"
	return nil
}

type mtEmbeddedUnmarshalerB struct{ B string }

func (v *mtEmbeddedUnmarshalerB) UnmarshalJSON(data []byte) error {
	v.B = "decoded by its own UnmarshalJSON"
	return nil
}

// MT_Stringified has fields with the ,string option,
// which encoding/json applies to the pointers of the types it applies to as well.
type MT_Stringified struct {
	P   *int     `json:",string"`
	F   *float64 `json:",string"`
	S   *string  `json:",string"`
	I   int      `json:",string"`
	Str string   `json:",string"`
}

func makeMTStringified(t *testcase.T) MT_Stringified {
	var v = MT_Stringified{I: t.Random.Int(), Str: t.Random.String()}
	if t.Random.Bool() {
		p, f, s := t.Random.Int(), t.Random.Float64(), t.Random.String()
		v.P, v.F, v.S = &p, &f, &s
	}
	return v
}

// MT_StringifiedWithCodec has the fields of MT_Stringified promoted,
// and its Greeter field makes the Codec decode it with its own reflection based decoding.
type MT_StringifiedWithCodec struct {
	Greeter Greeter
	MT_Stringified
}

// MT_StringifiedThroughPointer has the fields of MT_Stringified promoted through an embedded pointer.
type MT_StringifiedThroughPointer struct {
	Greeter Greeter
	*MT_Stringified
}

// MT_Bytes has byte slices, and its Greeter field makes the Codec decode it with its own reflection based decoding.
type MT_Bytes struct {
	Greeter Greeter
	Data    []byte
	List    [][]byte
}

type MT_TaggedEmbeddedPointer struct {
	*mtEmbeddedUnexported `json:"hidden"`
}

// MT_T20 embeds a struct of an unexported type, which has an interface field promoted from it.
type MT_T20 struct {
	mtEmbeddedFooer
}

type mtEmbeddedFooer struct{ Fooer testent.Fooer }

type MT_T21 struct {
	Inner MT_T20
}

type MT_EmbeddedInterface struct {
	Name string
	testent.Fooer
}

type MT_EmbeddedInterfaceFirst struct {
	testent.Fooer
	Name string
}

// MT_TagNames has json tags with names which encoding/json handles in a special way.
type MT_TagNames struct {
	Dash    string `json:"-,"`             // the member is named "-"
	Invalid string `json:"in\\valid"`      // the name is not valid, so the member is named after the field
	Unknown string `json:"unknown,inline"` // an unknown option is ignored
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

// MT_GreeterMap is a map with interface values, which implements the same interface itself.
type MT_GreeterMap map[string]Greeter

func (MT_GreeterMap) Hello() {}

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

	s.Test("pointer struct field implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		roundTrip(t, &c, MT_JM_PointerStruct{Inner: &MT_JM_Inner{V: t.Random.String()}})
	})

	s.Test("top-level value implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		roundTrip(t, &c, MT_JM_Inner{V: t.Random.String()})
	})

	s.Test("top-level value implementing json.Marshaler and json.Unmarshaler, which is not a struct", func(t *testcase.T) {
		roundTrip(t, &c, uuid.Must(uuid.MakeV4))
	})

	s.Test("top-level slice of json.Marshaler elements", func(t *testcase.T) {
		roundTrip(t, &c, random.Slice(t.Random.IntBetween(1, 5), func() MT_JM_Inner {
			return MT_JM_Inner{V: t.Random.String()}
		}))
	})

	s.Test("top-level array of json.Marshaler elements", func(t *testcase.T) {
		roundTrip(t, &c, [2]MT_JM_Inner{{V: t.Random.String()}, {V: t.Random.String()}})
	})

	s.Test("top-level map of json.Marshaler values", func(t *testcase.T) {
		roundTrip(t, &c, map[string]MT_JM_Inner{t.Random.HexN(4): {V: t.Random.String()}})
	})

	s.Test("interface field holding a json.Unmarshaler implementation", func(t *testcase.T) {
		roundTrip(t, &c, struct{ V any }{V: withTypeCustomJSON{Value: t.Random.String()}})
	})

	s.Test("slice of interfaces holding json.Unmarshaler implementations", func(t *testcase.T) {
		roundTrip(t, &c, []any{withTypeCustomJSON{Value: t.Random.String()}})
	})

	// jsonkit.Array's own MarshalJSON and UnmarshalJSON can only use the package level registry,
	// so the Codec processes a jsonkit.Array of interfaces as a plain slice, to keep using its own registry.
	s.Test("jsonkit.Array of interfaces as a struct field is decoded with the Codec's own registry", func(t *testcase.T) {
		var isolated jsonkit.Codec // the implementations are registered only on this Codec
		t.Defer(jsonkit.CodecRegisterTypeID[wfCircle](&isolated, "circle"))
		t.Defer(jsonkit.CodecRegisterTypeID[wfSquare](&isolated, "square"))

		roundTrip(t, &isolated, struct{ Shapes jsonkit.Array[wfShape] }{
			Shapes: jsonkit.Array[wfShape]{wfCircle{R: t.Random.Float64()}, wfSquare{S: t.Random.Float64()}},
		})
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

// TestCodecRegisterMigration covers type migrations.
// Data written under a retired @type (with a legacy shape) decodes into the current type,
// while marshalling keeps emitting the current type's own @type.
func TestCodecRegisterMigration(t *testing.T) {
	s := testcase.NewSpec(t)

	type legacyFooDTO struct {
		LegacyID string `json:"legacy_id"`
		Name     string `json:"name"`
	}

	migrateLegacyFoo := func(c *jsonkit.Codec, data []byte, p *testent.Foo) error {
		var dto legacyFooDTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return err
		}
		p.ID = testent.FooID(dto.LegacyID)
		p.Foo = dto.Name
		return nil
	}

	codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegister(&c, "foo", TestEntFooTypeCodec{}))
		t.Defer(jsonkit.CodecRegisterMigration(&c, "legacy-foo", migrateLegacyFoo))
		return &c
	})

	legacyID := let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
	legacyName := let.Var(s, func(t *testcase.T) string { return t.Random.String() })
	legacyData := let.Var(s, func(t *testcase.T) []byte {
		data, err := json.Marshal(map[string]string{
			"@type":     "legacy-foo",
			"legacy_id": legacyID.Get(t),
			"name":      legacyName.Get(t),
		})
		assert.Must(t).NoError(err)
		return data
	})
	expected := let.Var(s, func(t *testcase.T) testent.Foo {
		return testent.Foo{ID: testent.FooID(legacyID.Get(t)), Foo: legacyName.Get(t)}
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		type Box struct {
			Fooer  testent.Fooer
			Fooers []testent.Fooer
			Foo    testent.Foo
		}

		boxOf := func(foo []byte) []byte {
			f := string(foo)
			return []byte(`{"Fooer":` + f + `,"Fooers":[` + f + `],"Foo":` + f + `}`)
		}

		data := let.Var(s, func(t *testcase.T) []byte {
			return boxOf(legacyData.Get(t))
		})

		// currentFoo is a Foo written today, under the current @type.
		currentFoo := let.Var(s, func(t *testcase.T) []byte {
			foo, err := codec.Get(t).Marshal(expected.Get(t))
			assert.Must(t).NoError(err)
			return foo
		})

		// unmarshalInto fails the test instead of blocking the suite when Unmarshal never returns.
		unmarshalInto := func(t *testcase.T, ptr any) error {
			c, in := codec.Get(t), data.Get(t)
			var err error
			assert.Within(t, 5*time.Second, func(context.Context) {
				err = c.Unmarshal(in, ptr)
			}, "Unmarshal is expected to return")
			return err
		}

		act := func(t *testcase.T) (Box, error) {
			var got Box
			err := unmarshalInto(t, &got)
			return got, err
		}

		s.Then("legacy data is migrated into the current type, whatever the destination", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal[testent.Fooer](t, got.Fooer, expected.Get(t))
			assert.Equal(t, got.Fooers, []testent.Fooer{expected.Get(t)})
			assert.Equal(t, got.Foo, expected.Get(t))
		})

		s.When("the data is a top-level legacy value", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte { return legacyData.Get(t) })

			s.Then("it decodes into a concrete destination", func(t *testcase.T) {
				var got testent.Foo
				assert.NoError(t, unmarshalInto(t, &got))
				assert.Equal(t, got, expected.Get(t))
			})
		})

		s.When("the data uses the canonical type ID", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return boxOf(currentFoo.Get(t))
			})

			s.Then("the canonical codec is still used", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[testent.Fooer](t, got.Fooer, expected.Get(t))
				assert.Equal(t, got.Fooers, []testent.Fooer{expected.Get(t)})
				assert.Equal(t, got.Foo, expected.Get(t))
			})
		})

		// The migration is the only decoder registered for the type,
		// so data written under the current type ID has to be decoded the default way.
		s.When("the type is registered by type ID only, without a custom codec", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo"))
				t.Defer(jsonkit.CodecRegisterMigration(&c, "legacy-foo", migrateLegacyFoo))
				return &c
			})

			s.Then("legacy data is still migrated", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[testent.Fooer](t, got.Fooer, expected.Get(t))
				assert.Equal(t, got.Fooers, []testent.Fooer{expected.Get(t)})
				assert.Equal(t, got.Foo, expected.Get(t))
			})

			s.And("the data was written under the current type ID", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return boxOf(currentFoo.Get(t))
				})

				s.Then("it decodes, whatever the destination", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[testent.Fooer](t, got.Fooer, expected.Get(t))
					assert.Equal(t, got.Fooers, []testent.Fooer{expected.Get(t)})
					assert.Equal(t, got.Foo, expected.Get(t))
				})
			})

			s.And("a top-level value was written under the current type ID", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return currentFoo.Get(t)
				})

				s.Then("it decodes into a concrete destination", func(t *testcase.T) {
					var got testent.Foo
					assert.NoError(t, unmarshalInto(t, &got))
					assert.Equal(t, got, expected.Get(t))
				})
			})
		})
	})

	s.Describe("#Marshal", func(s *testcase.Spec) {
		act := func(t *testcase.T) (string, error) {
			var got testent.Fooer
			assert.Must(t).NoError(codec.Get(t).Unmarshal(legacyData.Get(t), &got))
			data, err := codec.Get(t).Marshal(got)
			return string(data), err
		}

		s.Then("a value decoded from legacy data is written under the canonical type ID", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Contains(t, got, `"@type":"foo"`)
			assert.NotContains(t, got, `legacy-foo`)
		})
	})

	s.Test("a migration from an ID that is already a registered type ID is rejected", func(t *testcase.T) {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegister(&c, "foo", TestEntFooTypeCodec{}))
		assert.Panic(t, func() {
			jsonkit.CodecRegisterMigration(&c, "foo", func(c *jsonkit.Codec, data []byte, p *testent.Foo) error { return nil })
		})
	})

	s.Test("the returned function removes the migration", func(t *testcase.T) {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegister(&c, "foo", TestEntFooTypeCodec{}))
		jsonkit.CodecRegisterMigration(&c, "legacy-foo", func(c *jsonkit.Codec, data []byte, p *testent.Foo) error { return nil })()
		var got testent.Fooer
		assert.Error(t, c.Unmarshal(legacyData.Get(t), &got))
	})
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

	// Like the UnmarshalJSON method of a value, which encoding/json decodes into.
	s.Test("the registered Unmarshal gets the value the destination already holds", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", jsonkit.TypeCodec[testent.Foo]{
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *testent.Foo) error {
				// it decodes only some of the fields
				var dto struct{ Foo string }
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				p.Foo = dto.Foo
				return nil
			},
		})()

		var (
			existing = MakeFoo(t)
			exp      = existing
		)
		exp.Foo = t.Random.String()
		foo := mustMarshalJSON(t, map[string]string{"Foo": exp.Foo})

		got := existing
		assert.NoError(t, c.Unmarshal([]byte(foo), &got))
		assert.Equal(t, exp, got)

		box := struct{ Foo testent.Foo }{Foo: existing}
		assert.NoError(t, c.Unmarshal([]byte(`{"Foo":`+foo+`}`), &box))
		assert.Equal(t, exp, box.Foo)
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

	s.Test("slices, maps and nested structs of the registered type as struct fields", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type Inner struct{ Foo testent.Foo }
		type T struct {
			List   []testent.Foo
			ByName map[string]testent.Foo
			Inner  Inner
		}

		exp := T{
			List:   []testent.Foo{testent.MakeFoo(t), testent.MakeFoo(t)},
			ByName: map[string]testent.Foo{t.Random.HexN(8): testent.MakeFoo(t)},
			Inner:  Inner{Foo: testent.MakeFoo(t)},
		}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// Like with encoding/json, the fields of an embedded struct are promoted into the JSON object of the outer struct,
	// so the custom codec of an embedded type is not used.
	s.Test("an embedded registered type has its fields promoted, like with encoding/json", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct {
			Name string
			testent.Foo
		}

		assertEncodesLikeEncodingJSON(t, &c, T{Name: t.Random.String(), Foo: testent.MakeFoo(t)})
	})

	s.Test("a struct field of the registered type is decoded as its zero value when its member is missing", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct {
			Name string
			Foo  testent.Foo `json:",omitzero"`
		}
		exp := T{Name: t.Random.String()}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assert.ContainsExactly(t, []string{"Name"}, jsonMemberNames(t, data))

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("a pointer struct field of the registered type round-trips, also when it is nil", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct{ Set, Nil *testent.Foo }
		foo := testent.MakeFoo(t)
		exp := T{Set: &foo}

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

	// A custom codec only applies to the Codec it was registered on.
	// Any other Codec writes the type in its default JSON shape, and has to read that shape back,
	// also when the type is a struct field.
	s.Test("another codec, without the registration, round-trips the type as a struct field", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			X string `json:"x"`
		}

		var registered jsonkit.Codec
		t.Defer(jsonkit.CodecRegister[T](&registered, "registered_elsewhere_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO{X: v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				p.V = dto.X
				return nil
			},
		}))

		type Container struct{ Val T }
		exp := Container{Val: T{V: t.Random.String()}}

		var other jsonkit.Codec
		data, err := other.Marshal(exp)
		assert.NoError(t, err)

		var got Container
		assert.NoError(t, other.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
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
	// dispatch of a slice must also receive the codec. The assertion below
	// drives the same regression class via a heterogeneous slice round-trip.
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

	// Companion scenario for registered container elements without their own @type envelope.
	// The element type is known from the container, so the element is decoded straight into it,
	// and the element type's custom unmarshal must receive the codec there too.
	s.Test("registered container dispatch propagates the codec to the custom unmarshal of elements without @type", func(t *testcase.T) {
		type MT_Inners []MT_Inner

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_Inner](&c, "inner",
			jsonkit.TypeCodec[MT_Inner]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_Inner) ([]byte, error) {
					return json.Marshal(map[string]string{"marker": v.Marker})
				},
				UnmarshalFunc: func(supplied *jsonkit.Codec, data []byte, p *MT_Inner) error {
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
		defer jsonkit.CodecRegisterTypeID[MT_Inners](&c, "inners")()

		marker := t.Random.HexN(8)
		data := []byte(`{"V":{"@type":"inners","@value":[{"marker":` + strconv.Quote(marker) + `}]}}`)

		var got struct{ V any }
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal[any](t, MT_Inners{{Marker: marker}}, got.V)
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

// TestCodec_sliceElementCustomCodecReceivesOriginatingCodec pins the rule
// that when the codec marshals a slice whose elements have user-registered
// custom Marshal closures, each closure receives the originating *Codec —
// not a nil pointer — so that any recursive `c.Marshal(...)` call inside
// the closure can resolve the slice element's nested registrations.
//
// Originally filed as: marshalling a []workflow.Event whose first element
// was workflow.EventUseDefinition carrying a workflow.ForEach{Do: …}
// panicked with:
//
//	runtime error: invalid memory address or nil pointer dereference
//	  ...WorkflowEventUseDefinition.Marshal(_, ...)
//	  defBytes, err := c.Marshal(v.Definition)
//	                ^^^   <-- c was nil
//
// Root cause: the slice marshalling path (marshalSequencePlaceholderWithReg)
// forwarded slice element marshaling with codec=nil. Custom closures that
// recurse via c.Marshal then dereferenced a nil *Codec.
//
// The bug was at the jsonkit level, surfacing through the wfjson package.
// This test pins the jsonkit-level contract directly so a future regression
// is caught by `go test ./pkg/jsonkit/...` alone, without needing to also
// run the workflow-level tests.
func TestCodec_sliceElementCustomCodecReceivesOriginatingCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	// MT_SliceInner is a struct with its own custom codec. Its wire
	// shape is a JSON object carrying a `marker` field.
	type MT_SliceInner struct{ Marker string }

	// MT_SliceOuter holds an inner value and its custom codec recurses
	// into the codec for the inner. This mirrors workflow.EventUseDefinition,
	// whose Marshal closure calls c.Marshal(v.Definition).
	type MT_SliceOuter struct{ Inner MT_SliceInner }

	s.Test("a custom codec inside a slice element receives a non-nil *Codec", func(t *testcase.T) {
		var c jsonkit.Codec

		defer jsonkit.CodecRegister[MT_SliceInner](&c, "slice-inner",
			jsonkit.TypeCodec[MT_SliceInner]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_SliceInner) ([]byte, error) {
					return json.Marshal(struct {
						Marker string `json:"marker"`
					}{Marker: v.Marker})
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_SliceInner) error {
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

		defer jsonkit.CodecRegister[MT_SliceOuter](&c, "slice-outer",
			jsonkit.TypeCodec[MT_SliceOuter]{
				MarshalFunc: func(supplied *jsonkit.Codec, v MT_SliceOuter) ([]byte, error) {
					// A nil supplied codec is the regression: the closure
					// must call supplied.Marshal(...) to round-trip the
					// nested MT_SliceInner, and a nil pointer panics at
					// the recursive call. Asserting here turns a deep
					// panic into a clear test failure.
					if supplied == nil {
						return nil, fmt.Errorf("custom codec for slice element received a nil *Codec")
					}
					innerBytes, err := supplied.Marshal(v.Inner)
					if err != nil {
						return nil, err
					}
					return json.Marshal(struct {
						Inner json.RawMessage `json:"inner"`
					}{Inner: innerBytes})
				},
				UnmarshalFunc: func(supplied *jsonkit.Codec, data []byte, p *MT_SliceOuter) error {
					var dto struct {
						Inner json.RawMessage `json:"inner"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					var inner MT_SliceInner
					if err := supplied.Unmarshal(dto.Inner, &inner); err != nil {
						return err
					}
					p.Inner = inner
					return nil
				},
			},
		)()

		outers := []MT_SliceOuter{
			{Inner: MT_SliceInner{Marker: "a"}},
			{Inner: MT_SliceInner{Marker: "b"}},
		}

		// Marshalling must not panic with a nil pointer dereference.
		data, err := c.Marshal(outers)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		// Each slice element emitted its @type envelope plus the inner
		// value's envelope. Both pieces are observable, so a regression
		// in the slice dispatch path shows up as a wire-shape failure.
		assert.Contains(t, string(data), `"@type":"slice-outer"`)
		assert.Contains(t, string(data), `"@type":"slice-inner"`)

		var got []MT_SliceOuter
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, outers, got)
	})
}
