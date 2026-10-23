package jsonkit_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestWithType_json(t *testing.T) {
	s := testcase.NewSpec(t)

	var roundTrip = func(t *testcase.T, exp, got any) {
		data, err := json.Marshal(exp)
		assert.NoError(t, err)
		assert.NoError(t, json.Unmarshal(data, got))
		assert.Equal(t, exp, reflect.ValueOf(got).Elem().Interface())
	}

	s.Test("interface containing a struct", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: withTypeStruct{
			Name: t.Random.String(),
			Age:  t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("interface containing a custom implementation", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeInterface]{V: withTypeString(t.Random.String())}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeInterface]))
	})

	s.Test("struct", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeStruct]{V: withTypeStruct{
			Name: t.Random.String(),
			Age:  t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeStruct]))
	})

	s.Test("map", func(t *testcase.T) {
		exp := jsonkit.WithType[map[string]int]{V: map[string]int{
			t.Random.String(): t.Random.Int(),
			t.Random.String(): t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[map[string]int]))
	})

	s.Test("primitive slice", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeSlice]{V: withTypeSlice{
			t.Random.Int(),
			t.Random.Int(),
			t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeSlice]))
	})

	s.Test("string", func(t *testcase.T) {
		exp := jsonkit.WithType[string]{V: t.Random.String()}
		roundTrip(t, exp, new(jsonkit.WithType[string]))
	})

	s.Test("integer", func(t *testcase.T) {
		exp := jsonkit.WithType[int]{V: t.Random.Int()}
		roundTrip(t, exp, new(jsonkit.WithType[int]))
	})

	s.Test("boolean", func(t *testcase.T) {
		exp := jsonkit.WithType[bool]{V: t.Random.Bool()}
		roundTrip(t, exp, new(jsonkit.WithType[bool]))
	})

	s.Test("float", func(t *testcase.T) {
		exp := jsonkit.WithType[float64]{V: t.Random.Float64()}
		roundTrip(t, exp, new(jsonkit.WithType[float64]))
	})

	s.Test("pointer to struct", func(t *testcase.T) {
		exp := jsonkit.WithType[*withTypeStruct]{V: &withTypeStruct{
			Name: t.Random.String(),
			Age:  t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[*withTypeStruct]))
	})

	s.Test("nil pointer", func(t *testcase.T) {
		exp := jsonkit.WithType[*withTypeStruct]{V: nil}
		roundTrip(t, exp, new(jsonkit.WithType[*withTypeStruct]))
	})

	s.Test("nil interface", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: nil}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("interface containing a primitive", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: t.Random.Int()}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("interface containing a primitive slice", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: withTypeSlice{
			t.Random.Int(),
			t.Random.Int(),
			t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("empty slice", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeSlice]{V: withTypeSlice{}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeSlice]))
	})

	s.Test("nil slice", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeSlice]{V: nil}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeSlice]))
	})

	s.Test("empty map", func(t *testcase.T) {
		exp := jsonkit.WithType[map[string]int]{V: map[string]int{}}
		roundTrip(t, exp, new(jsonkit.WithType[map[string]int]))
	})

	s.Test("nil map", func(t *testcase.T) {
		exp := jsonkit.WithType[map[string]int]{V: nil}
		roundTrip(t, exp, new(jsonkit.WithType[map[string]int]))
	})

	s.Test("array", func(t *testcase.T) {
		exp := jsonkit.WithType[[3]int]{V: [3]int{
			t.Random.Int(),
			t.Random.Int(),
			t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[[3]int]))
	})

	s.Test("nested struct", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeNested]{V: withTypeNested{
			Value:  &withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()},
			Labels: map[string]string{t.Random.String(): t.Random.String()},
			Items: []withTypeStruct{
				{Name: t.Random.String(), Age: t.Random.Int()},
				{Name: t.Random.String(), Age: t.Random.Int()},
			},
		}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeNested]))
	})

	s.Test("interface containing a pointer implementation", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeInterface]{V: &withTypePointerImpl{Value: t.Random.String()}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeInterface]))
	})

	s.Test("named integer", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeInt]{V: withTypeInt(t.Random.Int())}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeInt]))
	})

	s.Test("named boolean", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeBool]{V: withTypeBool(t.Random.Bool())}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeBool]))
	})

	s.Test("named float", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeFloat]{V: withTypeFloat(t.Random.Float64())}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeFloat]))
	})

	s.Test("custom JSON marshaler and unmarshaler", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeCustomJSON]{V: withTypeCustomJSON{Value: t.Random.String()}}
		roundTrip(t, exp, new(jsonkit.WithType[withTypeCustomJSON]))
	})

	s.Test("function value returns a marshal error", func(t *testcase.T) {
		_, err := json.Marshal(jsonkit.WithType[func()]{V: func() {}})
		assert.Error(t, err)
	})

	s.Test("channel value returns a marshal error", func(t *testcase.T) {
		_, err := json.Marshal(jsonkit.WithType[chan int]{V: make(chan int)})
		assert.Error(t, err)
	})

	s.Test("cyclic value returns a marshal error", func(t *testcase.T) {
		type node struct{ Next *node }
		value := &node{}
		value.Next = value
		_, err := json.Marshal(jsonkit.WithType[*node]{V: value})
		assert.Error(t, err)
	})

	s.Test("failed unmarshal preserves the previous value", func(t *testcase.T) {
		previous := t.Random.Int()
		dto := jsonkit.WithType[int]{V: previous}
		err := json.Unmarshal([]byte(`{"@type":"int","@value":"not-an-integer"}`), &dto)
		assert.Error(t, err)
		assert.Equal(t, previous, dto.V)
	})

	s.Test("JSON surrounded by whitespace is decoded the same way as without it", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeInterface]{V: withTypeString(t.Random.String())}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)
		data = []byte(makeJSONWhitespace(t) + string(data) + makeJSONWhitespace(t))

		var got jsonkit.WithType[withTypeInterface]
		assert.NotPanic(t, func() { err = got.UnmarshalJSON(data) })
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
	})

	s.Test("a JSON value which can not be decoded into T is rejected with an error, and the previous value is preserved", func(t *testcase.T) {
		previous := t.Random.Int()
		dto := jsonkit.WithType[int]{V: previous}

		var err error
		assert.NotPanic(t, func() { err = json.Unmarshal([]byte(strconv.Quote(t.Random.String())), &dto) })
		assert.Error(t, err)
		assert.Equal(t, previous, dto.V)
	})

	s.Test("a JSON value which the interface T can not hold is rejected with an error", func(t *testcase.T) {
		var (
			dto jsonkit.WithType[withTypeInterface]
			err error
		)
		assert.NotPanic(t, func() { err = json.Unmarshal([]byte(strconv.Itoa(t.Random.Int())), &dto) })
		assert.Error(t, err)
	})

	s.Test("bare integer JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, t.Random.Int())
	})

	s.Test("bare named integer JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeInt(t.Random.Int()))
	})

	s.Test("bare named string JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeString(t.Random.String()))
	})

	s.Test("bare struct JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()})
	})

	s.Test("bare struct JSON is decoded straight into a pointer T", func(t *testcase.T) {
		assertDecodesBareJSON(t, &withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()})
	})

	s.Test("bare nested struct JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeNested{
			Value:  &withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()},
			Labels: map[string]string{t.Random.String(): t.Random.String()},
			Items:  []withTypeStruct{{Name: t.Random.String(), Age: t.Random.Int()}},
		})
	})

	s.Test("bare map JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, map[string]int{t.Random.String(): t.Random.Int()})
	})

	s.Test("bare slice JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeSlice{t.Random.Int(), t.Random.Int()})
	})

	s.Test("bare array JSON is decoded straight into T", func(t *testcase.T) {
		assertDecodesBareJSON(t, [3]int{t.Random.Int(), t.Random.Int(), t.Random.Int()})
	})

	s.Test("bare JSON of a T with its own UnmarshalJSON is decoded with it", func(t *testcase.T) {
		assertDecodesBareJSON(t, withTypeCustomJSON{Value: t.Random.String()})
	})

	s.Test("bare struct JSON keeps the @type dispatch of its interface fields", func(t *testcase.T) {
		type T struct{ V withTypeInterface }
		exp := T{V: withTypeString(t.Random.String())}
		// The Codec puts a @type envelope only on the interface field, as T itself is not registered.
		data, err := (&jsonkit.Codec{}).Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.WithType[T]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got.V)
	})

	s.Test("null is decoded as the zero value of T", func(t *testcase.T) {
		dto := jsonkit.WithType[withTypeStruct]{V: withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()}}
		assert.NoError(t, json.Unmarshal([]byte(`null`), &dto))
		assert.Equal(t, withTypeStruct{}, dto.V)
	})

	s.Test("null is decoded as the zero value of T, even when T has its own UnmarshalJSON", func(t *testcase.T) {
		dto := jsonkit.WithType[withTypeCustomJSON]{V: withTypeCustomJSON{Value: t.Random.String()}}
		assert.NoError(t, json.Unmarshal([]byte(`null`), &dto))
		assert.Equal(t, withTypeCustomJSON{}, dto.V)
	})
}

