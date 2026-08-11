package jsonkit_test

import (
	"encoding/json"
	"reflect"
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
}
