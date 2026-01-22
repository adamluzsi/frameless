package codec_test

import (
	"encoding/json"
	"math/rand"
	"testing"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase/assert"
)

type T1 struct{ N int }
type T2 struct{ V string }
type T3 struct{ B bool }
type T4 struct{ F float64 }

type ImplT[T any] struct {
	codec.MarshalTFunc[T]
	codec.UnmarshalTFunc[T]
}

var C1 = ImplT[T1]{
	MarshalTFunc: func(v T1) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalTFunc: func(data []byte, p *T1) error {
		return json.Unmarshal(data, p)
	},
}

var C2 = ImplT[T2]{
	MarshalTFunc: func(v T2) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalTFunc: func(data []byte, p *T2) error {
		return json.Unmarshal(data, p)
	},
}

var C3 = ImplT[T3]{
	MarshalTFunc: func(v T3) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalTFunc: func(data []byte, p *T3) error {
		return json.Unmarshal(data, p)
	},
}

var C4 = ImplT[T4]{
	MarshalTFunc: func(v T4) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalTFunc: func(data []byte, p *T4) error {
		return json.Unmarshal(data, p)
	},
}

func ExampleRegister() {
	var reg codec.Bundle
	reg = codec.Register(reg, C1)
	reg = codec.Register(reg, C2)
	reg = codec.Register(reg, C3)
}

func TestRegister(t *testing.T) {
	var reg codec.Bundle
	reg = codec.Register(reg, C1)
	reg = codec.Register(reg, C2)
	reg = codec.Register(reg, C3)

	var exp1 = T1{N: 42}
	data, err := reg.Marshal(exp1)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	var got1 T1
	assert.NoError(t, reg.Unmarshal(data, &got1))
	assert.Equal(t, exp1, got1)

	var exp2 = T2{V: "foo"}
	data, err = reg.Marshal(exp2)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	var got2 T2
	assert.NoError(t, reg.Unmarshal(data, &got2))
	assert.Equal(t, exp2, got2)

	var exp3 = T3{B: rand.Intn(2) == 0}
	data, err = reg.Marshal(exp3)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var got3 T3
	assert.NoError(t, reg.Unmarshal(data, &got3))
	assert.Equal(t, exp3, got3)

	type unknown struct{}
	_, err = reg.Marshal(unknown{})
	assert.ErrorIs(t, err, codec.ErrNotSupported{})
}

func ExampleMerge() {
	var b1 codec.Bundle
	b1 = codec.Register(b1, C1)
	b1 = codec.Register(b1, C2)

	var b2 codec.Bundle
	b2 = codec.Register(b2, C3)
	b2 = codec.Register(b2, C4)

	var all codec.Bundle
	all = codec.Merge(b1, b2)

	_ = all
}

func TestMerge(t *testing.T) {
	var bundle codec.Bundle
	bundle = codec.Merge(bundle, C1)
	bundle = codec.Merge(bundle, C2)
	bundle = codec.Merge(bundle, C3)

	var exp1 = T1{N: 42}
	data, err := bundle.Marshal(exp1)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	var got1 T1
	assert.NoError(t, bundle.Unmarshal(data, &got1))
	assert.Equal(t, exp1, got1)

	var exp2 = T2{V: "foo"}
	data, err = bundle.Marshal(exp2)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	var got2 T2
	assert.NoError(t, bundle.Unmarshal(data, &got2))
	assert.Equal(t, exp2, got2)

	var exp3 = T3{B: rand.Intn(2) == 0}
	data, err = bundle.Marshal(exp3)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var got3 T3
	assert.NoError(t, bundle.Unmarshal(data, &got3))
	assert.Equal(t, exp3, got3)

	type unknown struct{}
	_, err = bundle.Marshal(unknown{})
	assert.ErrorIs(t, err, codec.ErrNotSupported{})
}