// assertDecodesBareJSON asserts that a JSON value without a @type envelope,
// like what encoding/json makes from v, is decoded straight into T.
// The JSON carries no type information, but WithType[T] already tells the type.
func assertDecodesBareJSON[T any](t *testcase.T, v T) {
	t.Helper()
	data, err := json.Marshal(v)
	assert.NoError(t, err)

	var got jsonkit.WithType[T]
	assert.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, v, got.V)
}

func TestWithTypeIDOf_json(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("interface containing a custom implementation", func(t *testcase.T) {
		exp := jsonkit.WithTypeIDOf[withTypeInterface, string]{V: withTypeString(t.Random.String())}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.WithTypeIDOf[withTypeInterface, string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.Test("bare struct JSON is decoded straight into T", func(t *testcase.T) {
		exp := withTypeStruct{Name: t.Random.String(), Age: t.Random.Int()}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.WithTypeIDOf[withTypeStruct, string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got.V)
	})

	s.Test("a JSON value which the interface T can not hold is rejected with an error", func(t *testcase.T) {
		var (
			dto jsonkit.WithTypeIDOf[withTypeInterface, string]
			err error
		)
		assert.NotPanic(t, func() { err = json.Unmarshal([]byte(strconv.Itoa(t.Random.Int())), &dto) })
		assert.Error(t, err)
	})
}
