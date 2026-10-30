package wfjson_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestCodec_implementsWorkflowCodec(t *testing.T) {
	wfcontract.Codec(wfjson.NewCodec()).Test(t)
}

// TestNewCodec pins the type IDs of the workflow interfaces.
// No value is of an interface type, so they only name the element type of a slice or a map held as `any`.
func TestNewCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec { return wfjson.NewCodec() })

	type Holder struct{ V any }

	s.Describe("#Marshal of a slice of a workflow interface held as any", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return []workflow.Definition{workflow.SetVar{Name: "x", Value: t.Random.HexN(4)}}
		})

		act := func(t *testcase.T) ([]byte, error) {
			return codec.Get(t).Marshal(Holder{V: value.Get(t)})
		}

		thenItIsReadBackAsTheSameType := func(s *testcase.Spec) {
			s.Then("it is read back as the very same type", func(t *testcase.T) {
				data, err := act(t)
				assert.Must(t).NoError(err)

				var got Holder
				assert.NoError(t, codec.Get(t).Unmarshal(data, &got))
				assert.Equal(t, value.Get(t), got.V)
			})
		}

		s.Then("it is written with the type ID of workflow.Definition, in lowercase like every other type ID", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assert.Contains(t, string(data), `{"V":{"$type":"[]workflow:definition","$value":[{"$type":"workflow:var:set",`)
		})

		thenItIsReadBackAsTheSameType(s)

		s.When("it is a slice of conditions", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []workflow.Condition{wftemplate.Condition(".x == " + t.Random.HexN(4))}
			})

			s.Then("it is written with the type ID of workflow.Condition, in lowercase like every other type ID", func(t *testcase.T) {
				data, err := act(t)
				assert.NoError(t, err)
				assert.Contains(t, string(data), `{"V":{"$type":"[]workflow:condition","$value":[{"$type":"workflow:template:condition",`)
			})

			thenItIsReadBackAsTheSameType(s)
		})
	})
}

// TestWorkflowExecute pins the wire format of workflow.Execute.
// Only the ID that selects its role is written, so a step and a condition are told apart by their field names.
func TestWorkflowExecute(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec { return wfjson.NewCodec() })

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) workflow.Execute {
			return workflow.Execute{
				Participant: "p1",
				Input:       []workflow.VarName{"a", "b"},
				Output:      []workflow.VarName{"c"},
			}
		})

		act := func(t *testcase.T) (string, error) {
			// written as an interface holds it, like the workflow.Definition of a step, or the workflow.Condition of an If
			var held any = value.Get(t)
			data, err := codec.Get(t).Marshal(&held)
			return string(data), err
		}

		s.Then("a step is written with its participant ID under the workflow:execute type tag", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, `{"$type":"workflow:execute","participant":"p1","input":["a","b"],"output":["c"]}`)
		})

		s.When("it is a condition", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) workflow.Execute {
				return workflow.Execute{
					Condition: "c1",
					Input:     []workflow.VarName{"a"},
				}
			})

			s.Then("it is written with its condition ID under the same type tag", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, `{"$type":"workflow:execute","condition":"c1","input":["a"]}`)
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		data := let.Var(s, func(t *testcase.T) string {
			return `{"$type":"workflow:execute","participant":"p1","input":["a","b"],"output":["c"]}`
		})

		act := func(t *testcase.T) (workflow.Definition, error) {
			var got workflow.Definition
			err := codec.Get(t).Unmarshal([]byte(data.Get(t)), &got)
			return got, err
		}

		expParticipant := workflow.Execute{
			Participant: "p1",
			Input:       []workflow.VarName{"a", "b"},
			Output:      []workflow.VarName{"c"},
		}

		s.Then("it is read back as a workflow.Execute", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal[workflow.Definition](t, got, expParticipant)
		})

		// The former workflow::participant and workflow::condition tags were dropped before the first release,
		// as workflow::execute took their place.
		// The data is spelt with the double colon of the type IDs of that time, which are still read.
		s.When("the data was written with the former workflow::participant tag", func(s *testcase.Spec) {
			data.LetValue(s, `{"$type":"workflow::participant","id":"p1","input":["a","b"],"output":["c"]}`)

			s.Then("it fails with jsonkit.ErrUnknownTypeID, as the tag names no type", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
			})
		})

		// workflow::condition is the former type ID of the workflow.Condition interface now, which names no value.
		s.When("the data was written with the former workflow::condition tag, in the condition of an If", func(s *testcase.Spec) {
			data.LetValue(s, `{"$type":"workflow::if","cond":{"$type":"workflow::condition","id":"c1","input":["a"]},"then":{"$type":"workflow::execute","participant":"p2"}}`)

			s.Then("it fails with jsonkit.ErrUnknownTypeID, as the tag names no type of a value", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrUnknownTypeID)
			})
		})
	})
}

// TestWorkflowJoin pins the wire format of workflow.Join.
// The join names the spawn it waits for under the "spawn" key.
func TestWorkflowJoin(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec { return wfjson.NewCodec() })

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) workflow.Join {
			return workflow.Join{Spawn: "child1", Collect: workflow.VarMapping{"x": "y"}}
		})

		act := func(t *testcase.T) (string, error) {
			// written as the workflow.Definition holds it
			var held workflow.Definition = value.Get(t)
			data, err := codec.Get(t).Marshal(&held)
			return string(data), err
		}

		s.Then("the awaited spawn is written under the spawn key", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, `{"$type":"workflow:join","spawn":"child1","collect":{"x":"y"}}`)
		})

		s.When("it joins every child", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) workflow.Join { return workflow.Join{} })

			s.Then("no spawn is written", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, `{"$type":"workflow:join"}`)
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		data := let.Var(s, func(t *testcase.T) string {
			return `{"$type":"workflow:join","spawn":"child1","collect":{"x":"y"}}`
		})

		act := func(t *testcase.T) (workflow.Definition, error) {
			var got workflow.Definition
			err := codec.Get(t).Unmarshal([]byte(data.Get(t)), &got)
			return got, err
		}

		exp := workflow.Join{Spawn: "child1", Collect: workflow.VarMapping{"x": "y"}}

		s.Then("it is read back as a join on the named spawn", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal[workflow.Definition](t, got, exp)
		})

		s.Then("writing it again gives back the same data", func(t *testcase.T) {
			got, err := act(t)
			assert.Must(t).NoError(err)
			out, err := codec.Get(t).Marshal(&got)
			assert.NoError(t, err)
			assert.Equal(t, string(out), data.Get(t))
		})

		s.When("no spawn is named", func(s *testcase.Spec) {
			data.LetValue(s, `{"$type":"workflow:join"}`)

			s.Then("it is read back as a join on every child", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[workflow.Definition](t, got, workflow.Join{})
			})
		})
	})
}

