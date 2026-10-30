// Package codeccontract holds the contract of the codec port,
// which a codec.Codec implementation can check itself against.
package codeccontract

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

// Codec is the contract of a codec.Codec.
//
// What a codec marshals, it unmarshals back into an equal value,
// and like encoding/json, it marshals a value which has its own encoding.TextMarshaler with its MarshalText,
// and unmarshals into a value which has its own encoding.TextUnmarshaler with its UnmarshalText.
//
// A codec which doesn't support a kind of value, like a slice or a map,
// returns an error from Marshal which wraps codec.ErrNotSupported,
// and then the expectations about that kind of value are skipped.
//
// A codec which is also a codec.StreamProducer and a codec.StreamConsumer
// decodes from a stream the values it encoded into it, in their order,
// the same way as Unmarshal reads back what Marshal writes.
func Codec(subject codec.Codec, opts ...CodecOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	var (
		// text is a value which implements encoding.TextMarshaler and encoding.TextUnmarshaler.
		text = let.Var(s, func(t *testcase.T) textValue {
			token := t.Random.UUID()
			t.Cleanup(func() { textCallsByToken.Delete(token) })
			return textValue{token: token}
		})
	)

	s.Describe("#Marshal", func(s *testcase.Spec) {
		var (
			value = let.Var(s, func(t *testcase.T) any {
				return makeRecord(t)
			})
		)
		act := func(t *testcase.T) ([]byte, error) {
			return subject.Marshal(value.Get(t))
		}

		s.Then("it marshals the value into data", func(t *testcase.T) {
			data, err := act(t)
			skipIfNotSupported(t, err)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)
		})

		s.When("the value implements encoding.TextMarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return text.Get(t)
			})

			s.Then("it marshals the value with its MarshalText", func(t *testcase.T) {
				_, err := act(t)
				skipIfNotSupported(t, err)
				assert.NoError(t, err)
				assert.True(t, 0 < text.Get(t).calls().marshal.Load(), "MarshalText was expected to be called")
			})
		})

		s.When("a field of the value implements encoding.TextMarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return textHolder{Name: makeString(t), Text: text.Get(t)}
			})

			s.Then("it marshals the field with its MarshalText", func(t *testcase.T) {
				_, err := act(t)
				skipIfNotSupported(t, err)
				assert.NoError(t, err)
				assert.True(t, 0 < text.Get(t).calls().marshal.Load(), "MarshalText was expected to be called")
			})
		})

		s.When("an element of the value implements encoding.TextMarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []textValue{text.Get(t)}
			})

			s.Then("it marshals the element with its MarshalText", func(t *testcase.T) {
				_, err := act(t)
				skipIfNotSupported(t, err)
				assert.NoError(t, err)
				assert.True(t, 0 < text.Get(t).calls().marshal.Load(), "MarshalText was expected to be called")
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			value = let.Var(s, func(t *testcase.T) any {
				return makeRecord(t)
			})
			data = let.Var(s, func(t *testcase.T) []byte {
				data, err := subject.Marshal(value.Get(t))
				skipIfNotSupported(t, err)
				assert.Must(t).NoError(err)
				return data
			})
			ptr = let.Var(s, func(t *testcase.T) any {
				return reflect.New(reflect.TypeOf(value.Get(t))).Interface()
			})
		)
		act := func(t *testcase.T) error {
			return subject.Unmarshal(data.Get(t), ptr.Get(t))
		}

		s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
		})

		s.When("the value is a zero value", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return record{}
			})

			s.Then("it unmarshals the data into a zero value", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal[any](t, record{}, reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("the value is a pointer", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				r := makeRecord(t)
				return &r
			})

			s.Then("it unmarshals the data into a pointer to a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("the value has nested structs", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				pointer := makeRecord(t)
				return nested{Record: makeRecord(t), Pointer: &pointer}
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("the value has slices", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return withSlices{
					Strings: random.Slice(t.Random.IntBetween(1, 3), func() string { return makeString(t) }),
					Records: random.Slice(t.Random.IntBetween(1, 3), func() record { return makeRecord(t) }),
				}
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("the value has maps", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return withMaps{
					Ints:    map[string]int{makeKey(t): t.Random.Int()},
					Records: map[string]record{makeKey(t): makeRecord(t)},
				}
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("the value implements encoding.TextUnmarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return text.Get(t)
			})

			s.Then("it unmarshals the value with its UnmarshalText", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.True(t, 0 < text.Get(t).calls().unmarshal.Load(), "UnmarshalText was expected to be called")
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("a field of the value implements encoding.TextUnmarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return textHolder{Name: makeString(t), Text: text.Get(t)}
			})

			s.Then("it unmarshals the field with its UnmarshalText", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.True(t, 0 < text.Get(t).calls().unmarshal.Load(), "UnmarshalText was expected to be called")
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		s.When("an element of the value implements encoding.TextUnmarshaler", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []textValue{text.Get(t)}
			})

			s.Then("it unmarshals the element with its UnmarshalText", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.True(t, 0 < text.Get(t).calls().unmarshal.Load(), "UnmarshalText was expected to be called")
			})

			s.Then("it unmarshals the data into a value equal to the marshaled one", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, value.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})

		if c.SupportInterface {
			s.When("the data is marshaled from a pointer to an interface", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					// plain JSON reads a number back into an interface as a float64,
					// so an int only comes back if the codec keeps its type.
					return t.Random.Int()
				})
				data.Let(s, func(t *testcase.T) []byte {
					v := value.Get(t)
					data, err := subject.Marshal(&v)
					assert.Must(t).NoError(err)
					return data
				})
				ptr.Let(s, func(t *testcase.T) any {
					return new(any)
				})

				s.Then("it unmarshals the data into an interface holding an int, equal to the marshaled one", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertInterfaceHolds(t, value.Get(t).(int), ptr.Get(t))
				})

				s.And("the interface holds a string", func(s *testcase.Spec) {
					value.Let(s, func(t *testcase.T) any {
						return makeString(t)
					})

					s.Then("it unmarshals the data into an interface holding the string", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assertInterfaceHolds(t, value.Get(t).(string), ptr.Get(t))
					})
				})

				s.And("the interface is nil", func(s *testcase.Spec) {
					value.LetValue(s, nil)

					s.Then("it unmarshals the data into a nil interface", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.Nil(t, *ptr.Get(t).(*any))
					})
				})
			})
		}

		s.When("the destination is not a pointer", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return reflect.New(reflect.TypeOf(value.Get(t))).Elem().Interface()
			})

			s.Then("it returns an error", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})

		s.When("the destination is a nil pointer", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return reflect.Zero(reflect.PointerTo(reflect.TypeOf(value.Get(t)))).Interface()
			})

			s.Then("it returns an error", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})

		s.When("the destination is nil", func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return nil
			})

			s.Then("it returns an error", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})

	s.Describe("stream round trip", func(s *testcase.Spec) {
		producer, isProducer := subject.(codec.StreamProducer)
		consumer, isConsumer := subject.(codec.StreamConsumer)
		s.Before(func(t *testcase.T) {
			if !isProducer || !isConsumer {
				t.Skipf("the codec doesn't support streams, it is not a codec.StreamProducer and a codec.StreamConsumer: %T", subject)
			}
		})

		var (
			makeValue = let.Var(s, func(t *testcase.T) func() any {
				return func() any { return makeRecord(t) }
			})
			values = let.Var(s, func(t *testcase.T) []any {
				return random.Slice(t.Random.IntBetween(1, 5), makeValue.Get(t))
			})
			closes = let.VarOf(s, 1)
		)
		// act encodes the values into a stream, closes it, then decodes each value from it,
		// into the type of the value which was encoded at the same position.
		act := func(t *testcase.T) ([]any, error) {
			var buf bytes.Buffer
			enc := producer.NewStreamEncoder(&buf)
			for _, v := range values.Get(t) {
				if err := enc.Encode(v); err != nil {
					return nil, err
				}
			}
			for range closes.Get(t) {
				if err := enc.Close(); err != nil {
					return nil, err
				}
			}
			var got []any
			for dec, err := range consumer.NewStreamDecoder(&buf) {
				if err != nil {
					return got, err
				}
				if len(values.Get(t)) <= len(got) {
					return got, fmt.Errorf("the stream has more values than the %d which were encoded into it", len(values.Get(t)))
				}
				ptr := reflect.New(reflect.TypeOf(values.Get(t)[len(got)]))
				if err := dec.Decode(ptr.Interface()); err != nil {
					return got, err
				}
				got = append(got, ptr.Elem().Interface())
			}
			return got, nil
		}

		s.Then("it decodes the values which were encoded, in their order", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, values.Get(t), got)
		})

		s.When("no value is encoded", func(s *testcase.Spec) {
			values.Let(s, func(t *testcase.T) []any {
				return []any{}
			})

			s.Then("it decodes no value", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Empty(t, got)
			})
		})

		s.When("the encoder is closed more than once", func(s *testcase.Spec) {
			closes.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(2, 3)
			})

			s.Then("it decodes the values which were encoded, in their order", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, values.Get(t), got)
			})
		})

		s.When("the values have a field of the empty interface", func(s *testcase.Spec) {
			makeValue.Let(s, func(t *testcase.T) func() any {
				return func() any { return withAny{Record: makeRecord(t), Any: t.Random.Int()} }
			})

			s.Then("it decodes each value the way Unmarshal reads back what Marshal writes", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, len(values.Get(t)), len(got))
				for i, v := range values.Get(t) {
					data, err := subject.Marshal(v)
					assert.Must(t).NoError(err)
					exp := reflect.New(reflect.TypeOf(v))
					assert.Must(t).NoError(subject.Unmarshal(data, exp.Interface()))
					assert.Equal(t, exp.Elem().Interface(), got[i])
				}
			})
		})
	})

	return s.AsSuite("Codec")
}

