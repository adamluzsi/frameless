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

var C1 = codec.CodecImpl[T1]{
	MarshalFunc: func(v T1) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalFunc: func(data []byte, p *T1) error {
		return json.Unmarshal(data, p)
	},
}

var C2 = codec.CodecImpl[T2]{
	MarshalFunc: func(v T2) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalFunc: func(data []byte, p *T2) error {
		return json.Unmarshal(data, p)
	},
}

var C3 = codec.CodecImpl[T3]{
	MarshalFunc: func(v T3) ([]byte, error) {
		return json.Marshal(v)
	},
	UnmarshalFunc: func(data []byte, p *T3) error {
		return json.Unmarshal(data, p)
	},
}

func ExampleRegister() {
	reg := codec.NewRegistry()
	reg = codec.Register(reg, C1)
	reg = codec.Register(reg, C2)
	reg = codec.Register(reg, C3)
}

func TestRegister(t *testing.T) {
	reg := codec.NewRegistry()
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
	assert.ErrorIs(t, err, codec.ErrNotSupported)
}