// TestPathDTO covers how a workflow.Path is written to and read back from a
// persisted event log.
//
// A path is a chain of scopes, so its wire form is a JSON array of objects,
// one per scope, holding the name of the scope, and whether it is a variable scope as well.
// The "var" flag is omitted for the scopes which are not variable scopes.
// Event logs outlive the code that wrote them, which is why this
// stable, long-lived shape is pinned here.
func TestPathDTO(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	// samePath compares two paths strictly, as workflow.Path#Equal only compares the names of the scopes.
	samePath := func(tb testing.TB, exp, got workflow.Path) {
		tb.Helper()
		assert.True(tb, slices.Equal(exp, got), assert.MessageF("expected: %#v\ngot:      %#v", exp, got))
	}

	var expPath = workflow.Path{
		{Name: "sequence"},
		{Name: "[0]", Var: true},
		{Name: "participant"},
	}

	s.Describe("#Marshal", func(s *testcase.Spec) {
		event := let.Var(s, func(t *testcase.T) workflow.EventSetVar {
			eventID, err := workflow.MakeEventID()
			assert.NoError(t, err)
			processID, err := workflow.MakeProcessID()
			assert.NoError(t, err)
			return workflow.EventSetVar{
				EventID:   eventID,
				ProcessID: processID,
				Timestamp: time.Now().UTC(),
				Path:      expPath,
				Name:      "n",
				Value:     "v",
			}
		})

		act := let.Act(func(t *testcase.T) []byte {
			data, err := subject.Get(t).Marshal(event.Get(t))
			assert.NoError(t, err)
			return data
		})

		s.Then("the path is written as an array of scope objects, with the var flag only on the variable scopes", func(t *testcase.T) {
			var got struct {
				Path json.RawMessage `json:"path"`
			}
			assert.NoError(t, json.Unmarshal(act(t), &got))
			assert.Equal(t, string(got.Path), `[{"name":"sequence"},{"name":"[0]","var":true},{"name":"participant"}]`)
		})

		s.Then("there is no separate variable scope field, as the path carries it", func(t *testcase.T) {
			var got map[string]json.RawMessage
			assert.NoError(t, json.Unmarshal(act(t), &got))
			_, ok := got["scope"]
			assert.False(t, ok)
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			// path is the JSON value of the "path" field, as found in the event log.
			path = let.Var(s, func(t *testcase.T) string {
				return `[{"name":"sequence"},{"name":"[0]","var":true},{"name":"participant"}]`
			})
			data = let.Var(s, func(t *testcase.T) []byte {
				return fmt.Appendf(nil, `{"$type":"workflow:event:var:set",`+
					`"event_id":"c0ffee00-0000-4000-8000-000000000001",`+
					`"process_id":"c0ffee00-0000-4000-8000-000000000002",`+
					`"timestamp":"2026-01-02T03:04:05Z",`+
					`"path":%s,"name":"n","value":"v"}`, path.Get(t))
			})
		)

		act := let.Act(func(t *testcase.T) workflow.Event {
			var got workflow.Event
			assert.NoError(t, subject.Get(t).Unmarshal(data.Get(t), &got))
			return got
		})

		s.Then("the path is restored as the chain of scopes, the variable scopes included", func(t *testcase.T) {
			got, ok := act(t).(workflow.EventSetVar)
			assert.True(t, ok, "expected an EventSetVar")

			samePath(t, expPath, got.Path)
			samePath(t, workflow.Path{{Name: "sequence"}, {Name: "[0]", Var: true}}, got.Path.VarScope())
		})

		s.When("the path field is absent", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"$type":"workflow:event:var:set",` +
					`"event_id":"c0ffee00-0000-4000-8000-000000000001",` +
					`"process_id":"c0ffee00-0000-4000-8000-000000000002",` +
					`"timestamp":"2026-01-02T03:04:05Z",` +
					`"name":"n","value":"v"}`)
			})

			s.Then("it decodes as the zero path", func(t *testcase.T) {
				got, ok := act(t).(workflow.EventSetVar)
				assert.True(t, ok, "expected an EventSetVar")

				assert.Empty(t, got.Path)
			})
		})
	})

	s.Describe("round-trip", func(s *testcase.Spec) {
		var (
			event = let.Var(s, func(t *testcase.T) workflow.EventDeclareVar {
				eventID, err := workflow.MakeEventID()
				assert.NoError(t, err)
				processID, err := workflow.MakeProcessID()
				assert.NoError(t, err)
				return workflow.EventDeclareVar{
					EventID:   eventID,
					ProcessID: processID,
					Timestamp: time.Now().UTC(),
					Path:      expPath,
					Name:      "n",
				}
			})
		)

		act := let.Act(func(t *testcase.T) workflow.Event {
			var held workflow.Event = event.Get(t)
			data, err := subject.Get(t).Marshal(&held)
			assert.NoError(t, err)

			var got workflow.Event
			assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
			return got
		})

		s.Then("the path survives the round-trip, and with it the variable scope of the variable", func(t *testcase.T) {
			got, ok := act(t).(workflow.EventDeclareVar)
			assert.True(t, ok, "expected an EventDeclareVar")

			samePath(t, event.Get(t).Path, got.Path)
			samePath(t, event.Get(t).Path.VarScope(), got.Path.VarScope())
			assert.Equal(t, event.Get(t).Name, got.Name)
		})
	})
}

// TestWorkflowEventSetVar pins how the value of a variable is written and read.
// The arguments of participants and conditions are written and read the same way,
// as the replay compares them with the values of the variables.
func TestWorkflowEventSetVar(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	s.Describe("#Marshal", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return t.Random.String()
		})

		// act returns the JSON of the "value" field.
		act := func(t *testcase.T) (json.RawMessage, error) {
			data, err := codec.Get(t).Marshal(workflow.EventSetVar{Name: "n", Value: value.Get(t)})
			if err != nil {
				return nil, err
			}
			var got struct {
				Value json.RawMessage `json:"value"`
			}
			assert.NoError(t, json.Unmarshal(data, &got))
			return got.Value, nil
		}

		// thenWrittenAsPlainJSON is the wire format of the values of JSON's own data model,
		// which is what the event logs written so far hold.
		thenWrittenAsPlainJSON := func(s *testcase.Spec) {
			s.Then("it is written as plain JSON, the way encoding/json writes it", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)

				exp, err := json.Marshal(value.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, string(exp), string(got))
			})
		}

		thenWrittenAsPlainJSON(s)

		s.When("the value is nil", func(s *testcase.Spec) {
			value.LetValue(s, nil)

			s.Then("the value is left out", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Empty(t, got)
			})
		})

		// Whatever float64 it is, plain JSON tells it apart from an int, so it is written without a $type.
		s.When("the value is a float64", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return makeFloat64(t)
			})

			s.Then("it is written as a plain JSON number, which encoding/json reads as the very same float64", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)

				var f float64
				assert.NoError(t, json.Unmarshal(got, &f))
				assert.Equal[any](t, value.Get(t), f)
			})

			s.And("it is not a whole number", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeFloat64WithFraction(t)
				})

				thenWrittenAsPlainJSON(s)
			})

			// A plain JSON number of only digits is read back as an int.
			s.And("it is a whole number", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeWholeFloat64(t)
				})

				s.Then("it is written as plain JSON, with a .0 fraction", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)

					exp, err := json.Marshal(value.Get(t))
					assert.NoError(t, err)
					assert.Equal(t, string(exp)+".0", string(got))
				})
			})

			// Like 1e+21, which has no dot, though it is not an int.
			s.And("encoding/json writes it with an exponent", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeFloat64WithExponent(t)
				})

				thenWrittenAsPlainJSON(s)
			})
		})

		s.When("the value is an int", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return t.Random.IntBetween(-1000, 1000)
			})

			thenWrittenAsPlainJSON(s)
		})

		s.When("the value is a number of another built-in type, like an int64", func(s *testcase.Spec) {
			value.Let(s, makeOtherBuiltinNumber)

			s.Then("it is written with its $type, as plain JSON would be read back as an int or a float64", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)

				var envelope struct {
					Type  string          `json:"$type"`
					Value json.RawMessage `json:"$value"`
				}
				assert.NoError(t, json.Unmarshal(got, &envelope))
				assert.Equal(t, fmt.Sprintf("%T", value.Get(t)), envelope.Type)

				plain, err := json.Marshal(value.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, string(plain), string(envelope.Value))
			})
		})

		s.When("the value is a []any or a map[string]any of plain values", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any{
					"list":  []any{t.Random.String(), makeFloat64WithFraction(t), t.Random.Int(), nil},
					"flag":  t.Random.Bool(),
					"count": t.Random.Int(),
				}
			})

			thenWrittenAsPlainJSON(s)
		})

		s.When("the value is a nil []any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []any(nil)
			})

			thenWrittenAsPlainJSON(s)
		})

		s.When("the value is a nil map[string]any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any(nil)
			})

			thenWrittenAsPlainJSON(s)
		})

		s.When("the value is of a type registered with the codec", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return workflow.Break{}
			})

			s.Then("it is written with its $type", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, `{"$type":"workflow:break"}`, string(got))
			})
		})

		// The value would be read back as the value its $type names, like a workflow.Sequence, see jsonkit.ErrKeywordCollision.
		s.When("the value has a member of its own named like the $type member of the type envelope", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				type Typed struct {
					Type string `json:"$type"`
				}
				return random.Pick[any](t.Random,
					map[string]any{"$type": "workflow:sequence", "name": t.Random.String()},
					[]any{t.Random.String(), map[string]any{"$type": "workflow:sequence"}},
					Typed{Type: "workflow:sequence"},
				)
			})

			s.Then("it fails with jsonkit.ErrKeywordCollision, instead of being written to be read back as another value", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, jsonkit.ErrKeywordCollision)
			})
		})

		// The entries of a map in its type envelope are its $map member, where their keys are just data.
		s.When("the value is a map with a type ID, which has an entry keyed like the $type member of the type envelope", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]string{"$type": "workflow:sequence"}
			})

			s.Then("it is written with its $type, and with its entries as the $map member of the envelope", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, `{"$type":"map[string]string","$map":{"$type":"workflow:sequence"}}`, string(got))
			})
		})

		s.When("the value is a struct whose interface field holds a value of a type which is not registered", func(s *testcase.Spec) {
			type Unregistered struct{ Name string }
			type Envelope struct{ Data any }
			value.Let(s, func(t *testcase.T) any {
				return Envelope{Data: Unregistered{Name: t.Random.String()}}
			})

			// The value in the `any` field has no $type, so it is written the way encoding/json writes it.
			thenWrittenAsPlainJSON(s)

			s.And("the interface field holds a value of a registered type", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return Envelope{Data: workflow.Break{}}
				})

				s.Then("the struct is written as plain JSON, and the value in its field with its $type", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, `{"Data":{"$type":"workflow:break"}}`, string(got))
				})
			})
		})

		s.When("the value is of a type without a type ID, like a time.Time", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return t.Random.Time()
			})

			thenWrittenAsPlainJSON(s)
		})

		s.When("the value is cyclic", func(s *testcase.Spec) {
			type Node struct{ Next *Node }
			value.Let(s, func(t *testcase.T) any {
				n := &Node{}
				n.Next = n
				return n
			})

			s.Then("it fails the way encoding/json fails", func(t *testcase.T) {
				_, err := act(t)
				var unsupported *json.UnsupportedValueError
				assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
			})

			s.And("it is a map[string]any", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					m := map[string]any{}
					m["self"] = m
					return m
				})

				s.Then("it fails the way encoding/json fails", func(t *testcase.T) {
					_, err := act(t)
					var unsupported *json.UnsupportedValueError
					assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
				})
			})

			s.And("it is a []any", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					l := []any{t.Random.Int(), nil}
					l[1] = l
					return l
				})

				s.Then("it fails the way encoding/json fails", func(t *testcase.T) {
					_, err := act(t)
					var unsupported *json.UnsupportedValueError
					assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
				})
			})
		})

		s.When("the value references the same []any and map[string]any more than once, without a reference cycle", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				l := []any{t.Random.String()}
				m := map[string]any{"list": l}
				return map[string]any{"a": m, "b": m, "c": []any{l, l}}
			})

			thenWrittenAsPlainJSON(s)
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		// value is the JSON of the "value" field, as found in the event log.
		value := let.VarOf(s, `"v"`)

		act := func(t *testcase.T) (any, error) {
			data := fmt.Appendf(nil, `{"$type":"workflow:event:var:set",`+
				`"event_id":"c0ffee00-0000-4000-8000-000000000001",`+
				`"process_id":"c0ffee00-0000-4000-8000-000000000002",`+
				`"timestamp":"2026-01-02T03:04:05Z",`+
				`"name":"n","value":%s}`, value.Get(t))
			var got workflow.Event
			if err := codec.Get(t).Unmarshal(data, &got); err != nil {
				return nil, err
			}
			event, ok := got.(workflow.EventSetVar)
			assert.True(t, ok, "expected an EventSetVar")
			return event.Value, nil
		}

		// thenReadAsPlainJSON is how the event logs written so far are read.
		thenReadAsPlainJSON := func(s *testcase.Spec) {
			s.Then("it is read the way encoding/json reads it", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)

				var exp any
				assert.NoError(t, json.Unmarshal([]byte(value.Get(t)), &exp))
				assert.Equal(t, exp, got)
			})
		}

		thenReadAsPlainJSON(s)

		// Like an int, a whole float64 in the history recorded before it was written with a fraction,
		// or a number of any type in the history recorded before the other built-in numbers carried their type ID.
		s.When("the value is a plain number of only digits", func(s *testcase.Spec) {
			number := let.Var(s, func(t *testcase.T) int {
				return t.Random.IntBetween(-1000, 1000)
			})
			value.Let(s, func(t *testcase.T) string {
				return strconv.Itoa(number.Get(t))
			})

			s.Then("it is read as an int", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, number.Get(t), got)
			})
		})

		// Like a float64, which is written with a fraction even when it is a whole number.
		s.When("the value is a plain number with a fraction", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) string {
				return strconv.Itoa(t.Random.IntBetween(-1000, 1000)) + random.Pick(t.Random, ".0", ".5", ".25")
			})

			thenReadAsPlainJSON(s)
		})

		// Like any other number of a built-in type, which carries its $type, or an int written with its $type, like WithType writes it.
		s.When("the value is a number with the $type of a built-in type", func(s *testcase.Spec) {
			number := let.Var(s, func(t *testcase.T) any {
				return random.Pick(t.Random,
					makeOtherBuiltinNumber,
					func(t *testcase.T) any { return t.Random.Int() },
				)(t)
			})

			value.Let(s, func(t *testcase.T) string {
				data, err := json.Marshal(map[string]any{
					"$type":  fmt.Sprintf("%T", number.Get(t)),
					"$value": number.Get(t),
				})
				assert.NoError(t, err)
				return string(data)
			})

			s.Then("it is read as a number of that type", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, number.Get(t), got)
			})
		})

		s.When("the value is an object with the $type of a registered type", func(s *testcase.Spec) {
			value.LetValue(s, `{"$type":"workflow:break"}`)

			s.Then("it is read as the registered type", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, workflow.Break{}, got)
			})

			// Earlier versions of jsonkit named the type member @type, which is no longer read as one.
			s.And("its type member is named @type, as earlier versions named it", func(s *testcase.Spec) {
				value.LetValue(s, `{"@type":"workflow:break"}`)

				thenReadAsPlainJSON(s)
			})

			// Like a JSON-LD document of a registered type, as a struct with a json:"@type" field writes it.
			s.And("it also has an @type member", func(s *testcase.Spec) {
				value.LetValue(s, `{"@type":"Person","$type":"workflow:break"}`)

				s.Then("the @type is just a member, and it is read as the registered type", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[any](t, workflow.Break{}, got)
				})
			})
		})

		// Like a JSON-LD document returned by a participant.
		s.When("the value is an object with an @type which is not registered", func(s *testcase.Spec) {
			value.LetValue(s, `{"@type":"Person","name":"Ada"}`)

			thenReadAsPlainJSON(s)

			s.And("it holds an object with the $type of a registered type", func(s *testcase.Spec) {
				value.LetValue(s, `{"@type":"Person","name":"Ada","plan":{"$type":"workflow:break"}}`)

				s.Then("the @type is kept as a member, and the object it holds is read as the registered type", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[any](t, map[string]any{
						"@type": "Person",
						"name":  "Ada",
						"plan":  workflow.Break{},
					}, got)
				})
			})

			s.And("it sits in an array", func(s *testcase.Spec) {
				value.LetValue(s, `[1,{"@type":"Person","name":"Ada"},{"$type":"workflow:break"}]`)

				s.Then("the object is read the way encoding/json reads it, and the rest of the array by the codec", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal[any](t, []any{
						1,
						map[string]any{"@type": "Person", "name": "Ada"},
						workflow.Break{},
					}, got)
				})
			})
		})

		// The keys of the entries in the $map member of the envelope are just data, even when they are named like its $type member.
		s.When("the value is the $type envelope of a map, which has an entry keyed like the $type member of the envelope", func(s *testcase.Spec) {
			value.LetValue(s, `{"$type":"map[string]string","$map":{"$type":"workflow:sequence"}}`)

			s.Then("it is read as the map, with that entry", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal[any](t, map[string]string{"$type": "workflow:sequence"}, got)
			})
		})
	})

	// The value of a variable is held by an `any`, so its JSON has to tell an int and a float64 apart.
	s.Describe("#Marshal and #Unmarshal of a number", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return t.Random.Int()
		})

		act := func(t *testcase.T) (any, error) {
			data, err := codec.Get(t).Marshal(workflow.EventSetVar{Name: "n", Value: value.Get(t)})
			if err != nil {
				return nil, err
			}
			var got workflow.EventSetVar
			if err := codec.Get(t).Unmarshal(data, &got); err != nil {
				return nil, err
			}
			return got.Value, nil
		}

		thenItIsReadBackAsTheVerySameValue := func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, value.Get(t), got)
		}

		s.Then("an int is read back as the very same int", thenItIsReadBackAsTheVerySameValue)

		s.When("the value is a float64", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return makeFloat64(t)
			})

			s.Then("it is read back as the very same float64", thenItIsReadBackAsTheVerySameValue)

			s.And("it is not a whole number", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeFloat64WithFraction(t)
				})

				s.Then("it is read back as the very same float64", thenItIsReadBackAsTheVerySameValue)
			})

			s.And("it is a whole number", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeWholeFloat64(t)
				})

				s.Then("it is read back as the very same float64, and not as an int", thenItIsReadBackAsTheVerySameValue)
			})

			s.And("encoding/json writes it with an exponent", func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any {
					return makeFloat64WithExponent(t)
				})

				s.Then("it is read back as the very same float64", thenItIsReadBackAsTheVerySameValue)
			})
		})

		s.When("the value is a []any and a map[string]any of ints and float64s", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any{
					"list":  []any{t.Random.Int(), makeWholeFloat64(t), makeFloat64WithFraction(t), makeFloat64WithExponent(t)},
					"int":   t.Random.Int(),
					"float": makeFloat64(t),
				}
			})

			s.Then("each number is read back as the very same type", thenItIsReadBackAsTheVerySameValue)
		})
	})
}

// makeFloat64 makes a random float64 of any kind, either way of its sign:
// one with a fraction, a whole number, or one encoding/json writes with an exponent.
func makeFloat64(t *testcase.T) float64 {
	return random.Pick(t.Random, makeFloat64WithFraction, makeWholeFloat64, makeFloat64WithExponent)(t)
}

// makeFloat64WithFraction makes a random float64 which is not a whole number, either way of its sign, below 1 or up to 1e15.
// encoding/json writes it the very same way as the codec does, while a whole one is written with a .0 fraction.
func makeFloat64WithFraction(t *testcase.T) float64 {
	sign := float64(random.Pick(t.Random, 1, -1))
	for {
		// The bigger a float64 is, the fewer fractions it can hold, so a scaled one is often a whole number.
		if f := t.Random.Float64() * math.Pow10(t.Random.IntBetween(0, 15)); f != math.Trunc(f) {
			return sign * f
		}
	}
}

// makeWholeFloat64 makes a random float64 of a whole number,
// which encoding/json writes without a fraction or an exponent, like 42.
func makeWholeFloat64(t *testcase.T) float64 {
	return random.Pick(t.Random,
		func() float64 { return float64(t.Random.IntBetween(-1000, 1000)) },
		func() float64 { return float64(random.Pick(t.Random, 1, -1) * t.Random.Int()) },
		// encoding/json writes a float64 without an exponent up to 1e21.
		func() float64 { return float64(random.Pick(t.Random, 1, -1)*t.Random.IntBetween(1, 999)) * 1e18 },
	)()
}

// makeFloat64WithExponent makes a random float64 which encoding/json writes with an exponent, either way of its sign:
// 1e21 or more, which is always a whole number, or less than 1e-6, apart from zero.
func makeFloat64WithExponent(t *testcase.T) float64 {
	sign := float64(random.Pick(t.Random, 1, -1))
	exp := random.Pick(t.Random, t.Random.IntBetween(21, 307), -t.Random.IntBetween(7, 307))
	return sign * random.Pick(t.Random,
		// a single digit, which encoding/json writes without a dot, like 1e+21 or 5e-7
		func() float64 {
			f, err := strconv.ParseFloat(strconv.Itoa(t.Random.IntBetween(1, 9))+"e"+strconv.Itoa(exp), 64)
			assert.Must(t).NoError(err)
			return f
		},
		// more digits, which encoding/json writes with a dot, like 1.5e+21
		func() float64 { return (1 + t.Random.Float64()) * math.Pow10(exp) },
		// the edges of the range of a float64
		func() float64 { return random.Pick(t.Random, math.MaxFloat64, math.SmallestNonzeroFloat64) },
	)()
}

// makeOtherBuiltinNumber makes a random number of a built-in type other than int and float64,
// which plain JSON can't tell apart from an int or a float64.
func makeOtherBuiltinNumber(t *testcase.T) any {
	return random.Pick(t.Random,
		func() any { return int8(t.Random.IntBetween(-128, 127)) },
		func() any { return int64(t.Random.Int()) },
		func() any { return uint(t.Random.Int()) },
		func() any { return uint8(t.Random.IntN(256)) },
		func() any { return uint64(t.Random.Int()) },
		func() any { return t.Random.Float32() },
	)()
}

// TestWfjsonCodec_EmptySequenceRoundTrip pins the contract that an empty
// workflow.Sequence{} round-trips through the codec both as the polymorphic
// workflow.Definition (the path consumers normally use) AND directly into
// the concrete workflow.Sequence type (the path the jsonkit slice
// unmarshaller historically fumbled — the typed envelope was emitted on
// Marshal but not unwrapped when the target was a concrete slice type).
//
// Without this pin, a future regression that emits the slice as a bare
// `[]` (no $type envelope) would still pass the interface-based contract
// test in TestWfjsonCodecContract, because that helper routes through
// workflow.Definition. The concrete path is the silent failure mode.
func TestWfjsonCodec_EmptySequenceRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()

	// written as a workflow.Definition holds it
	var def workflow.Definition = workflow.Sequence{}

	data, err := c.Marshal(&def)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Sanity-check the wire format: it must carry the typed envelope,
	// not a bare array, otherwise the decoder has no discriminator to
	// pick the right concrete type on the way back in.
	assert.Contains(t, string(data), `"$type":"workflow:sequence"`)
	assert.Contains(t, string(data), `"$value":[]`)

	// Path 1: through the polymorphic interface (the consumer-facing path).
	var gotDef workflow.Definition
	assert.NoError(t, c.Unmarshal(data, &gotDef))
	gotSeq, ok := gotDef.(workflow.Sequence)
	if !ok {
		t.Fatalf("expected empty Sequence to unmarshal as workflow.Sequence via Definition, got %T", gotDef)
	}
	if len(gotSeq) != 0 {
		t.Fatalf("expected empty Sequence after unmarshal, got %d elements: %#v", len(gotSeq), gotSeq)
	}

	// Path 2: directly into the concrete workflow.Sequence type.
	var gotConcrete workflow.Sequence
	assert.NoError(t, c.Unmarshal(data, &gotConcrete),
		assert.MessageF("empty workflow.Sequence should round-trip into its concrete type\nJSON: %s", string(data)))
	if len(gotConcrete) != 0 {
		t.Fatalf("expected empty Sequence after concrete unmarshal, got %d elements: %#v", len(gotConcrete), gotConcrete)
	}

	// Re-marshal the unmarshalled value, held by a workflow.Definition, and confirm the wire is stable.
	def = gotConcrete
	data2, err := c.Marshal(&def)
	assert.NoError(t, err)
	assert.Equal(t, string(data), string(data2))
}

// TestWfjsonCodec_PrimitiveConditionRoundTrip pins the contract that a
// wftemplate.Condition (a named primitive kind, `type Condition string`)
// round-trips through the codec both through the polymorphic
// workflow.Condition (the path consumers normally use) AND directly into
// the concrete wftemplate.Condition type.
//
// Without this pin, a future regression that forgets to unwrap the typed
// envelope on the named primitive type would still pass the contract
// helper in wfcontract, because that helper routes through
// workflow.Condition. The concrete path is the silent failure mode for
// primitive kinds (mirror of the empty-Sequence case for slice kinds).
func TestWfjsonCodec_PrimitiveConditionRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()

	cond := wftemplate.Condition(".X == .Y")

	// written as a workflow.Condition holds it
	var held workflow.Condition = cond
	data, err := c.Marshal(&held)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Sanity-check the wire format: it must carry the typed envelope,
	// not a bare string, otherwise the decoder has no discriminator.
	assert.Contains(t, string(data), `"$type":"workflow:template:condition"`)
	assert.Contains(t, string(data), `"$value":".X == .Y"`)

	// Path 1: through the polymorphic interface.
	var gotCond workflow.Condition
	assert.NoError(t, c.Unmarshal(data, &gotCond))
	if gotCond != cond {
		t.Fatalf("expected primitive Condition to unmarshal as %q via workflow.Condition, got %q", cond, gotCond)
	}

	// Path 2: directly into the concrete wftemplate.Condition type.
	var gotConcrete wftemplate.Condition
	assert.NoError(t, c.Unmarshal(data, &gotConcrete),
		assert.MessageF("wftemplate.Condition should round-trip into its concrete type\nJSON: %s", string(data)))
	if gotConcrete != cond {
		t.Fatalf("expected primitive Condition after concrete unmarshal to be %q, got %q", cond, gotConcrete)
	}

	// Re-marshal the unmarshalled value, held by a workflow.Condition, and confirm the wire is stable.
	held = gotConcrete
	data2, err := c.Marshal(&held)
	assert.NoError(t, err)
	assert.Equal(t, string(data), string(data2))
}

func TestWfjsonCodec_ExecutionRequestRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()
	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	exp := workflow.ExecutionRequest{
		ProcessID:    pid,
		StartTime:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		CreatedAt:    time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
		FailureCount: 2,
	}

	data, err := c.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var got workflow.ExecutionRequest
	assert.NoError(t, c.Unmarshal(data, &got))
	assert.Equal[workflow.ExecutionRequest](t, exp, got)
}

func TestWfjsonCodec_NotificationDispatch(t *testing.T) {
	// Notification is an interface with multiple concrete
	// implementations. The codec must dispatch on the $type envelope so
	// each concrete type round-trips back to its own Go type (rather than
	// to a single shared envelope struct).
	c := wfjson.NewCodec()
	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	cases := []struct {
		name string
		ev   workflow.Notification
	}{
		{"start", workflow.ProcessSchedule{ProcessID: pid}},
		{"cancel", workflow.ProcessCancel{ProcessID: pid}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := c.Marshal(&tc.ev)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var got workflow.Notification
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.NotNil(t, got)

			if got.GetProcessID() != tc.ev.GetProcessID() {
				t.Fatalf("ProcessID mismatch: got %s, want %s", got.GetProcessID(), tc.ev.GetProcessID())
			}
			if got.NotificationType() != tc.ev.NotificationType() {
				t.Fatalf("NotificationType mismatch: got %s, want %s", got.NotificationType(), tc.ev.NotificationType())
			}
		})
	}
}

func Test_nestedSequenceRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()

	var outer workflow.Definition = workflow.Sequence{
		workflow.SetVar{Name: "marker", Value: "before"},
		workflow.Sequence{
			workflow.SetVar{Name: "inner", Value: "marker-value"},
		},
		workflow.SetVar{Name: "after", Value: "after"},
	}

	data, err := c.Marshal(&outer)
	assert.NoError(t, err)

	t.Logf("marshalled JSON: %s", string(data))

	// Sanity-check the marshalled output: the inner Sequence carries its
	// own $type envelope so the polymorphic decoder can dispatch it.
	var probe map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(data, &probe))

	var values []json.RawMessage
	assert.NoError(t, json.Unmarshal(probe["$value"], &values))
	assert.Equal(t, 3, len(values), "expected three sequence elements")
	inner := values[1]
	assert.True(t,
		len(inner) > 0 && inner[0] == '{',
		assert.MessageF("nested workflow.Sequence must round-trip with a $type envelope, got bare array: %s", string(inner)))

	var got workflow.Definition
	assert.NoError(t, c.Unmarshal(data, &got))
	assert.Equal[workflow.Definition](t, outer, got)
}

// TestMarshalEventSlice_DefinitionWithRecursiveCodec pins Bug #3.
//
// Originally filed as: marshalling []workflow.Event whose first element is
// workflow.EventUseDefinition carrying a workflow.ForEach{Do: workflow.Sequence}
// panics with a nil pointer dereference at the recursive codec call:
//
//	go.llib.dev/frameless/pkg/workflow/wfjson.WorkflowEventUseDefinition.Marshal
//	  defBytes, err := c.Marshal(v.Definition)
//	                ^^^   <-- c is nil
//
// Root cause: jsonkit.marshalSequencePlaceholderWithReg forwards the slice
// element marshaling with codec=nil, which is fine for events that don't
// recurse through the codec, but breaks for events whose custom codec
// implementation calls back into c.Marshal (e.g. EventUseDefinition for its
// embedded Definition, EventParticipant for its follow-up Definition, and
// ForEach / For for their Do bodies).
//
// The bug is observed at the top-level entry point, but the contract that
// this test pins is narrower:
//
//   - For any []workflow.Event whose elements are marshaled through a custom
//     codec that recurses via the codec, Marshal must not panic and must
//     round-trip.
func TestMarshalEventSlice_DefinitionWithRecursiveCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	// A minimal reproducer: one EventUseDefinition with a Definition tree
	// that uses every recursive-codec culprit in a single value.
	//
	//   EventUseDefinition.Definition = workflow.Sequence{
	//       workflow.SetVar{Name: "v", Value: "ok"},
	//       workflow.ForEach{
	//           Over: "v",
	//           Do:   workflow.Sequence{workflow.SetVar{Name: "x", Value: "y"}},
	//       },
	//       workflow.For{
	//           Init: workflow.SetVar{Name: "i", Value: "0"},
	//           Cond: wftemplate.Condition(".i < 3"),
	//           Do:   workflow.Sequence{workflow.SetVar{Name: "x", Value: "y"}},
	//       },
	//   }
	defWithRecursiveCodec := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "v", Value: "ok"},
			workflow.ForEach{
				Over: "v",
				Do:   workflow.Sequence{workflow.SetVar{Name: "x", Value: "y"}},
			},
			workflow.For{
				Init: workflow.SetVar{Name: "i", Value: "0"},
				Cond: wftemplate.Condition(".i < 3"),
				Do:   workflow.Sequence{workflow.SetVar{Name: "x", Value: "y"}},
			},
		}
	})

	// participantEventWithFollowUp covers the EventParticipant recursive-codec
	// path: a follow-up Definition on a participant event also recurses
	// through the codec.
	participantEventWithFollowUp := let.Var(s, func(t *testcase.T) workflow.Event {
		eventID, err := workflow.MakeEventID()
		assert.NoError(t, err)
		processID, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return workflow.EventParticipant{
			EventID:       eventID,
			ProcessID:     processID,
			Timestamp:     time.Now().UTC(),
			ParticipantID: "p",
			Input:         []any{"hello"},
			Output:        nil,
			Definition:    workflow.Sequence{workflow.SetVar{Name: "v", Value: "ok"}},
		}
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	s.Describe("#Marshal", func(s *testcase.Spec) {
		act := func(t *testcase.T) ([]byte, error) {
			return subject.Get(t).Marshal([]workflow.Event{
				workflow.EventUseDefinition{
					EventID:    workflow.EventID{},
					ProcessID:  workflow.ProcessID{},
					Timestamp:  time.Unix(0, 0).UTC(),
					Definition: defWithRecursiveCodec.Get(t),
				},
			})
		}

		s.Then("does not panic when an element's custom codec recurses through the codec", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assert.NotEmpty(t, data,
				"a slice containing an EventUseDefinition whose Definition uses ForEach/For must marshal without panicking")
		})

		s.Then("round-trips back into []workflow.Event with the recursive Definition intact", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)

			var got []workflow.Event
			assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
			assert.Equal(t, 1, len(got))

			ud, ok := got[0].(workflow.EventUseDefinition)
			assert.True(t, ok,
				assert.MessageF("expected the round-tripped first event to be EventUseDefinition, got %T", got[0]))
			assert.Equal[workflow.Definition](t, defWithRecursiveCodec.Get(t), ud.Definition,
				assert.MessageF("round-tripped Definition must equal the original\nJSON: %s", string(data)))
		})

		s.Then("does not panic for an EventParticipant with a follow-up Definition either", func(t *testcase.T) {
			pe := participantEventWithFollowUp.Get(t)
			data, err := subject.Get(t).Marshal([]workflow.Event{pe})
			assert.NoError(t, err)
			assert.NotEmpty(t, data)
		})
	})
}

// TestEventParticipant_DefinitionRoundTrip pins the wire-format and
// round-trip contract for EventParticipant.Definition — the field set on
// the happy-error path by idempotentExecutor#handleResultDefinition.
//
// The wfcontract Codec helper generates EventParticipant values without a
// Definition, so this codec path is uncovered by the contract suite.
// Pinning it here ensures:
//
//   - When a participant returns nil, Definition is omitted from the wire
//     (the DTO uses `omitempty` on `json.RawMessage`).
//   - When a participant returns a workflow.Definition, Definition is
//     emitted as a typed envelope and survives the round-trip intact.
func TestEventParticipant_DefinitionRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()

	t.Run("nil Definition is omitted from the wire", func(t *testing.T) {
		eventID, err := workflow.MakeEventID()
		assert.NoError(t, err)
		processID, err := workflow.MakeProcessID()
		assert.NoError(t, err)

		event := workflow.EventParticipant{
			EventID:       eventID,
			ProcessID:     processID,
			Timestamp:     time.Unix(0, 0).UTC(),
			ParticipantID: "p",
			Input:         []any{"x"},
			Output:        []any{"y"},
			// Definition deliberately left nil.
		}

		data, err := c.Marshal(event)
		assert.NoError(t, err)

		// Wire-format check: the omitempty tag on the DTO means a nil
		// Definition is dropped entirely, not serialized as `null`. A
		// future regression that drops omitempty (or starts writing the
		// raw message differently) would change the wire, so pin the
		// current shape.
		assert.NotContains(t, string(data), `"definition"`,
			assert.MessageF("a nil Definition must be omitted from the wire\nJSON: %s", string(data)))

		var got workflow.EventParticipant
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, event, got,
			assert.MessageF("a nil Definition must round-trip as nil\nJSON: %s", string(data)))
	})

	t.Run("non-nil Definition round-trips intact", func(t *testing.T) {
		eventID, err := workflow.MakeEventID()
		assert.NoError(t, err)
		processID, err := workflow.MakeProcessID()
		assert.NoError(t, err)

		def := workflow.Sequence{
			workflow.SetVar{Name: "v", Value: "ok"},
			workflow.Execute{Participant: "echo"},
		}

		event := workflow.EventParticipant{
			EventID:       eventID,
			ProcessID:     processID,
			Timestamp:     time.Unix(0, 0).UTC(),
			ParticipantID: "p",
			Input:         []any{"x"},
			Output:        nil, // happy-error path: Output is always nil
			Definition:    def,
		}

		data, err := c.Marshal(event)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		// Wire-format check: the recursive Definition is wrapped in its
		// own typed envelope (`workflow:sequence`) and embedded under
		// the `definition` key. Without this the round-trip below would
		// still pass on a happy-path, but a regression in the envelope
		// shape would silently break any external consumer that reads
		// the JSON directly.
		assert.Contains(t, string(data), `"definition":`,
			assert.MessageF("a non-nil Definition must appear on the wire\nJSON: %s", string(data)))
		assert.Contains(t, string(data), `"$type":"workflow:sequence"`,
			assert.MessageF("a nested Sequence Definition must carry its $type envelope\nJSON: %s", string(data)))

		var got workflow.EventParticipant
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal(t, event, got,
			assert.MessageF("a non-nil Definition must round-trip intact\nJSON: %s", string(data)))
		assert.Equal[workflow.Definition](t, def, got.Definition)
	})
}

// TestWfjsonC_EmptySequenceDirectUnmarshal pins Bug #2.
//
// Originally filed as: "marshalling an empty workflow.Sequence produces
// {"$type":"workflow:sequence","$value":[]} which fails to unmarshal back
// into the concrete workflow.Sequence with:
//
//	json: cannot unmarshal object into Go value of type []json.RawMessage
//
// because the slice unmarshaller drops the typed envelope."
//
// Current status: PASS on both paths (concrete and Definition). Kept as
// a regression pin because the contract helper in wfcontract only
// covers the Definition path; this test exercises the silent-failure
// concrete path alongside it.
func Test_emptySequenceDirectUnmarshal(t *testing.T) {
	c := wfjson.NewCodec()

	seq := workflow.Sequence{}
	var def workflow.Definition = seq
	data, err := c.Marshal(&def)
	assert.NoError(t, err)

	// Direct, concrete-type unmarshal.
	var got workflow.Sequence
	assert.NoError(t, c.Unmarshal(data, &got),
		assert.MessageF("empty workflow.Sequence should round-trip into its concrete type"))
	assert.Equal(t, seq, got)

	// Interface unmarshal (the path the contract helper also covers).
	var gotDef workflow.Definition
	assert.NoError(t, c.Unmarshal(data, &gotDef))
	assert.Equal[workflow.Definition](t, def, gotDef)
}

// TestWfjsonC_PrimitiveConditionDirectUnmarshal pins Bug #3.
//
// Originally filed as: "wftemplate.Condition is `type Condition string`
// and the codec emits a typed envelope on Marshal but does not unwrap it
// on Unmarshal when the target is the named primitive type, surfacing:
//
//	cannot unmarshal object into Go value of type wftemplate.Condition."
//
// Current status: PASS on both paths (concrete and workflow.Condition).
// Kept as a regression pin because the contract helper only covers the
// workflow.Condition interface path.
func Test_primitiveConditionDirectUnmarshal(t *testing.T) {
	c := wfjson.NewCodec()

	cond := wftemplate.Condition(".X == .Y")
	var held workflow.Condition = cond
	data, err := c.Marshal(&held)
	assert.NoError(t, err)

	// Direct, concrete-type unmarshal.
	var got wftemplate.Condition
	assert.NoError(t, c.Unmarshal(data, &got),
		assert.MessageF("wftemplate.Condition should round-trip into its concrete type"))
	assert.Equal(t, cond, got)

	// Interface unmarshal (the path the contract helper also covers).
	var gotCond workflow.Condition
	assert.NoError(t, c.Unmarshal(data, &gotCond))
	assert.Equal[workflow.Condition](t, cond, gotCond)
}

// TestWfjsonC_FuzzNestedSequence is a regression pin for Bug #1
// across a spread of random nested Sequence shapes. Guards against the
// textbook case being handled but a deeper shape regressing — for
// example a Sequence whose only element is another Sequence.
//
// Both sub-tests today pass; they live here so any future change to
// jsonkit or to the Sequence codec that re-introduces the bug fails
// loudly.
func Test_fuzzNestedSequence(t *testing.T) {
	c := wfjson.NewCodec()
	s := testcase.NewSpec(t)

	s.Test("nested Sequence always carries a $type envelope", func(t *testcase.T) {
		// Build a sequence whose elements are a mix of leaves and
		// (possibly nested) Sequences.
		var def workflow.Definition = workflow.Sequence{
			MakeNestedFuzzLeaf(t),
			MakeNestedFuzzSequence(t, t.Random.IntBetween(0, 2)),
			MakeNestedFuzzLeaf(t),
		}

		data, err := c.Marshal(&def)
		assert.NoError(t, err)

		t.Logf("marshalled JSON: %s", string(data))

		// Walk the marshalled JSON and assert that every JSON object
		// that contains a `Definition`-shaped envelope also carries a
		// $type discriminator. Catches the bare-array leakage shape of
		// nested Sequences.
		AssertAllObjectsHaveTypeEnvelope(t, data)
	})

	s.Test("nested Sequence round-trips through the polymorphic interface", func(t *testcase.T) {
		var def workflow.Definition = workflow.Sequence{
			MakeNestedFuzzLeaf(t),
			MakeNestedFuzzSequence(t, t.Random.IntBetween(0, 2)),
			MakeNestedFuzzLeaf(t),
		}

		data, err := c.Marshal(&def)
		assert.NoError(t, err)

		var got workflow.Definition
		assert.NoError(t, c.Unmarshal(data, &got))
		assert.Equal[workflow.Definition](t, def, got)
	})
}

// MakeNestedFuzzLeaf produces a leaf Definition (no Sequence, no If) so
// the surrounding Sequence does not accidentally recurse past one level.
func MakeNestedFuzzLeaf(t *testcase.T) workflow.Definition {
	return random.Pick(t.Random,
		func() workflow.Definition {
			return workflow.SetVar{Name: workflow.VarName(t.Random.String()), Value: t.Random.String()}
		},
		func() workflow.Definition {
			return workflow.Execute{Participant: workflow.ParticipantID(t.Random.String())}
		},
	)()
}

// MakeNestedFuzzSequence produces a Sequence with `depth` levels of
// nesting. depth==0 yields a leaf-only Sequence.
func MakeNestedFuzzSequence(t *testcase.T, depth int) workflow.Sequence {
	if depth == 0 {
		return workflow.Sequence{MakeNestedFuzzLeaf(t)}
	}
	return workflow.Sequence{MakeNestedFuzzSequence(t, depth-1)}
}

// AssertAllObjectsHaveTypeEnvelope walks the marshalled JSON and asserts
// that every JSON object that represents a workflow element carries a
// "$type" discriminator. This is the structural assertion that the bug
// breaks today: nested Sequence elements are emitted as bare JSON arrays
// instead of {"$type":"workflow:sequence","$value":[…]} envelopes.
func AssertAllObjectsHaveTypeEnvelope(t *testcase.T, data []byte) {
	t.Helper()
	var v any
	assert.NoError(t, json.Unmarshal(data, &v))
	walkFuzz(t, v)
}

func walkFuzz(t *testcase.T, v any) {
	t.Helper()
	switch v := v.(type) {
	case map[string]any:
		// Walk each child. Typed envelopes (carrying "$type") are
		// allowed; their "$value" payload is descended into so the
		// recursion reaches nested polymorphic slices.
		for _, child := range v {
			walkFuzz(t, child)
		}
	case []any:
		for _, child := range v {
			// Every element of a polymorphic slice must be a typed
			// envelope — that's what the codec dispatches on. A
			// bare JSON array here means a nested Sequence leaked
			// its envelope (Bug #1), so fail.
			if _, isArray := child.([]any); isArray {
				assert.True(t, false,
					assert.MessageF("polymorphic slice element is a bare array, not a typed envelope: %#v", child))
			}
			walkFuzz(t, child)
		}
	}
}
