package wfjson_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestWorkflowSetVar(t *testing.T) {
	s := testcase.NewSpec(t)

	type Money struct {
		Amount int `json:"amount"`
	}
	errNegative := errors.New("amount cannot be negative")
	codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		c := wfjson.NewCodec()
		jsonkit.CodecRegister(c, "test:money", jsonkit.TypeCodec[Money]{
			MarshalFunc: func(_ *jsonkit.Codec, m Money) ([]byte, error) {
				if m.Amount < 0 {
					return nil, errNegative
				}
				return json.Marshal(m)
			},
			UnmarshalFunc: func(_ *jsonkit.Codec, data []byte, m *Money) error {
				if err := json.Unmarshal(data, m); err != nil {
					return err
				}
				if m.Amount < 0 {
					return errNegative
				}
				return nil
			},
		})
		return c
	})

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any { return Money{Amount: 7} })
		act := func(t *testcase.T) ([]byte, error) {
			return codec.Get(t).Marshal(workflow.SetVar{Name: "v", Value: value.Get(t)})
		}

		s.Test("writes the registered type tag", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, `{"name":"v","value":{"$type":"test:money","amount":7}}`, string(data))
		})
		s.When("the registered encoder rejects the amount", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any { return Money{Amount: -1} })
			s.Then("returns its error instead of writing an untagged amount", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, errNegative)
			})
		})
		s.When("an unregistered struct holds an unregistered implementation of a non-empty interface", func(s *testcase.Spec) {
			type Item struct {
				Count fmt.Stringer `json:"count"`
				Price any          `json:"price"`
			}
			value.Let(s, func(t *testcase.T) any { return Item{Count: json.Number("3"), Price: Money{Amount: 7}} })
			s.Then("writes the unknown field as plain JSON without dropping the known sibling's tag", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, `{"name":"v","value":{"count":3,"price":{"$type":"test:money","amount":7}}}`, string(data))
			})
			s.And("the registered encoder rejects the sibling", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any { return Item{Count: json.Number("3"), Price: Money{Amount: -1}} })
				s.Then("returns its error", func(t *testcase.T) {
					_, err := act(t)
					assert.ErrorIs(t, err, errNegative)
				})
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		value := let.VarOf(s, `{"$type":"test:money","amount":7}`)
		act := func(t *testcase.T) (any, error) {
			data := []byte(`{"name":"v","value":` + value.Get(t) + `}`)
			var got workflow.SetVar
			err := codec.Get(t).Unmarshal(data, &got)
			return got.Value, err
		}

		s.Test("restores the registered type", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal[any](t, Money{Amount: 7}, got)
		})
		s.When("the registered decoder rejects the tagged amount", func(s *testcase.Spec) {
			value.LetValue(s, `{"$type":"test:money","amount":-1}`)
			s.Then("returns its error instead of reading a map", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, errNegative)
			})
		})
		s.When("a stored object has an unknown type ID", func(s *testcase.Spec) {
			value.LetValue(s, `{"$type":"retired:order","items":[{"$type":"test:money","amount":7}]}`)
			s.Then("keeps its type member as data and decodes its registered children", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, map[string]any{"$type": "retired:order", "items": []any{Money{Amount: 7}}}, got)
			})
			s.And("a registered child rejects the amount", func(s *testcase.Spec) {
				value.LetValue(s, `{"$type":"retired:order","items":[{"$type":"test:money","amount":-1}]}`)
				s.Then("returns the child's error", func(t *testcase.T) {
					_, err := act(t)
					assert.ErrorIs(t, err, errNegative)
				})
			})
		})
		s.When("a registered sequence cannot decode one of its definitions", func(s *testcase.Spec) {
			value.LetValue(s, `{"$type":"workflow:sequence","$value":[{"$type":"missing:step"}]}`)
			s.Then("returns the decoder's unknown type error instead of degrading the sequence to a map", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
			})
		})
	})
}
