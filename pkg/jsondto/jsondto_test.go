package jsondto_test

import (
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/jsondto"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
	"testing"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleTyped() {

}

func TestTyped_json(t *testing.T) {
	t.Run("concrete type", func(t *testing.T) {
		var exp jsondto.Typed[string]
		exp.V = rnd.String()

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Typed[string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsondto.Typed[Greeter]
		exp.V = TypeA{V: rnd.String()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Typed[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsondto.Typed[Greeter]
		exp.V = &TypeC{V: rnd.Float32()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Typed[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsondto.Typed[Greeter]
		exp.V = nil

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsondto.Typed[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

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

func TestList_json(t *testing.T) {
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
	assert.Contain(t, out, "Unable to register xx")
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

type Greeter interface{ Hello() }

var ( // register types
	_ = jsondto.Register[TypeA]("type_a")
	_ = jsondto.Register[TypeB]("type_b")
	_ = jsondto.Register[TypeC]("type_c")
)

type TypeA struct{ V string }

func (TypeA) Hello() {}

type TypeB struct{ V int }

func (TypeB) Hello() {}

type TypeC struct{ V float32 }

func (*TypeC) Hello() {}
