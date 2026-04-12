package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
)

func TestProcess_json_smoke(tt *testing.T) {
	// Smoke test: verify that workflow.Process can be JSON encoded and decoded
	// after going through a process definition execution.

	var fooOut = "test-foo-value"
	barOut := 42

	participants := workflow.Participants{
		"foo": func(ctx context.Context) (string, error) {
			return fooOut, nil
		},
		"bar": func(ctx context.Context, in string) (int, error) {
			assert.Equal(tt, in, fooOut)
			return barOut, nil
		},
	}

	var pdef workflow.Definition = &workflow.Sequence{
		&workflow.ExecuteParticipant{
			ID:     "foo",
			Output: []workflow.VariableKey{"foo-val"},
		},
		&workflow.ExecuteParticipant{
			ID:     "bar",
			Input:  []workflow.VariableKey{"foo-val"},
			Output: []workflow.VariableKey{"bar-val"},
		},
	}

	r := workflow.Runtime{Participants: participants}
	var p workflow.Process

	// Execute the workflow definition to populate process with variables and events
	assert.NoError(tt, pdef.Execute(r.Context(context.Background()), &p))

	// Verify initial state before JSON round-trip
	assert.Equal[any](tt, p.Variables.Get("foo-val"), fooOut)
	assert.Equal[any](tt, p.Variables.Get("bar-val"), barOut)
	assert.NotEmpty(tt, p.Events)

	// Encode to JSON
	jsonData, err := json.Marshal(p)
	assert.NoError(tt, err)

	pp.PP(jsonData)
	// Decode back from JSON
	var decoded workflow.Process
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(tt, err)

	// Verify data is preserved after round-trip
	// Note: JSON decodes numbers as float64 by default
	assert.Equal[any](tt, decoded.Variables.Get("foo-val"), fooOut)
	// TODO: it should keep the base type if possible
	assert.Equal(tt, barOut, int(decoded.Variables.Get("bar-val").(float64)))
}

func TestDefinition_json_smoke(tt *testing.T) {
	// Smoke test: verify that various workflow.Definition implementations can be
	// JSON encoded and decoded using jsonkit.Interface[workflow.Definition]

	s := testcase.NewSpec(tt)

	s.Test("ExecuteParticipant", func(t *testcase.T) {
		pdef := &workflow.ExecuteParticipant{
			ID:     "my-participant",
			Input:  []workflow.VariableKey{"input-var"},
			Output: []workflow.VariableKey{"output-var"},
		}

		data, err := json.Marshal(jsonkit.Interface[workflow.Definition]{V: pdef})
		assert.NoError(t, err)
		t.Logf("Encoded ExecuteParticipant: %s", string(data))

		var decoded jsonkit.Interface[workflow.Definition]
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.NotNil(t, decoded.V)
	})

	s.Test("Sequence", func(t *testcase.T) {
		pdef := &workflow.Sequence{
			&workflow.ExecuteParticipant{
				ID:     "step1",
				Output: []workflow.VariableKey{"var1"},
			},
			&workflow.ExecuteParticipant{
				ID:    "step2",
				Input: []workflow.VariableKey{"var1"},
			},
		}

		data, err := json.Marshal(jsonkit.Interface[workflow.Definition]{V: pdef})
		assert.NoError(t, err)
		t.Logf("Encoded Sequence: %s", string(data))

		var decoded jsonkit.Interface[workflow.Definition]
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.NotNil(t, decoded.V)
	})

	s.Test("If with ExecuteParticipant", func(t *testcase.T) {
		pdef := &workflow.If{
			Cond: &workflow.ExecuteCondition{
				ID:    "my-condition",
				Input: []workflow.VariableKey{"cond-input"},
			},
			Then: &workflow.ExecuteParticipant{
				ID:     "then-step",
				Output: []workflow.VariableKey{"then-var"},
			},
			Else: &workflow.ExecuteParticipant{
				ID:    "else-step",
				Input: []workflow.VariableKey{"input-var"},
			},
		}

		data, err := json.Marshal(jsonkit.Interface[workflow.Definition]{V: pdef})
		assert.NoError(t, err)
		t.Logf("Encoded If: %s", string(data))

		var decoded jsonkit.Interface[workflow.Definition]
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.NotNil(t, decoded.V)
	})

	s.Test("Nested Sequence with If", func(t *testcase.T) {
		pdef := &workflow.Sequence{
			&workflow.ExecuteParticipant{
				ID:     "setup",
				Output: []workflow.VariableKey{"setup-var"},
			},
			&workflow.If{
				Cond: &workflow.ExecuteCondition{
					ID:    "check-condition",
					Input: []workflow.VariableKey{"cond-input"},
				},
				Then: &workflow.Sequence{
					&workflow.ExecuteParticipant{
						ID:    "then-a",
						Input: []workflow.VariableKey{"setup-var"},
					},
					&workflow.ExecuteParticipant{
						ID:    "then-b",
						Input: []workflow.VariableKey{"setup-var"},
					},
				},
				Else: &workflow.ExecuteParticipant{
					ID: "else-only",
				},
			},
		}

		data, err := json.Marshal(jsonkit.Interface[workflow.Definition]{V: pdef})
		assert.NoError(t, err)
		t.Logf("Encoded nested structure: %s", string(data))

		var decoded jsonkit.Interface[workflow.Definition]
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.NotNil(t, decoded.V)
	})

	s.Test("ExecuteCondition as Definition", func(t *testcase.T) {
		pdef := &workflow.ExecuteCondition{
			ID:    "my-condition",
			Input: []workflow.VariableKey{"cond-input"},
		}

		data, err := json.Marshal(jsonkit.Interface[workflow.Definition]{V: pdef})
		assert.NoError(t, err)
		t.Logf("Encoded ExecuteCondition: %s", string(data))

		var decoded jsonkit.Interface[workflow.Definition]
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.NotNil(t, decoded.V)
	})
}
