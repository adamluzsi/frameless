package codec_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestMarshalFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type T struct{ V string }

	called := let.VarOf(s, false)

	fn := let.Var(s, func(t *testcase.T) codec.MarshalFunc[T] {
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

		fn.Let(s, func(t *testcase.T) codec.MarshalFunc[T] {
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

	fn := let.Var(s, func(t *testcase.T) codec.UnmarshalFunc[T] {
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

		fn.Let(s, func(t *testcase.T) codec.UnmarshalFunc[T] {
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

	impl := let.Var(s, func(t *testcase.T) codec.CodecImpl[T] {
		return codec.CodecImpl[T]{
			MarshalFunc: func(v T) ([]byte, error) {
				mCalled.Set(t, true)
				return json.Marshal(v)
			},
			UnmarshalFunc: func(data []byte, p *T) error {
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

		impl.Let(s, func(t *testcase.T) codec.CodecImpl[T] {
			i := impl.Super(t)
			i.MarshalFunc = func(v T) ([]byte, error) {
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

		impl.Let(s, func(t *testcase.T) codec.CodecImpl[T] {
			i := impl.Super(t)
			i.UnmarshalFunc = func(data []byte, p *T) error {
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
