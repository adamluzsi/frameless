package jsonkit_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math"
	"net"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/codec/codeccontract"
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

func TestCodec_implementsCodecContract(t *testing.T) {
	codeccontract.Codec(&jsonkit.Codec{}, codeccontract.CodecConfig{SupportInterface: true}).Test(t)
}

func TestLinesCodec_implementsCodecContract(t *testing.T) {
	codeccontract.Codec(jsonkit.LinesCodec{}).Test(t)
}

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

func TestNewArrayStreamDecoder(t *testing.T) {
	t.Run("the elements of a JSON array are decoded one by one", func(t *testing.T) {
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

func TestNewArrayStreamEncoder(t *testing.T) {
	t.Run("the encoded values are written as the elements of a JSON array", func(t *testing.T) {
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
	s.Before(RegisterTypeIDs)

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

		s.Then("JSON without $type is decoded into `any` the same way as encoding/json does, except that a whole number is an int", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var exp any
			unmarshalPlainJSON(t, data.Get(t), &exp)
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

		// JSON without $type carries no type information,
		// so wherever the `any` is, the Codec decodes it the same way as encoding/json does,
		// except that a number of only digits is read as an int, see "the JSON value is a number of only digits".
		thenItIsDecodedLikeWithEncodingJSON := func(t *testcase.T) {
			exp := reflect.New(reflect.TypeOf(ptr.Get(t)).Elem()).Interface()
			unmarshalPlainJSON(t, data.Get(t), exp)

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

			s.Then("JSON without $type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a struct field with a map of `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V map[string]any })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"k":` + value.Get(t) + `}}`)
			})

			s.Then("JSON without $type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a struct field with a slice of `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V []any })
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":[` + value.Get(t) + `]}`)
			})

			s.Then("JSON without $type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		s.When("the destination is a map of `any` with integer keys", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[int]any)
			})
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"` + strconv.Itoa(t.Random.Int()) + `":` + value.Get(t) + `}`)
			})

			s.Then("JSON without $type is decoded the same way as encoding/json does", thenItIsDecodedLikeWithEncodingJSON)
		})

		// encoding/json reads every JSON number into an `any` as a float64,
		// so an int held by an `any` would come back as a float64, and a big one would be rounded on the way.
		// The Codec reads a number of only digits as an int instead, using json.Decoder.UseNumber,
		// so an int held by an `any` is written as plain JSON, without a $type, and it is still read back as an int.
		s.When("the JSON value is a number of only digits", func(s *testcase.Spec) {
			number := let.Var(s, makeSignedInt)
			value.Let(s, func(t *testcase.T) string {
				return strconv.Itoa(number.Get(t))
			})

			s.Then("it is read as an int", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			s.And("it is negative", func(s *testcase.Spec) {
				number.Let(s, func(t *testcase.T) int {
					return -t.Random.IntBetween(1, math.MaxInt)
				})

				s.Then("it is read as a negative int", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
				})
			})

			// A float64 holds every int up to 2^53 exactly, but only every second one above that.
			s.And("it is an int which a float64 can't hold exactly", func(s *testcase.Spec) {
				number.Let(s, func(t *testcase.T) int {
					return 1<<53 + 1 + 2*t.Random.IntN(1000)
				})

				s.Then("it is read as the very same int, without being rounded to a float64 on the way", func(t *testcase.T) {
					assert.Must(t).NotEqual(number.Get(t), int(float64(number.Get(t))))

					assert.NoError(t, act(t))
					assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
				})
			})

			s.And("it is too big for an int", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) string {
					return strconv.FormatUint(math.MaxUint64-uint64(t.Random.IntN(1000)), 10)
				})

				s.Then("it is read as a float64, like encoding/json reads it", func(t *testcase.T) {
					assert.NoError(t, act(t))

					exp, err := strconv.ParseFloat(value.Get(t), 64)
					assert.Must(t).NoError(err)
					assert.Equal[any](t, exp, *ptr.Get(t).(*any))
				})
			})

			s.And("it is quoted", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) string {
					return strconv.Quote(value.Super(t))
				})

				s.Then("it is read as a string, like encoding/json reads it", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, strconv.Itoa(number.Get(t)), *ptr.Get(t).(*any))
				})
			})

			s.And("it is followed by more JSON", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) string {
					return value.Super(t) + random.Pick(t.Random, " 42", "]", "}", ",1", "x", ` "x"`)
				})

				s.Then("it fails with a *json.SyntaxError, like with encoding/json", func(t *testcase.T) {
					err := act(t)
					var syntaxErr *json.SyntaxError
					assert.True(t, errors.As(err, &syntaxErr), assert.MessageF("expected a *json.SyntaxError, got: %v", err))
				})
			})

			s.And("the destination is a struct field with an `any` type", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ V any })
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"V":` + value.Get(t) + `}`)
				})

				s.Then("the field is read as an int", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, number.Get(t), ptr.Get(t).(*struct{ V any }).V)
				})
			})

			s.And("the destination is a struct field with a slice of `any`", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ V []any })
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"V":[` + value.Get(t) + `]}`)
				})

				s.Then("the element is read as an int", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, []any{number.Get(t)}, ptr.Get(t).(*struct{ V []any }).V)
				})
			})

			s.And("the destination is a map of `any`", func(s *testcase.Spec) {
				key := let.Var(s, func(t *testcase.T) string {
					return t.Random.HexN(8)
				})
				ptr.Let(s, func(t *testcase.T) any {
					return new(map[string]any)
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{` + strconv.Quote(key.Get(t)) + `:` + value.Get(t) + `}`)
				})

				s.Then("the entry is read as an int", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]any{key.Get(t): number.Get(t)}, *ptr.Get(t).(*map[string]any))
				})
			})
		})

		s.When("the JSON value is a number with a fraction or an exponent", func(s *testcase.Spec) {
			number := let.Var(s, makeFloat64WithFraction)
			value.Let(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, number.Get(t))
			})

			s.Then("it is read as the very same float64, like encoding/json reads it", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			// Marshal writes a whole float64 held by an `any` with a .0 fraction, which tells it apart from an int.
			s.And("it is a whole number with a .0 fraction", func(s *testcase.Spec) {
				number.Let(s, makeWholeFloat64)
				value.Let(s, func(t *testcase.T) string {
					return mustMarshalJSON(t, number.Get(t)) + ".0"
				})

				s.Then("it is read as the very same float64, and not as an int", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
				})
			})

			// Marshal writes a negative zero float64 held by an `any` as -0.0.
			s.And("it is a negative zero", func(s *testcase.Spec) {
				value.LetValue(s, "-0.0")

				s.Then("it is read as a float64 which keeps its sign", func(t *testcase.T) {
					assert.NoError(t, act(t))

					got, ok := (*ptr.Get(t).(*any)).(float64)
					assert.True(t, ok, assert.MessageF("expected a float64, got %T", *ptr.Get(t).(*any)))
					assert.Equal(t, 0.0, got)
					assert.True(t, math.Signbit(got))
				})
			})

			// Like 1e+21 or 5E-7: Marshal writes a float64 with an exponent, when it is 1e21 or more, or less than 1e-6, apart from zero,
			// and other JSON writers may write any number with an exponent.
			s.And("it has an exponent", func(s *testcase.Spec) {
				number.Let(s, makeFloat64WithExponent)
				value.Let(s, func(t *testcase.T) string {
					mantissa, exp, _ := strings.Cut(strconv.FormatFloat(number.Get(t), 'e', -1, 64), "e")
					if t.Random.Bool() {
						exp = strings.TrimPrefix(exp, "+") // the sign of a positive exponent is optional
					}
					return mantissa + random.Pick(t.Random, "e", "E") + exp
				})

				s.Then("it is read as the very same float64, like encoding/json reads it", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, number.Get(t), *ptr.Get(t).(*any))
				})

				s.And("it is a whole number which fits into an int", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) string {
						return strconv.Itoa(t.Random.IntBetween(-9, 9)) + random.Pick(t.Random, "e", "E", "e+", "E+") + strconv.Itoa(t.Random.IntBetween(0, 15))
					})

					s.Then("it is read as a float64, like encoding/json reads it, and not as an int", func(t *testcase.T) {
						assert.NoError(t, act(t))

						exp, err := strconv.ParseFloat(value.Get(t), 64)
						assert.Must(t).NoError(err)
						assert.Equal[any](t, exp, *ptr.Get(t).(*any))
					})
				})
			})

			// encoding/json fails on a number which is out of the range of a float64.
			s.And("it is out of the range of a float64", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) string {
					return "1e" + strconv.Itoa(t.Random.IntBetween(309, 999))
				})

				s.Then("it fails with a *json.UnmarshalTypeError, like with encoding/json", func(t *testcase.T) {
					err := act(t)
					var typeErr *json.UnmarshalTypeError
					assert.True(t, errors.As(err, &typeErr), assert.MessageF("expected a *json.UnmarshalTypeError, got: %v", err))
				})
			})
		})

		s.When("the JSON value holds numbers nested in arrays and objects", func(s *testcase.Spec) {
			var (
				whole             = let.Var(s, makeSignedInt)
				nested            = let.Var(s, makeSignedInt)
				withFraction      = let.Var(s, makeFloat64WithFraction)
				wholeWithFraction = let.Var(s, makeWholeFloat64)
				withExponent      = let.Var(s, makeFloat64WithExponent)
			)
			value.Let(s, func(t *testcase.T) string {
				return fmt.Sprintf(`{"a":[%d,{"b":[%d]}],"c":%s,"d":%s.0,"e":%s}`,
					whole.Get(t), nested.Get(t),
					strconv.FormatFloat(withFraction.Get(t), 'f', -1, 64),
					strconv.FormatFloat(wholeWithFraction.Get(t), 'f', -1, 64),
					mustMarshalJSON(t, withExponent.Get(t)))
			})

			s.Then("each number of only digits is read as an int, and each other one as a float64", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, map[string]any{
					"a": []any{whole.Get(t), map[string]any{"b": []any{nested.Get(t)}}},
					"c": withFraction.Get(t),
					"d": wholeWithFraction.Get(t),
					"e": withExponent.Get(t),
				}, *ptr.Get(t).(*any))
			})
		})

		// WithType writes a built-in type with its type, like {"$type":"int","$value":42}.
		s.When("the JSON value is a $value envelope of a built-in type", func(s *testcase.Spec) {
			builtin := let.Var(s, func(t *testcase.T) any {
				return random.Pick[any](t.Random,
					t.Random.String(),
					t.Random.Bool(),
					t.Random.Float64(),
					t.Random.Int(),
					uint8(t.Random.IntN(256)),
				)
			})
			value.Let(s, func(t *testcase.T) string {
				return makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(builtin.Get(t)).String()), builtin.Get(t))
			})

			s.Then("it is decoded into a value of that type", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, builtin.Get(t), *ptr.Get(t).(*any))
			})

			// The members of a JSON object are unordered, and a store may reorder them, like a PostgreSQL jsonb column.
			s.And("the $type is not the first member of the envelope", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) string {
					return `{"$value":` + mustMarshalJSON(t, builtin.Get(t)) + `,"$type":` +
						mustMarshalJSON(t, reflect.TypeOf(builtin.Get(t)).String()) + `}`
				})

				s.Then("it is decoded into a value of that type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, builtin.Get(t), *ptr.Get(t).(*any))
				})

				s.And("the destination is of the type the $type names", func(s *testcase.Spec) {
					ptr.Let(s, func(t *testcase.T) any {
						return reflect.New(reflect.TypeOf(builtin.Get(t))).Interface()
					})

					s.Then("the $value is decoded into it", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal(t, builtin.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
					})
				})
			})
		})

		s.When("the JSON value is a $type envelope of a registered type", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[testent.FooerT1](c, "fooer-t1"))
				return c
			})
			fooer := let.Var(s, func(t *testcase.T) testent.FooerT1 {
				return testent.FooerT1{V: t.Random.String()}
			})
			value.Let(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, map[string]any{"$type": "fooer-t1", "V": fooer.Get(t).V})
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

			// A JSON array carries no type information, so like with encoding/json, an `any` holds it as a []any.
			// Its elements are still decoded by their $type, like the values of a JSON object are.
			s.And("it is an element of a JSON array", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`[` + value.Get(t) + `]`)
				})

				s.Then("the array is decoded into a slice of `any`, which holds the registered type", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, []any{fooer.Get(t)}, *ptr.Get(t).(*any))
				})

				itIgnoresTheWhitespaceAroundTheJSON(s)

				s.And("the array is an element of another JSON array", func(s *testcase.Spec) {
					data.Let(s, func(t *testcase.T) []byte {
						return []byte(`[` + string(data.Super(t)) + `]`)
					})

					s.Then("each array is decoded into a slice of `any`, and the innermost one holds the registered type", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal[any](t, []any{[]any{fooer.Get(t)}}, *ptr.Get(t).(*any))
					})
				})

				s.And("the destination is a struct field with an `any` type", func(s *testcase.Spec) {
					ptr.Let(s, func(t *testcase.T) any {
						return new(struct{ V any })
					})
					data.Let(s, func(t *testcase.T) []byte {
						return []byte(`{"V":` + string(data.Super(t)) + `}`)
					})

					s.Then("the array is decoded into a slice of `any`, which holds the registered type", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal[any](t, []any{fooer.Get(t)}, ptr.Get(t).(*struct{ V any }).V)
					})

					itIgnoresTheWhitespaceAroundTheJSON(s)
				})
			})
		})

		s.When("the JSON value is a $type envelope of a type ID which the Codec doesn't know", func(s *testcase.Spec) {
			typeID := let.Var(s, func(t *testcase.T) jsonkit.TypeID {
				return jsonkit.TypeID("unknown-" + t.Random.HexN(8))
			})
			value.Let(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, map[string]any{"$type": typeID.Get(t), "V": t.Random.String()})
			})

			s.Then("it fails with ErrUnknownTypeID, which names the type ID", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
				assert.Contains(t, err.Error(), strconv.Quote(string(typeID.Get(t))))
			})

			s.And("the type ID is registered for an interface type", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[testent.Fooer](c, typeID.Get(t)))
					return c
				})

				s.Then("it fails with ErrUnknownTypeID, which tells that the type ID names an interface type, which no value is of", func(t *testcase.T) {
					err := act(t)
					assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
					assert.Contains(t, err.Error(), strconv.Quote(string(typeID.Get(t))))
					assert.Contains(t, err.Error(), reflect.TypeFor[testent.Fooer]().String())
				})
			})
		})

		s.When("the JSON value is a $type envelope of a registered container type", func(s *testcase.Spec) {
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

		// Like with encoding/json, the JSON of a value can be read into another type of the same shape, like into a DTO.
		// Once the destination tells the type, the $type of a type the Codec knows has done its job.
		s.When("the JSON value is a $type envelope of a registered slice type, and the destination is a plain slice of the same shape", func(s *testcase.Spec) {
			elems := let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(1, 3), t.Random.Int)
			})
			value.Let(s, func(t *testcase.T) string {
				return makeValueEnvelopeJSON(t, "with_type_slice", elems.Get(t))
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new([]int)
			})

			s.Then("it is decoded from the $value of the envelope", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, elems.Get(t), *ptr.Get(t).(*[]int))
			})

			itIgnoresTheWhitespaceAroundTheJSON(s)

			s.And("it is the member of a struct field of the plain slice type", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ V []int })
				})
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"V":` + value.Get(t) + `}`)
				})

				s.Then("it is decoded from the $value of the envelope", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, elems.Get(t), ptr.Get(t).(*struct{ V []int }).V)
				})
			})
		})

		s.When("the JSON value is a $map envelope of a registered map which has an entry with a $ prefixed key", func(s *testcase.Spec) {
			entries := let.Var(s, func(t *testcase.T) map[string]int {
				return map[string]int{"$" + t.Random.HexN(4): t.Random.Int()}
			})
			value.Let(s, func(t *testcase.T) string {
				return `{"$type":"with_type_map","$map":` + mustMarshalJSON(t, entries.Get(t)) + `}`
			})

			s.Then("the entry is kept, as only a flat envelope has a $type next to the entries", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, entries.Get(t), *ptr.Get(t).(*any))
			})

			s.And("the destination is the registered map type itself", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(map[string]int)
				})

				s.Then("the entry is kept, as only a flat envelope has a $type next to the entries", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, entries.Get(t), *ptr.Get(t).(*map[string]int))
				})
			})
		})

		s.When("the JSON value is a $type envelope of a registered map with interface values", func(s *testcase.Spec) {
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
				assert.NoError(t, act(t))
				assert.Equal[Greeter](t, greeters.Get(t), *ptr.Get(t).(*Greeter))
			})
		})

		// A non-empty interface can only hold a value that implements it.
		// When the JSON describes anything else, Unmarshal reports it as an error.
		itRejectsValuesTheInterfaceCannotHold := func(s *testcase.Spec) {
			thenItFails := func(t *testcase.T) {
				assert.Error(t, act(t))
			}

			s.Then("JSON without $type is rejected with an error", thenItFails)

			s.And("the JSON value is a bare array", func(s *testcase.Spec) {
				value.Let(s, makeUntypedJSONArray)

				s.Then("it is rejected with an error", thenItFails)
			})

			s.And("the $type names a registered struct type which does not implement the interface", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](c, "foo"))
					return c
				})
				value.LetValue(s, `{"$type":"foo","$value":{}}`)

				s.Then("it is rejected with an error", thenItFails)
			})

			s.And("the $type names a registered slice type which does not implement the interface", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[MT_Fooers](c, "fooers"))
					return c
				})
				value.LetValue(s, `{"$type":"fooers","$value":[]}`)

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

			s.And("it is a struct without interface fields", func(s *testcase.Spec) {
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

			s.And("it is a map of numbers", func(s *testcase.Spec) {
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

			s.And("it is an array of numbers, and the JSON array has fewer elements", func(s *testcase.Spec) {
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

	s.Describe("#Marshal", func(s *testcase.Spec) {
		var (
			// value is the value under test, which an `any` holds.
			// Each context decides where that `any` sits within v, see hold.
			value = let.Var(s, func(t *testcase.T) any {
				return makeJSONTypeScalar(t)
			})
			// valueJSON is the JSON the Codec is expected to write for value.
			valueJSON = let.Var(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, value.Get(t))
			})
			// hold makes v, by putting a value into the `any` of the context, wherever that `any` sits within v.
			hold = let.Var(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return x }
			})
			// holdBack makes what ptr is expected to point to, once the JSON is read back,
			// by putting what the `any` is expected to hold into the `any` of the context.
			// It is hold, unless the JSON is read back into another type than the type of v.
			holdBack = let.Var(s, func(t *testcase.T) func(any) any {
				return hold.Get(t)
			})
			v = let.Var(s, func(t *testcase.T) any {
				return hold.Get(t)(value.Get(t))
			})
			// expJSON is the JSON the Codec is expected to write for v.
			expJSON = let.Var(s, func(t *testcase.T) string {
				return valueJSON.Get(t)
			})
			// ptr points to the value the JSON is read back into.
			ptr = let.Var(s, func(t *testcase.T) any {
				return new(any)
			})
			// readBack is what the `any` is expected to hold, once the JSON is read back, see thenItIsReadBackLikeEncodingJSON.
			// It is what encoding/json reads valueJSON into, when the destination is an `any`,
			// except that a number of only digits is an int.
			readBack = let.Var(s, func(t *testcase.T) any {
				var x any
				unmarshalPlainJSON(t, []byte(valueJSON.Get(t)), &x)
				return x
			})
		)
		act := func(t *testcase.T) ([]byte, error) {
			return subject.Get(t).Marshal(v.Get(t))
		}

		thenItIsWrittenAsExpected := func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, expJSON.Get(t), string(data))
		}

		thenItIsReadBackAsTheSameValue := func(t *testcase.T) {
			data, err := act(t)
			assert.Must(t).NoError(err)

			assert.NoError(t, subject.Get(t).Unmarshal(data, ptr.Get(t)))
			assert.Equal(t, holdBack.Get(t)(value.Get(t)), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
		}

		thenItIsReadBackLikeEncodingJSON := func(t *testcase.T) {
			data, err := act(t)
			assert.Must(t).NoError(err)

			assert.NoError(t, subject.Get(t).Unmarshal(data, ptr.Get(t)))
			assert.Equal(t, holdBack.Get(t)(readBack.Get(t)), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
		}

		// ptrToTheTypeOfV is for reading the JSON back into the type of v, which the caller of Marshal knows.
		ptrToTheTypeOfV := func(t *testcase.T) any {
			return reflect.New(reflect.TypeOf(v.Get(t))).Interface()
		}

		// The Codec reads plain JSON into an `any` as a nil, bool, int, float64 or string, or a []any or map[string]any of these:
		// like encoding/json, except that a number of only digits is an int, see #Unmarshal.
		// So a value of these types needs no $type to be read back as the very same value,
		// wherever the `any` holding it sits.
		itWritesTheTypesOfPlainJSONWithoutTypeID := func(s *testcase.Spec) {
			s.Then("a value of the types plain JSON is read into is written without a $type, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)

			s.When("the value is nil", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return nil
				})

				s.Then("it is written as null, like with encoding/json", thenItIsWrittenAsExpected)

				s.Then("it is read back as nil", thenItIsReadBackAsTheSameValue)
			})

			s.When("the value is a slice or a map of these", func(s *testcase.Spec) {
				value.Let(s, makeJSONTypesContainer)

				s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsExpected)

				s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.When("the value is an int", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeSignedInt(t)
				})

				s.Then("it is written as a plain JSON number, without a $type", thenItIsWrittenAsExpected)

				s.Then("it is read back as the very same int", thenItIsReadBackAsTheSameValue)

				s.And("the Codec has a registration of its own for int", func(s *testcase.Spec) {
					typeID := let.Var(s, func(t *testcase.T) jsonkit.TypeID {
						return jsonkit.TypeID(t.Random.HexN(8))
					})
					subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
						c := subject.Super(t)
						t.Defer(jsonkit.CodecRegisterTypeID[int](c, typeID.Get(t)))
						return c
					})
					valueJSON.Let(s, func(t *testcase.T) string {
						return makeValueEnvelopeJSON(t, typeID.Get(t), value.Get(t))
					})

					s.Then("it is written with the $type of the registration", thenItIsWrittenAsExpected)

					s.Then("it is read back as the very same int", thenItIsReadBackAsTheSameValue)
				})
			})

			// Whatever float64 it is, it is read back as a float64, while the Codec reads a number of only digits into an `any` as an int.
			s.When("the value is a float64", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeFloat64(t)
				})

				s.Then("it is read back as the very same float64", thenItIsReadBackAsTheSameValue)

				s.And("it is not a whole number", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return makeFloat64WithFraction(t)
					})

					s.Then("it is written like with encoding/json, without a $type", thenItIsWrittenAsExpected)

					s.Then("it is read back as the very same float64", thenItIsReadBackAsTheSameValue)
				})

				// The Codec reads a number of only digits into an `any` as an int,
				// so a whole float64 is written with a fraction, which tells it apart from an int.
				s.And("it is a whole number", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return makeWholeFloat64(t)
					})
					valueJSON.Let(s, func(t *testcase.T) string {
						return mustMarshalJSON(t, value.Get(t)) + ".0"
					})

					s.Then("it is written like with encoding/json, but with a .0 fraction, without a $type", thenItIsWrittenAsExpected)

					s.Then("it is read back as the very same float64, and not as an int", thenItIsReadBackAsTheSameValue)
				})

				s.And("it is a negative zero", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return math.Copysign(0, -1)
					})
					valueJSON.LetValue(s, "-0.0")

					s.Then("it is written as -0.0", thenItIsWrittenAsExpected)

					// A negative zero equals a zero, so its sign is checked by writing what is read back once more.
					s.Then("it is read back as a negative zero, which is written as -0.0 again", func(t *testcase.T) {
						data, err := act(t)
						assert.Must(t).NoError(err)
						assert.NoError(t, subject.Get(t).Unmarshal(data, ptr.Get(t)))

						again, err := subject.Get(t).Marshal(ptr.Get(t))
						assert.NoError(t, err)
						assertJSONEqual(t, string(data), string(again))
					})
				})

				// encoding/json writes a float64 with an exponent, when it is 1e21 or more, or less than 1e-6, apart from zero,
				// like 1e+21, which has no dot, though it is not an int.
				s.And("encoding/json writes it with an exponent", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return makeFloat64WithExponent(t)
					})

					s.Then("it is written like with encoding/json, without a .0 fraction, as the exponent tells it is not an int", thenItIsWrittenAsExpected)

					s.Then("it is read back as the very same float64", thenItIsReadBackAsTheSameValue)
				})

				s.And("the Codec has a registration of its own for float64", func(s *testcase.Spec) {
					typeID := let.Var(s, func(t *testcase.T) jsonkit.TypeID {
						return jsonkit.TypeID(t.Random.HexN(8))
					})
					subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
						c := subject.Super(t)
						t.Defer(jsonkit.CodecRegisterTypeID[float64](c, typeID.Get(t)))
						return c
					})
					valueJSON.Let(s, func(t *testcase.T) string {
						return makeValueEnvelopeJSON(t, typeID.Get(t), value.Get(t))
					})

					s.Then("it is written with the $type of the registration, and its $value like with encoding/json, as the $type tells its type", thenItIsWrittenAsExpected)

					s.Then("it is read back as the very same float64", thenItIsReadBackAsTheSameValue)
				})
			})

			s.When("the value is a number of another built-in type, like an int64", func(s *testcase.Spec) {
				value.Let(s, makeOtherBuiltinNumber)
				valueJSON.Let(s, func(t *testcase.T) string {
					return makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(value.Get(t)).String()), value.Get(t))
				})

				s.Then("it is written with its $type, as plain JSON would be read back as an int or a float64", thenItIsWrittenAsExpected)

				s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.When("the value is of a named type, which is registered with the Codec", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[MT_Text](c, "mt-text"))
					return c
				})
				value.Let(s, func(t *testcase.T) any {
					return MT_Text(t.Random.String())
				})
				valueJSON.Let(s, func(t *testcase.T) string {
					return makeValueEnvelopeJSON(t, "mt-text", value.Get(t))
				})

				s.Then("it is written with its $type", thenItIsWrittenAsExpected)

				s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.When("the Codec has a registration of its own for the type", func(s *testcase.Spec) {
				typeID := let.Var(s, func(t *testcase.T) jsonkit.TypeID {
					return jsonkit.TypeID(t.Random.HexN(8))
				})
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[string](c, typeID.Get(t)))
					return c
				})
				value.Let(s, func(t *testcase.T) any {
					return t.Random.String()
				})
				valueJSON.Let(s, func(t *testcase.T) string {
					return makeValueEnvelopeJSON(t, typeID.Get(t), value.Get(t))
				})

				s.Then("it is written with the $type of the registration", thenItIsWrittenAsExpected)

				s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.When("the value is of a type which is registered with a custom codec", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegister[MT_Custom](c, "mt-custom", mtCustomCodec))
					return c
				})
				value.Let(s, func(t *testcase.T) any {
					return MT_Custom{N: t.Random.Int()}
				})
				valueJSON.Let(s, func(t *testcase.T) string {
					return `{"$type":"mt-custom",` + strings.TrimPrefix(mtCustomJSON(value.Get(t).(MT_Custom)), `{`)
				})

				s.Then("it is written by the custom codec, with its $type", thenItIsWrittenAsExpected)

				s.Then("it is read back by the custom codec as the very same value", thenItIsReadBackAsTheSameValue)

				// The members of a JSON object are unordered, and a store may reorder them, like a PostgreSQL jsonb column.
				s.Then("its JSON is read by the custom codec, when the $type is not the first member", func(t *testcase.T) {
					v := value.Get(t).(MT_Custom)
					reordered := strings.TrimSuffix(mtCustomJSON(v), `}`) + `,"$type":"mt-custom"}`

					var viaAny any
					assert.NoError(t, subject.Get(t).Unmarshal([]byte(reordered), &viaAny))
					assert.Equal[any](t, v, viaAny)

					var concrete struct{ V MT_Custom }
					assert.NoError(t, subject.Get(t).Unmarshal([]byte(`{"V":`+reordered+`}`), &concrete))
					assert.Equal(t, v, concrete.V)
				})
			})

			// Like encoding/json, which writes any value held by an `any`, and reads its JSON back as the generic shape of it,
			// a value of a type without a type ID, which can't tell its type, is written the way encoding/json writes it, as a best effort.
			s.When("the value is of a type without a type ID, like a time.Time", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return t.Random.Time()
				})

				s.Then("it is written the way encoding/json writes it, without a $type", thenItIsWrittenAsExpected)

				s.Then("it is read back the way encoding/json reads it back, like a time.Time as a string", thenItIsReadBackLikeEncodingJSON)
			})

			s.When("the value is a struct without a type ID", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return MT_NoTypeID{Name: t.Random.String(), Count: t.Random.Int()}
				})

				s.Then("it is written the way encoding/json writes it, without a $type", thenItIsWrittenAsExpected)

				s.Then("it is read back the way encoding/json reads it back, as a map[string]any", thenItIsReadBackLikeEncodingJSON)

				s.And("it is held through a pointer", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						v := value.Super(t).(MT_NoTypeID)
						return &v
					})

					s.Then("it is written the way encoding/json writes it, without a $type", thenItIsWrittenAsExpected)

					s.Then("it is read back the way encoding/json reads it back, as a map[string]any", thenItIsReadBackLikeEncodingJSON)
				})

				s.And("it holds a value of a registered type in an `any` of its own", func(s *testcase.Spec) {
					inner := let.Var(s, func(t *testcase.T) MT_Text {
						return MT_Text(t.Random.String())
					})
					subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
						c := subject.Super(t)
						t.Defer(jsonkit.CodecRegisterTypeID[MT_Text](c, "mt-text"))
						return c
					})
					value.Let(s, func(t *testcase.T) any {
						return struct{ V any }{V: inner.Get(t)}
					})
					valueJSON.Let(s, func(t *testcase.T) string {
						return `{"V":` + makeValueEnvelopeJSON(t, "mt-text", inner.Get(t)) + `}`
					})
					readBack.Let(s, func(t *testcase.T) any {
						return map[string]any{"V": inner.Get(t)}
					})

					s.Then("the value of the registered type keeps its $type", thenItIsWrittenAsExpected)

					s.Then("the value of the registered type is read back as the very same value", thenItIsReadBackLikeEncodingJSON)
				})
			})
		}

		// v is given to Marshal as it is, so its type is the one the caller knows, and reads the JSON back into.
		// So like with encoding/json, v is written without a $type, even if its type has one,
		// while the values within v, which are held by an interface, are written with their $type.

		s.Then("v is written like with encoding/json", thenItIsWrittenAsExpected)

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)

		s.When("v is nil", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return nil
			})

			s.Then("it is written as null, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back as nil", thenItIsReadBackAsTheSameValue)
		})

		s.When("v is a number of another built-in type, like an int64", func(s *testcase.Spec) {
			value.Let(s, makeOtherBuiltinNumber)
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
		})

		s.When("v is an int", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return makeSignedInt(t)
			})

			s.Then("it is written as a plain JSON number, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back as the very same int, even into an `any`", thenItIsReadBackAsTheSameValue)
		})

		// The caller reads v back into the type it knows, so a float64 needs no .0 fraction to be read back as a float64.
		s.When("v is a float64", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return makeFloat64(t)
			})
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)

			s.And("it is a whole number", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeWholeFloat64(t)
				})

				s.Then("it is written without a fraction, like with encoding/json, as the caller reads it back into a float64", thenItIsWrittenAsExpected)

				s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.And("it is a float64 field of v", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return struct{ F, W, E float64 }{F: makeFloat64WithFraction(t), W: makeWholeFloat64(t), E: makeFloat64WithExponent(t)}
				})

				s.Then("the fields are written like with encoding/json, a whole number without a fraction, as the type of the field tells its type", thenItIsWrittenAsExpected)

				s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.And("it is an element of a []float64 in v", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return []float64{makeFloat64WithFraction(t), makeWholeFloat64(t), makeFloat64WithExponent(t)}
				})

				s.Then("the elements are written like with encoding/json, a whole number without a fraction, as the type of the slice tells their type", thenItIsWrittenAsExpected)

				s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
			})
		})

		s.When("v is of a named type, which is registered with the Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_Text](c, "mt-text"))
				return c
			})
			value.Let(s, func(t *testcase.T) any {
				return MT_Text(t.Random.String())
			})
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written without its $type, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
		})

		s.When("v is of a type the Codec has a registration of its own for", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[string](c, jsonkit.TypeID(t.Random.HexN(8))))
				return c
			})
			value.Let(s, func(t *testcase.T) any {
				return t.Random.String()
			})
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written without the $type of the registration, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
		})

		s.When("v is a struct of a type which is registered with the Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_PlainNode](c, "mt-plain-node"))
				return c
			})
			value.Let(s, func(t *testcase.T) any {
				return MT_PlainNode{V: t.Random.Int(), Next: &MT_PlainNode{V: t.Random.Int()}}
			})
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written without its $type, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)

			s.And("v is a pointer to it", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					node := value.Super(t).(MT_PlainNode)
					return &node
				})

				s.Then("it is written without its $type, like with encoding/json", thenItIsWrittenAsExpected)

				s.Then("it is read back into its type as the very same value", thenItIsReadBackAsTheSameValue)
			})
		})

		s.When("v is of a type which is registered with a custom codec", func(s *testcase.Spec) {
			custom := let.Var(s, func(t *testcase.T) MT_Custom {
				return MT_Custom{N: t.Random.Int()}
			})
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegister[MT_Custom](c, "mt-custom", mtCustomCodec))
				return c
			})
			value.Let(s, func(t *testcase.T) any {
				return custom.Get(t)
			})
			valueJSON.Let(s, func(t *testcase.T) string {
				return mtCustomJSON(custom.Get(t))
			})
			ptr.Let(s, ptrToTheTypeOfV)

			s.Then("it is written by the custom codec, without its $type", thenItIsWrittenAsExpected)

			s.Then("it is read back by the custom codec as the very same value", thenItIsReadBackAsTheSameValue)

			s.And("v is a pointer to it", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					c := custom.Get(t)
					return &c
				})

				s.Then("it is written by the custom codec, without its $type", thenItIsWrittenAsExpected)

				s.Then("it is read back by the custom codec as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.And("it is a field of v", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return struct{ C MT_Custom }{C: custom.Get(t)}
				})
				valueJSON.Let(s, func(t *testcase.T) string {
					return `{"C":` + mtCustomJSON(custom.Get(t)) + `}`
				})

				s.Then("the field is written by the custom codec, without its $type, as the type of the field tells its type", thenItIsWrittenAsExpected)

				s.Then("it is read back by the custom codec as the very same value", thenItIsReadBackAsTheSameValue)
			})

			s.And("it is an element of v", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return []MT_Custom{custom.Get(t)}
				})
				valueJSON.Let(s, func(t *testcase.T) string {
					return `[` + mtCustomJSON(custom.Get(t)) + `]`
				})

				s.Then("the element is written by the custom codec, without its $type, as the type of the slice tells its type", thenItIsWrittenAsExpected)

				s.Then("it is read back by the custom codec as the very same value", thenItIsReadBackAsTheSameValue)
			})
		})

		// Like Unmarshal, which reads JSON by its $type into the interface which the pointer given to it points to,
		// Marshal writes the value, which is held by the interface v points to, with its $type.
		s.When("v is a pointer to the `any`", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return &x }
			})
			holdBack.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return x }
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is a struct field", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return struct{ V any }{V: x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `{"V":` + valueJSON.Get(t) + `}`
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V any })
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is an element of a slice", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return []any{x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `[` + valueJSON.Get(t) + `]`
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new([]any)
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is a value of a map", func(s *testcase.Spec) {
			key := let.Var(s, func(t *testcase.T) string {
				return t.Random.HexN(8)
			})
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return map[string]any{key.Get(t): x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `{` + mustMarshalJSON(t, key.Get(t)) + `:` + valueJSON.Get(t) + `}`
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[string]any)
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is held by a jsonkit.Interface", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return jsonkit.Interface[any]{V: x} }
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(jsonkit.Interface[any])
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is an element of a jsonkit.Array", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return jsonkit.Array[any]{x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `[` + valueJSON.Get(t) + `]`
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(jsonkit.Array[any])
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is an element of a slice type, which is registered with the Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_List](c, "mt-list"))
				return c
			})
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return MT_List{x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `[` + valueJSON.Get(t) + `]`
			})
			ptr.Let(s, ptrToTheTypeOfV)

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		s.When("the `any` is a value of a map type, which is registered with the Codec", func(s *testcase.Spec) {
			key := let.Var(s, func(t *testcase.T) string {
				return t.Random.HexN(8)
			})
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_Bag](c, "mt-bag"))
				return c
			})
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return MT_Bag{key.Get(t): x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `{` + mustMarshalJSON(t, key.Get(t)) + `:` + valueJSON.Get(t) + `}`
			})
			ptr.Let(s, ptrToTheTypeOfV)

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		// encoding/json reads plain JSON the same way into every empty interface, not only into an `any`.
		s.When("the `any` is a struct field of a named empty interface type", func(s *testcase.Spec) {
			hold.Let(s, func(t *testcase.T) func(any) any {
				return func(x any) any { return struct{ V MT_Any }{V: x} }
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return `{"V":` + valueJSON.Get(t) + `}`
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ V MT_Any })
			})

			itWritesTheTypesOfPlainJSONWithoutTypeID(s)
		})

		// Unlike an empty interface, a non-empty interface can only hold a value of a type which implements it,
		// so its JSON can't be read back without the $type, which tells that type.
		s.When("a value of a type without a type ID is held by a non-empty interface", func(s *testcase.Spec) {
			v.Let(s, func(t *testcase.T) any {
				return struct{ V MT_Namer }{V: MT_NoTypeID{Name: t.Random.String()}}
			})

			s.Then("it fails, as the value couldn't be read back into the interface", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)
			})

			s.And("the non-empty interface is a jsonkit.Interface given to Marshal", func(s *testcase.Spec) {
				v.Let(s, func(t *testcase.T) any {
					return jsonkit.Interface[MT_Namer]{V: MT_NoTypeID{Name: t.Random.String()}}
				})

				s.Then("it fails, as the value couldn't be read back into the interface", func(t *testcase.T) {
					_, err := act(t)
					assert.Error(t, err)
				})
			})
		})

		s.When("v references back to itself", func(s *testcase.Spec) {
			v.Let(s, func(t *testcase.T) any {
				n := &MT_PlainNode{V: t.Random.Int()}
				n.Next = n
				return n
			})

			thenItFailsLikeEncodingJSON := func(t *testcase.T) {
				_, err := act(t)

				var unsupported *json.UnsupportedValueError
				assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
			}

			s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)

			s.And("its type is registered with the Codec, and v is a pointer to an `any` holding it, so it is written with its $type", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[MT_PlainNode](c, "mt-plain-node"))
					return c
				})
				v.Let(s, func(t *testcase.T) any {
					var x any = v.Super(t)
					return &x
				})

				s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)
			})

			s.And("the reference cycle goes through a map", func(s *testcase.Spec) {
				v.Let(s, func(t *testcase.T) any {
					m := map[string]any{t.Random.HexN(8): t.Random.String()}
					m[t.Random.HexN(8)+"-self"] = m
					return m
				})

				s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)
			})

			s.And("the reference cycle goes through a slice", func(s *testcase.Spec) {
				v.Let(s, func(t *testcase.T) any {
					l := []any{t.Random.String(), nil}
					l[1] = l
					return l
				})

				s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)
			})

			s.And("the reference cycle goes through an interface", func(s *testcase.Spec) {
				v.Let(s, func(t *testcase.T) any {
					type node struct{ V any }
					n := &node{}
					n.V = n
					return n
				})

				s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)
			})

			s.And("the reference cycle goes through a custom codec, which writes the value it references with the Codec", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegister[MT_CyclicB](c, "mt-cyclic-b", mtCyclicBCodec))
					return c
				})
				v.Let(s, func(t *testcase.T) any {
					a := &MT_CyclicA{}
					a.B = &MT_CyclicB{A: a}
					return a
				})

				s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)

				s.And("the value it references is of a type registered with the Codec", func(s *testcase.Spec) {
					subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
						c := subject.Super(t)
						t.Defer(jsonkit.CodecRegisterTypeID[MT_CyclicA](c, "mt-cyclic-a"))
						return c
					})
					v.Let(s, func(t *testcase.T) any {
						return *v.Super(t).(*MT_CyclicA)
					})

					s.Then("it fails the way encoding/json fails, instead of recursing forever", thenItFailsLikeEncodingJSON)
				})
			})
		})

		s.When("v references the same values more than once, without a reference cycle", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				node := &MT_PlainNode{V: t.Random.Int()}
				m := map[string]any{t.Random.HexN(8): t.Random.String()}
				l := []any{t.Random.String()}
				return MT_SharedReferences{
					Node1: node, Node2: node,
					Map1: m, Map2: map[string]any{"a": m, "b": m},
					List1: l, List2: []any{l, l},
				}
			})
			expJSON.Let(s, func(t *testcase.T) string {
				return mustMarshalJSON(t, v.Get(t))
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(MT_SharedReferences)
			})

			s.Then("a value is written at each place it is referenced, like with encoding/json", thenItIsWrittenAsExpected)

			s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)

			s.And("the value referenced more than once is written by a custom codec", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegister[MT_CyclicB](c, "mt-cyclic-b", mtCyclicBCodec))
					return c
				})
				v.Let(s, func(t *testcase.T) any {
					b := &MT_CyclicB{A: &MT_CyclicA{}}
					return struct{ B1, B2 *MT_CyclicB }{B1: b, B2: b}
				})
				expJSON.Let(s, func(t *testcase.T) string {
					const b = `{"a":{"B":null}}`
					return `{"B1":` + b + `,"B2":` + b + `}`
				})

				s.Then("a value is written at each place it is referenced, like with encoding/json", thenItIsWrittenAsExpected)
			})
		})
	})
}

// MT_List and MT_Bag are a slice and a map type, which hold their values as `any`.
type (
	MT_List []any
	MT_Bag  map[string]any
)

// MT_Any is a named empty interface type, which encoding/json treats like an `any`.
type MT_Any interface{}

// MT_NoTypeID is a struct which is registered with no Codec, so it has no type ID.
type MT_NoTypeID struct {
	Name  string
	Count int
}

func (v MT_NoTypeID) GetName() string { return v.Name }

// MT_Namer is a non-empty interface, which MT_NoTypeID implements.
type MT_Namer interface{ GetName() string }

// MT_SharedReferences references the same values more than once, without a reference cycle.
type MT_SharedReferences struct {
	Node1, Node2 *MT_PlainNode
	Map1, Map2   map[string]any
	List1, List2 []any
}

// MT_CyclicA and MT_CyclicB reference each other, so they make a reference cycle,
// which goes through the custom codec of MT_CyclicB, see mtCyclicBCodec.
type (
	MT_CyclicA struct{ B *MT_CyclicB }
	MT_CyclicB struct{ A *MT_CyclicA }
)

// mtCyclicBCodec writes the MT_CyclicA, which a MT_CyclicB references, with the Codec it receives.
// It writes the value the pointer points to, so the pointer itself is not written by the Codec.
var mtCyclicBCodec = jsonkit.TypeCodec[MT_CyclicB]{
	MarshalFunc: func(c *jsonkit.Codec, b MT_CyclicB) ([]byte, error) {
		if b.A == nil {
			return []byte(`{}`), nil
		}
		a, err := c.Marshal(*b.A)
		if err != nil {
			return nil, err
		}
		return fmt.Appendf(nil, `{"a":%s}`, a), nil
	},
	UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_CyclicB) error {
		return fmt.Errorf("MT_CyclicB is not read in the tests")
	},
}

// MT_Text is a named type of a built-in kind, for the cases where it is registered with a Codec.
type MT_Text string

// MT_Custom is written by a custom codec, see mtCustomCodec, in a shape of its own, like a DTO, which tells it apart from its fields.
type MT_Custom struct{ N int }

var mtCustomCodec = jsonkit.TypeCodec[MT_Custom]{
	MarshalFunc: func(c *jsonkit.Codec, v MT_Custom) ([]byte, error) {
		return []byte(mtCustomJSON(v)), nil
	},
	UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_Custom) error {
		var dto struct {
			Num string `json:"num"`
		}
		if err := json.Unmarshal(data, &dto); err != nil {
			return err
		}
		n, err := strconv.Atoi(dto.Num)
		if err != nil {
			return err
		}
		*p = MT_Custom{N: n}
		return nil
	},
}

// mtCustomJSON is the JSON mtCustomCodec writes for v.
func mtCustomJSON(v MT_Custom) string {
	return `{"num":"` + strconv.Itoa(v.N) + `"}`
}

// makeJSONTypesContainer makes a random []any or map[string]any of the values encoding/json reads plain JSON into,
// when the destination is an `any`: a nil, bool, float64 or string.
func makeJSONTypesContainer(t *testcase.T) any {
	return random.Pick(t.Random, makeJSONTypesSlice, makeJSONTypesMap)(t)
}

// makeJSONTypesSlice makes a random []any of the values encoding/json reads plain JSON into, see makeJSONTypesContainer.
func makeJSONTypesSlice(t *testcase.T) any {
	// start from a non-nil slice, as encoding/json reads an empty JSON array as an empty slice.
	return append([]any{}, random.Slice(t.Random.IntBetween(0, 3), func() any { return makeJSONScalarValue(t) })...)
}

// makeJSONTypesMap makes a random map[string]any of the values encoding/json reads plain JSON into, see makeJSONTypesContainer.
func makeJSONTypesMap(t *testcase.T) any {
	return random.Map(t.Random.IntBetween(0, 3), func() (string, any) {
		return t.Random.HexN(8), makeJSONScalarValue(t)
	})
}

// makeJSONScalarValue makes a random nil, bool, int, float64 or string,
// which is what the Codec reads a JSON value, which is not an array or an object, into, when the destination is an `any`.
func makeJSONScalarValue(t *testcase.T) any {
	return random.Pick(t.Random,
		func(*testcase.T) any { return nil },
		makeJSONTypeScalar,
	)(t)
}

// makeJSONTypeScalar makes a random bool, int, float64 or string.
// The float64 is not a whole number, so encoding/json writes it the very same way as the Codec does, see makeFloat64WithFraction.
func makeJSONTypeScalar(t *testcase.T) any {
	return random.Pick[any](t.Random,
		t.Random.Bool(),
		makeSignedInt(t),
		makeFloat64WithFraction(t),
		t.Random.String(),
	)
}

// makeSignedInt makes a random int, which is negative or not.
func makeSignedInt(t *testcase.T) int {
	return random.Pick(t.Random, 1, -1) * t.Random.Int()
}

// makeFloat64 makes a random float64 of any kind, either way of its sign:
// one with a fraction, a whole number, or one encoding/json writes with an exponent.
func makeFloat64(t *testcase.T) float64 {
	return random.Pick(t.Random, makeFloat64WithFraction, makeWholeFloat64, makeFloat64WithExponent)(t)
}

// makeFloat64WithFraction makes a random float64 which is not a whole number, either way of its sign, below 1 or up to 1e15.
// encoding/json writes it the very same way as the Codec does, while a whole one isn't, see makeWholeFloat64.
func makeFloat64WithFraction(t *testcase.T) float64 {
	sign := float64(random.Pick(t.Random, 1, -1))
	for {
		// The bigger a float64 is, the fewer fractions it can hold, so a scaled one is often a whole number.
		if f := t.Random.Float64() * math.Pow10(t.Random.IntBetween(0, 15)); f != math.Trunc(f) {
			return sign * f
		}
	}
}

// makeFloat64WithExponent makes a random float64 which encoding/json writes with an exponent, either way of its sign:
// 1e21 or more, which is always a whole number, or less than 1e-6, apart from zero.
func makeFloat64WithExponent(t *testcase.T) float64 {
	sign := float64(random.Pick(t.Random, 1, -1))
	exp := random.Pick(t.Random, t.Random.IntBetween(21, 307), -t.Random.IntBetween(7, 307))
	return sign * random.Pick(t.Random,
		// a single digit, which encoding/json writes without a dot, like 1e+21 or 5e-7
		func() float64 {
			f, err := strconv.ParseFloat(strconv.Itoa(t.Random.IntBetween(1, 9))+"e"+strconv.Itoa(exp), 64)
			assert.Must(t).NoError(err)
			return f
		},
		// more digits, which encoding/json writes with a dot, like 1.5e+21
		func() float64 { return (1 + t.Random.Float64()) * math.Pow10(exp) },
		// the edges of the range of a float64
		func() float64 { return random.Pick(t.Random, math.MaxFloat64, math.SmallestNonzeroFloat64) },
	)()
}

// makeWholeFloat64 makes a random float64 of a whole number,
// which encoding/json writes without a fraction or an exponent, like 42, while the Codec writes it held by an `any` as 42.0.
func makeWholeFloat64(t *testcase.T) float64 {
	return random.Pick(t.Random,
		func() float64 { return float64(t.Random.IntBetween(-1000, 1000)) },
		func() float64 { return float64(makeSignedInt(t)) },
		// encoding/json writes a float64 without an exponent up to 1e21.
		func() float64 { return float64(random.Pick(t.Random, 1, -1)*t.Random.IntBetween(1, 999)) * 1e18 },
	)()
}

// makeOtherBuiltinNumber makes a random number of a built-in type other than int and float64,
// which plain JSON can't tell apart from an int or a float64.
func makeOtherBuiltinNumber(t *testcase.T) any {
	return random.Pick[any](t.Random,
		int64(t.Random.Int()),
		int8(t.Random.IntBetween(-128, 127)),
		uint(t.Random.Int()),
		uint8(t.Random.IntN(256)),
		t.Random.Float32(),
	)
}

// makeValueEnvelopeJSON makes the {"$type":"...","$value":...} JSON of v, which is not a JSON object.
func makeValueEnvelopeJSON(t *testcase.T, typeID jsonkit.TypeID, v any) string {
	return mustMarshalJSON(t, map[string]any{"$type": typeID, "$value": v})
}

// makeObjectEnvelopeJSON makes the JSON of v, which is a JSON object, with a $type member next to its own members.
// v can be a json.RawMessage too, like the JSON a custom codec writes.
func makeObjectEnvelopeJSON(t *testcase.T, typeID jsonkit.TypeID, v any) string {
	t.Helper()
	members := jsonObjectMembers(t, []byte(mustMarshalJSON(t, v)))
	members["$type"] = json.RawMessage(mustMarshalJSON(t, typeID))
	return mustMarshalJSON(t, members)
}

// makeUntypedJSONValue makes a random JSON value without a $type discriminator:
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

// unmarshalPlainJSON reads data, JSON without a $type, into the value ptr points to,
// the way the Codec is expected to read it: the way encoding/json reads it,
// except that a number held by an empty interface is an int when it is only digits and fits an int, and a float64 otherwise.
// The numbers are read with json.Decoder.UseNumber, and parsed from their text,
// so an int which a float64 can't hold is read exactly.
func unmarshalPlainJSON(t *testcase.T, data []byte, ptr any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	assert.Must(t).NoError(dec.Decode(ptr))
	plainJSONNumbers(t, reflect.ValueOf(ptr).Elem())
}

// plainJSONNumbers replaces each json.Number held by an interface in v with the int or float64 the Codec reads it as.
func plainJSONNumbers(t *testcase.T, v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		if n, ok := v.Interface().(json.Number); ok {
			if i, err := strconv.ParseInt(n.String(), 10, strconv.IntSize); err == nil {
				v.Set(reflect.ValueOf(int(i)))
				return
			}
			f, err := n.Float64()
			assert.Must(t).NoError(err)
			v.Set(reflect.ValueOf(f))
			return
		}
		// The value an interface holds can't be changed in place, so a copy of it is.
		elem := reflect.New(v.Elem().Type()).Elem()
		elem.Set(v.Elem())
		plainJSONNumbers(t, elem)
		v.Set(elem)
	case reflect.Pointer:
		if !v.IsNil() {
			plainJSONNumbers(t, v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				plainJSONNumbers(t, v.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			plainJSONNumbers(t, v.Index(i))
		}
	case reflect.Map:
		for iter := v.MapRange(); iter.Next(); {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(iter.Value())
			plainJSONNumbers(t, elem)
			v.SetMapIndex(iter.Key(), elem)
		}
	}
}

// assertEncodesLikeEncodingJSON asserts that the Codec writes v as the same JSON as encoding/json does,
// and that it reads that JSON back the same way as encoding/json does.
func assertEncodesLikeEncodingJSON[T any](t *testcase.T, c *jsonkit.Codec, v T) {
	t.Helper()
	exp, err := json.Marshal(v)
	assert.Must(t).NoError(err)

	got, err := c.Marshal(v)
	assert.Must(t).NoError(err)
	assertJSONEqual(t, string(exp), string(got))

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

// The helpers below are for a struct with a Greeter field, which holds an interface value next to the fields under test.
// The Codec writes that value with its $type, and encoding/json doesn't, so encoding/json can't read it back.
// So the Greeter member is compared to greeterJSON, and the other members to what encoding/json does.

// assertEncodesLikeEncodingJSONBesideGreeter asserts that the Codec writes the struct v as the same JSON as encoding/json does,
// apart from the Greeter member, which is the greeterJSON of the value the Greeter field holds,
// and that it reads that JSON back the same way as encoding/json reads the other members.
// v may be a pointer to the struct too.
func assertEncodesLikeEncodingJSONBesideGreeter[T any](t *testcase.T, c *jsonkit.Codec, v T) {
	t.Helper()
	greeter, _ := greeterField(&v).Interface().(Greeter)
	plain, err := json.Marshal(v)
	assert.Must(t).NoError(err)

	got, err := c.Marshal(v)
	assert.Must(t).NoError(err)
	assertJSONEqual(t, string(withGreeterMember(t, plain, greeter)), string(got))

	members := jsonObjectMembers(t, plain)
	delete(members, "Greeter")
	assertDecodesLikeEncodingJSONBesideGreeter[T](t, c, []byte(mustMarshalJSON(t, members)), greeter)
}

// assertDecodesLikeEncodingJSONBesideGreeter asserts that the Codec reads data, with a Greeter member for greeter,
// into a T the same way as encoding/json reads data, and that the Greeter field gets greeter.
func assertDecodesLikeEncodingJSONBesideGreeter[T any](t *testcase.T, c *jsonkit.Codec, data []byte, greeter Greeter) {
	t.Helper()
	assertDecodesIntoLikeEncodingJSONBesideGreeter(t, c, data, greeter, func() T {
		var zero T
		return zero
	})
}

// assertDecodesIntoLikeEncodingJSONBesideGreeter asserts that the Codec reads data, with a Greeter member for greeter,
// into the struct made by makeDestination the same way as encoding/json reads data, and that the Greeter field gets greeter.
// makeDestination is called for both of them, so each of them decodes into a value of its own.
func assertDecodesIntoLikeEncodingJSONBesideGreeter[T any](t *testcase.T, c *jsonkit.Codec, data []byte, greeter Greeter, makeDestination func() T) {
	t.Helper()
	exp, got := makeDestination(), makeDestination()
	assert.Must(t).NoError(json.Unmarshal(data, &exp))
	greeterField(&exp).Set(reflect.ValueOf(&greeter).Elem())

	assert.Must(t).NoError(c.Unmarshal(withGreeterMember(t, data, greeter), &got))
	assert.Equal(t, exp, got)
}

// greeterJSON is the JSON the Codec writes for g, where an interface holds it.
func greeterJSON(t *testcase.T, g Greeter) string {
	t.Helper()
	switch g := g.(type) {
	case TypeA:
		return `{"$type":"type_a","V":` + mustMarshalJSON(t, g.V) + `}`
	case TypeB:
		return `{"$type":"type_b","V":` + mustMarshalJSON(t, g.V) + `}`
	default:
		t.Fatalf("the JSON of %T is unknown", g)
		return ""
	}
}

// withGreeterMember returns the JSON object in data with a Greeter member, which is the greeterJSON of greeter.
func withGreeterMember(t *testcase.T, data []byte, greeter Greeter) []byte {
	t.Helper()
	members := jsonObjectMembers(t, data)
	members["Greeter"] = json.RawMessage(greeterJSON(t, greeter))
	return []byte(mustMarshalJSON(t, members))
}

// greeterField returns the Greeter field of the struct ptr points to, through as many pointers as there are.
func greeterField(ptr any) reflect.Value {
	v := reflect.ValueOf(ptr)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v.FieldByName("Greeter")
}

// assertJSONEqual asserts that got is the same JSON as exp, apart from the order of the members of its objects,
// as the members of a JSON object are unordered.
// Anything else is compared as it is written, like a 1.0 differs from a 1, or a "\u003c" from a "<".
func assertJSONEqual(tb testing.TB, exp, got string, msg ...assert.Message) {
	tb.Helper()
	assert.Equal(tb, sortJSONMembers(tb, exp), sortJSONMembers(tb, got), msg...)
}

// sortJSONMembers writes the JSON in data with the members of its objects sorted by their names,
// while every other token stays as it is written.
func sortJSONMembers(tb testing.TB, data string) string {
	tb.Helper()
	opts := []jsontext.Options{jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)}
	var sortValue func(v jsontext.Value) string
	sortValue = func(v jsontext.Value) string {
		kind := v.Kind()
		if kind != '{' && kind != '[' {
			return string(v)
		}
		dec := jsontext.NewDecoder(bytes.NewReader(v), opts...)
		_, err := dec.ReadToken()
		assert.Must(tb).NoError(err)
		var parts []string
		for dec.PeekKind() != '}' && dec.PeekKind() != ']' {
			var name string
			if kind == '{' {
				raw, err := dec.ReadValue()
				assert.Must(tb).NoError(err)
				name = string(raw) + ":"
			}
			elem, err := dec.ReadValue()
			assert.Must(tb).NoError(err)
			parts = append(parts, name+sortValue(elem))
		}
		if kind == '[' {
			return "[" + strings.Join(parts, ",") + "]"
		}
		slices.Sort(parts) // a member's name is a JSON string, which ends at the first unescaped quote, so sorting the members sorts their names
		return "{" + strings.Join(parts, ",") + "}"
	}
	dec := jsontext.NewDecoder(strings.NewReader(data), opts...)
	v, err := dec.ReadValue()
	assert.Must(tb).NoError(err, assert.Message("invalid JSON: "+data))
	return sortValue(v.Clone())
}

// jsonObjectMembers returns the members of the JSON object in data, by their names.
func jsonObjectMembers(t *testcase.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	assert.Must(t).NoError(json.Unmarshal(data, &object))
	return object
}

// MT_Merge has a field of each kind, which encoding/json decodes into the value it already holds in its own way,
// and interface fields, which hold values with a $type.
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

// MT_MergeLeaf is a struct with an interface field, which holds a value with a $type.
type MT_MergeLeaf struct {
	Greeter Greeter
	A       string
	B       int
}

// MT_PlainLeaf is a struct without interface fields.
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

	s.Test("a struct with an interface field", func(t *testcase.T) {
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

	s.Test("a struct with a slice of interfaces", func(t *testcase.T) {
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
		name, greeter := t.Random.String(), makeGreeter(t)
		var exp MT_UnexportedEmbeddedPointerAndName
		assert.Must(t).Error(json.Unmarshal([]byte(`{"Hidden":true,"Name":`+mustMarshalJSON(t, name)+`}`), &exp))
		exp.Greeter = greeter

		var got MT_UnexportedEmbeddedPointerAndName
		data := `{"Hidden":true,"Name":` + mustMarshalJSON(t, name) + `,"Greeter":` + greeterJSON(t, greeter) + `}`
		assert.Error(t, c.Get(t).Unmarshal([]byte(data), &got))
		assert.Equal(t, exp, got)
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
		foo := testent.MakeFoo(t)
		exp := MT_EmbeddedInterface{Name: t.Random.String(), Fooer: foo}

		assertWireFormat(t, c.Get(t), exp, `{"Name":`+mustMarshalJSON(t, exp.Name)+`,"Fooer":`+makeObjectEnvelopeJSON(t, "foo", foo)+`}`)
	})

	// Where the embedded interface sits among the fields makes no difference.
	s.Test("an embedded interface as the first field is a field named after its type, like with encoding/json", func(t *testcase.T) {
		foo := testent.MakeFoo(t)
		exp := MT_EmbeddedInterfaceFirst{Fooer: foo, Name: t.Random.String()}

		assertWireFormat(t, c.Get(t), exp, `{"Fooer":`+makeObjectEnvelopeJSON(t, "foo", foo)+`,"Name":`+mustMarshalJSON(t, exp.Name)+`}`)
	})

	s.Test("an interface field is written under the name in its json tag, and one tagged with - is left out, like with encoding/json", func(t *testcase.T) {
		foo := testent.MakeFoo(t)

		data, err := c.Get(t).Marshal(MT_T4{Fooer: foo, Ignored: testent.MakeFoo(t)})
		assert.NoError(t, err)
		assertJSONEqual(t, `{"fooer":`+makeObjectEnvelopeJSON(t, "foo", foo)+`}`, string(data))

		var got MT_T4
		assert.NoError(t, c.Get(t).Unmarshal(data, &got))
		assert.Equal(t, MT_T4{Fooer: foo}, got)
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

	s.Test("a slice of `any` in an `any` keeps the types of its elements", func(t *testcase.T) {
		exp := struct{ V any }{V: []any{
			testent.MakeFoo(t),
			TypeA{V: t.Random.String()},
			map[string]any{t.Random.HexN(4): TypeB{V: t.Random.Int()}},
			[]any{TypeA{V: t.Random.String()}, []any{TypeB{V: t.Random.Int()}}},
			t.Random.String(),
			float64(t.Random.Int()),
			t.Random.Bool(),
			nil,
		}}
		data, err := c.Get(t).Marshal(exp)
		assert.NoError(t, err)

		var got struct{ V any }
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
		assertJSONEqual(t, `{}`, string(data))
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
		assertJSONEqual(t, `{}`, string(data))

		expKeep := MT_T12{List: emptyList}
		data, err = c.Get(t).Marshal(expKeep)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"list":[]}`, string(data))

		// And both drop nil collections.
		expNilEmpty := MT_T11{}
		data, err = c.Get(t).Marshal(expNilEmpty)
		assert.NoError(t, err)
		assertJSONEqual(t, `{}`, string(data))

		expNilZero := MT_T12{}
		data, err = c.Get(t).Marshal(expNilZero)
		assert.NoError(t, err)
		assertJSONEqual(t, `{}`, string(data))
	})

	s.Test("omitzero omits zero values across supported kinds", func(t *testcase.T) {
		data, err := c.Get(t).Marshal(MT_T12{})
		assert.NoError(t, err)
		assertJSONEqual(t, `{}`, string(data))

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
		assertJSONEqual(t, `{"count":"42","enabled":"true","ratio":"0.5"}`, string(data))
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
		assertEncodesLikeEncodingJSONBesideGreeter(t, c.Get(t), MT_StringifiedEmbedded{Greeter: makeGreeter(t), MT_Stringified: makeMTStringified(t)})
		stringified := makeMTStringified(t)
		assertEncodesLikeEncodingJSONBesideGreeter(t, c.Get(t), MT_StringifiedThroughPointer{Greeter: makeGreeter(t), MT_Stringified: &stringified})
	})

	s.Test("a quoted null makes a pointer with the ,string option nil, like with encoding/json", func(t *testcase.T) {
		data := []byte(`{"P":"null","F":null,"S":"null"}`)
		allocated := func() MT_Stringified {
			p, f, s := t.Random.Int(), t.Random.Float64(), t.Random.String()
			return MT_Stringified{P: &p, F: &f, S: &s}
		}

		assertDecodesIntoLikeEncodingJSON(t, c.Get(t), data, allocated)
		assertDecodesIntoLikeEncodingJSONBesideGreeter(t, c.Get(t), data, makeGreeter(t), func() MT_StringifiedEmbedded {
			return MT_StringifiedEmbedded{MT_Stringified: allocated()}
		})
		assertDecodesIntoLikeEncodingJSONBesideGreeter(t, c.Get(t), data, makeGreeter(t), func() MT_StringifiedThroughPointer {
			stringified := allocated()
			return MT_StringifiedThroughPointer{MT_Stringified: &stringified}
		})
	})

	s.Test("a byte slice is a base64 encoded string, like with encoding/json", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, c.Get(t), []byte(t.Random.String()))
		assertEncodesLikeEncodingJSONBesideGreeter(t, c.Get(t), MT_Bytes{
			Greeter: makeGreeter(t),
			Data:    []byte(t.Random.String()),
			List:    [][]byte{[]byte(t.Random.String())},
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

	s.Test("a type registered with RegisterTypeID is known by every Codec", func(t *testcase.T) {
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

	s.Test("interface values nested in each other, through structs, slices and maps", func(t *testcase.T) {
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

	s.Test("a Codec can be used from multiple goroutines at once", func(t *testcase.T) {
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

// MT_UnexportedEmbeddedPointerAndName has fields after the fields of its embedded pointer to an unexported struct type.
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

// MT_TaggedEmbeddedUnmarshalers embeds structs of unexported types, which have their own UnmarshalJSON.
// They are embedded at the same depth, so their UnmarshalJSON methods are not promoted,
// as neither of them would take over the whole struct then.
type MT_TaggedEmbeddedUnmarshalers struct {
	mtEmbeddedUnmarshalerA `json:"a"`
	mtEmbeddedUnmarshalerB `json:"b"`
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

// MT_StringifiedEmbedded has the fields of MT_Stringified promoted, next to an interface field.
type MT_StringifiedEmbedded struct {
	Greeter Greeter
	MT_Stringified
}

// MT_StringifiedThroughPointer has the fields of MT_Stringified promoted through an embedded pointer, next to an interface field.
type MT_StringifiedThroughPointer struct {
	Greeter Greeter
	*MT_Stringified
}

// MT_Bytes has byte slices, next to an interface field.
type MT_Bytes struct {
	Greeter Greeter
	Data    []byte
	List    [][]byte
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

// assertWireFormat asserts that c writes exp as the JSON wire, and reads wire back as exp.
// Unlike a round-trip, which any encoding that reads back what it writes passes,
// it tells apart the encodings which write the same value as different JSON.
func assertWireFormat[T any](t *testcase.T, c *jsonkit.Codec, exp T, wire string) {
	t.Helper()
	data, err := c.Marshal(exp)
	assert.NoError(t, err)
	assertJSONEqual(t, wire, string(data))

	var got T
	assert.NoError(t, c.Unmarshal([]byte(wire), &got))
	assert.Equal(t, exp, got)
}

func TestCodec_jsonMarshaler(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(RegisterTypeIDs)
	var c jsonkit.Codec

	defer jsonkit.RegisterTypeID[MT_JM_PolyFooerReg]("jm_poly")()
	defer jsonkit.RegisterTypeID[MT_JM_PolyReg]("jm_poly_reg")()

	// MT_JM_Inner writes itself as {"wrapped":V}, while its field is tagged as "v",
	// so only its own MarshalJSON and UnmarshalJSON write and read it the way encoding/json does.
	s.Test("struct field implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		exp := MT_JM_Box{Inner: MT_JM_Inner{V: t.Random.String()}}
		assertEncodesLikeEncodingJSON(t, &c, exp)
	})

	s.Test("slice of json.Marshaler elements", func(t *testcase.T) {
		exp := MT_JM_Slice{Items: random.Slice(t.Random.IntBetween(1, 5), func() MT_JM_Inner {
			return MT_JM_Inner{V: t.Random.String()}
		})}
		assertEncodesLikeEncodingJSON(t, &c, exp)
	})

	s.Test("map of json.Marshaler values", func(t *testcase.T) {
		exp := MT_JM_Map{Items: map[string]MT_JM_Inner{
			t.Random.String(): {V: t.Random.String()},
			t.Random.String(): {V: t.Random.String()},
		}}
		assertEncodesLikeEncodingJSON(t, &c, exp)
	})

	s.Test("pointer struct field implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, MT_JM_PointerStruct{Inner: &MT_JM_Inner{V: t.Random.String()}})
	})

	s.Test("top-level value implementing json.Marshaler and json.Unmarshaler", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, MT_JM_Inner{V: t.Random.String()})
	})

	s.Test("top-level value implementing json.Marshaler and json.Unmarshaler, which is not a struct", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, uuid.Must(uuid.MakeV4))
	})

	s.Test("top-level slice of json.Marshaler elements", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, random.Slice(t.Random.IntBetween(1, 5), func() MT_JM_Inner {
			return MT_JM_Inner{V: t.Random.String()}
		}))
	})

	s.Test("top-level array of json.Marshaler elements", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, [2]MT_JM_Inner{{V: t.Random.String()}, {V: t.Random.String()}})
	})

	s.Test("top-level map of json.Marshaler values", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, map[string]MT_JM_Inner{t.Random.HexN(4): {V: t.Random.String()}})
	})

	// withTypeCustomJSON writes itself as the JSON string "encoded:" + Value,
	// so an interface holding it writes it as the $value of its type envelope.
	s.Test("interface field holding a json.Unmarshaler implementation", func(t *testcase.T) {
		v := withTypeCustomJSON{Value: t.Random.String()}
		assertWireFormat(t, &c, struct{ V any }{V: v},
			`{"V":{"$type":"with_type_custom_json","$value":`+mustMarshalJSON(t, "encoded:"+v.Value)+`}}`)
	})

	s.Test("slice of interfaces holding json.Unmarshaler implementations", func(t *testcase.T) {
		v := withTypeCustomJSON{Value: t.Random.String()}
		assertWireFormat(t, &c, []any{v},
			`[{"$type":"with_type_custom_json","$value":`+mustMarshalJSON(t, "encoded:"+v.Value)+`}]`)
	})

	// jsonkit.Array's own MarshalJSON and UnmarshalJSON can only use the package level registry,
	// so the Codec processes a jsonkit.Array of interfaces as a plain slice, to keep using its own registry.
	s.Test("jsonkit.Array of interfaces as a struct field is decoded with the Codec's own registry", func(t *testcase.T) {
		var isolated jsonkit.Codec // the implementations are registered only on this Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Circle](&isolated, "circle"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Square](&isolated, "square"))

		roundTrip(t, &isolated, struct{ Shapes jsonkit.Array[MT_Shape] }{
			Shapes: jsonkit.Array[MT_Shape]{MT_Circle{R: t.Random.Float64()}, MT_Square{S: t.Random.Float64()}},
		})
	})

	s.Test("jsonkit.Array of interfaces behind a pointer is encoded with the Codec's own registry", func(t *testcase.T) {
		var isolated jsonkit.Codec // the implementations are registered only on this Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Circle](&isolated, "circle"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Square](&isolated, "square"))

		shapes := jsonkit.Array[MT_Shape]{MT_Circle{R: t.Random.Float64()}, MT_Square{S: t.Random.Float64()}}
		roundTrip(t, &isolated, struct{ Shapes *jsonkit.Array[MT_Shape] }{Shapes: &shapes})
		roundTrip(t, &isolated, &shapes)

		// Like with encoding/json, a JSON array in an interface is read back as a []any.
		data, err := isolated.Marshal(struct{ Shapes any }{Shapes: &shapes})
		assert.NoError(t, err)
		var got struct{ Shapes any }
		assert.NoError(t, isolated.Unmarshal(data, &got))
		assert.Equal[any](t, []any{shapes[0], shapes[1]}, got.Shapes)
	})

	// The elements of a jsonkit.Array can hold interface values, even when they are not interfaces themselves.
	s.Test("jsonkit.Array of structs with an interface field is encoded with the Codec's own registry", func(t *testcase.T) {
		var isolated jsonkit.Codec // the implementations are registered only on this Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Circle](&isolated, "circle"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Square](&isolated, "square"))

		type Holder struct{ Shape MT_Shape }
		holders := jsonkit.Array[Holder]{
			{Shape: MT_Circle{R: t.Random.Float64()}},
			{Shape: MT_Square{S: t.Random.Float64()}},
		}
		roundTrip(t, &isolated, holders)
		roundTrip(t, &isolated, &holders)
		roundTrip(t, &isolated, struct{ Holders jsonkit.Array[Holder] }{Holders: holders})
		roundTrip(t, &isolated, struct{ Holders *jsonkit.Array[Holder] }{Holders: &holders})
		roundTrip(t, &isolated, []jsonkit.Array[Holder]{holders, nil})
	})

	s.Test("jsonkit.Array of values without interfaces is encoded like encoding/json does", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, &c, jsonkit.Array[string]{t.Random.String(), t.Random.String()})
		assertEncodesLikeEncodingJSON(t, &c, jsonkit.Array[string](nil))
		assertEncodesLikeEncodingJSON(t, &c, jsonkit.Array[MT_JM_Inner]{{V: t.Random.String()}})
		assertEncodesLikeEncodingJSON(t, &c, struct{ Numbers jsonkit.Array[int] }{Numbers: jsonkit.Array[int]{t.Random.Int()}})
	})

	// The Codec writes a registered map in an interface in the $map form, which holds the members its MarshalJSON wrote.
	s.Test("interface holding a registered map with its own json.Unmarshaler reads back the members its MarshalJSON wrote", func(t *testcase.T) {
		var isolated jsonkit.Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_OwnJSONMap](&isolated, "own_json_map"))

		roundTrip(t, &isolated, struct{ V any }{V: MT_OwnJSONMap{t.Random.HexN(4): t.Random.String()}})
	})

	// Like jsonkit.Array, jsonkit.Interface's own MarshalJSON and UnmarshalJSON can only use the package level registry,
	// so the Codec processes a jsonkit.Interface as the interface value it holds.
	s.Test("jsonkit.Interface is encoded with the Codec's own registry", func(t *testcase.T) {
		var isolated jsonkit.Codec // the implementations are registered only on this Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Circle](&isolated, "circle"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Square](&isolated, "square"))

		circle := jsonkit.Interface[MT_Shape]{V: MT_Circle{R: t.Random.Float64()}}
		assertWireFormat(t, &isolated, circle, makeObjectEnvelopeJSON(t, "circle", circle.V))
		roundTrip(t, &isolated, &circle)
		roundTrip(t, &isolated, struct{ Shape jsonkit.Interface[MT_Shape] }{Shape: circle})
		roundTrip(t, &isolated, struct{ Shape *jsonkit.Interface[MT_Shape] }{Shape: &circle})
		roundTrip(t, &isolated, []jsonkit.Interface[MT_Shape]{circle, {V: MT_Square{S: t.Random.Float64()}}, {}})
	})

	s.Test("jsonkit.Interface reads what its own MarshalJSON writes for a nil value", func(t *testcase.T) {
		data, err := json.Marshal(jsonkit.Interface[MT_Shape]{})
		assert.NoError(t, err)

		got := jsonkit.Interface[MT_Shape]{V: MT_Circle{R: t.Random.Float64()}}
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Nil(t, got.V)
	})

	// MT_JM_PolyReg writes itself as {"v":"poly"} with its own MarshalJSON, so the $type is written next to that member.
	s.Test("jsonkit.Interface wrapping a json.Marshaler implementation", func(t *testcase.T) {
		assertWireFormat(t, &c, MT_JM_PolyBox{V: jsonkit.Interface[MT_X_Interface]{V: MT_JM_PolyReg{}}},
			`{"V":{"$type":"jm_poly_reg","v":"poly"}}`)
	})

	s.Test("interface field holding a json.Marshaler implementation includes both $type and the marshaler's wire format", func(t *testcase.T) {
		assertWireFormat(t, &c, MT_PolyMarshalerBox{Inner: MT_JM_PolyReg{}},
			`{"Inner":{"$type":"jm_poly_reg","v":"poly"}}`)
	})

	s.Test("a registered struct held by an `any` is written with its $type, and the values of its interface fields with theirs", func(t *testcase.T) {
		defer jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo")()
		defer jsonkit.CodecRegisterTypeID[MT_TypedContainer](&c, "typed_container")()
		foo := testent.MakeFoo(t)
		var container any = MT_TypedContainer{Fooer: foo}

		data, err := c.Marshal(&container)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"typed_container","Fooer":`+makeObjectEnvelopeJSON(t, "foo", foo)+`}`, string(data))

		var got any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, container, got)
	})
}

// TestCodec_textMarshaler pins that, like encoding/json, the Codec writes a value with its own encoding.TextMarshaler
// as the JSON string of its text, and reads a JSON string into a value with its own encoding.TextUnmarshaler with it.
// A json.Marshaler or json.Unmarshaler wins over them, like with encoding/json, and a custom codec wins over all of them.
func TestCodec_textMarshaler(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		return &jsonkit.Codec{}
	})

	point := let.Var(s, func(t *testcase.T) MT_TM_Point {
		return MT_TM_Point{X: t.Random.Int(), Y: t.Random.Int()}
	})

	addresses := let.Var(s, func(t *testcase.T) MT_TM_StdlibAddresses {
		octets := [4]byte{byte(t.Random.IntN(256)), byte(t.Random.IntN(256)), byte(t.Random.IntN(256)), byte(t.Random.IntN(256))}
		return MT_TM_StdlibAddresses{
			IP:     net.IPv4(octets[0], octets[1], octets[2], octets[3]),
			Addr:   netip.AddrFrom4(octets),
			Prefix: netip.PrefixFrom(netip.AddrFrom4(octets), t.Random.IntBetween(0, 32)),
		}
	})

	s.Test("a struct with its own TextMarshaler is encoded like encoding/json does, wherever it sits", func(t *testcase.T) {
		p := point.Get(t)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), p)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &p)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), struct{ Point MT_TM_Point }{Point: p})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []MT_TM_Point{p})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), map[string]MT_TM_Point{t.Random.HexN(4): p})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), map[MT_TM_Point]string{p: t.Random.String()})
	})

	s.Test("the TextMarshalers of the standard library are encoded like encoding/json does", func(t *testcase.T) {
		assertEncodesLikeEncodingJSON(t, subject.Get(t), addresses.Get(t))
	})

	s.Test("the TextMarshalers of the standard library are encoded like encoding/json does, next to an interface value", func(t *testcase.T) {
		v := MT_TM_Addresses{Addresses: addresses.Get(t), Greeter: makeGreeter(t)}
		assertEncodesLikeEncodingJSONBesideGreeter(t, subject.Get(t), v)
		assertEncodesLikeEncodingJSONBesideGreeter(t, subject.Get(t), &v)
	})

	// The Codec would write the Greeter of MT_TM_Greeting with its $type, and fail to read it back as a struct from a JSON string,
	// so only its own methods are able to write and read it.
	s.Test("a TextMarshaler with an interface field is written and read with its own methods", func(t *testcase.T) {
		greeting := MT_TM_Greeting{Greeter: TypeA{V: t.Random.String()}}
		assertEncodesLikeEncodingJSON(t, subject.Get(t), greeting)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), struct{ Greeting MT_TM_Greeting }{Greeting: greeting})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []MT_TM_Greeting{greeting})
		roundTrip(t, subject.Get(t), greeting)
	})

	s.Test("an interface holding a registered TextMarshaler is written as its text with its $type", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegisterTypeID[MT_TM_Point](subject.Get(t), "point"))
		p := point.Get(t)
		exp := struct{ V any }{V: p}

		data, err := subject.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assertJSONEqual(t, fmt.Sprintf(`{"V":{"$type":"point","$value":"%d,%d"}}`, p.X, p.Y), string(data))

		roundTrip(t, subject.Get(t), exp)
		roundTrip(t, subject.Get(t), []any{p})
	})

	// The field tells the type, so like with encoding/json, the JSON of the `any` can be read into a DTO.
	s.Test("what an interface holding a registered TextMarshaler is written as can be read into a field of its type", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegisterTypeID[MT_TM_Point](subject.Get(t), "point"))
		p := point.Get(t)

		data, err := subject.Get(t).Marshal(struct{ V any }{V: p})
		assert.Must(t).NoError(err)

		var got struct{ V MT_TM_Point }
		assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
		assert.Equal(t, p, got.V)
	})

	s.Test("a json.Marshaler and json.Unmarshaler win over a TextMarshaler and TextUnmarshaler, like with encoding/json", func(t *testcase.T) {
		both := MT_TM_Both{V: t.Random.String()}
		assertEncodesLikeEncodingJSON(t, subject.Get(t), both)
		assertEncodesLikeEncodingJSONBesideGreeter(t, subject.Get(t), struct {
			Both    MT_TM_Both
			Greeter Greeter
		}{Both: both, Greeter: makeGreeter(t)})
	})

	s.Test("a custom codec wins over a TextMarshaler and TextUnmarshaler", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegister[MT_TM_Point](subject.Get(t), "point", jsonkit.TypeCodec[MT_TM_Point]{
			MarshalFunc: func(c *jsonkit.Codec, v MT_TM_Point) ([]byte, error) {
				return json.Marshal([2]int{v.X, v.Y})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_TM_Point) error {
				var xy [2]int
				if err := json.Unmarshal(data, &xy); err != nil {
					return err
				}
				*p = MT_TM_Point{X: xy[0], Y: xy[1]}
				return nil
			},
		}))
		p := point.Get(t)

		assertWireFormat(t, subject.Get(t), struct{ Point MT_TM_Point }{Point: p}, fmt.Sprintf(`{"Point":[%d,%d]}`, p.X, p.Y))
	})

	s.Test("the errors of a TextMarshaler and a TextUnmarshaler are returned", func(t *testcase.T) {
		type Failing struct{ Failing MT_TM_Failing }
		_, err := subject.Get(t).Marshal(Failing{})
		assert.ErrorIs(t, err, errMTTMFailing)
		assert.ErrorIs(t, subject.Get(t).Unmarshal([]byte(`{"Failing":"text"}`), new(Failing)), errMTTMFailing)
	})

	s.Test("the errors of a TextMarshaler and a TextUnmarshaler are returned, next to an interface value", func(t *testcase.T) {
		type Failing struct {
			Failing MT_TM_Failing
			Greeter Greeter
		}
		greeter := makeGreeter(t)
		_, err := subject.Get(t).Marshal(Failing{Greeter: greeter})
		assert.ErrorIs(t, err, errMTTMFailing)
		data := `{"Failing":"text","Greeter":` + greeterJSON(t, greeter) + `}`
		assert.ErrorIs(t, subject.Get(t).Unmarshal([]byte(data), new(Failing)), errMTTMFailing)
	})
}

// MT_TM_StdlibAddresses has fields of the types of the standard library, which have their own encoding.TextMarshaler,
// and which encoding/json writes as JSON strings.
type MT_TM_StdlibAddresses struct {
	IP     net.IP
	Addr   netip.Addr
	Prefix netip.Prefix
}

// MT_TM_Addresses is MT_TM_StdlibAddresses next to an interface field.
type MT_TM_Addresses struct {
	Addresses MT_TM_StdlibAddresses
	Greeter   Greeter
}

// MT_TM_Point has its own encoding.TextMarshaler and encoding.TextUnmarshaler, which write it as "x,y".
type MT_TM_Point struct{ X, Y int }

func (p MT_TM_Point) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%d,%d", p.X, p.Y), nil
}

func (p *MT_TM_Point) UnmarshalText(text []byte) error {
	_, err := fmt.Sscanf(string(text), "%d,%d", &p.X, &p.Y)
	return err
}

// MT_TM_Greeting has its own encoding.TextMarshaler and encoding.TextUnmarshaler,
// which write it as the text of the TypeA its Greeter interface field holds.
type MT_TM_Greeting struct{ Greeter Greeter }

func (g MT_TM_Greeting) MarshalText() ([]byte, error) {
	a, ok := g.Greeter.(TypeA)
	if !ok {
		return nil, fmt.Errorf("unexpected Greeter: %T", g.Greeter)
	}
	return []byte(a.V), nil
}

func (g *MT_TM_Greeting) UnmarshalText(text []byte) error {
	g.Greeter = TypeA{V: string(text)}
	return nil
}

// MT_TM_Both has its own json.Marshaler and json.Unmarshaler, next to its own encoding.TextMarshaler and encoding.TextUnmarshaler,
// which write it differently, so it tells which of them is used.
type MT_TM_Both struct{ V string }

func (b MT_TM_Both) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"json": b.V})
}

func (b *MT_TM_Both) UnmarshalJSON(data []byte) error {
	var dto map[string]string
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	b.V = dto["json"]
	return nil
}

func (b MT_TM_Both) MarshalText() ([]byte, error) {
	return []byte("text:" + b.V), nil
}

func (b *MT_TM_Both) UnmarshalText(text []byte) error {
	b.V = strings.TrimPrefix(string(text), "text:")
	return nil
}

var errMTTMFailing = errors.New("MT_TM_Failing")

// MT_TM_Failing has its own encoding.TextMarshaler and encoding.TextUnmarshaler, which always fail.
type MT_TM_Failing struct{}

func (MT_TM_Failing) MarshalText() ([]byte, error) { return nil, errMTTMFailing }

func (*MT_TM_Failing) UnmarshalText([]byte) error { return errMTTMFailing }

// encoding/json uses a MarshalJSON or MarshalText method with a pointer receiver only when it can take the address of the value,
// like with a field of a struct reached through a pointer, or with an element of a slice.
// Elsewhere, like with the top-level value, a map value, or a value held by an interface, it writes the value as if it had no such method.
func TestCodec_addressableMarshalers(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		return &jsonkit.Codec{}
	})

	prJSON := let.Var(s, func(t *testcase.T) MT_PR_JSON {
		return MT_PR_JSON{V: t.Random.String()}
	})

	prText := let.Var(s, func(t *testcase.T) MT_PR_Text {
		return MT_PR_Text{V: t.Random.String()}
	})

	s.Test("a MarshalJSON with a pointer receiver is used for an addressable value, like encoding/json does", func(t *testcase.T) {
		v := prJSON.Get(t)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &v)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &struct{ F MT_PR_JSON }{F: v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &struct{ Inner struct{ F MT_PR_JSON } }{Inner: struct{ F MT_PR_JSON }{F: v}})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []MT_PR_JSON{v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &[2]MT_PR_JSON{v, v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []struct{ F MT_PR_JSON }{{F: v}})

		// jsonkit.Array is processed as the slice it is defined as, so its elements are addressable too.
		data, err := subject.Get(t).Marshal(jsonkit.Array[MT_PR_JSON]{v})
		assert.NoError(t, err)
		assertJSONEqual(t, mustMarshalJSON(t, []MT_PR_JSON{v}), string(data))
	})

	s.Test("a MarshalText with a pointer receiver is used for an addressable value, like encoding/json does", func(t *testcase.T) {
		v := prText.Get(t)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &v)
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &struct{ F MT_PR_Text }{F: v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &struct{ Inner struct{ F MT_PR_Text } }{Inner: struct{ F MT_PR_Text }{F: v}})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []MT_PR_Text{v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), &[2]MT_PR_Text{v, v})
		assertEncodesLikeEncodingJSON(t, subject.Get(t), []struct{ F MT_PR_Text }{{F: v}})

		data, err := subject.Get(t).Marshal(jsonkit.Array[MT_PR_Text]{v})
		assert.NoError(t, err)
		assertJSONEqual(t, mustMarshalJSON(t, []MT_PR_Text{v}), string(data))
	})

	s.Test("a value which is not addressable is written as if it had no pointer receiver methods, like encoding/json does", func(t *testcase.T) {
		j, x := prJSON.Get(t), prText.Get(t)
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), j)
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), x)
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), struct {
			JSON MT_PR_JSON
			Text MT_PR_Text
		}{JSON: j, Text: x})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), [1]MT_PR_JSON{j})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), map[string]MT_PR_JSON{t.Random.HexN(4): j})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), map[string]MT_PR_Text{t.Random.HexN(4): x})
	})

	s.Test("a value held by an interface is not addressable, so it is written as if it had no pointer receiver methods, next to its $type", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegisterTypeID[MT_PR_JSON](subject.Get(t), "pr-json"))
		v := prJSON.Get(t)

		data, err := subject.Get(t).Marshal(&struct{ V any }{V: v})
		assert.NoError(t, err)
		assertJSONEqual(t, `{"V":{"$type":"pr-json","V":`+mustMarshalJSON(t, v.V)+`}}`, string(data))
	})

	s.Test("a MarshalJSON with a pointer receiver wins over a MarshalText with a value receiver only for an addressable value, like with encoding/json", func(t *testcase.T) {
		v := MT_PR_Mixed{V: t.Random.String()}
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), &struct{ F MT_PR_Mixed }{F: v})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), []MT_PR_Mixed{v})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), struct{ F MT_PR_Mixed }{F: v})
		assertMarshalsLikeEncodingJSON(t, subject.Get(t), map[string]MT_PR_Mixed{t.Random.HexN(4): v})
	})

	s.Test("the fields of a registered struct reached through a pointer are written like encoding/json does, next to its $type", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegisterTypeID[MT_PR_Registered](subject.Get(t), "pr-registered"))
		v := &MT_PR_Registered{JSON: prJSON.Get(t), Text: prText.Get(t)}

		var holder any = v
		data, err := subject.Get(t).Marshal(&holder)
		assert.NoError(t, err)
		assertJSONEqual(t, makeObjectEnvelopeJSON(t, "pr-registered", map[string]any{
			"JSON": &v.JSON,
			"Text": &v.Text,
		}), string(data))

		roundTrip(t, subject.Get(t), v)
	})

	s.Test("the fields of a registered struct with an interface field, reached through a pointer, are written like encoding/json does, next to its $type", func(t *testcase.T) {
		t.Defer(jsonkit.CodecRegisterTypeID[MT_PR_RegisteredGreeting](subject.Get(t), "pr-registered-greeting"))
		v := &MT_PR_RegisteredGreeting{JSON: prJSON.Get(t), Text: prText.Get(t), Greeter: TypeA{V: t.Random.String()}}

		var holder any = v
		data, err := subject.Get(t).Marshal(&holder)
		assert.NoError(t, err)
		assertJSONEqual(t, makeObjectEnvelopeJSON(t, "pr-registered-greeting", map[string]any{
			"JSON":    &v.JSON,
			"Text":    &v.Text,
			"Greeter": json.RawMessage(greeterJSON(t, v.Greeter)),
		}), string(data))

		roundTrip(t, subject.Get(t), v)
	})
}

// assertMarshalsLikeEncodingJSON asserts that the Codec writes v as the same JSON as encoding/json does.
// Unlike assertEncodesLikeEncodingJSON, it doesn't read the JSON back,
// for the values which encoding/json itself can't read back, like a TextUnmarshaler which is written as the JSON object of its fields.
func assertMarshalsLikeEncodingJSON(t *testcase.T, c *jsonkit.Codec, v any) {
	t.Helper()
	exp, err := json.Marshal(v)
	assert.Must(t).NoError(err)

	got, err := c.Marshal(v)
	assert.Must(t).NoError(err)
	assertJSONEqual(t, string(exp), string(got))
}

// MT_PR_JSON has its own json.Marshaler and json.Unmarshaler with pointer receivers, which write it as {"pr-json":V}.
type MT_PR_JSON struct{ V string }

func (j *MT_PR_JSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"pr-json": j.V})
}

func (j *MT_PR_JSON) UnmarshalJSON(data []byte) error {
	var dto map[string]string
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	j.V = dto["pr-json"]
	return nil
}

// MT_PR_Text has its own encoding.TextMarshaler and encoding.TextUnmarshaler with pointer receivers, which write it as "pr-text:V".
type MT_PR_Text struct{ V string }

func (p *MT_PR_Text) MarshalText() ([]byte, error) {
	return []byte("pr-text:" + p.V), nil
}

func (p *MT_PR_Text) UnmarshalText(text []byte) error {
	p.V = strings.TrimPrefix(string(text), "pr-text:")
	return nil
}

// MT_PR_Mixed has its own json.Marshaler with a pointer receiver, and its own encoding.TextMarshaler with a value receiver,
// so encoding/json writes an addressable value with MarshalJSON, and any other value with MarshalText.
type MT_PR_Mixed struct{ V string }

func (m *MT_PR_Mixed) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"json": m.V})
}

func (m MT_PR_Mixed) MarshalText() ([]byte, error) {
	return []byte("text:" + m.V), nil
}

// MT_PR_Registered has fields with marshalers which have pointer receivers.
type MT_PR_Registered struct {
	JSON MT_PR_JSON
	Text MT_PR_Text
}

// MT_PR_RegisteredGreeting is like MT_PR_Registered, with an interface field, which the Codec writes with its $type.
type MT_PR_RegisteredGreeting struct {
	JSON    MT_PR_JSON
	Text    MT_PR_Text
	Greeter Greeter
}

// MT_Shape is an interface with a few implementations, which a slice of interfaces holds.
type (
	MT_Shape    interface{ Area() float64 }
	MT_Circle   struct{ R float64 }
	MT_Square   struct{ S float64 }
	MT_Triangle struct{ B, H float64 }
)

func (c MT_Circle) Area() float64   { return 3.14159 * c.R * c.R }
func (s MT_Square) Area() float64   { return s.S * s.S }
func (t MT_Triangle) Area() float64 { return 0.5 * t.B * t.H }

// A slice of interfaces, either a plain []Shape or a jsonkit.Array[Shape], is written as a JSON array,
// without a $type around it, where each element is written with its own $type,
// so it is read back as the same values, into either form.
// The $type of an element is the type ID of its type on the Codec which writes it,
// so two Codecs, which register the same types under different type IDs, can't read what the other one writes.
func TestCodec_sliceOfInterfaces(t *testing.T) {
	mkCircle := func(r float64) MT_Shape { return MT_Circle{R: r} }
	mkSquare := func(s float64) MT_Shape { return MT_Square{S: s} }
	mkTriangle := func(b, h float64) MT_Shape { return MT_Triangle{B: b, H: h} }

	shapes := jsonkit.Array[MT_Shape]{
		mkCircle(1),
		mkSquare(2),
		mkTriangle(3, 4),
	}

	var legacyCodec, currentCodec jsonkit.Codec
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Circle](&legacyCodec, "circle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Square](&legacyCodec, "rectangle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Triangle](&legacyCodec, "polygon"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Circle](&currentCodec, "shape.circle"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Square](&currentCodec, "shape.square"))
	t.Cleanup(jsonkit.CodecRegisterTypeID[MT_Triangle](&currentCodec, "shape.triangle"))

	modernData, err := currentCodec.Marshal(shapes)
	assert.NoError(t, err)
	const modernWire = `[{"$type":"shape.circle","R":1},{"$type":"shape.square","S":2},{"$type":"shape.triangle","B":3,"H":4}]`
	assertJSONEqual(t, modernWire, string(modernData))

	var modernGot []MT_Shape
	assert.NoError(t, currentCodec.Unmarshal(modernData, &modernGot))
	assert.Equal(t, []MT_Shape(shapes), modernGot)

	legacyData, err := legacyCodec.Marshal(shapes)
	assert.NoError(t, err)
	assertJSONEqual(t, `[{"$type":"circle","R":1},{"$type":"rectangle","S":2},{"$type":"polygon","B":3,"H":4}]`, string(legacyData))
	var legacyGot []MT_Shape
	assert.NoError(t, legacyCodec.Unmarshal(legacyData, &legacyGot))
	assert.Equal(t, []MT_Shape(shapes), legacyGot)

	assert.ErrorIs(t, legacyCodec.Unmarshal(modernData, &legacyGot), jsonkit.ErrUnknownTypeID)
	assert.ErrorIs(t, currentCodec.Unmarshal(legacyData, &modernGot), jsonkit.ErrUnknownTypeID)

	plainSlice, err := currentCodec.Marshal([]MT_Shape(shapes))
	assert.NoError(t, err)
	assertJSONEqual(t, modernWire, string(plainSlice))

	var fromPlain []MT_Shape
	assert.NoError(t, currentCodec.Unmarshal(plainSlice, &fromPlain))
	assert.Equal(t, []MT_Shape(shapes), fromPlain)
	var fromWrapped jsonkit.Array[MT_Shape]
	assert.NoError(t, currentCodec.Unmarshal(plainSlice, &fromWrapped))
	assert.Equal(t, shapes, fromWrapped)
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

// fooDTOJSON is the JSON which TestEntFooTypeCodec writes for foo: a testent.FooDTO,
// whose members, like foov, are not the fields of testent.Foo,
// so testent.Foo is written and read back as this JSON only with the custom codec.
func fooDTOJSON(t *testcase.T, foo testent.Foo) string {
	return mustMarshalJSON(t, testent.FooDTO{ID: foo.ID.String(), FooV: foo.Foo, BarV: foo.Bar, BazV: foo.Baz})
}

// fooDTOEnvelopeJSON is the JSON of foo, written by TestEntFooTypeCodec, registered under the "foo" type ID,
// where an interface holds it.
func fooDTOEnvelopeJSON(t *testcase.T, foo testent.Foo) string {
	return makeObjectEnvelopeJSON(t, "foo", json.RawMessage(fooDTOJSON(t, foo)))
}

// TestCodecRegisterMigration covers type migrations.
// Data written under a retired $type (with a legacy shape) decodes into the current type,
// while marshalling keeps emitting the current type's own $type.
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
			"$type":     "legacy-foo",
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

		// currentFoo is a Foo written today, under the current $type, the way an interface holding it is written.
		currentFoo := let.Var(s, func(t *testcase.T) []byte {
			var holder testent.Fooer = expected.Get(t)
			foo, err := codec.Get(t).Marshal(&holder)
			assert.Must(t).NoError(err)
			return foo
		})

		unmarshalInto := func(t *testcase.T, ptr any) error {
			return codec.Get(t).Unmarshal(data.Get(t), ptr)
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

		// The members of a JSON object are unordered, and a store may reorder them, like a PostgreSQL jsonb column.
		s.When("the $type of the legacy data is not its first member", func(s *testcase.Spec) {
			legacyData.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"legacy_id":` + mustMarshalJSON(t, legacyID.Get(t)) +
					`,"name":` + mustMarshalJSON(t, legacyName.Get(t)) + `,"$type":"legacy-foo"}`)
			})

			s.Then("legacy data is migrated into the current type, whatever the destination", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[testent.Fooer](t, got.Fooer, expected.Get(t))
				assert.Equal(t, got.Fooers, []testent.Fooer{expected.Get(t)})
				assert.Equal(t, got.Foo, expected.Get(t))
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
			data, err := codec.Get(t).Marshal(&got)
			return string(data), err
		}

		s.Then("a value decoded from legacy data is written under the canonical type ID", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, fooDTOEnvelopeJSON(t, expected.Get(t)), got)
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

	s.Test("a type which is not a struct is migrated too, whatever the destination", func(t *testcase.T) {
		type legacyDTO struct {
			Items []string `json:"legacy_items"`
		}
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_CustomSlice](&c, "custom-slice"))
		t.Defer(jsonkit.CodecRegisterMigration(&c, "legacy-slice", func(c *jsonkit.Codec, data []byte, p *MT_CustomSlice) error {
			var dto legacyDTO
			if err := json.Unmarshal(data, &dto); err != nil {
				return err
			}
			*p = dto.Items
			return nil
		}))

		type Box struct {
			Any   any
			Value MT_CustomSlice
			Slice []MT_CustomSlice
		}
		var (
			exp    = MT_CustomSlice(random.Slice(t.Random.IntBetween(1, 3), t.Random.String))
			legacy = mustMarshalJSON(t, map[string]any{"$type": "legacy-slice", "legacy_items": []string(exp)})
		)

		var got Box
		assert.NoError(t, c.Unmarshal([]byte(`{"Any":`+legacy+`,"Value":`+legacy+`,"Slice":[`+legacy+`]}`), &got))
		assert.Equal(t, Box{Any: exp, Value: exp, Slice: []MT_CustomSlice{exp}}, got)

		// and what is written under the current type ID still decodes
		current, err := c.Marshal(Box{Any: exp, Value: exp, Slice: []MT_CustomSlice{exp}})
		assert.NoError(t, err)
		got = Box{}
		assert.NoError(t, c.Unmarshal(current, &got))
		assert.Equal(t, Box{Any: exp, Value: exp, Slice: []MT_CustomSlice{exp}}, got)
	})

	// The $type has done its job by telling which migration reads the legacy value,
	// so the migration reads the members of the legacy value, and nothing else.
	s.Test("the migration gets the members of the legacy value, in their order, whatever the destination", func(t *testcase.T) {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Pairs](&c, "pairs"))
		t.Defer(jsonkit.CodecRegisterMigration(&c, "legacy-pairs", unmarshalMTPairs))

		exp := makeMTPairs(t)
		members, err := marshalMTPairs(nil, exp)
		assert.Must(t).NoError(err)
		legacy := `{"$type":"legacy-pairs",` + string(bytes.TrimPrefix(members, []byte("{")))

		type Box struct {
			Any   any
			Value MT_Pairs
			Slice []MT_Pairs
		}
		var got Box
		assert.NoError(t, c.Unmarshal([]byte(`{"Any":`+legacy+`,"Value":`+legacy+`,"Slice":[`+legacy+`]}`), &got))
		assert.Equal(t, Box{Any: exp, Value: exp, Slice: []MT_Pairs{exp}}, got)

		var top MT_Pairs
		assert.NoError(t, c.Unmarshal([]byte(legacy), &top))
		assert.Equal(t, exp, top)
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

	s.Test("jsonkit.TypeCodec[T]", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		t.Random.Repeat(3, 7, func() {
			foo := MakeFoo(t)
			assertWireFormat(t, &c, foo, fooDTOJSON(t, foo))
		})
	})

	s.Test("impl of jsonkit.ITypeCodec[T]", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", TestEntFooTypeCodec{})()

		t.Random.Repeat(3, 7, func() {
			foo := MakeFoo(t)
			assertWireFormat(t, &c, foo, fooDTOJSON(t, foo))
		})
	})

	// The former type IDs of a type with a custom codec are linked to its type ID with CodecRegisterTypeID, as aliases,
	// which are read back as the type, while the value is written with its type ID.
	s.Context("its former type IDs are linked to its type ID with CodecRegisterTypeID", func(s *testcase.Spec) {
		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t)))
			t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo", "former-foo", "older-foo"))
			return &c
		})
		foo := let.Var(s, func(t *testcase.T) testent.Foo {
			return MakeFoo(t)
		})

		s.Describe("#Unmarshal", func(s *testcase.Spec) {
			typeID := let.VarOf[jsonkit.TypeID](s, "former-foo")
			// written in the wire format of the custom codec, which differs from the fields of testent.Foo
			data := let.Var(s, func(t *testcase.T) []byte {
				return []byte(mustMarshalJSON(t, map[string]string{
					"$type": string(typeID.Get(t)),
					"id":    foo.Get(t).ID.String(),
					"foov":  foo.Get(t).Foo,
					"barv":  foo.Get(t).Bar,
					"bazv":  foo.Get(t).Baz,
				}))
			})

			act := func(t *testcase.T) (testent.Fooer, error) {
				var got testent.Fooer
				err := codec.Get(t).Unmarshal(data.Get(t), &got)
				return got, err
			}

			s.Then("the data written under an alias is read back as the registered type, through its custom codec", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[testent.Fooer](t, foo.Get(t), got)
			})

			s.When("the data is written under another alias", func(s *testcase.Spec) {
				typeID.LetValue(s, "older-foo")

				s.Then("it is read back as the registered type as well", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[testent.Fooer](t, foo.Get(t), got)
				})
			})

			s.When("the link is removed", func(s *testcase.Spec) {
				codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
					var c jsonkit.Codec
					t.Defer(jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t)))
					jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo", "former-foo", "older-foo")()
					return &c
				})

				s.Then("the alias names no type anymore", func(t *testcase.T) {
					_, err := act(t)
					assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
				})

				s.And("the data is written under the type ID", func(s *testcase.Spec) {
					typeID.LetValue(s, "foo")

					s.Then("it is still read back as the registered type, through its custom codec", func(t *testcase.T) {
						got, err := act(t)
						assert.NoError(t, err)
						assert.Equal[testent.Fooer](t, foo.Get(t), got)
					})
				})
			})

			s.When("the link is removed, then made again with the same aliases", func(s *testcase.Spec) {
				codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
					var c jsonkit.Codec
					t.Defer(jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t)))
					jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo", "former-foo", "older-foo")()
					t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo", "former-foo", "older-foo"))
					return &c
				})

				s.Then("the data written under an alias is read back as the registered type again", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[testent.Fooer](t, foo.Get(t), got)
				})
			})
		})

		s.Describe("#Marshal", func(s *testcase.Spec) {
			act := func(t *testcase.T) ([]byte, error) {
				var held testent.Fooer = foo.Get(t)
				return codec.Get(t).Marshal(&held)
			}

			s.Then("the value is written with its type ID, not with an alias", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, fooDTOEnvelopeJSON(t, foo.Get(t)), string(data))
			})
		})

		s.Describe("linking aliases to another type ID", func(s *testcase.Spec) {
			act := func(t *testcase.T) {
				jsonkit.CodecRegisterTypeID[testent.Foo](codec.Get(t), "bar", "former-bar")
			}

			s.Then("it panics, as the type is registered with another type ID", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })
			})
		})
	})

	// Each registration, made with CodecRegister or CodecRegisterTypeID, is removed with the function it returned, and only with that,
	// so removing one leaves the others of the type in place, whichever order they were made or removed in.
	s.Describe("the registrations of a type", func(s *testcase.Spec) {
		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t)))
			return &c
		})
		secondFooCodec := let.Var(s, func(t *testcase.T) jsonkit.TypeCodec[testent.Foo] {
			return jsonkit.TypeCodec[testent.Foo]{
				MarshalFunc: func(c *jsonkit.Codec, v testent.Foo) ([]byte, error) {
					return []byte(`{"codec":"second"}`), nil
				},
				UnmarshalFunc: fooCodec.Get(t).UnmarshalFunc,
			}
		})
		foo := let.Var(s, func(t *testcase.T) testent.Foo {
			return MakeFoo(t)
		})

		// the value is held by an interface, so it is written with the type ID of its type
		act := func(t *testcase.T) (string, error) {
			var held testent.Fooer = foo.Get(t)
			data, err := codec.Get(t).Marshal(&held)
			return string(data), err
		}

		s.Then("the value is written through the custom codec, with the type ID", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, fooDTOEnvelopeJSON(t, foo.Get(t)), got)
		})

		s.When("CodecRegisterTypeID registered the type ID before CodecRegister, whose registration is removed", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo"))
				jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()
				return &c
			})

			s.Then("the type ID stays registered, so the value is still written with it, without the custom codec", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, makeObjectEnvelopeJSON(t, "foo", foo.Get(t)), got)
			})
		})

		s.When("CodecRegisterTypeID registered the type ID after CodecRegister, whose registration is removed", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				remove := jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))
				t.Defer(jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo"))
				remove()
				return &c
			})

			s.Then("the type ID stays registered, so the value is still written with it, without the custom codec", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, makeObjectEnvelopeJSON(t, "foo", foo.Get(t)), got)
			})
		})

		s.When("CodecRegisterTypeID registered the type ID before CodecRegister, and its registration is removed", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				remove := jsonkit.CodecRegisterTypeID[testent.Foo](&c, "foo")
				t.Defer(jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t)))
				remove()
				return &c
			})

			s.Then("the CodecRegister registration stays, so the value is still written through the custom codec, with the type ID", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, fooDTOEnvelopeJSON(t, foo.Get(t)), got)
			})
		})

		s.When("the type is registered with CodecRegister twice, and the first registration is removed", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				remove := jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))
				t.Defer(jsonkit.CodecRegister(&c, "foo", secondFooCodec.Get(t)))
				remove()
				return &c
			})

			s.Then("the second registration stays, so the value is written through its custom codec, with the type ID", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"$type":"foo","codec":"second"}`, got)
			})
		})

		s.When("the type is registered with CodecRegister twice, and both registrations are removed, the first one first", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				var c jsonkit.Codec
				removeFirst := jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))
				removeSecond := jsonkit.CodecRegister(&c, "foo", secondFooCodec.Get(t))
				removeFirst()
				removeSecond()
				return &c
			})

			s.Then("no registration is left, so the value held by the interface can't be written", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)
			})
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

	s.Test("a value of the registered type is written by the custom codec as a field, and with its $type where an interface holds it", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct {
			Foo     testent.Foo
			Fooer   testent.Fooer
			FooerVS []testent.Fooer
		}

		foo, fooer, elem := testent.MakeFoo(t), testent.MakeFoo(t), testent.MakeFoo(t)
		assertWireFormat(t, &c, T{Foo: foo, Fooer: fooer, FooerVS: []testent.Fooer{elem}},
			`{"Foo":`+fooDTOJSON(t, foo)+`,"Fooer":`+fooDTOEnvelopeJSON(t, fooer)+`,"FooerVS":[`+fooDTOEnvelopeJSON(t, elem)+`]}`)
	})

	// The element type of a slice, the value type of a map and the type of a field tell the type of the value, so it needs no $type.
	s.Test("slices, maps and nested structs of the registered type as struct fields are written by the custom codec, without a $type", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type Inner struct{ Foo testent.Foo }
		type T struct {
			List   []testent.Foo
			ByName map[string]testent.Foo
			Inner  Inner
		}

		key := t.Random.HexN(8)
		exp := T{
			List:   []testent.Foo{testent.MakeFoo(t), testent.MakeFoo(t)},
			ByName: map[string]testent.Foo{key: testent.MakeFoo(t)},
			Inner:  Inner{Foo: testent.MakeFoo(t)},
		}
		assertWireFormat(t, &c, exp, `{`+
			`"List":[`+fooDTOJSON(t, exp.List[0])+`,`+fooDTOJSON(t, exp.List[1])+`],`+
			`"ByName":{`+mustMarshalJSON(t, key)+`:`+fooDTOJSON(t, exp.ByName[key])+`},`+
			`"Inner":{"Foo":`+fooDTOJSON(t, exp.Inner.Foo)+`}}`)
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

		assertWireFormat(t, &c, exp, `{"Name":`+mustMarshalJSON(t, exp.Name)+`}`)
	})

	s.Test("a pointer struct field of the registered type is written by the custom codec, and as null when it is nil", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		type T struct{ Set, Nil *testent.Foo }
		foo := testent.MakeFoo(t)

		assertWireFormat(t, &c, T{Set: &foo}, `{"Set":`+fooDTOJSON(t, foo)+`,"Nil":null}`)
	})

	s.Test("a value of the registered type deep in nested map[string]any values is read back as the value", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		// Each level is a map[string]any, with the registered value in the innermost one.
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

	s.Test("values of the registered type in a mix of nested map[string]any and []any values are read back as the values", func(t *testcase.T) {
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

	s.Test("values of the registered type in a top-level []any are read back as the values", func(t *testcase.T) {
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

	s.Test("an empty map[string]any is written as an empty JSON object, and read back as an empty map", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		exp := map[string]any{}

		data, err := c.Marshal(exp)
		assert.NoError(t, err)
		assertJSONEqual(t, "{}", string(data))

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// The $type tells the type of the value, and not whether it was held through a pointer,
	// so a pointer to a value of the registered type, which an `any` holds, is read back as the value.
	s.Test("a pointer to a value of the registered type in a map[string]any is read back as the value", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister(&c, "foo", fooCodec.Get(t))()

		valueKey, pointerKey := t.Random.UUID(), t.Random.UUID()
		value, pointed := testent.MakeFoo(t), testent.MakeFoo(t)

		data, err := c.Marshal(map[string]any{valueKey: value, pointerKey: &pointed})
		assert.NoError(t, err)

		var got map[string]any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, map[string]any{valueKey: value, pointerKey: pointed}, got)
	})

	// A custom codec registered for a pointer type is used for the values of the type it points to as well.
	s.Test("a custom codec registered for *T writes and reads a T too", func(t *testcase.T) {
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
		assertJSONEqual(t, mustMarshalJSON(t, DTO(exp)), string(data),
			"the custom codec of *T writes the T, without a $type, as the caller knows its type")

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	// Like encoding/json with a json.Marshaler which writes null, the custom codec can write null to tell that there is no value.
	s.Test("a value which the custom codec writes as null is written as null", func(t *testcase.T) {
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
		assertJSONEqual(t, "null", string(data))

		var got T
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, T{}, got)
	})

	// Like encoding/json with a json.Marshaler which writes null,
	// a value in an interface field which the custom codec writes as null is a plain null, without a $type around it.
	s.Test("a value in an interface field which the custom codec writes as null is written as a plain null", func(t *testcase.T) {
		type T struct{ V string }
		type Box struct{ Any any }

		var c jsonkit.Codec
		defer jsonkit.CodecRegister[T](&c, "null_in_any_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return []byte("null"), nil
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				*p = T{}
				return nil
			},
		})()

		data, err := c.Marshal(Box{Any: T{V: t.Random.String()}})
		assert.NoError(t, err)
		assertJSONEqual(t, `{"Any":null}`, string(data))

		var got Box
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, Box{}, got)
	})

	s.Test("the error of the custom codec's Marshal is returned by Marshal", func(t *testcase.T) {
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

	// Like encoding/json with the JSON of a json.Marshaler, the Codec checks the JSON the custom codec writes,
	// which has to be a single JSON value, wherever the value sits.
	// A custom codec writes null to tell that there is no value.
	s.Context("the custom codec writes something which is not a JSON value", func(s *testcase.Spec) {
		type T struct{ V string }
		type Box struct {
			Value T
			Any   any
		}

		output := let.Var(s, func(t *testcase.T) []byte {
			return []byte(`{"v":`)
		})
		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "invalid_t", jsonkit.TypeCodec[T]{
				MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
					return output.Get(t), nil
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
					return nil
				},
			}))
			return &c
		})
		value := let.Var(s, func(t *testcase.T) any {
			v := T{V: t.Random.String()}
			return random.Pick[any](t.Random, v, &v, Box{Value: v}, Box{Any: v})
		})

		act := func(t *testcase.T) error {
			_, err := codec.Get(t).Marshal(value.Get(t))
			return err
		}

		thenItIsAnErrorWhichTellsTheType := func(s *testcase.Spec) {
			s.Then("Marshal returns an error which tells the type", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), reflect.TypeFor[T]().String())
			})
		}

		thenItIsAnErrorWhichTellsTheType(s)

		s.When("it writes nothing", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []byte { return nil })

			thenItIsAnErrorWhichTellsTheType(s)
		})

		s.When("it writes an empty byte slice", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []byte { return []byte{} })

			thenItIsAnErrorWhichTellsTheType(s)
		})

		s.When("it writes only whitespace", func(s *testcase.Spec) {
			output.Let(s, func(t *testcase.T) []byte { return []byte(makeJSONWhitespace(t)) })

			thenItIsAnErrorWhichTellsTheType(s)
		})
	})

	s.Test("the error of the custom codec's Unmarshal is returned by Unmarshal", func(t *testcase.T) {
		type T struct{ V string }

		var c jsonkit.Codec
		sentinel := errors.New("user unmarshal failure")
		defer jsonkit.CodecRegister[T](&c, "unerr_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(map[string]string{"v": v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				return sentinel
			},
		})()

		data, err := c.Marshal(T{V: t.Random.String()})
		assert.NoError(t, err)

		var got T
		assert.ErrorIs(t, c.Unmarshal(data, &got), sentinel)
	})

	// Like CodecRegisterTypeID, which panics when the type is registered with another type ID.
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

		assert.Panic(t, func() {
			jsonkit.CodecRegister[T](&c, "second_t", jsonkit.TypeCodec[T]{
				MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
					return json.Marshal(v)
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
					return json.Unmarshal(data, p)
				},
			})
		})
	})

	// Like with encoding/json/v2, where the marshalers given as options take precedence over the type's own methods,
	// the custom codec registered on a Codec wins over the json.Marshaler and json.Unmarshaler of the type,
	// wherever the value sits.
	s.Context("the registered type has its own json.Marshaler and json.Unmarshaler too", func(s *testcase.Spec) {
		type DTO struct {
			Custom string `json:"custom"`
		}
		type Box struct {
			Value   MT_OwnAndCustomJSON
			Pointer *MT_OwnAndCustomJSON
			Any     any
			Slice   []MT_OwnAndCustomJSON
			Map     map[string]MT_OwnAndCustomJSON
		}
		const typeID = "own-and-custom"

		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, typeID, jsonkit.TypeCodec[MT_OwnAndCustomJSON]{
				MarshalFunc: func(c *jsonkit.Codec, v MT_OwnAndCustomJSON) ([]byte, error) {
					return json.Marshal(DTO{Custom: v.V})
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_OwnAndCustomJSON) error {
					var dto DTO
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					p.V = dto.Custom
					return nil
				},
			}))
			return &c
		})
		value := let.Var(s, func(t *testcase.T) MT_OwnAndCustomJSON {
			return MT_OwnAndCustomJSON{V: t.Random.String()}
		})
		box := let.Var(s, func(t *testcase.T) Box {
			v := value.Get(t)
			return Box{
				Value:   v,
				Pointer: &v,
				Any:     v,
				Slice:   []MT_OwnAndCustomJSON{v},
				Map:     map[string]MT_OwnAndCustomJSON{t.Random.HexN(4): v},
			}
		})

		s.Describe("#Marshal", func(s *testcase.Spec) {
			input := let.Var(s, func(t *testcase.T) any { return box.Get(t) })

			act := func(t *testcase.T) ([]byte, error) {
				return codec.Get(t).Marshal(input.Get(t))
			}

			s.Then("the custom codec writes the value wherever it sits", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var (
					custom = mustMarshalJSON(t, DTO{Custom: value.Get(t).V})
					// an interface value needs its $type
					typed = mustMarshalJSON(t, map[string]string{"$type": typeID, "custom": value.Get(t).V})
					key   = mustMarshalJSON(t, slices.Collect(maps.Keys(box.Get(t).Map))[0])
				)
				assertJSONEqual(t, `{"Value":`+custom+`,"Pointer":`+custom+`,"Any":`+typed+
					`,"Slice":[`+custom+`],"Map":{`+key+`:`+custom+`}}`, string(data))
			})

			s.When("the value is the top-level value", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) any { return value.Get(t) })

				s.Then("the custom codec writes it", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, mustMarshalJSON(t, DTO{Custom: value.Get(t).V}), string(data))
				})
			})

			s.When("the value is held by an `any` in a slice, which is the top-level value", func(s *testcase.Spec) {
				input.Let(s, func(t *testcase.T) any { return []any{value.Get(t)} })

				s.Then("the custom codec writes it, with its $type", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, `[`+mustMarshalJSON(t, map[string]string{"$type": typeID, "custom": value.Get(t).V})+`]`, string(data))
				})
			})
		})

		s.Describe("#Unmarshal", func(s *testcase.Spec) {
			data := let.Var(s, func(t *testcase.T) string {
				var (
					custom = mustMarshalJSON(t, DTO{Custom: value.Get(t).V})
					// an interface value needs its $type
					typed = mustMarshalJSON(t, map[string]string{"$type": typeID, "custom": value.Get(t).V})
					key   = mustMarshalJSON(t, slices.Collect(maps.Keys(box.Get(t).Map))[0])
				)
				return `{"Value":` + custom + `,"Pointer":` + custom + `,"Any":` + typed +
					`,"Slice":[` + custom + `],"Map":{` + key + `:` + custom + `}}`
			})

			act := func(t *testcase.T) (Box, error) {
				var got Box
				err := codec.Get(t).Unmarshal([]byte(data.Get(t)), &got)
				return got, err
			}

			s.Then("the custom codec reads the value wherever it sits", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, box.Get(t), got)
			})
		})

		s.Test("the value round-trips as the top-level value", func(t *testcase.T) {
			roundTrip(t, codec.Get(t), value.Get(t))
		})

		// The custom codec belongs to the Codec it was registered on.
		s.Test("another Codec, without the registration, uses the type's own methods", func(t *testcase.T) {
			_ = codec.Get(t) // the type has a custom codec in the process

			type Box struct {
				Value   MT_OwnAndCustomJSON
				Pointer *MT_OwnAndCustomJSON
				Slice   []MT_OwnAndCustomJSON
				Map     map[string]MT_OwnAndCustomJSON
			}
			v := value.Get(t)

			var other jsonkit.Codec
			assertEncodesLikeEncodingJSON(t, &other, v)
			assertEncodesLikeEncodingJSON(t, &other, Box{
				Value:   v,
				Pointer: &v,
				Slice:   []MT_OwnAndCustomJSON{v},
				Map:     map[string]MT_OwnAndCustomJSON{t.Random.HexN(4): v},
			})
		})
	})

	// A custom codec is not limited to struct types.
	s.Context("the registered type is not a struct", func(s *testcase.Spec) {
		type DTO struct {
			Items []string `json:"items"`
		}
		type Box struct {
			Value   MT_CustomSlice
			Pointer *MT_CustomSlice
			Any     any
			Slice   []MT_CustomSlice
			Map     map[string]MT_CustomSlice
		}

		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "custom-slice", jsonkit.TypeCodec[MT_CustomSlice]{
				MarshalFunc: func(c *jsonkit.Codec, v MT_CustomSlice) ([]byte, error) {
					return json.Marshal(DTO{Items: v})
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_CustomSlice) error {
					var dto DTO
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = dto.Items
					return nil
				},
			}))
			return &c
		})
		value := let.Var(s, func(t *testcase.T) MT_CustomSlice {
			return random.Slice(t.Random.IntBetween(1, 3), func() string { return t.Random.String() })
		})

		s.Test("the value is written with the custom codec", func(t *testcase.T) {
			data, err := codec.Get(t).Marshal(value.Get(t))
			assert.NoError(t, err)
			assertJSONEqual(t, mustMarshalJSON(t, DTO{Items: value.Get(t)}), string(data))
		})

		s.Test("the value round-trips as the top-level value", func(t *testcase.T) {
			roundTrip(t, codec.Get(t), value.Get(t))
		})

		s.Test("the value round-trips wherever it sits", func(t *testcase.T) {
			v := value.Get(t)
			roundTrip(t, codec.Get(t), Box{
				Value:   v,
				Pointer: &v,
				Any:     v,
				Slice:   []MT_CustomSlice{v},
				Map:     map[string]MT_CustomSlice{t.Random.HexN(4): v},
			})
		})
	})

	// A struct registered with CodecRegisterTypeID is written in its $type envelope.
	// The custom codecs of its fields receive the Codec there too,
	// as a custom codec may write the values it holds through it.
	s.Context("the registered type is a field of a struct registered with CodecRegisterTypeID", func(s *testcase.Spec) {
		type Inner struct{ X string }
		type Outer struct{ Inner Inner }
		type DTO struct {
			X string `json:"x"`
		}

		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "inner", jsonkit.TypeCodec[Inner]{
				MarshalFunc: func(c *jsonkit.Codec, v Inner) ([]byte, error) {
					if c == nil {
						return nil, fmt.Errorf("the custom codec of Inner received no Codec")
					}
					return c.Marshal(DTO{X: v.X})
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *Inner) error {
					var dto DTO
					if err := c.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = Inner{X: dto.X}
					return nil
				},
			}))
			t.Defer(jsonkit.CodecRegisterTypeID[Outer](&c, "outer"))
			return &c
		})
		value := let.Var(s, func(t *testcase.T) Outer {
			return Outer{Inner: Inner{X: t.Random.String()}}
		})

		s.Test("the custom codec of the field receives the Codec", func(t *testcase.T) {
			assertWireFormat(t, codec.Get(t), value.Get(t), `{"Inner":{"x":`+mustMarshalJSON(t, value.Get(t).Inner.X)+`}}`)
		})

		s.Test("the custom codec of the field receives the Codec when an interface holds the struct", func(t *testcase.T) {
			assertWireFormat(t, codec.Get(t), struct{ Any any }{Any: value.Get(t)},
				`{"Any":{"$type":"outer","Inner":{"x":`+mustMarshalJSON(t, value.Get(t).Inner.X)+`}}}`)
		})
	})

	// The Codec adds the $type next to the members of the JSON object the custom codec writes.
	// The $type has done its job by telling which custom codec reads the value,
	// so the custom codec reads back the members it wrote, and nothing else.
	s.Context("the custom codec writes the value as a JSON object with arbitrary members", func(s *testcase.Spec) {
		type Box struct {
			Value   MT_Pairs
			Pointer *MT_Pairs
			Any     any
			Slice   []MT_Pairs
			Map     map[string]MT_Pairs
		}

		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "pairs", jsonkit.TypeCodec[MT_Pairs]{
				MarshalFunc:   marshalMTPairs,
				UnmarshalFunc: unmarshalMTPairs,
			}))
			return &c
		})
		value := let.Var(s, makeMTPairs)

		s.Test("the registered Unmarshal gets the members it wrote, in their order", func(t *testcase.T) {
			roundTrip(t, codec.Get(t), value.Get(t))
		})

		s.Test("the registered Unmarshal gets the members it wrote, wherever the value sits", func(t *testcase.T) {
			v := value.Get(t)
			roundTrip(t, codec.Get(t), Box{
				Value:   v,
				Pointer: &v,
				Any:     v,
				Slice:   []MT_Pairs{v},
				Map:     map[string]MT_Pairs{t.Random.HexN(4): v},
			})
		})
	})

	// Like encoding/json with the JSON of a json.Marshaler,
	// the Codec takes the JSON the custom codec writes, whatever insignificant whitespace it has.
	s.Context("the custom codec writes JSON with insignificant whitespace in it", func(s *testcase.Spec) {
		type Struct struct{ X string }
		type Name string
		type Box struct {
			Value   Struct
			Pointer *Struct
			Any     any
			Slice   []Struct
			Map     map[string]Struct
			Zero    Struct
			Name    Name
			AnyName any
		}

		codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
			// spacedObject writes a JSON object, which has a member with the value unless it is empty,
			// with whitespace around each of its tokens.
			spacedObject := func(member, value string) []byte {
				ws := func() string { return makeJSONWhitespace(t) }
				if value == "" {
					return []byte(ws() + "{" + ws() + "}" + ws())
				}
				return []byte(ws() + "{" + ws() + mustMarshalJSON(t, member) + ws() + ":" + ws() + mustMarshalJSON(t, value) + ws() + "}" + ws())
			}

			var c jsonkit.Codec
			t.Defer(jsonkit.CodecRegister(&c, "spaced-struct", jsonkit.TypeCodec[Struct]{
				MarshalFunc: func(c *jsonkit.Codec, v Struct) ([]byte, error) {
					return spacedObject("x", v.X), nil
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *Struct) error {
					var dto struct {
						X string `json:"x"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = Struct{X: dto.X}
					return nil
				},
			}))
			t.Defer(jsonkit.CodecRegister(&c, "spaced-name", jsonkit.TypeCodec[Name]{
				MarshalFunc: func(c *jsonkit.Codec, v Name) ([]byte, error) {
					return spacedObject("name", string(v)), nil
				},
				UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *Name) error {
					var dto struct {
						Name string `json:"name"`
					}
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = Name(dto.Name)
					return nil
				},
			}))
			return &c
		})
		value := let.Var(s, func(t *testcase.T) Struct {
			return Struct{X: t.Random.String()}
		})
		name := let.Var(s, func(t *testcase.T) Name {
			return Name(t.Random.String())
		})

		s.Test("the value round-trips as the top-level value", func(t *testcase.T) {
			roundTrip(t, codec.Get(t), value.Get(t))
		})

		s.Test("the zero value round-trips as the top-level value", func(t *testcase.T) {
			roundTrip(t, codec.Get(t), Struct{})
		})

		s.Test("the value round-trips wherever it sits", func(t *testcase.T) {
			v := value.Get(t)
			roundTrip(t, codec.Get(t), Box{
				Value:   v,
				Pointer: &v,
				Any:     v,
				Slice:   []Struct{v},
				Map:     map[string]Struct{t.Random.HexN(4): v},
				Name:    name.Get(t),
				AnyName: name.Get(t),
			})
		})

		s.Test("the $type is written next to the members the custom codec writes", func(t *testcase.T) {
			x := mustMarshalJSON(t, value.Get(t).X)

			var holder any = value.Get(t)
			data, err := codec.Get(t).Marshal(&holder)
			assert.NoError(t, err)
			assertJSONEqual(t, `{"$type":"spaced-struct","x":`+x+`}`, string(data))

			data, err = codec.Get(t).Marshal(Box{Any: value.Get(t), AnyName: name.Get(t)})
			assert.NoError(t, err)
			assertJSONEqual(t, `{"Value":{},"Pointer":null,"Any":{"$type":"spaced-struct","x":`+x+`},"Slice":null,"Map":null,"Zero":{},"Name":{},`+
				`"AnyName":{"$type":"spaced-name","name":`+mustMarshalJSON(t, string(name.Get(t)))+`}}`, string(data))
		})
	})

	// Codecs are independent of each other, so a Codec can be set up while another one is in use,
	// and what the other one has written and read before makes no difference to it, wherever the value sits.
	s.Test("a registration made while another Codec is in use is in effect afterwards, on its own Codec only", func(t *testcase.T) {
		type T struct{ V string }
		type Box struct {
			Value T
			Any   any
		}
		tc := jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(map[string]string{"custom": v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto struct {
					Custom string `json:"custom"`
				}
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T{V: dto.Custom}
				return nil
			},
		}
		v := T{V: t.Random.UUID()}
		box := Box{Value: v, Any: makeGreeter(t)}
		defaultJSON := mustMarshalJSON(t, v)
		customJSON := `{"custom":` + mustMarshalJSON(t, v.V) + `}`
		boxJSON := func(value string) string {
			return `{"Value":` + value + `,"Any":` + greeterJSON(t, box.Any.(Greeter)) + `}`
		}

		var c, other jsonkit.Codec
		assertWireFormat(t, &other, v, defaultJSON)
		assertWireFormat(t, &other, box, boxJSON(defaultJSON))

		var (
			unregister func()
			otherData  []byte
			otherErr   error
		)
		testcase.Race(func() {
			otherData, otherErr = other.Marshal(box)
		}, func() {
			unregister = jsonkit.CodecRegister(&c, "race", tc)
		})
		t.Defer(unregister)
		assert.NoError(t, otherErr)
		assertJSONEqual(t, boxJSON(defaultJSON), string(otherData))

		assertWireFormat(t, &c, v, customJSON)
		assertWireFormat(t, &c, box, boxJSON(customJSON))
		assertWireFormat(t, &other, v, defaultJSON)
		assertWireFormat(t, &other, box, boxJSON(defaultJSON))
	})

	// The registrations of a type on independent Codecs don't affect each other,
	// so removing the registration from one of them keeps the other one's working.
	s.Test("unregistering from a Codec leaves another Codec's registration of the same type in use", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			X string `json:"x"`
		}
		tc := jsonkit.TypeCodec[T]{
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
		}

		var kept, removed jsonkit.Codec
		t.Defer(jsonkit.CodecRegister(&kept, "kept_t", tc))
		jsonkit.CodecRegister(&removed, "removed_t", tc)()

		type Box struct {
			Value T
			Slice []T
		}
		exp := Box{
			Value: T{V: t.Random.String()},
			Slice: []T{{V: t.Random.String()}},
		}
		assertWireFormat(t, &kept, exp, mustMarshalJSON(t, struct {
			Value DTO
			Slice []DTO
		}{Value: DTO{X: exp.Value.V}, Slice: []DTO{{X: exp.Slice[0].V}}}))
	})

	// Like when the function is deferred, and also called on the way.
	s.Test("unregistering more than once still leaves another Codec's registration of the same type in use", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			X string `json:"x"`
		}
		tc := jsonkit.TypeCodec[T]{
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
		}

		var kept, removed jsonkit.Codec
		t.Defer(jsonkit.CodecRegister(&kept, "kept_t", tc))
		unregister := jsonkit.CodecRegister(&removed, "removed_t", tc)
		unregister()
		unregister()
		removeMigration := jsonkit.CodecRegisterMigration(&removed, "legacy_t", tc.UnmarshalFunc)
		removeMigration()
		removeMigration()

		exp := struct{ Value T }{Value: T{V: t.Random.String()}}
		assertWireFormat(t, &kept, exp, mustMarshalJSON(t, struct{ Value DTO }{Value: DTO{X: exp.Value.V}}))
	})

	// Codecs which register the same type under different type IDs write it with their own type ID,
	// so neither of them reads what the other one writes.
	s.Test("two codecs with different TypeIDs for the same type are isolated", func(t *testcase.T) {
		type T struct{ V string }

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

		// The $type is written for a value held by an interface.
		var exp any = T{V: t.Random.String()}
		v := mustMarshalJSON(t, exp.(T).V)

		dataA, err := cA.Marshal(&exp)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"a_t","side":"a","v":`+v+`}`, string(dataA))

		dataB, err := cB.Marshal(&exp)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"b_t","side":"b","v":`+v+`}`, string(dataB))

		var got any
		assert.NoError(t, cA.Unmarshal(dataA, &got))
		assert.Equal(t, exp, got)

		assert.ErrorIs(t, cB.Unmarshal(dataA, new(any)), jsonkit.ErrUnknownTypeID)
		assert.ErrorIs(t, cA.Unmarshal(dataB, new(any)), jsonkit.ErrUnknownTypeID)
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
		var other jsonkit.Codec
		assertEncodesLikeEncodingJSON(t, &other, Container{Val: T{V: t.Random.String()}})
	})

	// Removing a registration takes it back, even after the Codec has written and read the type with it.
	s.Test("after its registration is removed, the Codec writes and reads the type like encoding/json does, wherever it sits", func(t *testcase.T) {
		type T struct{ V string }
		type DTO struct {
			Custom string `json:"custom"`
		}
		type Container struct {
			Val T `json:"val"`
		}

		var c jsonkit.Codec
		unregister := jsonkit.CodecRegister[T](&c, "unregistered_t", jsonkit.TypeCodec[T]{
			MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
				return json.Marshal(DTO{Custom: v.V})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
				var dto DTO
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = T{V: dto.Custom}
				return nil
			},
		})
		exp := Container{Val: T{V: t.Random.String()}}
		assertWireFormat(t, &c, exp, `{"val":`+mustMarshalJSON(t, DTO{Custom: exp.Val.V})+`}`)

		unregister()

		assertEncodesLikeEncodingJSON(t, &c, exp)
		assertEncodesLikeEncodingJSON(t, &c, exp.Val)
	})
}

