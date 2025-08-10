package jsonkit_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleArray() {
	var greeters = jsonkit.Array[Greeter]{
		TypeA{V: "42"},
		TypeB{V: 42},
	}

	data, err := json.Marshal(greeters)
	if err != nil {
		panic(err)
	}

	var result jsonkit.Array[Greeter]
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}

	// "result" will contain the same as the "greeters".
}

func ExampleInterface() {
	var exp = jsonkit.Interface[Greeter]{
		V: &TypeC{V: 42.24},
	}

	data, err := json.Marshal(exp)
	if err != nil {
		panic(err)
	}
	// {"__type":"type_c","v":42.24}

	var got jsonkit.Interface[Greeter]
	if err := json.Unmarshal(data, &got); err != nil {
		panic(err)
	}

	// got == exp
	// got.V -> *TypeC{V: 42.24}
}

func TestInterface(t *testing.T) {
	t.Run("non interface type raise error on marshalling/unmarshaling", func(t *testing.T) {
		var x jsonkit.Interface[string]
		_, err := json.Marshal(x)
		assert.ErrorIs(t, err, jsonkit.ErrNotInterfaceType)
		assert.ErrorIs(t, json.Unmarshal([]byte(`"foo"`), &x), jsonkit.ErrNotInterfaceType)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeA{V: rnd.String()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("inteface type that is implemented by any primitive type", func(t *testing.T) {
		var exp jsonkit.Interface[any]
		exp.V = rnd.Int()

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with base-type based implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeD(rnd.String())

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = &TypeC{V: rnd.Float32()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = nil

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func TestArray_json(t *testing.T) {
	t.Run("concrete type", func(t *testing.T) {
		var exp jsonkit.Array[string] = random.Slice[string](rnd.IntB(3, 7), rnd.String)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			return &TypeC{V: rnd.Float32()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsonkit.Array[Greeter]
		exp = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})
		exp = append(exp, nil, nil, nil)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with various implementations", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = jsonkit.Array[Greeter]{
			TypeA{V: rnd.String()},
			TypeB{V: rnd.Int()},
			&TypeC{V: rnd.Float32()},
		}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func ExampleRegister() {
	type MyDTO struct {
		V string `json:"v"`
	}
	var ( // register types
		_ = jsonkit.Register[MyDTO]("my_dto")
	)
}

func TestRegister_doubleRegisterPanics(t *testing.T) {
	type X struct{}
	defer jsonkit.Register[X]("x")()
	panicResults := assert.Panic(t, func() { jsonkit.Register[X]("xx") })
	assert.NotNil(t, panicResults)
	out, ok := panicResults.(string)
	assert.True(t, ok, "panic value suppose to be a string")
	assert.Contains(t, out, `Unable to register "xx"`)
	assert.Contains(t, out, fmt.Sprintf("%T", X{}))
}

func TestRegister_race(t *testing.T) {
	type (
		RaceType1 struct{}
		RaceType2 struct{}
		RaceType3 struct{}
	)
	var ary = jsonkit.Array[Greeter]{TypeA{}, TypeB{}, &TypeC{}}
	testcase.Race(func() {
		t.Cleanup(jsonkit.Register[RaceType1]("race_type_1"))
	}, func() {
		t.Cleanup(jsonkit.Register[RaceType2]("race_type_2"))
	}, func() {
		t.Cleanup(jsonkit.Register[RaceType3]("race_type_3"))
	}, func() {
		data, err := json.Marshal(ary)
		assert.NoError(t, err)
		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
	})
}

func TestRegister_supportAliases(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		var (
			val  jsonkit.Interface[any]
			data = []byte(`{"__type":"integer","__value":42}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[int](t, val.V.(int), 42)
	})
	t.Run("boolean", func(t *testing.T) {
		var (
			val  jsonkit.Interface[any]
			data = []byte(`{"__type":"boolean","__value":true}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[bool](t, val.V.(bool), true)
	})
}

type Greeter interface{ Hello() }

var ( // register types
	_ = jsonkit.Register[TypeA]("type_a")
	_ = jsonkit.Register[TypeB]("type_b")
	_ = jsonkit.Register[TypeC]("type_c")
	_ = jsonkit.Register[TypeD]("type_d")
)

type TypeA struct{ V string }

func (TypeA) Hello() {}

type TypeB struct{ V int }

func (TypeB) Hello() {}

type TypeC struct{ V float32 }

func (*TypeC) Hello() {}

type TypeD string

func (str TypeD) Hello() {}

// func TestArrayStream(t *testing.T) {
// 	type ItemDTO struct {
// 		V string
// 	}
//
// 	type ExampleDTO struct {
// 		Metadata string                       `json:"metadata"`
// 		Items    jsonkit.ArrayStream[ItemDTO] `json:"items"`
// 	}
//
// 	items := []ItemDTO{
// 		{V: "1"},
// 		{V: "2"},
// 		{V: "c"},
// 		{V: "d"},
// 	}
//
// 	var dto = ExampleDTO{
// 		Metadata: "Hello, world!",
// 		Items:    jsonkit.ArrayStream[ItemDTO]{Iter: iterkit.Slice(items)},
// 	}
//
// 	var buf bytes.Buffer
// 	assert.NoError(t, json.NewEncoder(&buf).Encode(dto))
//
// 	type ExampleDTOGot struct {
// 		Metadata string    `json:"metadata"`
// 		Items    []ItemDTO `json:"items"`
// 	}
// 	var got ExampleDTOGot
// 	assert.NoError(t, json.Unmarshal(buf.Bytes(), &got))
//
// 	assert.Equal(t, ExampleDTOGot{
// 		Metadata: dto.Metadata,
// 		Items:    items,
// 	}, got)
// }