type CodecOption option.Option[CodecConfig]

type CodecConfig struct {
	// SupportInterface flags that the codec keeps the type of a value which sits in an interface.
	// What it marshals from a pointer to an interface, such as a *any,
	// it unmarshals back into a pointer to the same interface type, holding a value equal to the marshaled one,
	// even when plain JSON would lose its type, like an int, which it reads back as a float64.
	SupportInterface bool
}

func (c CodecConfig) Configure(t *CodecConfig) {
	t.SupportInterface = cmp.Or(c.SupportInterface, t.SupportInterface)
}

// assertInterfaceHolds asserts with a type assertion that the interface ptr points to holds a T equal to exp,
// which proves that the codec kept the type of the value, not only its JSON.
func assertInterfaceHolds[T any](t *testcase.T, exp T, ptr any) {
	t.Helper()
	held := *ptr.(*any)
	got, ok := held.(T)
	assert.True(t, ok, assert.MessageF("expected the interface to hold a %T, got a %T: %#v", exp, held, held))
	assert.Equal(t, exp, got)
}

func skipIfNotSupported(t *testcase.T, err error) {
	t.Helper()
	if errors.Is(err, codec.ErrNotSupported) {
		t.Skipf("the codec doesn't support the value: %v", err)
	}
}

// record is made of the basic kinds, which every codec is expected to support.
type record struct {
	String string
	Int    int
	Bool   bool
	Float  float64
}