// MT_OwnAndCustomJSON has its own json.Marshaler and json.Unmarshaler, which write it as a JSON string.
// The tests register a custom codec for it, which writes it as a JSON object.
type MT_OwnAndCustomJSON struct{ V string }

func (v MT_OwnAndCustomJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal("own:" + v.V)
}

func (v *MT_OwnAndCustomJSON) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v.V = strings.TrimPrefix(s, "own:")
	return nil
}

// MT_CustomSlice is a type which is not a struct, which the tests register a custom codec for.
type MT_CustomSlice []string

// MT_OwnJSONMap is a map type with its own json.Marshaler and json.Unmarshaler,
// which write its entries as the members of a JSON object, with their values prefixed by "own:".
type MT_OwnJSONMap map[string]string

func (m MT_OwnJSONMap) MarshalJSON() ([]byte, error) {
	prefixed := make(map[string]string, len(m))
	for k, v := range m {
		prefixed[k] = "own:" + v
	}
	return json.Marshal(prefixed)
}

func (m *MT_OwnJSONMap) UnmarshalJSON(data []byte) error {
	var prefixed map[string]string
	if err := json.Unmarshal(data, &prefixed); err != nil {
		return err
	}
	*m = make(MT_OwnJSONMap, len(prefixed))
	for k, v := range prefixed {
		(*m)[k] = strings.TrimPrefix(v, "own:")
	}
	return nil
}

