package codec_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase/assert"
)

func TestMergeRegistry(t *testing.T) {
	type T1 struct{}
	type T2 struct{ V string }
	type T3 struct{}

	c1 := codec.CodecImpl[T1]{
		MarshalFunc: func(v T1) ([]byte, error) {
			return json.Marshal(v)
		},
		UnmarshalFunc: func(data []byte, p *T1) error {
			return json.Unmarshal(data, p)
		},
	}

	c2 := codec.CodecImpl[T2]{
		MarshalFunc: func(v T2) ([]byte, error) {
			return json.Marshal(v)
		},
		UnmarshalFunc: func(data []byte, p *T2) error {
			return json.Unmarshal(data, p)
		},
	}

	c3 := codec.CodecImpl[T3]{
		MarshalFunc: func(v T3) ([]byte, error) {
			return json.Marshal(v)
		},
		UnmarshalFunc: func(data []byte, p *T3) error {
			return json.Unmarshal(data, p)
		},
	}

	reg := codec.MergeRegistry(c1.Registry(), c2.Registry(), c3.Registry())

	assert.True(t, reg.Supports(T1{}))
	assert.True(t, reg.Supports(&T1{}))

	assert.True(t, reg.Supports(T2{}))
	assert.True(t, reg.Supports(&T2{}))

	assert.True(t, reg.Supports(T3{}))
	assert.True(t, reg.Supports(&T3{}))

	var exp = T2{V: "foo"}
	data, err := reg.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, exp)

	var got T2
	assert.NoError(t, reg.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}
