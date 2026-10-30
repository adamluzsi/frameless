package jsonkit_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestWithType_json(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Before(RegisterTypeIDs)

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

	// An interface holding a map is written with its entries as the $map member of the type envelope,
	// which keeps them apart from the members of the envelope.
	s.Test("interface containing a map is written with its entries as the $map member of the type envelope", func(t *testcase.T) {
		key, v := t.Random.HexN(4), t.Random.HexN(4)
		exp := `{"$type":"map[string]string","$map":{` + strconv.Quote(key) + `:` + strconv.Quote(v) + `}}`

		data, err := json.Marshal(jsonkit.WithType[any]{V: map[string]string{key: v}})
		assert.NoError(t, err)
		assertJSONEqual(t, exp, string(data))

		data, err = json.Marshal(jsonkit.WithType[map[string]string]{V: map[string]string{key: v}})
		assert.NoError(t, err)
		assertJSONEqual(t, exp, string(data))
	})

	s.Test("interface containing a nil map", func(t *testcase.T) {
		data, err := json.Marshal(jsonkit.WithType[any]{V: map[string]string(nil)})
		assert.NoError(t, err)
		assertJSONEqual(t, `{"$type":"map[string]string","$map":null}`, string(data))

		roundTrip(t, jsonkit.WithType[any]{V: map[string]string(nil)}, new(jsonkit.WithType[any]))
	})

	// Only the members named by the keywords of the Codec make up the type envelope,
	// so a key like the @type of a JSON-LD document is just an entry of the map.
	s.Test("interface containing a map with an @type entry", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: map[string]int{
			"@type":          t.Random.Int(),
			t.Random.HexN(4): t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	// The keys of the entries in the $map member are just data, see TestCodec_keywordCollision.
	s.Test("interface containing a map with entries named like the members of the type envelope", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: map[string]int{
			"$type":          t.Random.Int(),
			"$value":         t.Random.Int(),
			"$map":           t.Random.Int(),
			t.Random.HexN(4): t.Random.Int(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("map with entries named like the members of the type envelope", func(t *testcase.T) {
		exp := jsonkit.WithType[map[string]string]{V: map[string]string{
			"$type":  "with_type_struct",
			"$value": t.Random.String(),
			"$map":   t.Random.String(),
		}}
		roundTrip(t, exp, new(jsonkit.WithType[map[string]string]))
	})

	// A map without a type ID is written in an envelope which names no map type, so it would be read back as the type its entry names.
	s.Test("interface containing a map without a type ID, with an entry named like the $type member of the type envelope, fails with ErrKeywordCollision", func(t *testcase.T) {
		_, err := json.Marshal(jsonkit.WithType[any]{V: map[string]any{"$type": "with_type_struct"}})
		assert.ErrorIs(t, err, jsonkit.ErrKeywordCollision)
	})

	s.Test("interface containing a map with its own json.Unmarshaler, which reads back the members its MarshalJSON wrote", func(t *testcase.T) {
		t.Defer(jsonkit.RegisterTypeID[MT_OwnJSONMap]("own_json_map"))
		exp := jsonkit.WithType[any]{V: MT_OwnJSONMap{t.Random.HexN(4): t.Random.String()}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	// The $type of a slice or a map, which is not registered itself, is derived from the type IDs of its elements.
	s.Test("interface containing a slice of a registered type keeps its type", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: []TypeA{{V: t.Random.String()}, {V: t.Random.String()}}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
	})

	s.Test("interface containing a map of a registered type keeps its type, even with keys named like the members of the type envelope", func(t *testcase.T) {
		exp := jsonkit.WithType[any]{V: map[string]TypeA{
			"$type":          {V: t.Random.String()},
			"$value":         {V: t.Random.String()},
			"$map":           {V: t.Random.String()},
			t.Random.HexN(4): {V: t.Random.String()},
		}}
		roundTrip(t, exp, new(jsonkit.WithType[any]))
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

	s.Test("pointer to struct with an interface field keeps the $type of the value the field holds", func(t *testcase.T) {
		t.Defer(jsonkit.RegisterTypeID[withTypeHolder]("with_type_holder"))
		exp := jsonkit.WithType[*withTypeHolder]{V: &withTypeHolder{V: withTypeString(t.Random.String())}}
		roundTrip(t, exp, new(jsonkit.WithType[*withTypeHolder]))
	})

	s.Test("pointer to struct which references back to itself fails the way encoding/json fails, instead of recursing forever", func(t *testcase.T) {
		t.Defer(jsonkit.RegisterTypeID[withTypeNode]("with_type_node"))
		n := &withTypeNode{}
		n.Next = n

		_, err := json.Marshal(jsonkit.WithType[*withTypeNode]{V: n})

		var unsupported *json.UnsupportedValueError
		assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
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

	// WithType asks for the $type, so it is written even for a value which plain JSON would be read back as.
	s.Test("interface containing a value of the types plain JSON is read into keeps its $type", func(t *testcase.T) {
		v := makeJSONTypeScalar(t)
		data, err := json.Marshal(jsonkit.WithType[any]{V: v})
		assert.NoError(t, err)
		assertJSONEqual(t, makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(v).String()), v), string(data))
		roundTrip(t, jsonkit.WithType[any]{V: v}, new(jsonkit.WithType[any]))
	})

	s.Test("WithTypeIDOf interface containing a value of the types plain JSON is read into keeps its $type", func(t *testcase.T) {
		v := makeJSONTypeScalar(t)
		data, err := json.Marshal(jsonkit.WithTypeIDOf[any, string]{V: v})
		assert.NoError(t, err)
		assertJSONEqual(t, makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(v).String()), v), string(data))
		roundTrip(t, jsonkit.WithTypeIDOf[any, string]{V: v}, new(jsonkit.WithTypeIDOf[any, string]))
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

	s.Test("failed unmarshal preserves the previous value", func(t *testcase.T) {
		previous := t.Random.Int()
		dto := jsonkit.WithType[int]{V: previous}
		err := json.Unmarshal([]byte(`{"$type":"int","$value":"not-an-integer"}`), &dto)
		assert.Error(t, err)
		assert.Equal(t, previous, dto.V)
	})

	s.Test("JSON surrounded by whitespace is decoded the same way as without it", func(t *testcase.T) {
		exp := jsonkit.WithType[withTypeInterface]{V: withTypeString(t.Random.String())}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)
		data = []byte(makeJSONWhitespace(t) + string(data) + makeJSONWhitespace(t))

		var got jsonkit.WithType[withTypeInterface]
		assert.NoError(t, got.UnmarshalJSON(data))
		assert.Equal(t, exp, got)
	})

	s.Test("a JSON value which can not be decoded into T is rejected with an error, and the previous value is preserved", func(t *testcase.T) {
		previous := t.Random.Int()
		dto := jsonkit.WithType[int]{V: previous}

		assert.Error(t, json.Unmarshal([]byte(strconv.Quote(t.Random.String())), &dto))
		assert.Equal(t, previous, dto.V)
	})

	s.Test("a JSON value which the interface T can not hold is rejected with an error", func(t *testcase.T) {
		var dto jsonkit.WithType[withTypeInterface]
		assert.Error(t, json.Unmarshal([]byte(strconv.Itoa(t.Random.Int())), &dto))
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

	s.Test("bare struct JSON keeps the $type of the values of its interface fields", func(t *testcase.T) {
		type T struct{ V withTypeInterface }
		exp := T{V: withTypeString(t.Random.String())}
		// The Codec puts a $type envelope only on the interface field, as T itself is not registered.
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

// assertDecodesBareJSON asserts that a JSON value without a $type envelope,
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
	s.Before(RegisterTypeIDs)

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
		var dto jsonkit.WithTypeIDOf[withTypeInterface, string]
		assert.Error(t, json.Unmarshal([]byte(strconv.Itoa(t.Random.Int())), &dto))
	})
}