// MT_Pairs is an ordered list of key/value pairs,
// which marshalMTPairs writes as a JSON object, with a member for each pair, in their order,
// and unmarshalMTPairs reads back, like an ordered dictionary.
// The tests register them as the custom codec or the migration of MT_Pairs.
type MT_Pairs []MT_Pair

type MT_Pair struct{ Key, Value string }

func makeMTPairs(t *testcase.T) MT_Pairs {
	pairs := random.Slice(t.Random.IntBetween(2, 4), func() MT_Pair {
		return MT_Pair{Key: t.Random.HexN(8), Value: t.Random.String()}
	})
	// A member with a @ prefixed name, like the @context of JSON-LD, is just another member.
	return append(pairs, MT_Pair{Key: "@" + t.Random.HexN(4), Value: t.Random.String()})
}

func marshalMTPairs(_ *jsonkit.Codec, pairs MT_Pairs) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, pair := range pairs {
		if 0 < i {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(pair.Key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(pair.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func unmarshalMTPairs(_ *jsonkit.Codec, data []byte, p *MT_Pairs) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		return err
	}
	var pairs MT_Pairs
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		var value string
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("member %v: %w", key, err)
		}
		pairs = append(pairs, MT_Pair{Key: key.(string), Value: value})
	}
	*p = pairs
	return nil
}