type nested struct {
	Record  record
	Pointer *record
}

type withSlices struct {
	Strings []string
	Records []record
}

type withMaps struct {
	Ints    map[string]int
	Records map[string]record
}

// withAny has a field of the empty interface, which plain JSON reads a number into as a float64.
type withAny struct {
	Record record
	Any    any
}

func makeRecord(t *testcase.T) record {
	return record{
		String: makeString(t),
		Int:    t.Random.Int(),
		Bool:   t.Random.Bool(),
		Float:  t.Random.Float64(),
	}
}

// makeString makes a non-empty string of printable characters.
func makeString(t *testcase.T) string {
	return t.Random.StringN(t.Random.IntBetween(1, 16))
}

func makeKey(t *testcase.T) string {
	return t.Random.StringNC(t.Random.IntBetween(1, 16), random.CharsetAlpha())
}

// textValue implements encoding.TextMarshaler and encoding.TextUnmarshaler.
// It has no exported field, so its own methods are the only way to marshal and unmarshal it.
// The methods count their calls, so the contract can tell that the codec used them.
type textValue struct{ token string }

// textPrefix is what MarshalText writes before the token.
const textPrefix = "codeccontract.textValue:"

func (v textValue) MarshalText() ([]byte, error) {
	v.calls().marshal.Add(1)
	return []byte(textPrefix + v.token), nil
}

func (v *textValue) UnmarshalText(text []byte) error {
	token, ok := strings.CutPrefix(string(text), textPrefix)
	if !ok {
		return fmt.Errorf("codeccontract: textValue can't unmarshal %q", text)
	}
	v.token = token
	v.calls().unmarshal.Add(1)
	return nil
}

func (v textValue) calls() *textCalls {
	calls, _ := textCallsByToken.LoadOrStore(v.token, &textCalls{})
	return calls.(*textCalls)
}

type textCalls struct{ marshal, unmarshal atomic.Int64 }

// textCallsByToken holds the *textCalls of each textValue by its token.
// Each test makes a textValue with a token of its own, so the tests don't share their counts.
var textCallsByToken sync.Map

type textHolder struct {
	Name string
	Text textValue
}
