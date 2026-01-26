package codec_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

var _ codec.Marshaler = (codec.MarshalerFunc)(nil)
var _ codec.Unmarshaler = (codec.UnmarshalerFunc)(nil)

var _ codec.TypeMarshaler[testent.Foo] = (codec.TypeMarshalerFunc[testent.Foo])(nil)
var _ codec.TypeUnmarshaler[testent.Foo] = (codec.TypeUnmarshalerFunc[testent.Foo])(nil)

var _ codec.Encoder = (codec.EncoderFunc)(nil)
var _ codec.TypeEncoder[testent.Foo] = (codec.TypeEncoderFunc[testent.Foo])(nil)

var _ codec.Decoder = (codec.DecoderFunc)(nil)
var _ codec.TypeDecoder[testent.Foo] = (codec.TypeDecoderFunc[testent.Foo])(nil)

func TestTypeMarshalerFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type T struct{ V string }

	called := let.VarOf(s, false)

	fn := let.Var(s, func(t *testcase.T) codec.TypeMarshalerFunc[T] {
		return func(v T) ([]byte, error) {
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

		fn.Let(s, func(t *testcase.T) codec.TypeMarshalerFunc[T] {
			return func(v T) ([]byte, error) {
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

	fn := let.Var(s, func(t *testcase.T) codec.TypeUnmarshalerFunc[T] {
		return func(data []byte, p *T) error {
			uCalled.Set(t, true)
			return json.Unmarshal(data, p)
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

		fn.Let(s, func(t *testcase.T) codec.TypeUnmarshalerFunc[T] {
			return func(data []byte, p *T) error {
				return expErr.Get(t)
			}
		})

		s.Then("error is returned during unmarshaling", func(t *testcase.T) {
			assert.ErrorIs(t, fn.Get(t).Unmarshal([]byte{}, &T{}), expErr.Get(t))
		})
	})
}

func TestImplement(t *testing.T) {
	s := testcase.NewSpec(t)

	type T struct{ V string }

	mCalled := let.VarOf(s, false)
	uCalled := let.VarOf(s, false)

	impl := let.Var(s, func(t *testcase.T) ImplT[T] {
		return ImplT[T]{
			MarshalTFunc: func(v T) ([]byte, error) {
				mCalled.Set(t, true)
				return json.Marshal(v)
			},
			UnmarshalTFunc: func(data []byte, p *T) error {
				uCalled.Set(t, true)
				return json.Unmarshal(data, p)
			},
		}
	})

	s.Test("happy", func(t *testcase.T) {
		var exp = T{V: t.Random.String()}
		data, err := impl.Get(t).Marshal(exp)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
		assert.True(t, mCalled.Get(t))

		var got T
		assert.NoError(t, impl.Get(t).Unmarshal(data, &got))
		assert.True(t, uCalled.Get(t))
		assert.Equal(t, exp, got)
	})

	s.When("marshaling implementation has an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		impl.Let(s, func(t *testcase.T) ImplT[T] {
			i := impl.Super(t)
			i.MarshalTFunc = func(v T) ([]byte, error) {
				return nil, expErr.Get(t)
			}
			return i
		})

		s.Then("error is returned during unmarshaling", func(t *testcase.T) {
			_, err := impl.Get(t).Marshal(T{V: t.Random.String()})
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.When("unmarshaling implementation has an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		impl.Let(s, func(t *testcase.T) ImplT[T] {
			i := impl.Super(t)
			i.UnmarshalTFunc = func(data []byte, p *T) error {
				return expErr.Get(t)
			}
			return i
		})

		s.Then("error is returned during unmarshaling", func(t *testcase.T) {
			data, err := impl.Get(t).Marshal(T{V: t.Random.String()})
			assert.NoError(t, err)
			assert.NotEmpty(t, data)
			assert.ErrorIs(t, impl.Get(t).Unmarshal(data, &T{}), expErr.Get(t))
		})
	})
}