// TestCodec_customCodecEnvelopeShape pins the rule that the type envelope around what a custom codec writes,
// for a value held by an interface, follows the shape of that JSON:
//
//   - If the custom codec wrote a JSON object, the $type is written as a member of that same object,
//     next to the members the custom codec wrote, unless the registered type is a map.
//   - If the custom codec wrote any other JSON value (e.g. a JSON array for a registered slice type),
//     it is the $value of a {"$type":...,"$value":…} envelope, as such a value has no members to write the $type next to.
//   - If the registered type is a map, and the custom codec wrote a JSON object,
//     the object is the $map of a {"$type":...,"$map":{…}} envelope,
//     as the members of a map's object are its entries.
//
// The members of a JSON object are unordered, so where the $type is among them is not part of the rule.
func TestCodec_customCodecEnvelopeShape(t *testing.T) {
	s := testcase.NewSpec(t)

	// MT_WritesObject is a struct whose custom codec writes a JSON object.
	type MT_WritesObject struct{ V string }
	type MT_WritesObjectDTO struct {
		V string `json:"v"`
	}

	// MT_WritesArray is a slice type whose custom codec writes a JSON array.
	type MT_WritesArray []int

	// MT_MapWritesObject is a map type whose custom codec writes a JSON object, like the one of a struct.
	// The members of that object are the entries of the map, which may have any key, like an "$type".
	type MT_MapWritesObject map[string]string

	s.Test("a custom codec which writes a JSON object: the $type is written as a member of that object", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_WritesObject](&c, "writes-object",
			jsonkit.TypeCodec[MT_WritesObject]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_WritesObject) ([]byte, error) {
					return json.Marshal(MT_WritesObjectDTO{V: v.V})
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_WritesObject) error {
					var dto MT_WritesObjectDTO
					if err := json.Unmarshal(data, &dto); err != nil {
						return err
					}
					*p = MT_WritesObject{V: dto.V}
					return nil
				},
			},
		)()

		var exp any = MT_WritesObject{V: "hello"}
		data, err := c.Marshal(&exp)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"writes-object","v":"hello"}`, string(data))

		var got any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("a custom codec which writes a JSON array: the $type is written in an envelope, with the array as its $value", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_WritesArray](&c, "writes-array",
			jsonkit.TypeCodec[MT_WritesArray]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_WritesArray) ([]byte, error) {
					return json.Marshal([]int(v))
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_WritesArray) error {
					var arr []int
					if err := json.Unmarshal(data, &arr); err != nil {
						return err
					}
					*p = MT_WritesArray(arr)
					return nil
				},
			},
		)()

		var exp any = MT_WritesArray{1, 2, 3}
		data, err := c.Marshal(&exp)
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"writes-array","$value":[1,2,3]}`, string(data))

		var got any
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("a custom codec of a map type which writes a JSON object: the $type is written in an envelope, with the object as its $map", func(t *testcase.T) {
		const prefix = "custom:"
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegister[MT_MapWritesObject](&c, "map-writes-object",
			jsonkit.TypeCodec[MT_MapWritesObject]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_MapWritesObject) ([]byte, error) {
					prefixed := make(map[string]string, len(v))
					for k, e := range v {
						prefixed[k] = prefix + e
					}
					return json.Marshal(prefixed)
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_MapWritesObject) error {
					var prefixed map[string]string
					if err := json.Unmarshal(data, &prefixed); err != nil {
						return err
					}
					*p = make(MT_MapWritesObject, len(prefixed))
					for k, e := range prefixed {
						(*p)[k] = strings.TrimPrefix(e, prefix)
					}
					return nil
				},
			},
		))

		entry := t.Random.HexN(8)
		exp := struct{ V any }{V: MT_MapWritesObject{"$type": t.Random.HexN(8), entry: t.Random.HexN(8)}}
		data, err := c.Marshal(exp)
		assert.NoError(t, err)

		customOutput, err := json.Marshal(map[string]string{
			"$type": prefix + exp.V.(MT_MapWritesObject)["$type"],
			entry:   prefix + exp.V.(MT_MapWritesObject)[entry],
		})
		assert.Must(t).NoError(err)
		assertJSONEqual(t, `{"V":{"$type":"map-writes-object","$map":`+string(customOutput)+`}}`, string(data))

		var got struct{ V any }
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

