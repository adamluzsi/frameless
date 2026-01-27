package codec_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

var _ codec.Marshaler = (codec.MarshalerFunc)(nil)
var _ codec.Unmarshaler = (codec.UnmarshalerFunc)(nil)

var _ codec.Encoder = (codec.EncoderFunc)(nil)
var _ codec.Decoder = (codec.DecoderFunc)(nil)

func TestMarshalerFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type T struct{ V string }

	called := let.VarOf(s, false)

	fn := let.Var(s, func(t *testcase.T) codec.MarshalerFunc {
		return func(v any) ([]byte, error) {
			called.Set(t, true)
			return json.Marshal(v)
		}
	})

	s.Test("happy", func(t *testcase.T) {
		var exp = T{V: t.Random.String()}
		data, err := fn.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
		assert.True(t, called.Get(t))

		var got T
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})

	s.When("marshaling implementation has an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		fn.Let(s, func(t *testcase.T) codec.MarshalerFunc {
			return func(v any) ([]byte, error) {
				return nil, expErr.Get(t)
			}
		})

		s.Then("error is returned during unmarshaling", func(t *testcase.T) {
			_, err := fn.Get(t).Marshal(T{V: t.Random.String()})
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})
}

func TestUnmarshalFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type T struct{ V string }

	uCalled := let.VarOf(s, false)

	fn := let.Var(s, func(t *testcase.T) codec.UnmarshalerFunc {
		return func(data []byte, ptr any) error {
			uCalled.Set(t, true)
			return json.Unmarshal(data, ptr)
		}
	})

	s.Test("happy", func(t *testcase.T) {
		var exp = T{V: t.Random.String()}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var got T
		assert.NoError(t, fn.Get(t).Unmarshal(data, &got))
		assert.True(t, uCalled.Get(t))
		assert.Equal(t, exp, got)
	})

	s.When("unmarshaling implementation has an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		fn.Let(s, func(t *testcase.T) codec.UnmarshalerFunc {
			return func(data []byte, ptr any) error {
				return expErr.Get(t)
			}
		})

		s.Then("error is returned during unmarshaling", func(t *testcase.T) {
			assert.ErrorIs(t, fn.Get(t).Unmarshal([]byte{}, &T{}), expErr.Get(t))
		})
	})
}
