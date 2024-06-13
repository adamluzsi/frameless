package jsondto_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/dtokit/jsondto"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleArray() {
	var greeters = jsondto.Array[Greeter]{
		TypeA{V: "42"},
		TypeB{V: 42},
	}

	data, err := json.Marshal(greeters)
	if err != nil {
		panic(err)
	}

	var result jsondto.Array[Greeter]
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}

	// "result" will contain the same as the "greeters".
}

func ExampleInterface() {
	var exp = jsondto.Interface[Greeter]{
		V: &TypeC{V: 42.24},
	}

	data, err := json.Marshal(exp)
	if err != nil {
		panic(err)
	}
	// {"__type":"type_c","v":42.24}

	var got jsondto.Interface[Greeter]
	if err := json.Unmarshal(data, &got); err != nil {
		panic(err)
	}

	// got == exp
	// got.V -> *TypeC{V: 42.24}
}

func TestInterface(t *testing.T) {
	t.Run("non interface type raise error on marshalling/unmarshaling", func(t *testing.T) {
		var x jsondto.Interface[string]
		_, err := json.Marshal(x)
		assert.ErrorIs(t, err, jsondto.ErrNotInterfaceType)
		assert.ErrorIs(t, json.Unmarshal([]byte(`"foo"`), &x), jsondto.ErrNotInterfaceType)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsondto.Interface[Greeter]
		exp.V = TypeA{V: rnd.String()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("inteface type that is implemented by any primitive type", func(t *testing.T) {
		var exp jsondto.Interface[any]
		exp.V = rnd.Int()

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with base-type based implementation", func(t *testing.T) {
		var exp jsondto.Interface[Greeter]
		exp.V = TypeD(rnd.String())

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsondto.Interface[Greeter]
		exp.V = &TypeC{V: rnd.Float32()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsondto.Interface[Greeter]
		exp.V = nil

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func TestArray_json(t *testing.T) {
	t.Run("concrete type", func(t *testing.T) {
		var exp jsondto.Array[string]
		exp = random.Slice[string](rnd.IntB(3, 7), rnd.String)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Array[string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsondto.Array[Greeter]
		exp = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsondto.Array[Greeter]
		exp = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			return &TypeC{V: rnd.Float32()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsondto.Array[Greeter]
		exp = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})
		exp = append(exp, nil, nil, nil)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with various implementations", func(t *testing.T) {
		var exp jsondto.Array[Greeter]
		exp = jsondto.Array[Greeter]{
			TypeA{V: rnd.String()},
			TypeB{V: rnd.Int()},
			&TypeC{V: rnd.Float32()},
		}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func ExampleRegister() {
	type MyDTO struct {
		V string `json:"v"`
	}
	var ( // register types
		_ = jsondto.Register[MyDTO]("my_dto")
	)
}

func TestRegister_doubleRegisterPanics(t *testing.T) {
	type X struct{}
	defer jsondto.Register[X]("x")()
	panicResults := assert.Panic(t, func() { jsondto.Register[X]("xx") })
	assert.NotNil(t, panicResults)
	out, ok := panicResults.(string)
	assert.True(t, ok, "panic value suppose to be a string")
	assert.Contain(t, out, `Unable to register "xx"`)
	assert.Contain(t, out, fmt.Sprintf("%T", X{}))
}

func TestRegister_race(t *testing.T) {
	type (
		RaceType1 struct{}
		RaceType2 struct{}
		RaceType3 struct{}
	)
	var ary = jsondto.Array[Greeter]{TypeA{}, TypeB{}, &TypeC{}}
	testcase.Race(func() {
		t.Cleanup(jsondto.Register[RaceType1]("race_type_1"))
	}, func() {
		t.Cleanup(jsondto.Register[RaceType2]("race_type_2"))
	}, func() {
		t.Cleanup(jsondto.Register[RaceType3]("race_type_3"))
	}, func() {
		data, err := json.Marshal(ary)
		assert.NoError(t, err)
		var got jsondto.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
	})
}

func TestRegister_supportAliases(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		var (
			val  jsondto.Interface[any]
			data = []byte(`{"__type":"integer","__value":42}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[int](t, val.V.(int), 42)
	})
	t.Run("boolean", func(t *testing.T) {
		var (
			val  jsondto.Interface[any]
			data = []byte(`{"__type":"boolean","__value":true}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[bool](t, val.V.(bool), true)
	})
}

type Greeter interface{ Hello() }

var ( // register types
	_ = jsondto.Register[TypeA]("type_a")
	_ = jsondto.Register[TypeB]("type_b")
	_ = jsondto.Register[TypeC]("type_c")
	_ = jsondto.Register[TypeD]("type_d")
)

type TypeA struct{ V string }

func (TypeA) Hello() {}

type TypeB struct{ V int }

func (TypeB) Hello() {}

type TypeC struct{ V float32 }

func (*TypeC) Hello() {}

type TypeD string

func (str TypeD) Hello() {}