// MT_RM_Counts is a named map type, which TestCodec_registeredMap registers on the Codec.
type MT_RM_Counts map[string]int

// MT_RM_Tally is another named map type of the same shape as MT_RM_Counts, which is not registered.
type MT_RM_Tally map[string]int

// MT_RM_AB is a struct, which a JSON object with an "A" and a "B" member is read into.
type MT_RM_AB struct{ A, B int }

// MT_RM_Envelope is a DTO of the $map form of the $type envelope.
type MT_RM_Envelope struct {
	Type string          `json:"$type"`
	Map  json.RawMessage `json:"$map"`
}

// TestCodec_registeredMap covers a map type registered on the Codec.
//
// A value only needs its $type where its type would be lost otherwise, like where an interface holds it.
// The $type of a map is written in the $map form, {"$type":"...","$map":{...}},
// so its entries stay apart from the members of the envelope.
// Like with encoding/json, the JSON of a registered map can be read into another type of the same shape,
// like a plain map[string]int, the way a value is remapped into a DTO.
func TestCodec_registeredMap(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_RM_Counts](&c, "rm-counts"))
		return &c
	})
	counts := let.Var(s, func(t *testcase.T) MT_RM_Counts {
		return random.Map(t.Random.IntBetween(1, 3), func() (string, int) {
			return t.Random.HexN(8), t.Random.Int()
		})
	})
	// entriesJSON is the plain JSON object of the entries of counts.
	entriesJSON := func(t *testcase.T) string {
		return mustMarshalJSON(t, map[string]int(counts.Get(t)))
	}
	// envelopeJSON is counts in the $map form, which keeps its entries apart from the $type.
	envelopeJSON := func(t *testcase.T) string {
		return `{"$type":"rm-counts","$map":` + entriesJSON(t) + `}`
	}

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return counts.Get(t)
		})
		act := func(t *testcase.T) (string, error) {
			data, err := subject.Get(t).Marshal(value.Get(t))
			return string(data), err
		}

		// The value given to Marshal is read back into the type the caller knows.
		s.Then("it is written as plain JSON, like with encoding/json", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, entriesJSON(t), got)
		})

		s.When("the value is a pointer to the registered map", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				v := counts.Get(t)
				return &v
			})

			s.Then("it is written the same as the registered map itself", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, entriesJSON(t), got)
			})
		})

		s.When("the value is a pointer to an `any`, which holds the registered map", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				var v any = counts.Get(t)
				return &v
			})

			s.Then("it is written with its $type, in the $map form", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, envelopeJSON(t), got)
			})
		})

		s.When("the registered map is held by a field of its own type", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return struct{ F MT_RM_Counts }{F: counts.Get(t)}
			})

			s.Then("it is written as plain JSON, like with encoding/json, as the field tells its type", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"F":`+entriesJSON(t)+`}`, got)
			})
		})

		s.When("the registered map is held by an `any` field", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return struct{ F any }{F: counts.Get(t)}
			})

			s.Then("it is written with its $type, in the $map form", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"F":`+envelopeJSON(t)+`}`, got)
			})

			s.And("the registered map has entries keyed like the members of the envelope", func(s *testcase.Spec) {
				counts.Let(s, func(t *testcase.T) MT_RM_Counts {
					c := maps.Clone(counts.Super(t))
					c["$type"] = t.Random.Int()
					c["$value"] = t.Random.Int()
					c["$map"] = t.Random.Int()
					return c
				})

				s.Then("they are written as entries within the $map", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, `{"F":`+envelopeJSON(t)+`}`, got)
				})
			})
		})

		s.When("the registered map is an element of a slice of `any`", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []any{counts.Get(t)}
			})

			s.Then("it is written with its $type, in the $map form", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `[`+envelopeJSON(t)+`]`, got)
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		// data is what Marshal writes for an `any` holding the registered map, which has its $type.
		data := let.Var(s, func(t *testcase.T) []byte {
			var holder any = counts.Get(t)
			data, err := subject.Get(t).Marshal(&holder)
			assert.Must(t).NoError(err)
			return data
		})
		ptr := let.Var(s, func(t *testcase.T) any {
			return new(MT_RM_Counts)
		})
		act := func(t *testcase.T) error {
			return subject.Get(t).Unmarshal(data.Get(t), ptr.Get(t))
		}

		s.Then("what Marshal wrote is read back as the registered map", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
		})

		s.When("the destination is a plain map of the same shape", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[string]int)
			})

			s.Then("the entries are read into it, the way encoding/json remaps a value into a DTO", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, map[string]int(counts.Get(t)), *ptr.Get(t).(*map[string]int))
			})
		})

		s.When("the destination is another named map type of the same shape", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(MT_RM_Tally)
			})

			s.Then("the entries are read into it", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, MT_RM_Tally(counts.Get(t)), *ptr.Get(t).(*MT_RM_Tally))
			})
		})

		s.When("the destination is a map of `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[string]any)
			})

			s.Then("the entries are read into it, like encoding/json reads them, except that a whole number is an int", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var exp map[string]any
				unmarshalPlainJSON(t, []byte(entriesJSON(t)), &exp)
				assert.Equal(t, exp, *ptr.Get(t).(*map[string]any))
			})
		})

		s.When("the destination is an `any`", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(any)
			})

			s.Then("it is read back as the registered map, as its $type tells", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, counts.Get(t), *ptr.Get(t).(*any))
			})
		})

		// Like encoding/json reads a JSON object into a struct, whose fields are named after its members.
		s.When("the destination is a struct with a field for each key", func(s *testcase.Spec) {
			counts.Let(s, func(t *testcase.T) MT_RM_Counts {
				return MT_RM_Counts{"A": t.Random.Int(), "B": t.Random.Int()}
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(MT_RM_AB)
			})

			s.Then("the entries are read into the fields", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, MT_RM_AB{A: counts.Get(t)["A"], B: counts.Get(t)["B"]}, *ptr.Get(t).(*MT_RM_AB))
			})
		})

		// A DTO of the envelope itself asks for the $type and the $map, so like with encoding/json, it gets them.
		s.When("the destination is a struct with a member for the $type and one for the $map", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(MT_RM_Envelope)
			})

			s.Then("the envelope is read into it as it is", func(t *testcase.T) {
				assert.NoError(t, act(t))
				got := *ptr.Get(t).(*MT_RM_Envelope)
				assert.Equal(t, "rm-counts", got.Type)
				assert.Equal(t, entriesJSON(t), string(got.Map))
			})
		})

		// What Marshal writes for an `any` holding the registered map can be read into a DTO,
		// whose field tells the type, the way encoding/json remaps a value into a DTO.
		s.When("the registered map is held by an `any` field, and the destination has a plain map field for it", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				data, err := subject.Get(t).Marshal(struct{ F any }{F: counts.Get(t)})
				assert.Must(t).NoError(err)
				return data
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ F map[string]int })
			})

			s.Then("the entries are read into the field", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, map[string]int(counts.Get(t)), ptr.Get(t).(*struct{ F map[string]int }).F)
			})

			// encoding/json reads an escaped character of a member name the same as the character itself.
			s.And("the $ of the members of the envelope is escaped in the JSON", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					escaped := strings.ReplaceAll(string(data.Super(t)), `"$`, `"\u0024`)
					assert.Must(t).NotEqual(string(data.Super(t)), escaped, "the envelope was expected to have $ prefixed members")
					return []byte(escaped)
				})

				s.Then("the entries are read into the field", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]int(counts.Get(t)), ptr.Get(t).(*struct{ F map[string]int }).F)
				})
			})

			// The members of a JSON object are unordered, and a store may reorder them, like a PostgreSQL jsonb column,
			// which gives {"$type":"...","$map":{...}} back as {"$map":{...},"$type":"..."}.
			s.And("the $type is not the first member of the envelope", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"F":{"$map":` + entriesJSON(t) + `,"$type":"rm-counts"}}`)
				})

				s.Then("the entries are read into the field", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]int(counts.Get(t)), ptr.Get(t).(*struct{ F map[string]int }).F)
				})

				s.And("the field is of the registered map type", func(s *testcase.Spec) {
					ptr.Let(s, func(t *testcase.T) any {
						return new(struct{ F MT_RM_Counts })
					})

					s.Then("the entries are read into the field", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal(t, counts.Get(t), ptr.Get(t).(*struct{ F MT_RM_Counts }).F)
					})
				})

				s.And("the field is an `any`", func(s *testcase.Spec) {
					ptr.Let(s, func(t *testcase.T) any {
						return new(struct{ F any })
					})

					s.Then("it is read back as the registered map, as its $type tells", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal[any](t, counts.Get(t), ptr.Get(t).(*struct{ F any }).F)
					})
				})
			})

			s.And("the field is of the registered map type", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ F MT_RM_Counts })
				})

				s.Then("the entries are read into the field", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, counts.Get(t), ptr.Get(t).(*struct{ F MT_RM_Counts }).F)
				})
			})

			s.And("the field is a pointer to a plain map", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ F *map[string]int })
				})

				s.Then("the entries are read into the map it points to", func(t *testcase.T) {
					assert.NoError(t, act(t))
					got := ptr.Get(t).(*struct{ F *map[string]int }).F
					assert.NotNil(t, got)
					assert.Equal(t, map[string]int(counts.Get(t)), *got)
				})
			})

			s.And("the field is a struct with a field for each key", func(s *testcase.Spec) {
				counts.Let(s, func(t *testcase.T) MT_RM_Counts {
					return MT_RM_Counts{"A": t.Random.Int(), "B": t.Random.Int()}
				})
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ F MT_RM_AB })
				})

				s.Then("the entries are read into the fields of the struct", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, MT_RM_AB{A: counts.Get(t)["A"], B: counts.Get(t)["B"]}, ptr.Get(t).(*struct{ F MT_RM_AB }).F)
				})
			})

			// A json.RawMessage captures the JSON of a member to be read later, like into an `any`, which needs the $type.
			s.And("the field is a json.RawMessage", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ F json.RawMessage })
				})

				s.Then("it captures the JSON as it is, with the $type, like with encoding/json", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertJSONEqual(t, envelopeJSON(t), string(ptr.Get(t).(*struct{ F json.RawMessage }).F))
				})
			})
		})

		s.When("the registered map is an element of a slice of `any`, and the destination is a slice of plain maps", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				data, err := subject.Get(t).Marshal([]any{counts.Get(t)})
				assert.Must(t).NoError(err)
				return data
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new([]map[string]int)
			})

			s.Then("the entries are read into the element", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, []map[string]int{counts.Get(t)}, *ptr.Get(t).(*[]map[string]int))
			})
		})

		s.When("the registered map has entries keyed like the members of the envelope", func(s *testcase.Spec) {
			counts.Let(s, func(t *testcase.T) MT_RM_Counts {
				c := maps.Clone(counts.Super(t))
				c["$type"] = t.Random.Int()
				c["$value"] = t.Random.Int()
				c["$map"] = t.Random.Int()
				return c
			})

			s.Then("they are read back as entries", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
			})

			s.And("the destination is an `any`", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(any)
				})

				s.Then("they are read back as entries of the registered map", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, counts.Get(t), *ptr.Get(t).(*any))
				})
			})

			s.And("the destination is a plain map of the same shape", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(map[string]int)
				})

				s.Then("they are read into it as entries", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]int(counts.Get(t)), *ptr.Get(t).(*map[string]int))
				})
			})
		})

		// A map is a value too, so like the JSON of any other value, its entries can be the value member of the envelope,
		// which is where they are read from, when the envelope has no map member.
		s.When("the entries are in the $value of the envelope, instead of a $map", func(s *testcase.Spec) {
			var (
				typeMember  = let.VarOf(s, "$type")
				valueMember = let.VarOf(s, "$value")
			)
			// object is the JSON of an object with the given members.
			// The members of a JSON object are unordered, so they are written in a random order.
			object := func(t *testcase.T, members ...string) string {
				members = slices.Clone(members)
				random.Shuffle(t.Random, members)
				return `{` + strings.Join(members, `,`) + `}`
			}
			valueEnvelopeJSON := func(t *testcase.T) string {
				return object(t,
					mustMarshalJSON(t, typeMember.Get(t))+`:"rm-counts"`,
					mustMarshalJSON(t, valueMember.Get(t))+`:`+entriesJSON(t),
				)
			}
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(valueEnvelopeJSON(t))
			})

			s.Then("they are read back as the registered map", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
			})

			s.And("the destination is an `any`", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(any)
				})

				s.Then("they are read back as the registered map, as its $type tells", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal[any](t, counts.Get(t), *ptr.Get(t).(*any))
				})
			})

			s.And("the destination is a plain map of the same shape", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return new(map[string]int)
				})

				s.Then("they are read into it, the way encoding/json remaps a value into a DTO", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]int(counts.Get(t)), *ptr.Get(t).(*map[string]int))
				})
			})

			s.And("the destination is a struct with a field for each key", func(s *testcase.Spec) {
				counts.Let(s, func(t *testcase.T) MT_RM_Counts {
					return MT_RM_Counts{"A": t.Random.Int(), "B": t.Random.Int()}
				})
				ptr.Let(s, func(t *testcase.T) any {
					return new(MT_RM_AB)
				})

				s.Then("they are read into the fields", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, MT_RM_AB{A: counts.Get(t)["A"], B: counts.Get(t)["B"]}, *ptr.Get(t).(*MT_RM_AB))
				})
			})

			s.And("the envelope is held by a member, and the destination has a plain map field for it", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(`{"F":` + valueEnvelopeJSON(t) + `}`)
				})
				ptr.Let(s, func(t *testcase.T) any {
					return new(struct{ F map[string]int })
				})

				s.Then("they are read into the field", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, map[string]int(counts.Get(t)), ptr.Get(t).(*struct{ F map[string]int }).F)
				})
			})

			// The $map is where Marshal writes the entries of a map, so it is what tells them.
			s.And("the envelope has a $map as well", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) []byte {
					// The keys of other start with an x, which no hex key of counts does, so they can't be mistaken for the entries.
					other := map[string]int{"x" + t.Random.HexN(8): t.Random.Int()}
					return []byte(object(t,
						`"$type":"rm-counts"`,
						`"$value":`+mustMarshalJSON(t, other),
						`"$map":`+entriesJSON(t),
					))
				})

				s.Then("the entries are read from the $map", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
				})
			})

			s.And("the Codec names the type and the value member otherwise, like @type and @value", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					c.TypeKeyword, c.ValueKeyword = "@type", "@value"
					return c
				})
				typeMember.LetValue(s, "@type")
				valueMember.LetValue(s, "@value")

				s.Then("they are read back as the registered map", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
				})

				s.And("the destination is a plain map of the same shape", func(s *testcase.Spec) {
					ptr.Let(s, func(t *testcase.T) any {
						return new(map[string]int)
					})

					s.Then("they are read into it", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Equal(t, map[string]int(counts.Get(t)), *ptr.Get(t).(*map[string]int))
					})
				})
			})
		})

		s.When("the JSON is a plain object, without $type", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(entriesJSON(t))
			})

			s.Then("it is read as the registered map, like encoding/json reads it", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, counts.Get(t), *ptr.Get(t).(*MT_RM_Counts))
			})
		})

		// Like with encoding/json, an "$type" which names no type the Codec knows is just another member of the JSON object.
		s.When("the JSON object has a $type which names no type the Codec knows", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				unknown := mustMarshalJSON(t, "unknown-"+t.Random.HexN(8))
				return []byte(`{"$type":` + unknown + `,` + strings.TrimPrefix(entriesJSON(t), `{`))
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(map[string]any)
			})

			s.Then("the $type is read as an entry of the map, like encoding/json reads it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var exp map[string]any
				unmarshalPlainJSON(t, data.Get(t), &exp)
				assert.Equal(t, exp, *ptr.Get(t).(*map[string]any))
			})
		})
	})
}

// TestCodec_registeredOwnJSONUnmarshaler pins that a type registered on the Codec, which has its own JSON marshaler and unmarshaler,
// a json.Marshaler and a json.Unmarshaler, or a json.MarshalerTo and a json.UnmarshalerFrom of encoding/json/v2, which encoding/json calls too,
// is read back with its own unmarshaler, wherever the destination tells its type,
// like an `any` by the $type, or a field of the registered type itself.
// The Codec writes the $type envelope around what its own marshaler wrote, when an interface holds it,
// so its own unmarshaler gets what its own marshaler wrote, without the envelope.
func TestCodec_registeredOwnJSONUnmarshaler(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		var c jsonkit.Codec
		t.Defer(jsonkit.CodecRegisterTypeID[MT_OJ_String](&c, "oj-string"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_OJ_Object](&c, "oj-object"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_OJ_V2String](&c, "oj-v2-string"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_OJ_V2Object](&c, "oj-v2-object"))
		return &c
	})

	// Held by an interface, it is written in the $value form, like {"$type":"oj-string","$value":"own:..."}.
	s.Context("its MarshalJSON writes a JSON value which is not an object", func(s *testcase.Spec) {
		specRegisteredOwnJSONUnmarshaler(s, subject, func(t *testcase.T) MT_OJ_String {
			return MT_OJ_String{V: t.Random.String()}
		})
	}, testcase.Group("not an object"))

	// Held by an interface, it is written with the $type next to the members its MarshalJSON wrote, like {"$type":"oj-object","own":"..."}.
	s.Context("its MarshalJSON writes a JSON object", func(s *testcase.Spec) {
		specRegisteredOwnJSONUnmarshaler(s, subject, func(t *testcase.T) MT_OJ_Object {
			return MT_OJ_Object{V: t.Random.String()}
		})
	}, testcase.Group("an object"))

	s.Context("its MarshalJSONTo writes a JSON value which is not an object", func(s *testcase.Spec) {
		specRegisteredOwnJSONUnmarshaler(s, subject, func(t *testcase.T) MT_OJ_V2String {
			return MT_OJ_V2String{V: t.Random.String()}
		})
	}, testcase.Group("v2: not an object"))

	s.Context("its MarshalJSONTo writes a JSON object", func(s *testcase.Spec) {
		specRegisteredOwnJSONUnmarshaler(s, subject, func(t *testcase.T) MT_OJ_V2Object {
			return MT_OJ_V2Object{V: t.Random.String()}
		})
	}, testcase.Group("v2: an object"))
}

// specRegisteredOwnJSONUnmarshaler is the spec of TestCodec_registeredOwnJSONUnmarshaler
// for the registered type T, whose values makeValue makes.
func specRegisteredOwnJSONUnmarshaler[T any](s *testcase.Spec, subject testcase.Var[*jsonkit.Codec], makeValue func(*testcase.T) T) {
	value := let.Var(s, makeValue)

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		// data is what Marshal writes for an `any` field holding the value, which tells its type with the $type.
		data := let.Var(s, func(t *testcase.T) []byte {
			data, err := subject.Get(t).Marshal(struct{ F any }{F: value.Get(t)})
			assert.Must(t).NoError(err)
			return data
		})
		ptr := let.Var(s, func(t *testcase.T) any {
			return new(struct{ F any })
		})
		act := func(t *testcase.T) error {
			return subject.Get(t).Unmarshal(data.Get(t), ptr.Get(t))
		}

		s.Then("the `any` field is read back as the registered type, with its own unmarshaler", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal[any](t, value.Get(t), ptr.Get(t).(*struct{ F any }).F)
		})

		// Like encoding/json remaps a value into a DTO, whose field tells the type.
		s.When("the destination has a field of the registered type", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ F T })
			})

			s.Then("the field is read with its own unmarshaler, from what its own marshaler wrote", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), ptr.Get(t).(*struct{ F T }).F)
			})
		})

		s.When("the destination has a pointer field of the registered type", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ F *T })
			})

			s.Then("the value it points to is read with its own unmarshaler, from what its own marshaler wrote", func(t *testcase.T) {
				assert.NoError(t, act(t))
				got := ptr.Get(t).(*struct{ F *T }).F
				assert.NotNil(t, got)
				assert.Equal(t, value.Get(t), *got)
			})
		})

		s.When("the value is an element of a slice of `any`, and the destination is a slice of the registered type", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				data, err := subject.Get(t).Marshal([]any{value.Get(t)})
				assert.Must(t).NoError(err)
				return data
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new([]T)
			})

			s.Then("the element is read with its own unmarshaler, from what its own marshaler wrote", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, []T{value.Get(t)}, *ptr.Get(t).(*[]T))
			})
		})

		s.When("the JSON is the envelope a jsonkit.Interface writes, and the destination is the registered type itself", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				data, err := subject.Get(t).Marshal(jsonkit.Interface[any]{V: value.Get(t)})
				assert.Must(t).NoError(err)
				return data
			})
			ptr.Let(s, func(t *testcase.T) any {
				return new(T)
			})

			s.Then("it is read with its own unmarshaler, from what its own marshaler wrote", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), *ptr.Get(t).(*T))
			})
		})

		// A json.RawMessage captures the JSON of a member to be read later, like into an `any`, which needs the $type.
		s.When("the destination has a json.RawMessage field", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return new(struct{ F json.RawMessage })
			})

			s.Then("it captures the JSON as it is, with the $type, like with encoding/json", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var exp struct{ F json.RawMessage }
				assert.Must(t).NoError(json.Unmarshal(data.Get(t), &exp))
				assert.Equal(t, string(exp.F), string(ptr.Get(t).(*struct{ F json.RawMessage }).F))
			})
		})

		// Like with encoding/json, nothing tells the type of a value in an `any`, without its $type,
		// so it is written as plain JSON, as a best effort.
		s.When("the type is not registered on the Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				return &jsonkit.Codec{}
			})

			s.Then("the `any` field is read back the way encoding/json reads it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var exp struct{ F any }
				assert.Must(t).NoError(json.Unmarshal(data.Get(t), &exp))
				assert.Equal(t, exp, *ptr.Get(t).(*struct{ F any }))
			})
		})
	})
}

// MT_OJ_String has its own json.Marshaler and json.Unmarshaler, which write it as a JSON string, prefixed by "own:",
// so it can only be read back with its own UnmarshalJSON, which reads nothing but what its MarshalJSON writes.
type MT_OJ_String struct{ V string }

func (v MT_OJ_String) MarshalJSON() ([]byte, error) {
	return json.Marshal("own:" + v.V)
}

func (v *MT_OJ_String) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	text, ok := strings.CutPrefix(s, "own:")
	if !ok {
		return fmt.Errorf("MT_OJ_String: %q is not what its MarshalJSON writes", s)
	}
	v.V = text
	return nil
}

// MT_OJ_Object has its own json.Marshaler and json.Unmarshaler, which write it as a JSON object with an "own" member,
// so it can only be read back with its own UnmarshalJSON, which reads nothing but the members its MarshalJSON writes,
// so it tells whether it got the $type, which the Codec writes next to them.
type MT_OJ_Object struct{ V string }

func (v MT_OJ_Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"own": v.V})
}

func (v *MT_OJ_Object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var dto struct {
		Own string `json:"own"`
	}
	if err := dec.Decode(&dto); err != nil {
		return fmt.Errorf("MT_OJ_Object: %w", err)
	}
	v.V = dto.Own
	return nil
}

// MT_OJ_V2String is MT_OJ_String with the methods of encoding/json/v2, a json.MarshalerTo and a json.UnmarshalerFrom,
// which encoding/json calls too, instead of a json.Marshaler and a json.Unmarshaler.
type MT_OJ_V2String struct{ V string }

func (v MT_OJ_V2String) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(jsontext.String("own:" + v.V))
}

func (v *MT_OJ_V2String) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	text, ok := strings.CutPrefix(tok.String(), "own:")
	if tok.Kind() != '"' || !ok {
		return fmt.Errorf("MT_OJ_V2String: %s is not what its MarshalJSONTo writes", tok)
	}
	v.V = text
	return nil
}

// MT_OJ_V2Object is MT_OJ_Object with the methods of encoding/json/v2, a json.MarshalerTo and a json.UnmarshalerFrom,
// which encoding/json calls too, instead of a json.Marshaler and a json.Unmarshaler.
// Its UnmarshalJSONFrom reads the members on its own, so the options of the decoder it gets don't tell what it reads.
type MT_OJ_V2Object struct{ V string }

func (v MT_OJ_V2Object) MarshalJSONTo(enc *jsontext.Encoder) error {
	return jsonv2.MarshalEncode(enc, map[string]string{"own": v.V})
}

func (v *MT_OJ_V2Object) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	var dto struct {
		Own string `json:"own"`
	}
	if err := jsonv2.Unmarshal(raw, &dto, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("MT_OJ_V2Object: %w", err)
	}
	v.V = dto.Own
	return nil
}

// TestCodec_customCodecReceivesTheCodecOfTheCall pins the rule that a custom codec gets the Codec
// whose Marshal or Unmarshal call writes or reads the value, wherever the value sits,
// so the custom codec can write and read the values it holds with the registrations of that Codec.
func TestCodec_customCodecReceivesTheCodecOfTheCall(t *testing.T) {
	s := testcase.NewSpec(t)

	type MT_Inner struct{ Marker string }
	type MT_Outer struct {
		Name  string
		Inner MT_Inner
	}

	// The custom codec of MT_Outer writes and reads its MT_Inner with the Codec it gets,
	// which writes and reads it with the custom codec of MT_Inner only when it is the Codec of the call.
	s.Test("the custom codec of a value writes and reads the values it holds with the Codec of the call", func(t *testcase.T) {
		var c jsonkit.Codec
		defer jsonkit.CodecRegister[MT_Inner](&c, "inner",
			jsonkit.TypeCodec[MT_Inner]{
				MarshalFunc: func(_ *jsonkit.Codec, v MT_Inner) ([]byte, error) {
					return json.Marshal(map[string]string{"inner_marker": v.Marker})
				},
				UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_Inner) error {
					// The key doesn't match the name of the Marker field, even case-insensitively,
					// so a Codec without this registration couldn't read it back.
					var dto struct {
						Marker string `json:"inner_marker"`
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
				MarshalFunc: func(supplied *jsonkit.Codec, v MT_Outer) ([]byte, error) {
					innerBytes, err := supplied.Marshal(v.Inner)
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
						return err
					}
					p.Name = dto.Name
					p.Inner = inner
					return nil
				},
			},
		)()

		exp := MT_Outer{Name: t.Random.HexN(8), Inner: MT_Inner{Marker: t.Random.HexN(8)}}

		// The wire is the one of the custom codecs, of the outer and of the nested value.
		// As they are not held by an interface, they are written without a $type.
		assertWireFormat(t, &c, exp, mustMarshalJSON(t, map[string]any{
			"name":  exp.Name,
			"inner": map[string]string{"inner_marker": exp.Inner.Marker},
		}))
	})

	// MT_Marker has a custom codec on the Codec of a test, see registerInner, which writes it with a "marker:" prefix,
	// so a Codec without that registration would read the prefix back as part of it.
	type MT_Marker string
	type MT_InnerDTO struct {
		Marker MT_Marker `json:"marker"`
	}

	// registerInner registers the custom codecs of MT_Marker and MT_Inner on c.
	// The custom codec of MT_Inner writes and reads its marker as an MT_Marker, with the Codec it gets,
	// so the marker is written with its prefix, and read back without it, only when it gets c.
	registerInner := func(t *testcase.T, c *jsonkit.Codec) {
		t.Defer(jsonkit.CodecRegister[MT_Marker](c, "marker", jsonkit.TypeCodec[MT_Marker]{
			MarshalFunc: func(_ *jsonkit.Codec, v MT_Marker) ([]byte, error) {
				return json.Marshal("marker:" + string(v))
			},
			UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, p *MT_Marker) error {
				var text string
				if err := json.Unmarshal(data, &text); err != nil {
					return err
				}
				*p = MT_Marker(strings.TrimPrefix(text, "marker:"))
				return nil
			},
		}))
		t.Defer(jsonkit.CodecRegister[MT_Inner](c, "inner", jsonkit.TypeCodec[MT_Inner]{
			MarshalFunc: func(supplied *jsonkit.Codec, v MT_Inner) ([]byte, error) {
				return supplied.Marshal(MT_InnerDTO{Marker: MT_Marker(v.Marker)})
			},
			UnmarshalFunc: func(supplied *jsonkit.Codec, data []byte, p *MT_Inner) error {
				var dto MT_InnerDTO
				if err := supplied.Unmarshal(data, &dto); err != nil {
					return err
				}
				p.Marker = string(dto.Marker)
				return nil
			},
		}))
	}
	// innerMembers are the members of the JSON object of an MT_Inner with the marker, which registerInner registers the custom codec of.
	innerMembers := func(t *testcase.T, marker string) string {
		return `"marker":` + mustMarshalJSON(t, "marker:"+marker)
	}

	s.Test("the custom codec of a slice element gets the Codec of the call", func(t *testcase.T) {
		var c jsonkit.Codec
		registerInner(t, &c)

		exp := []MT_Inner{{Marker: t.Random.HexN(8)}, {Marker: t.Random.HexN(8)}}
		assertWireFormat(t, &c, exp, `[{`+innerMembers(t, exp[0].Marker)+`},{`+innerMembers(t, exp[1].Marker)+`}]`)
	})

	// The element type is known from the registered container, so the elements are written without a $type.
	s.Test("the custom codec of an element of a registered container, which an interface holds, gets the Codec of the call", func(t *testcase.T) {
		type MT_Inners []MT_Inner
		var c jsonkit.Codec
		registerInner(t, &c)
		t.Defer(jsonkit.CodecRegisterTypeID[MT_Inners](&c, "inners"))

		marker := t.Random.HexN(8)
		assertWireFormat(t, &c, struct{ V any }{V: MT_Inners{{Marker: marker}}},
			`{"V":{"$type":"inners","$value":[{`+innerMembers(t, marker)+`}]}}`)
	})

	// An `any` holds the elements of a []any and the values of a map[string]any, so they are written with their $type.
	s.Test("the custom codec of a value in a []any or a map[string]any, which an interface holds, gets the Codec of the call", func(t *testcase.T) {
		var c jsonkit.Codec
		registerInner(t, &c)

		elem, value, key := t.Random.HexN(8), t.Random.HexN(8), t.Random.HexN(4)
		assertWireFormat(t, &c, struct{ V any }{V: []any{
			MT_Inner{Marker: elem},
			map[string]any{key: MT_Inner{Marker: value}},
		}}, `{"V":[`+
			`{"$type":"inner",`+innerMembers(t, elem)+`},`+
			`{`+mustMarshalJSON(t, key)+`:{"$type":"inner",`+innerMembers(t, value)+`}}]}`)
	})
}

// MT_TC_Item is the element type of the slices and maps in TestCodec_typedContainers.
type MT_TC_Item struct{ V string }

// MT_TC_Items is a named slice type of MT_TC_Item.
type MT_TC_Items []MT_TC_Item

// MT_TC_Unregistered is a struct type which is not registered with the Codec.
type MT_TC_Unregistered struct{ V string }

// MT_TC_Box holds its value in an interface field.
type MT_TC_Box struct{ V any }

// A slice or a map held by an interface, like by an `any` field, carries a $type,
// which is derived from the type IDs of its elements, the way Go spells its type, like "[]item" for a []Item,
// so it is read back as the very same slice or map type, instead of as a []any or a map[string]any.
func TestCodec_typedContainers(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		c := &jsonkit.Codec{}
		t.Defer(jsonkit.CodecRegisterTypeID[MT_TC_Item](c, "tc-item", "tc-item-alias"))
		return c
	})

	makeItem := func(t *testcase.T) MT_TC_Item {
		return MT_TC_Item{V: t.Random.String()}
	}

	// registerGreeter registers the Greeter interface type with the Codec,
	// which names it as the element type of a []Greeter, as no value is of an interface type.
	registerGreeter := func(s *testcase.Spec) {
		subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
			c := subject.Super(t)
			t.Defer(jsonkit.CodecRegisterTypeID[Greeter](c, "tc-greeter"))
			return c
		})
	}

	s.Describe("#Marshal of a slice, an array or a map held by an interface", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return []MT_TC_Item{makeItem(t), makeItem(t)}
		})

		act := func(t *testcase.T) ([]byte, error) {
			return subject.Get(t).Marshal(MT_TC_Box{V: value.Get(t)})
		}

		thenItIsReadBackAsTheSameType := func(s *testcase.Spec) {
			s.Then("it is read back as the very same type", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var got MT_TC_Box
				assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
				assert.Equal(t, value.Get(t), got.V)
			})
		}

		thenItIsWrittenAsPlainJSON := func(s *testcase.Spec) {
			s.Then("it is written as plain JSON, without a $type", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"V":`+mustMarshalJSON(t, value.Get(t))+`}`, string(data))
			})
		}

		thenItIsReadBackTheWayEncodingJSONReadsIt := func(s *testcase.Spec) {
			s.Then("it is read back the way encoding/json reads it into an any, except that a whole number is an int", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var got MT_TC_Box
				assert.NoError(t, subject.Get(t).Unmarshal(data, &got))

				var exp MT_TC_Box
				unmarshalPlainJSON(t, []byte(mustMarshalJSON(t, MT_TC_Box{V: value.Get(t)})), &exp)
				assert.Equal(t, exp, got)
			})
		}

		s.Then("it is written with the $type derived from the type ID of its elements", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, `{"V":{"$type":"[]tc-item","$value":`+mustMarshalJSON(t, value.Get(t))+`}}`, string(data))
		})

		thenItIsReadBackAsTheSameType(s)

		s.When("its elements are of an interface type registered with the Codec", func(s *testcase.Spec) {
			registerGreeter(s)
			value.Let(s, func(t *testcase.T) any {
				return []Greeter{TypeA{V: t.Random.String()}, TypeB{V: t.Random.Int()}}
			})

			s.Then("it is written with the type ID of the interface type, and its elements with theirs", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				greeters := value.Get(t).([]Greeter)
				assertJSONEqual(t, `{"V":{"$type":"[]tc-greeter","$value":[`+
					`{"$type":"type_a","V":`+mustMarshalJSON(t, greeters[0].(TypeA).V)+`},`+
					`{"$type":"type_b","V":`+mustMarshalJSON(t, greeters[1].(TypeB).V)+`}]}}`, string(data))
			})

			thenItIsReadBackAsTheSameType(s)
		})

		s.When("it is a slice of slices", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return [][]MT_TC_Item{{makeItem(t)}, {makeItem(t), makeItem(t)}}
			})

			thenItIsReadBackAsTheSameType(s)
		})

		s.When("it is a map with string keys", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]MT_TC_Item{t.Random.HexN(4): makeItem(t), t.Random.HexN(5): makeItem(t)}
			})

			// The $map keeps the entries of the map apart from the members of the envelope.
			s.Then("it is written with the $type derived from the type ID of its values, around its entries", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"V":{"$type":"map[string]tc-item","$map":`+mustMarshalJSON(t, value.Get(t))+`}}`, string(data))
			})

			thenItIsReadBackAsTheSameType(s)

			s.And("it has entries keyed like the members of the type envelope", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return map[string]MT_TC_Item{"$type": makeItem(t), "$value": makeItem(t), "$map": makeItem(t), t.Random.HexN(4): makeItem(t)}
				})

				thenItIsReadBackAsTheSameType(s)
			})
		})

		s.When("it is a map of slices", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string][]MT_TC_Item{t.Random.HexN(4): {makeItem(t), makeItem(t)}}
			})

			thenItIsReadBackAsTheSameType(s)
		})

		// Like an int64 held by an interface, which keeps its $type, as its plain JSON would be read back as an int.
		// A plain JSON array would be read back as a []any.
		s.When("its elements are of a built-in type", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []int{t.Random.Int(), t.Random.Int()}
			})

			s.Then("it is written with the $type derived from the type ID of the built-in type", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, `{"V":{"$type":"[]int","$value":`+mustMarshalJSON(t, value.Get(t))+`}}`, string(data))
			})

			thenItIsReadBackAsTheSameType(s)
		})

		// encoding/json writes a []byte as a base64 JSON string, which it reads back into an `any` as a string.
		s.When("it is a []byte", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []byte(t.Random.String())
			})

			thenItIsReadBackAsTheSameType(s)
		})

		s.When("its element type is not registered with the Codec", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []MT_TC_Unregistered{{V: t.Random.String()}}
			})

			thenItIsWrittenAsPlainJSON(s)

			s.Then("it is read back the way encoding/json reads it into an any", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var got MT_TC_Box
				assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
				v := value.Get(t).([]MT_TC_Unregistered)[0].V
				assert.Equal[any](t, []any{map[string]any{"V": v}}, got.V)
			})
		})

		// A pointer has no type ID, so the type of the elements can't be told.
		s.When("its elements are pointers", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				item := makeItem(t)
				return []*MT_TC_Item{&item}
			})

			thenItIsWrittenAsPlainJSON(s)
		})

		// The Codec reads a plain JSON array and object into an `any` as these, so they need no $type.
		s.When("it is a []any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []any{t.Random.String(), makeSignedInt(t), makeFloat64WithFraction(t)}
			})

			thenItIsWrittenAsPlainJSON(s)
			thenItIsReadBackAsTheSameType(s)

			s.And("it holds a float64 of a whole number", func(s *testcase.Spec) {
				whole := let.Var(s, makeWholeFloat64)
				value.Let(s, func(t *testcase.T) any {
					return []any{t.Random.String(), whole.Get(t)}
				})

				s.Then("the float64 is written with a .0 fraction", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					str := value.Get(t).([]any)[0]
					assertJSONEqual(t, `{"V":[`+mustMarshalJSON(t, str)+`,`+mustMarshalJSON(t, whole.Get(t))+`.0]}`, string(data))
				})

				thenItIsReadBackAsTheSameType(s)
			})
		})

		s.When("it is a map[string]any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any{t.Random.HexN(4): t.Random.String()}
			})

			thenItIsWrittenAsPlainJSON(s)
			thenItIsReadBackAsTheSameType(s)
		})

		s.When("the empty interface is registered with the Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[any](c, "tc-any"))
				return c
			})
			value.Let(s, func(t *testcase.T) any {
				return []any{t.Random.String(), map[string]any{t.Random.HexN(4): t.Random.String()}}
			})

			// A []any and a map[string]any are read back from plain JSON as the very same type.
			thenItIsWrittenAsPlainJSON(s)
		})

		s.When("another type is registered with the type ID derived for it", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				t.Defer(jsonkit.CodecRegisterTypeID[MT_TC_Items](c, "[]tc-item"))
				return c
			})

			// It is not written with the derived $type, as that would be read back as the registered type.
			thenItIsWrittenAsPlainJSON(s)
		})

		s.When("it is nested deeper than the Codec derives type IDs for", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				v := reflect.ValueOf([]int{t.Random.Int()})
				for range 63 {
					outer := reflect.MakeSlice(reflect.SliceOf(v.Type()), 1, 1)
					outer.Index(0).Set(v)
					v = outer
				}
				return v.Interface()
			})

			thenItIsWrittenAsPlainJSON(s)
		})

		// An array has no derived type ID, unlike a slice, as its type ID would hold its length:
		// the Codec builds the types it reads with reflect, and these stay in memory,
		// so any JSON could make it build new types without a limit, or allocate an array of any size.
		s.When("it is an array", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return [2]int{t.Random.Int(), t.Random.Int()}
			})

			thenItIsWrittenAsPlainJSON(s)

			thenItIsReadBackTheWayEncodingJSONReadsIt(s)

			s.And("its elements are of a registered type", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return [2]MT_TC_Item{makeItem(t), makeItem(t)}
				})

				thenItIsWrittenAsPlainJSON(s)

				thenItIsReadBackTheWayEncodingJSONReadsIt(s)
			})

			s.And("it is held through a pointer", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return &[2]int{t.Random.Int(), t.Random.Int()}
				})

				thenItIsWrittenAsPlainJSON(s)

				thenItIsReadBackTheWayEncodingJSONReadsIt(s)
			})

			s.And("its elements are held by an interface", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return [2]Greeter{TypeA{V: t.Random.String()}, TypeB{V: t.Random.Int()}}
				})

				s.Then("it is read back as a []any, whose elements keep their own type", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)

					var got MT_TC_Box
					assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
					greeters := value.Get(t).([2]Greeter)
					assert.Equal[any](t, []any{greeters[0], greeters[1]}, got.V)
				})
			})

			s.And("its type is registered with the Codec", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegisterTypeID[[2]int](c, "tc-pair"))
					return c
				})

				s.Then("it is written with the $type of the registration", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, `{"V":{"$type":"tc-pair","$value":`+mustMarshalJSON(t, value.Get(t))+`}}`, string(data))
				})

				thenItIsReadBackAsTheSameType(s)
			})
		})
	})

	// The caller tells the type of the top-level value, by the destination it gives to Unmarshal,
	// so like with encoding/json, a slice or a map is written as a plain JSON array or object.
	s.Describe("#Marshal of a slice as the top-level value", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) []MT_TC_Item {
			return []MT_TC_Item{makeItem(t), makeItem(t)}
		})

		act := func(t *testcase.T) ([]byte, error) {
			return subject.Get(t).Marshal(value.Get(t))
		}

		s.Then("it is written as a plain JSON array, like encoding/json writes it", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, mustMarshalJSON(t, value.Get(t)), string(data))
		})

		s.Then("it is read back into the type the caller gives", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)

			var got []MT_TC_Item
			assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
			assert.Equal(t, value.Get(t), got)
		})
	})

	s.Describe("#Marshal of a jsonkit.Interface holding a slice", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) []MT_TC_Item {
			return []MT_TC_Item{makeItem(t), makeItem(t)}
		})

		act := func(t *testcase.T) ([]byte, error) {
			return subject.Get(t).Marshal(jsonkit.Interface[any]{V: value.Get(t)})
		}

		s.Then("it is written with its $type, as the Interface holds it", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, `{"$type":"[]tc-item","$value":`+mustMarshalJSON(t, value.Get(t))+`}`, string(data))
		})

		s.Then("it is read back as the very same type, into an Interface, an any or the slice type itself", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)

			var gotInterface jsonkit.Interface[any]
			assert.NoError(t, subject.Get(t).Unmarshal(data, &gotInterface))
			assert.Equal[any](t, value.Get(t), gotInterface.V)

			var gotAny any
			assert.NoError(t, subject.Get(t).Unmarshal(data, &gotAny))
			assert.Equal[any](t, value.Get(t), gotAny)

			var gotSlice []MT_TC_Item
			assert.NoError(t, subject.Get(t).Unmarshal(data, &gotSlice))
			assert.Equal(t, value.Get(t), gotSlice)
		})
	})

	s.Describe("#Unmarshal of a slice or a map into an interface", func(s *testcase.Spec) {
		item := let.Var(s, makeItem)

		data := let.Var(s, func(t *testcase.T) []byte {
			return []byte(`{"V":{"$type":"[]tc-item","$value":[` + mustMarshalJSON(t, item.Get(t)) + `]}}`)
		})

		act := func(t *testcase.T) (any, error) {
			var got MT_TC_Box
			err := subject.Get(t).Unmarshal(data.Get(t), &got)
			return got.V, err
		}

		thenItFails := func(s *testcase.Spec) {
			s.Then("it fails with ErrUnknownTypeID, as the $type names no type of a value", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
			})
		}

		s.Then("the value is read as the type its $type names", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal[any](t, []MT_TC_Item{item.Get(t)}, got)
		})

		s.When("the element type is named by an alias of its type ID", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"map[string][]tc-item-alias","$map":{"k":[` + mustMarshalJSON(t, item.Get(t)) + `]}}}`)
			})

			s.Then("the value is read as the type which has the type ID of the alias", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, map[string][]MT_TC_Item{"k": {item.Get(t)}}, got)
			})
		})

		s.When("the data is a plain JSON array", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":[` + mustMarshalJSON(t, item.Get(t)) + `]}`)
			})

			s.Then("the value is read the way encoding/json reads it into an any", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, []any{map[string]any{"V": item.Get(t).V}}, got)
			})
		})

		s.When("the element type ID is not known by the Codec", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"[]tc-unknown","$value":[]}}`)
			})

			thenItFails(s)
		})

		s.When("the key type of the map is not a string", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"map[int]tc-item","$map":{}}}`)
			})

			thenItFails(s)
		})

		// The Codec doesn't build types without a limit for any $type it is given.
		s.When("the type ID is nested deeper than the Codec reads", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"` + strings.Repeat("[]", 64) + `tc-item","$value":[]}}`)
			})

			thenItFails(s)
		})

		s.When("the type ID names an interface type", func(s *testcase.Spec) {
			registerGreeter(s)
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"tc-greeter","V":` + mustMarshalJSON(t, t.Random.String()) + `}}`)
			})

			thenItFails(s)
		})

		s.When("the type ID is the Go name of the interface, and the envelope has no $value", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"V":{"$type":"interface {}","V":` + mustMarshalJSON(t, t.Random.String()) + `}}`)
			})

			s.Then("it fails, as the interface type is the type of no value, so the envelope needs a $value", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)
			})
		})
	})
}

func TestDataTransferObjectMapping_smoke(t *testing.T) {
	type Entity struct {
		Number int64
	}

	type EntityJSON struct {
		N string `json:"num"`
	}

	var c jsonkit.Codec
	t.Cleanup(jsonkit.CodecRegister(&c, "typeid", jsonkit.DataTransferObjectMapping[Entity, EntityJSON]{
		ToENT: func(ctx context.Context, dto EntityJSON) (Entity, error) {
			n, err := strconv.ParseInt(dto.N, 10, 64)
			return Entity{Number: n}, err
		},
		ToDTO: func(ctx context.Context, ent Entity) (EntityJSON, error) {
			return EntityJSON{N: strconv.FormatInt(ent.Number, 10)}, nil
		},
	}))

	var v Entity = Entity{Number: 42}

	// The value is written as its DTO, and as it is not held by an interface, without a $type.
	data, err := c.Marshal(v)
	assert.NoError(t, err)
	assertJSONEqual(t, `{"num":"42"}`, string(data))

	var got Entity
	assert.NoError(t, c.Unmarshal(data, &got))
	assert.Equal(t, v, got)
}

// MT_EM_Struct, MT_EM_Slice, MT_EM_Map and MT_EM_Scalar are the registered types of TestCodec_envelopeMembers,
// one for each shape of the type envelope.
type (
	MT_EM_Struct struct{ Name string }
	MT_EM_Slice  []string
	MT_EM_Map    map[string]int
	MT_EM_Scalar string
)

// MT_EM_LinkedData is a struct with a member named @type of its own, like a JSON-LD document.
type MT_EM_LinkedData struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

// strictTypeCodec is a custom codec which reads only the members of T,
// so it fails on any other member, like a member of the type envelope.
func strictTypeCodec[T any]() jsonkit.TypeCodec[T] {
	return jsonkit.TypeCodec[T]{
		MarshalFunc: func(c *jsonkit.Codec, v T) ([]byte, error) {
			return json.Marshal(v)
		},
		UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *T) error {
			dec := json.NewDecoder(bytes.NewReader(data))
			dec.DisallowUnknownFields()
			return dec.Decode(p)
		},
	}
}

// TestCodec_envelopeMembers pins the names of the members of the type envelope,
// which tells the type of a value where an interface holds it.
// The type member names the type, the map member holds the entries of a map,
// and the value member holds any other value, whose JSON is not an object.
// They are $type, $map and $value, unless the Codec names them otherwise, with its TypeKeyword, MapKeyword and ValueKeyword.
// Their names are matched exactly, so a member named otherwise, even if only in letter case, like an @type or a $TYPE,
// is just a member of the value.
func TestCodec_envelopeMembers(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		// typeKeyword, valueKeyword and mapKeyword are the keywords of the Codec, where the zero value stands for the default one.
		typeKeyword  = let.VarOf(s, "")
		valueKeyword = let.VarOf(s, "")
		mapKeyword   = let.VarOf(s, "")
	)
	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		c := &jsonkit.Codec{TypeKeyword: typeKeyword.Get(t), ValueKeyword: valueKeyword.Get(t), MapKeyword: mapKeyword.Get(t)}
		t.Defer(jsonkit.CodecRegisterTypeID[MT_EM_Struct](c, "em:struct"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_EM_Slice](c, "em:slice"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_EM_Map](c, "em:map"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_EM_Scalar](c, "em:scalar"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_EM_LinkedData](c, "em:linked-data"))
		return c
	})

	var (
		// typeMember, valueMember and mapMember are the names of the members of the envelope in the JSON.
		typeMember = let.Var(s, func(t *testcase.T) string {
			return cmp.Or(typeKeyword.Get(t), "$type")
		})
		valueMember = let.Var(s, func(t *testcase.T) string {
			return cmp.Or(valueKeyword.Get(t), "$value")
		})
		mapMember = let.Var(s, func(t *testcase.T) string {
			return cmp.Or(mapKeyword.Get(t), "$map")
		})
		name = let.Var(s, func(t *testcase.T) string {
			return t.Random.HexN(8)
		})
		items = let.Var(s, func(t *testcase.T) []string {
			return []string{t.Random.HexN(4), t.Random.HexN(4)}
		})
		entries = let.Var(s, func(t *testcase.T) map[string]int {
			return map[string]int{t.Random.HexN(4): t.Random.Int()}
		})
		ldType = let.Var(s, func(t *testcase.T) string {
			return "schema:" + t.Random.HexN(4)
		})
	)

	// member is the JSON of an object member, with the given name and value.
	member := func(t *testcase.T, name string, v any) string {
		return mustMarshalJSON(t, name) + `:` + mustMarshalJSON(t, v)
	}

	// object is the JSON of an object with the given members.
	// The members of a JSON object are unordered, so they are written in a random order,
	// the way a store which reorders them, like a PostgreSQL jsonb column, gives them back.
	object := func(t *testcase.T, members ...string) string {
		members = slices.Clone(members)
		random.Shuffle(t.Random, members)
		return `{` + strings.Join(members, `,`) + `}`
	}

	whenTheCodecHasKeywordsOfItsOwn := func(s *testcase.Spec, spec func(s *testcase.Spec)) {
		s.When("the Codec names the members of the envelope with keywords of its own", func(s *testcase.Spec) {
			keywords := let.Var(s, func(t *testcase.T) [3]string {
				return random.Pick(t.Random,
					[3]string{"@kind", "@data", "@entries"},
					[3]string{"_type", "_value", "_map"},
					[3]string{"@__type__", "@__value__", "@__map__"},
					[3]string{"kind", "value", "entries"},
				)
			})
			typeKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[0] })
			valueKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[1] })
			mapKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[2] })

			spec(s)
		})
	}

	// whenTheKeywordsAreTheSame arranges a Codec where two of the TypeKeyword, the ValueKeyword and the MapKeyword name the same member,
	// so the members of the envelope couldn't be told apart.
	whenTheKeywordsAreTheSame := func(s *testcase.Spec, spec func(s *testcase.Spec)) {
		s.When("two keywords of the Codec are the same", func(s *testcase.Spec) {
			// An empty keyword stands for the default one, so it is the same as another keyword set to that default.
			keywords := let.Var(s, func(t *testcase.T) [3]string {
				return random.Pick(t.Random,
					[3]string{"", "$type", ""},
					[3]string{"$value", "", ""},
					[3]string{"@kind", "@kind", ""},
					[3]string{"", "", "$type"},
					[3]string{"$map", "", ""},
					[3]string{"@kind", "", "@kind"},
					[3]string{"", "$map", ""},
					[3]string{"", "", "$value"},
					[3]string{"", "@data", "@data"},
				)
			})
			typeKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[0] })
			valueKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[1] })
			mapKeyword.Let(s, func(t *testcase.T) string { return keywords.Get(t)[2] })

			spec(s)
		})
	}

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return MT_EM_Struct{Name: name.Get(t)}
		})
		act := func(t *testcase.T) (string, error) {
			v := value.Get(t)
			data, err := subject.Get(t).Marshal(&v)
			return string(data), err
		}

		thenItIsWrittenInTheShapeOfItsEnvelope := func(s *testcase.Spec) {
			s.Then("a struct is written flat, with its type member next to its own members", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:struct"), member(t, "Name", name.Get(t))), data)
			})

			s.When("the value is a slice", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return MT_EM_Slice(items.Get(t))
				})

				s.Then("it is written as the value member of the envelope, next to the type member", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:slice"), member(t, valueMember.Get(t), items.Get(t))), data)
				})
			})

			// The keys of the entries are data, which may be named like any member of the envelope, so they have a member of their own.
			s.When("the value is a map", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return MT_EM_Map(entries.Get(t))
				})

				s.Then("its entries are written as the map member of the envelope, next to the type member", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:map"), member(t, mapMember.Get(t), entries.Get(t))), data)
				})

				s.And("the map is nil", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return MT_EM_Map(nil)
					})

					s.Then("the map member of the envelope is null", func(t *testcase.T) {
						data, err := act(t)
						assert.NoError(t, err)
						assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:map"), member(t, mapMember.Get(t), nil)), data)
					})
				})
			})

			s.When("the value is a string", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return MT_EM_Scalar(name.Get(t))
				})

				s.Then("it is written as the value member of the envelope, next to the type member", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:scalar"), member(t, valueMember.Get(t), name.Get(t))), data)
				})
			})

			s.When("the struct has a member named @type of its own, like a JSON-LD document", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return MT_EM_LinkedData{Type: ldType.Get(t), Name: name.Get(t)}
				})

				s.Then("its @type is written as its own member, next to the type member", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)
					assertJSONEqual(t, object(t, member(t, typeMember.Get(t), "em:linked-data"), member(t, "@type", ldType.Get(t)), member(t, "name", name.Get(t))), data)
				})
			})
		}

		thenItIsWrittenInTheShapeOfItsEnvelope(s)

		whenTheCodecHasKeywordsOfItsOwn(s, thenItIsWrittenInTheShapeOfItsEnvelope)

		whenTheKeywordsAreTheSame(s, func(s *testcase.Spec) {
			s.Then("it fails with ErrKeywordCollision", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrKeywordCollision)
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			data = let.Var(s, func(t *testcase.T) []byte {
				return []byte(object(t, member(t, typeMember.Get(t), "em:struct"), member(t, "Name", name.Get(t))))
			})
			ptr = let.Var(s, func(t *testcase.T) any {
				return new(any)
			})
			expected = let.Var(s, func(t *testcase.T) any {
				return MT_EM_Struct{Name: name.Get(t)}
			})
		)
		act := func(t *testcase.T) (any, error) {
			err := subject.Get(t).Unmarshal(data.Get(t), ptr.Get(t))
			return reflect.ValueOf(ptr.Get(t)).Elem().Interface(), err
		}

		// envelope is the JSON of the type envelope, whose member of the given name holds the JSON of v.
		envelope := func(t *testcase.T, typeID jsonkit.TypeID, memberName string, v any) []byte {
			return []byte(object(t, member(t, typeMember.Get(t), typeID), member(t, memberName, v)))
		}

		thenItIsReadAsTheTypeTheEnvelopeNames := func(s *testcase.Spec) {
			s.Then("it is read as the type the envelope names", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, expected.Get(t), got)
			})

			s.And("the destination is that type itself, instead of an `any`", func(s *testcase.Spec) {
				ptr.Let(s, func(t *testcase.T) any {
					return reflect.New(reflect.TypeOf(expected.Get(t))).Interface()
				})

				s.Then("it is read the same way", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, expected.Get(t), got)
				})
			})

			// The type member has done its job by telling the type, so the custom codec gets the value without it,
			// and only it, so the members of the value, like the @type of a JSON-LD document, stay.
			s.And("the type has a custom codec, which reads only the members of the value", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					t.Defer(jsonkit.CodecRegister(c, "em:struct", strictTypeCodec[MT_EM_Struct]()))
					t.Defer(jsonkit.CodecRegister(c, "em:slice", strictTypeCodec[MT_EM_Slice]()))
					t.Defer(jsonkit.CodecRegister(c, "em:map", strictTypeCodec[MT_EM_Map]()))
					t.Defer(jsonkit.CodecRegister(c, "em:scalar", strictTypeCodec[MT_EM_Scalar]()))
					t.Defer(jsonkit.CodecRegister(c, "em:linked-data", strictTypeCodec[MT_EM_LinkedData]()))
					return c
				})
				ptr.Let(s, func(t *testcase.T) any {
					// The custom codec reads it, whether the destination is an `any` or the type itself.
					return random.Pick(t.Random, ptr.Super(t), reflect.New(reflect.TypeOf(expected.Get(t))).Interface())
				})

				s.Then("the custom codec reads it, without the members of the envelope", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, expected.Get(t), got)
				})
			})
		}

		// The values whose JSON is not an object, which could have the type member next to its own members,
		// so they are the value member of the envelope, and the maps, whose entries are the map member of the envelope.
		whenTheEnvelopeHoldsTheValueInAMember := func(s *testcase.Spec) {
			s.When("the envelope holds a slice", func(s *testcase.Spec) {
				expected.Let(s, func(t *testcase.T) any {
					return MT_EM_Slice(items.Get(t))
				})
				data.Let(s, func(t *testcase.T) []byte {
					return envelope(t, "em:slice", valueMember.Get(t), items.Get(t))
				})

				thenItIsReadAsTheTypeTheEnvelopeNames(s)
			})

			s.When("the envelope holds a map", func(s *testcase.Spec) {
				expected.Let(s, func(t *testcase.T) any {
					return MT_EM_Map(entries.Get(t))
				})
				data.Let(s, func(t *testcase.T) []byte {
					return envelope(t, "em:map", mapMember.Get(t), entries.Get(t))
				})

				thenItIsReadAsTheTypeTheEnvelopeNames(s)

				// A map is a value too, so its JSON is read from the value member, like the JSON of any other value.
				s.And("its entries are in the value member of the envelope", func(s *testcase.Spec) {
					data.Let(s, func(t *testcase.T) []byte {
						return envelope(t, "em:map", valueMember.Get(t), entries.Get(t))
					})

					thenItIsReadAsTheTypeTheEnvelopeNames(s)
				})
			})

			s.When("the envelope holds a string", func(s *testcase.Spec) {
				expected.Let(s, func(t *testcase.T) any {
					return MT_EM_Scalar(name.Get(t))
				})
				data.Let(s, func(t *testcase.T) []byte {
					return envelope(t, "em:scalar", valueMember.Get(t), name.Get(t))
				})

				thenItIsReadAsTheTypeTheEnvelopeNames(s)
			})
		}

		// Unlike the members of a struct, which encoding/json matches case-insensitively,
		// the members of the envelope are matched by their exact names,
		// so JSON with a type member named otherwise, like an @type, carries no type.
		whenTheTypeMemberIsNamedOtherwise := func(s *testcase.Spec) {
			thenItIsReadAsAPlainJSONObject := func(s *testcase.Spec, otherMember testcase.Var[string]) {
				data.Let(s, func(t *testcase.T) []byte {
					return []byte(object(t, member(t, otherMember.Get(t), "em:struct"), member(t, "Name", name.Get(t))))
				})
				expected.Let(s, func(t *testcase.T) any {
					return map[string]any{otherMember.Get(t): "em:struct", "Name": name.Get(t)}
				})

				s.Then("it is read as a plain JSON object, like with encoding/json", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, expected.Get(t), got)
				})
			}

			s.When("the type member of the JSON is named other than the TypeKeyword of the Codec", func(s *testcase.Spec) {
				otherMember := let.Var(s, func(t *testcase.T) string {
					return random.Pick(t.Random, slices.DeleteFunc(
						[]string{"@type", "$type", "type"},
						func(name string) bool { return name == typeMember.Get(t) },
					)...)
				})

				thenItIsReadAsAPlainJSONObject(s, otherMember)
			})

			s.When("the type member of the JSON differs from the TypeKeyword of the Codec only in letter case", func(s *testcase.Spec) {
				otherMember := let.Var(s, func(t *testcase.T) string {
					return strings.ToUpper(typeMember.Get(t))
				})

				thenItIsReadAsAPlainJSONObject(s, otherMember)
			})
		}

		thenItIsReadAsTheTypeTheEnvelopeNames(s)

		whenTheEnvelopeHoldsTheValueInAMember(s)

		s.When("the struct has a member named @type of its own, like a JSON-LD document", func(s *testcase.Spec) {
			expected.Let(s, func(t *testcase.T) any {
				return MT_EM_LinkedData{Type: ldType.Get(t), Name: name.Get(t)}
			})
			data.Let(s, func(t *testcase.T) []byte {
				// The type member tells the type, wherever the @type is.
				return []byte(object(t, member(t, typeMember.Get(t), "em:linked-data"), member(t, "@type", ldType.Get(t)), member(t, "name", name.Get(t))))
			})

			thenItIsReadAsTheTypeTheEnvelopeNames(s)
		})

		whenTheTypeMemberIsNamedOtherwise(s)

		whenTheCodecHasKeywordsOfItsOwn(s, func(s *testcase.Spec) {
			thenItIsReadAsTheTypeTheEnvelopeNames(s)

			whenTheEnvelopeHoldsTheValueInAMember(s)

			whenTheTypeMemberIsNamedOtherwise(s)
		})

		whenTheKeywordsAreTheSame(s, func(s *testcase.Spec) {
			s.Then("it fails with ErrKeywordCollision", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrKeywordCollision)
			})
		})
	})
}

// MT_KC_Item is a registered struct of TestCodec_keywordCollision, which has no member named like a keyword of the Codec.
type MT_KC_Item struct{ Name string }

// MT_KC_Typed is a struct with a member named like the default TypeKeyword of the Codec.
type MT_KC_Typed struct {
	Type string `json:"$type"`
	Name string
}

// MT_KC_Valued is a struct with a member named like the default ValueKeyword of the Codec.
type MT_KC_Valued struct {
	Value string `json:"$value"`
	Name  string
}

// MT_KC_Mapped is a struct with a member named like the default MapKeyword of the Codec.
type MT_KC_Mapped struct {
	Map  string `json:"$map"`
	Name string
}

// MT_KC_Strings is a registered map type of TestCodec_keywordCollision.
type MT_KC_Strings map[string]string

// MT_KC_Custom is a struct whose custom codec writes it as a JSON object with a member named like the default TypeKeyword of the Codec.
type MT_KC_Custom struct{ Kind string }

// MT_KC_PairsJSON, MT_KC_PairsJSONTo, MT_KC_PairsText and MT_KC_PairsAppendText are map types which write their entries with their own marshaler,
// as a JSON array of their [key, value] pairs, or as the text of it, instead of as the members of a JSON object,
// so an entry keyed like the TypeKeyword is not a member of their JSON, and they read back what they write.
// They have the methods of encoding/json, of encoding/json/v2, and of encoding, which encoding/json calls all.
type (
	MT_KC_PairsJSON       map[string]string
	MT_KC_PairsJSONTo     map[string]string
	MT_KC_PairsText       map[string]string
	MT_KC_PairsAppendText map[string]string
)

func (m MT_KC_PairsJSON) MarshalJSON() ([]byte, error) { return marshalKCPairs(m) }

func (m *MT_KC_PairsJSON) UnmarshalJSON(data []byte) error {
	return unmarshalKCPairs(data, (*map[string]string)(m))
}

func (m MT_KC_PairsJSONTo) MarshalJSONTo(enc *jsontext.Encoder) error {
	data, err := marshalKCPairs(m)
	if err != nil {
		return err
	}
	return enc.WriteValue(data)
}

func (m *MT_KC_PairsJSONTo) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	data, err := dec.ReadValue()
	if err != nil {
		return err
	}
	return unmarshalKCPairs(data, (*map[string]string)(m))
}

func (m MT_KC_PairsText) MarshalText() ([]byte, error) { return marshalKCPairs(m) }

func (m *MT_KC_PairsText) UnmarshalText(text []byte) error {
	return unmarshalKCPairs(text, (*map[string]string)(m))
}

func (m MT_KC_PairsAppendText) AppendText(b []byte) ([]byte, error) {
	data, err := marshalKCPairs(m)
	return append(b, data...), err
}

func (m *MT_KC_PairsAppendText) UnmarshalText(text []byte) error {
	return unmarshalKCPairs(text, (*map[string]string)(m))
}

func marshalKCPairs[M ~map[string]string](m M) ([]byte, error) {
	pairs := make([][2]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		pairs = append(pairs, [2]string{k, m[k]})
	}
	return json.Marshal(pairs)
}

func unmarshalKCPairs(data []byte, m *map[string]string) error {
	var pairs [][2]string
	if err := json.Unmarshal(data, &pairs); err != nil {
		return err
	}
	*m = make(map[string]string, len(pairs))
	for _, pair := range pairs {
		(*m)[pair[0]] = pair[1]
	}
	return nil
}

// MT_KC_OwnTypedJSON and MT_KC_OwnTypedJSONTo are MT_KC_Typed, which write its member named like the default TypeKeyword of the Codec
// with their own marshaler on purpose, the way a json.RawMessage can have a type member,
// with the methods of encoding/json, and of encoding/json/v2, which encoding/json calls too.
type (
	MT_KC_OwnTypedJSON   MT_KC_Typed
	MT_KC_OwnTypedJSONTo MT_KC_Typed
)

func (v MT_KC_OwnTypedJSON) MarshalJSON() ([]byte, error) { return json.Marshal(MT_KC_Typed(v)) }

func (v MT_KC_OwnTypedJSONTo) MarshalJSONTo(enc *jsontext.Encoder) error {
	return jsonv2.MarshalEncode(enc, MT_KC_Typed(v))
}

// TestCodec_keywordCollision covers a value whose own members are named like the keywords of the Codec,
// the TypeKeyword, the ValueKeyword and the MapKeyword, which name the members of the type envelope.
// Where such a member would be read as a member of the envelope, instead of as a member of the value,
// Marshal fails with ErrKeywordCollision, instead of writing JSON which is read back as another value.
// The entries of a map in an envelope are its map member, where their keys are just data, so they can be named like anything.
// The Codec can name the members of the envelope otherwise, to have such a value written.
func TestCodec_keywordCollision(t *testing.T) {
	s := testcase.NewSpec(t)

	// knownTypeID is a type ID the Codec knows, so a member named like the TypeKeyword which holds it would be read as the type of the value.
	const knownTypeID = "kc:item"

	subject := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		c := &jsonkit.Codec{}
		t.Defer(jsonkit.CodecRegisterTypeID[MT_KC_Item](c, knownTypeID))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_KC_Typed](c, "kc:typed"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_KC_Valued](c, "kc:valued"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_KC_Mapped](c, "kc:mapped"))
		t.Defer(jsonkit.CodecRegisterTypeID[MT_KC_Strings](c, "kc:strings"))
		t.Defer(jsonkit.CodecRegister(c, "kc:custom", jsonkit.TypeCodec[MT_KC_Custom]{
			MarshalFunc: func(c *jsonkit.Codec, v MT_KC_Custom) ([]byte, error) {
				return json.Marshal(map[string]string{"$type": v.Kind})
			},
			UnmarshalFunc: func(c *jsonkit.Codec, data []byte, p *MT_KC_Custom) error {
				var dto map[string]string
				if err := json.Unmarshal(data, &dto); err != nil {
					return err
				}
				*p = MT_KC_Custom{Kind: dto["$type"]}
				return nil
			},
		}))
		return c
	})
	name := let.Var(s, func(t *testcase.T) string {
		return t.Random.HexN(8)
	})

	whenTheTypeKeywordIsAnotherName := func(s *testcase.Spec, spec func(s *testcase.Spec)) {
		s.And("the TypeKeyword of the Codec is another name", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := subject.Super(t)
				c.TypeKeyword = random.Pick(t.Random, "@type", "@kind", "_type")
				return c
			})

			spec(s)
		})
	}

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return MT_KC_Item{Name: name.Get(t)}
		})
		// act writes the value held by an `any`, the way an `any` field holds it, which is where its type member is written.
		act := func(t *testcase.T) ([]byte, error) {
			v := value.Get(t)
			return subject.Get(t).Marshal(&v)
		}

		thenItIsReadBackAsItIs := func(s *testcase.Spec) {
			s.Then("it is written, and read back as it is", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				got := reflect.New(reflect.TypeOf(value.Get(t)))
				assert.NoError(t, subject.Get(t).Unmarshal(data, got.Interface()))
				assert.Equal(t, value.Get(t), got.Elem().Interface())
			})
		}

		thenItFailsWithErrKeywordCollision := func(s *testcase.Spec) {
			s.Then("it fails with ErrKeywordCollision, instead of writing JSON which is read back as another value", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrKeywordCollision)
			})
		}

		thenItIsReadBackAsItIs(s)

		s.When("a struct held by an interface has a member named like the TypeKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return MT_KC_Typed{Type: knownTypeID, Name: name.Get(t)}
			})

			thenItFailsWithErrKeywordCollision(s)

			s.And("the type of the struct is not registered", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return struct {
						Type string `json:"$type"`
						Name string
					}{Type: knownTypeID, Name: name.Get(t)}
				})

				thenItFailsWithErrKeywordCollision(s)
			})

			// Like a json.RawMessage, what a value writes with its own marshaler is written as it is, so it can have a type member on purpose.
			s.And("the struct, whose type is not registered, writes itself with its own marshaler", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					typed := MT_KC_Typed{Type: knownTypeID, Name: name.Get(t)}
					return random.Pick[any](t.Random,
						MT_KC_OwnTypedJSON(typed),
						MT_KC_OwnTypedJSONTo(typed),
					)
				})

				s.Then("it is written with its own marshaler, like encoding/json writes it", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)

					exp, err := json.Marshal(value.Get(t))
					assert.Must(t).NoError(err)
					assertJSONEqual(t, string(exp), string(data))
				})
			})

			whenTheTypeKeywordIsAnotherName(s, thenItIsReadBackAsItIs)
		})

		s.When("a struct held by an interface has a member named like the ValueKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return MT_KC_Valued{Value: name.Get(t), Name: name.Get(t)}
			})

			thenItFailsWithErrKeywordCollision(s)

			s.And("the ValueKeyword of the Codec is another name", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					c.ValueKeyword = random.Pick(t.Random, "@value", "@data", "_value")
					return c
				})

				thenItIsReadBackAsItIs(s)
			})
		})

		s.When("a struct held by an interface has a member named like the MapKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return MT_KC_Mapped{Map: name.Get(t), Name: name.Get(t)}
			})

			thenItFailsWithErrKeywordCollision(s)

			s.And("the MapKeyword of the Codec is another name", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *jsonkit.Codec {
					c := subject.Super(t)
					c.MapKeyword = random.Pick(t.Random, "@map", "@entries", "_map")
					return c
				})

				thenItIsReadBackAsItIs(s)
			})
		})

		// The entries of a map in an envelope are its map member, where the keys are just data.
		s.When("a map held by an interface has entries keyed like the keywords", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				entries := map[string]string{"$type": knownTypeID, "$value": name.Get(t), "$map": name.Get(t), "Name": name.Get(t)}
				return random.Pick[any](t.Random,
					// a registered map type
					MT_KC_Strings(entries),
					// a map type, whose type ID is derived from the one of its values
					entries,
					// held by an interface, which is an element of a slice
					[]any{MT_KC_Strings(entries)},
				)
			})

			thenItIsReadBackAsItIs(s)

			s.Then("it is read back as it is into an interface too", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var got any
				assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
				assert.Equal(t, value.Get(t), got)
			})
		})

		// Without an envelope, the entry would be read as the type member of an envelope.
		s.When("a map written without an envelope has an entry keyed like the TypeKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return random.Pick[any](t.Random,
					// held by an interface, as plain JSON, as the map has no type ID
					map[string]any{"$type": knownTypeID, "Name": name.Get(t)},
					// held by a field of its own type, without an envelope
					struct{ F map[string]string }{F: map[string]string{"$type": knownTypeID, "Name": name.Get(t)}},
					// nested in a plain JSON array
					[]any{map[string]any{"$type": knownTypeID, "Name": name.Get(t)}},
				)
			})

			thenItFailsWithErrKeywordCollision(s)

			whenTheTypeKeywordIsAnotherName(s, thenItIsReadBackAsItIs)

			// What the map writes with its own marshaler is not a JSON object of its entries, so no entry of it is a member.
			s.And("the map writes its entries with its own marshaler", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					entries := map[string]string{"$type": knownTypeID, "Name": name.Get(t)}
					return random.Pick[any](t.Random,
						// held by an interface, as the map has no type ID
						MT_KC_PairsJSON(entries),
						MT_KC_PairsJSONTo(entries),
						MT_KC_PairsText(entries),
						MT_KC_PairsAppendText(entries),
						// held by a field of its own type
						struct{ F MT_KC_PairsJSON }{F: entries},
						struct{ F MT_KC_PairsJSONTo }{F: entries},
						struct{ F MT_KC_PairsText }{F: entries},
						struct{ F MT_KC_PairsAppendText }{F: entries},
					)
				})

				s.Then("it is written with its own marshaler, like encoding/json writes it", func(t *testcase.T) {
					data, err := act(t)
					assert.NoError(t, err)

					exp, err := json.Marshal(value.Get(t))
					assert.Must(t).NoError(err)
					assertJSONEqual(t, string(exp), string(data))
				})

				thenItIsReadBackAsItIs(s)
			})
		})

		// Only the type member makes a JSON object a type envelope.
		s.When("a map written without an envelope has entries keyed like the ValueKeyword and the MapKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return random.Pick[any](t.Random,
					map[string]any{"$value": name.Get(t), "$map": name.Get(t)},
					struct{ F map[string]string }{F: map[string]string{"$value": name.Get(t), "$map": name.Get(t)}},
				)
			})

			thenItIsReadBackAsItIs(s)
		})

		// No type member is written for a value whose type the place it sits in tells.
		s.When("a struct with a member named like the TypeKeyword is held by a field of its own type", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return struct{ F MT_KC_Typed }{F: MT_KC_Typed{Type: knownTypeID, Name: name.Get(t)}}
			})

			thenItIsReadBackAsItIs(s)
		})

		s.When("a custom codec writes a member named like the TypeKeyword", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				custom := MT_KC_Custom{Kind: knownTypeID}
				return random.Pick[any](t.Random,
					// held by an interface, so its type member is written next to its own members
					custom,
					// held by a field of its own type, without a type member,
					// where its member would still be read as the type member of an envelope, instead of being given to the custom codec
					struct{ F MT_KC_Custom }{F: custom},
				)
			})

			thenItFailsWithErrKeywordCollision(s)

			whenTheTypeKeywordIsAnotherName(s, thenItIsReadBackAsItIs)
		})

		s.When("the value is a jsonkit.Interface, which holds a value with the type member of its own", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return jsonkit.Interface[any]{V: MT_KC_Item{Name: name.Get(t)}}
			})

			s.Then("it is written as the value it holds", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)

				var held any = MT_KC_Item{Name: name.Get(t)}
				expected, err := subject.Get(t).Marshal(&held)
				assert.Must(t).NoError(err)
				assertJSONEqual(t, string(expected), string(data))
			})
		})

		// A json.RawMessage is JSON which is written as it is, so it can have a type member on purpose.
		s.When("the value is a json.RawMessage with a type member", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return json.RawMessage(`{"$type":"` + knownTypeID + `","Name":` + mustMarshalJSON(t, name.Get(t)) + `}`)
			})

			s.Then("it is written as it is, and read back as the type its type member names", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assertJSONEqual(t, string(value.Get(t).(json.RawMessage)), string(data))

				var got any
				assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
				assert.Equal[any](t, MT_KC_Item{Name: name.Get(t)}, got)
			})
		})
	})
}
