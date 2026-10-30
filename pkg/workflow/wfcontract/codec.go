package wfcontract

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

// randomVarNames builds a slice of random workflow.VarName values. The length
// is bounded (0..3) so the slice stays small while still exercising the
// slice-typed JSON wire format.
func randomVarNames(tc *testcase.T) []workflow.VarName {
	return random.Slice(tc.Random.IntBetween(0, 3), func() workflow.VarName {
		return workflow.VarName(tc.Random.String())
	})
}

// inSliceOfItsInterface returns v, a workflow.Definition or a workflow.Condition,
// in a slice of the workflow interface it implements, next to another value of it.
func inSliceOfItsInterface(tc *testcase.T, v any) any {
	switch v := v.(type) {
	case workflow.Definition:
		return []workflow.Definition{v, MakeDefinition(tc)}
	case workflow.Condition:
		return []workflow.Condition{MakeCondition(tc), v}
	default:
		panic(fmt.Sprintf("%T is neither a workflow.Definition nor a workflow.Condition", v))
	}
}

// inMapOfItsInterface returns v, a workflow.Definition or a workflow.Condition,
// in a map of the workflow interface it implements.
func inMapOfItsInterface(tc *testcase.T, v any) any {
	switch v := v.(type) {
	case workflow.Definition:
		return map[string]workflow.Definition{tc.Random.HexN(4): v}
	case workflow.Condition:
		return map[string]workflow.Condition{tc.Random.HexN(4): v}
	default:
		panic(fmt.Sprintf("%T is neither a workflow.Definition nor a workflow.Condition", v))
	}
}

// randomSetVarValue builds a random workflow.SetVar.Value payload. The
// payload varies in type (nil / bool / string / a built-in number) so the codec
// exercises more than just the string-typed branch. Slices/maps are
// covered by their own scenarios, which put a registered value in a []any,
// a map[string]any, or a slice or map of the workflow interface it implements.
//
// A number keeps its type too, like an int, which plain JSON would read back as a float64.
func randomSetVarValue(tc *testcase.T) any {
	switch tc.Random.IntN(5) {
	case 0:
		return nil
	case 1:
		return tc.Random.Bool()
	case 2:
		return tc.Random.Float64()
	case 3:
		return randomBuiltinNumber(tc)
	default:
		return tc.Random.String()
	}
}

// randomBuiltinNumber builds a random number of a built-in type other than float64,
// which a codec has to tell apart from a float64, to read it back as the same type.
func randomBuiltinNumber(tc *testcase.T) any {
	return random.Pick(tc.Random,
		func() any { return tc.Random.Int() },
		func() any { return int32(tc.Random.IntBetween(-1<<31, 1<<31-1)) },
		func() any { return int64(tc.Random.Int()) },
		func() any { return uint(tc.Random.Int()) },
		func() any { return uint8(tc.Random.IntN(256)) },
		func() any { return tc.Random.Float32() },
	)()
}

// randomTemplateCondition builds a wftemplate.Condition that is a valid
// text/template expression. It compares two optional variable references so
// the resulting template always parses to a boolean without depending on the
// template engine's parsing semantics for arbitrary user input.
func randomTemplateCondition(tc *testcase.T) wftemplate.Condition {
	return wftemplate.Condition(
		strings.Repeat(".X", 1+tc.Random.IntB(0, 3)) +
			" == " +
			strings.Repeat(".Y", 1+tc.Random.IntB(0, 3)),
	)
}

// MakeDefinition builds a random workflow.Definition implementation.
// It picks one of the registered concrete types and builds a shallow,
// round-trip-safe value (no nested interfaces to avoid combinatorial blow-up).
//
// The returned value uses t.Random so callers benefit from the testcase
// deterministic-seed setup. Reusable across any test that needs a randomly
// generated Definition (e.g. codec round-trips, fixture composition).
func MakeDefinition(tb testing.TB) workflow.Definition {
	var tc = testcase.ToT(&tb)
	return random.Pick[func() workflow.Definition](tc.Random,
		func() workflow.Definition {
			return workflow.Execute{
				Participant: workflow.ParticipantID(tc.Random.String()),
				Input:       randomVarNames(tc),
				Output:      randomVarNames(tc),
			}
		},
		func() workflow.Definition {
			return workflow.SetVar{
				Name:  workflow.VarName(tc.Random.String()),
				Value: randomSetVarValue(tc),
			}
		},
		func() workflow.Definition {
			return workflow.DeclareVar{
				Name:   workflow.VarName(tc.Random.String()),
				Global: tc.Random.Bool(),
			}
		},
		func() workflow.Definition {
			return workflow.DeleteVar{
				Name: workflow.VarName(tc.Random.String()),
			}
		},
		func() workflow.Definition {
			return workflow.Increment{
				Name: workflow.VarName(tc.Random.String()),
			}
		},
		func() workflow.Definition {
			return workflow.Break{}
		},
		func() workflow.Definition {
			return workflow.Sleep{
				While: MakeCondition(tc),
				Until: MakeCondition(tc),
			}
		},
		func() workflow.Definition {
			return workflow.If{
				Cond: MakeCondition(tc),
				Then: MakeDefinition(tc),
				Else: MakeDefinition(tc),
			}
		},
		func() workflow.Definition {
			return workflow.For{
				Init: MakeLeafDefinition(tc),
				Cond: MakeCondition(tc),
				Post: MakeLeafDefinition(tc),
				Do:   MakeLeafDefinition(tc),
			}
		},
		func() workflow.Definition {
			return workflow.ForEach{
				Over: workflow.VarName(tc.Random.String()),
				K:    workflow.VarName(tc.Random.String()),
				V:    workflow.VarName(tc.Random.String()),
				Do:   MakeLeafDefinition(tc),
			}
		},
		func() workflow.Definition {
			return workflow.Spawn{
				Name:       workflow.SpawnName(tc.Random.String()),
				Definition: MakeDefinition(tc),
				Vars:       makeVarMapping(tc),
			}
		},
		func() workflow.Definition {
			return workflow.Join{
				Spawn:   workflow.SpawnName(tc.Random.String()),
				Collect: makeVarMapping(tc),
			}
		},
		func() workflow.Definition {
			return workflow.Defer{Definition: MakeLeafDefinition(tc)}
		},
	)()
}

func makeVarMapping(tc *testcase.T) workflow.VarMapping {
	return random.Pick(tc.Random,
		func() workflow.VarMapping { return nil },
		func() workflow.VarMapping { return workflow.VarMapping{} },
		func() workflow.VarMapping {
			return random.Map(tc.Random.IntBetween(3, 7), func() (workflow.VarName, workflow.VarName) {
				return workflow.VarName(tc.Random.String()), workflow.VarName(tc.Random.String())
			})
		},
	)()
}

// MakeLeafDefinition builds a random workflow.Definition that does not itself
// recurse into other Definition-bearing types (no Sequence, no If). The
// codec contracts in this package rely on leaf definitions when verifying
// sequences/joins/etc. because the round-trip of nested polymorphic slices
// is exercised separately (and is currently limited by the codec).
func MakeLeafDefinition(tb testing.TB) workflow.Definition {
	var tc = testcase.ToT(&tb)
	return random.Pick[func() workflow.Definition](tc.Random,
		func() workflow.Definition {
			return workflow.Execute{
				Participant: workflow.ParticipantID(tc.Random.String()),
				Input:       randomVarNames(tc),
				Output:      randomVarNames(tc),
			}
		},
		func() workflow.Definition {
			return workflow.SetVar{
				Name:  workflow.VarName(tc.Random.String()),
				Value: randomSetVarValue(tc),
			}
		},
		func() workflow.Definition {
			return workflow.DeclareVar{
				Name:   workflow.VarName(tc.Random.String()),
				Global: tc.Random.Bool(),
			}
		},
		func() workflow.Definition {
			return workflow.DeleteVar{
				Name: workflow.VarName(tc.Random.String()),
			}
		},
		func() workflow.Definition {
			return workflow.Increment{
				Name: workflow.VarName(tc.Random.String()),
			}
		},
		func() workflow.Definition {
			return workflow.Break{}
		},
		func() workflow.Definition {
			return workflow.Sleep{
				While: MakeCondition(tc),
				Until: MakeCondition(tc),
			}
		},
	)()
}

func MakeCondition(tb testing.TB) workflow.Condition {
	var tc = testcase.ToT(&tb)
	if tc.Random.Bool() {
		return workflow.Execute{
			Condition: workflow.ConditionID(tc.Random.String()),
			Input:     randomVarNames(tc),
		}
	}
	return randomTemplateCondition(tc)
}

// assertRoundTripDefinition marshals def through codec, unmarshals the result
// back into workflow.Definition, and asserts equality. Both sides are compared
// via the workflow.Definition interface so the generic assert.Equal resolves
// to a single type parameter.
// def is marshaled from a pointer to the workflow.Definition interface,
// the same as it is unmarshaled into one, so its codec knows that it is held by an interface.
func assertRoundTripDefinition(tc *testcase.T, codec workflow.Codec, def workflow.Definition) {
	data, err := codec.Marshal(&def)
	assert.NoError(tc, err, assert.MessageF("expected the %T to round-trip through the codec, but marshalling failed", def))
	assert.NotEmpty(tc, data)

	var got workflow.Definition
	assert.NoError(tc, codec.Unmarshal(data, &got),
		assert.MessageF("expected the %T to round-trip through the codec, but unmarshalling failed\nJSON: %s", def, string(data)))
	assert.Equal(tc, def, got,
		assert.MessageF("expected the %T to round-trip through the codec\nJSON: %s", def, string(data)))
}

// assertRoundTripCondition is the Condition counterpart of
// assertRoundTripDefinition.
func assertRoundTripCondition(tc *testcase.T, codec workflow.Codec, cond workflow.Condition) {
	data, err := codec.Marshal(&cond)
	assert.NoError(tc, err, assert.MessageF("expected the %T to round-trip through the codec, but marshalling failed", cond))
	assert.NotEmpty(tc, data)

	var got workflow.Condition
	assert.NoError(tc, codec.Unmarshal(data, &got),
		assert.MessageF("expected the %T to round-trip through the codec, but unmarshalling failed\nJSON: %s", cond, string(data)))
	assert.Equal(tc, cond, got,
		assert.MessageF("expected the %T to round-trip through the codec\nJSON: %s", cond, string(data)))
}

// randomEventProcessID builds a non-zero ProcessID for event fixtures.
func randomEventProcessID(tc *testcase.T) workflow.ProcessID {
	pid, err := workflow.MakeProcessID()
	assert.NoError(tc, err)
	return pid
}

// randomEventID builds a non-zero EventID for event fixtures.
func randomEventID(tc *testcase.T) workflow.EventID {
	id, err := workflow.MakeEventID()
	assert.NoError(tc, err)
	return id
}

// randomPath builds a random workflow.Path fixture,
// with Scopes that are variable scopes and Scopes that are not.
func randomPath(tc *testcase.T) workflow.Path {
	tc.Helper()
	var length = tc.Random.IntBetween(0, 5)
	if length == 0 && tc.Random.Bool() {
		return nil
	}
	return random.Slice(length, func() workflow.Scope {
		return workflow.Scope{Name: tc.Random.UUID(), Var: tc.Random.Bool()}
	})
}

// Codec returns a contract.Contract that asserts a workflow.Codec faithfully
// round-trips every workflow.Definition and workflow.Condition implementation
// currently registered for polymorphic (de)serialisation.
//
// Each scenario is a top-level Spec#Test so it runs independently with a
// fresh codec instance and a fresh random seed drawn from t.Random. This
// means:
//
//   - reproducing a failure is just a matter of running with a fixed
//     TESTCASE_SEED;
//   - different wire formats (JSON, YAML, …) can be exercised against the
//     same contract without duplicating tests in their respective packages.
//
// The mk parameter is invoked once per Spec#Test, ensuring codec instances
// don't share state across scenarios (no leak of registered TypeIDs or
// similar per-codec mutable state).
//
// A value which is read back into an interface, like a workflow.Definition, is marshaled from a pointer to that interface,
// the same as it is unmarshaled into one, like Codec.Marshal(&def) and Codec.Unmarshal(data, &def),
// so a codec which writes the type of a value only where an interface holds it, like jsonkit.Codec, knows to write it.
// A value marshaled as itself is read back into its own type.
//
// Beyond the top-level "smoke" round-trip assertions, every concrete
// Definition, Condition, and Event implementation registered with the codec
// gets a commonSpec context. This context verifies the value can be
// round-tripped directly, can be unmarshalled back into the polymorphic
// interface (workflow.Definition / workflow.Condition / workflow.Event),
// and — for Definitions — can be embedded inside workflow.Sequence and
// workflow.If. Together this ensures a wire-format change that breaks any
// single registered type surfaces here, and that each type composes safely
// with the rest of the workflow.
func Codec(codec workflow.Codec) contract.Contract {
	s := testcase.NewSpec(nil)

	s.Test("Sequence round-trips", func(t *testcase.T) {
		def := workflow.Sequence{MakeDefinition(t), MakeDefinition(t)}
		assertRoundTripDefinition(t, codec, def)
	})

	// Empty Sequence is the degenerate-but-allowed Definition shape:
	// len==0 must marshal to {"$type":"workflow:sequence","$value":[]}
	// (the slice envelope, not an empty interface envelope) and must
	// round-trip both through the Definition interface and into the
	// concrete workflow.Sequence type. The concrete path is the one the
	// generic jsonkit slice unmarshaller historically fumbled — the typed
	// envelope was emitted on Marshal but not unwrapped when the target
	// was the concrete slice type — so the contract pins both paths.
	s.Test("empty Sequence round-trips through Definition and as concrete", func(t *testcase.T) {
		var def workflow.Definition = workflow.Sequence{}

		data, err := codec.Marshal(&def)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var gotDef workflow.Definition
		assert.NoError(t, codec.Unmarshal(data, &gotDef))
		gotSeq, ok := gotDef.(workflow.Sequence)
		assert.True(t, ok,
			assert.MessageF("expected empty Sequence to unmarshal as workflow.Sequence, got %T", gotDef))
		assert.Equal(t, 0, len(gotSeq))

		var gotConcrete workflow.Sequence
		assert.NoError(t, codec.Unmarshal(data, &gotConcrete),
			assert.MessageF("empty workflow.Sequence should round-trip into its concrete type\nJSON: %s", string(data)))
		assert.Equal(t, 0, len(gotConcrete))
	})

	s.Test("If round-trips", func(t *testcase.T) {
		def := workflow.If{
			Cond: MakeCondition(t),
			Then: MakeDefinition(t),
			Else: MakeDefinition(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("Suspend round-trips", func(t *testcase.T) {
		def := workflow.Sleep{
			While: MakeCondition(t),
			Until: MakeCondition(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("For round-trips", func(t *testcase.T) {
		def := workflow.For{
			Init: MakeDefinition(t),
			Cond: MakeCondition(t),
			Post: MakeDefinition(t),
			Do:   MakeDefinition(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("ForEach round-trips", func(t *testcase.T) {
		def := workflow.ForEach{
			Over: workflow.VarName(t.Random.String()),
			K:    workflow.VarName(t.Random.String()),
			V:    workflow.VarName(t.Random.String()),
			Do:   MakeDefinition(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("SetVar round-trips", func(t *testcase.T) {
		def := workflow.SetVar{
			Name:  workflow.VarName(t.Random.String()),
			Value: randomSetVarValue(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("DeclareVar round-trips", func(t *testcase.T) {
		def := workflow.DeclareVar{
			Name:   workflow.VarName(t.Random.String()),
			Global: t.Random.Bool(),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("DeleteVar round-trips", func(t *testcase.T) {
		def := workflow.DeleteVar{
			Name: workflow.VarName(t.Random.String()),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("Increment round-trips", func(t *testcase.T) {
		def := workflow.Increment{
			Name: workflow.VarName(t.Random.String()),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("Execute with ParticipantID round-trips as a Definition", func(t *testcase.T) {
		def := workflow.Execute{
			Participant: workflow.ParticipantID(t.Random.String()),
			Input:       randomVarNames(t),
			Output:      randomVarNames(t),
		}
		assertRoundTripDefinition(t, codec, def)
	})

	s.Test("Execute with ConditionID round-trips as a Condition", func(t *testcase.T) {
		cond := workflow.Execute{
			Condition: workflow.ConditionID(t.Random.String()),
			Input:     randomVarNames(t),
		}
		assertRoundTripCondition(t, codec, cond)
	})

	s.Test("wftemplate.Condition round-trips", func(t *testcase.T) {
		assertRoundTripCondition(t, codec, randomTemplateCondition(t))
	})

	// wftemplate.Condition is a named primitive kind (`type Condition
	// string`). Its concrete path is the silent failure mode for typed
	// envelopes on primitives — the registry wraps it on Marshal, but a
	// future regression in jsonkit could forget to unwrap the envelope
	// when the target is the named type. Pin both paths: through the
	// workflow.Condition interface (already covered by the helper
	// above) and directly into wftemplate.Condition.
	s.Test("wftemplate.Condition round-trips as concrete type", func(t *testcase.T) {
		cond := randomTemplateCondition(t)

		var held workflow.Condition = cond
		data, err := codec.Marshal(&held)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var got workflow.Condition
		assert.NoError(t, codec.Unmarshal(data, &got))
		assert.Equal[workflow.Condition](t, cond, got)

		var gotConcrete wftemplate.Condition
		assert.NoError(t, codec.Unmarshal(data, &gotConcrete),
			assert.MessageF("wftemplate.Condition should round-trip into its concrete type\nJSON: %s", string(data)))
		assert.Equal[wftemplate.Condition](t, cond, gotConcrete)
	})

	s.Test("UseDefinitionEvent round-trips", func(t *testcase.T) {
		event := workflow.EventUseDefinition{
			EventID:    randomEventID(t),
			ProcessID:  randomEventProcessID(t),
			Timestamp:  clock.Now().UTC(),
			Definition: MakeDefinition(t),
		}
		assertRoundTripEvent(t, codec, event)
	})

	// EventUseDefinition and EventParticipant carry a workflow.Definition field
	// whose custom codec recurses through c.Marshal. Marshalling a slice of
	// events goes through jsonkit's marshalSequencePlaceholderWithReg, which
	// must forward the codec down to each element so the recursive call has a
	// usable codec instead of a nil pointer. This test pins that contract from
	// the consumer's perspective: a slice of mixed events containing both
	// offenders must round-trip cleanly.
	s.Test("[]workflow.Event round-trips through the codec", func(t *testcase.T) {
		events := []workflow.Event{
			workflow.EventUseDefinition{
				EventID:    randomEventID(t),
				ProcessID:  randomEventProcessID(t),
				Timestamp:  clock.Now().UTC(),
				Definition: MakeDefinition(t),
			},
			workflow.EventParticipant{
				EventID:       randomEventID(t),
				ProcessID:     randomEventProcessID(t),
				Timestamp:     clock.Now().UTC(),
				ParticipantID: workflow.ParticipantID(t.Random.String()),
				Path:          randomPath(t),
				Input: random.Slice(t.Random.IntBetween(0, 3), func() any {
					return randomSetVarValue(t)
				}),
				Output: random.Slice(t.Random.IntBetween(0, 3), func() any {
					return randomSetVarValue(t)
				}),
				// Follow-up Definition forces the recursive-codec path on
				// EventParticipant; MakeDefinition covers ForEach/For as
				// recursive-codec offenders via EventUseDefinition above.
				Definition: MakeDefinition(t),
			},
		}

		data, err := codec.Marshal(events)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var got []workflow.Event
		assert.NoError(t, codec.Unmarshal(data, &got))
		assert.Equal(t, len(events), len(got))
		assert.Equal(t, events, got,
			assert.MessageF("expected a slice of events with recursive-codec fields to round-trip\nJSON: %s", string(data)))
	})

	// The variable values and the participant and condition arguments are held in `any` fields.
	// A value read back from them has to be the very same value, of the very same type.
	thenTheValueKeepsItsType := func(s *testcase.Spec, value testcase.Var[any]) {
		s.Test("EventSetVar Value keeps its type", func(t *testcase.T) {
			assertRoundTripEvent(t, codec, workflow.EventSetVar{
				EventID:   randomEventID(t),
				ProcessID: randomEventProcessID(t),
				Timestamp: randomEventTimestamp(t),
				Path:      randomPath(t),
				Name:      workflow.VarName(t.Random.String()),
				Value:     value.Get(t),
			})
		})

		s.Test("SetVar Value keeps its type", func(t *testcase.T) {
			assertRoundTripDefinition(t, codec, workflow.SetVar{
				Name:  workflow.VarName(t.Random.String()),
				Value: value.Get(t),
			})
		})

		s.Test("EventParticipant Input and Output keep its type", func(t *testcase.T) {
			assertRoundTripEvent(t, codec, workflow.EventParticipant{
				EventID:       randomEventID(t),
				ProcessID:     randomEventProcessID(t),
				Timestamp:     randomEventTimestamp(t),
				ParticipantID: workflow.ParticipantID(t.Random.String()),
				Path:          randomPath(t),
				Input:         []any{randomSetVarValue(t), value.Get(t)},
				Output:        []any{value.Get(t), randomSetVarValue(t)},
			})
		})

		s.Test("EventCondition Input keeps its type", func(t *testcase.T) {
			assertRoundTripEvent(t, codec, workflow.EventCondition{
				EventID:     randomEventID(t),
				ProcessID:   randomEventProcessID(t),
				Timestamp:   randomEventTimestamp(t),
				ConditionID: workflow.ConditionID(t.Random.String()),
				Path:        randomPath(t),
				Input:       []any{value.Get(t), randomSetVarValue(t)},
				Answer:      t.Random.Bool(),
			})
		})
	}

	// A number of a built-in type, like an int, has to come back as that type,
	// not as the float64 plain JSON is read back as, or a template would compare an int with a float64.
	s.Context("an any field holds a number of a built-in type other than float64", func(s *testcase.Spec) {
		value := let.Var(s, randomBuiltinNumber)

		thenTheValueKeepsItsType(s, value)

		s.And("the number sits in a []any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []any{value.Super(t), t.Random.String()}
			})

			thenTheValueKeepsItsType(s, value)
		})

		s.And("the number sits in a map[string]any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any{t.Random.HexN(4): value.Super(t)}
			})

			thenTheValueKeepsItsType(s, value)
		})
	})

	// A value whose type is registered with the codec must come back as that type,
	// not as the generic shape of its wire format, like a map[string]any.
	s.Context("an any field holds a value of a registered type", func(s *testcase.Spec) {
		value := let.Var(s, func(t *testcase.T) any {
			return random.Pick(t.Random,
				func() any { return MakeDefinition(t) },
				func() any { return MakeCondition(t) },
			)()
		})

		thenTheValueKeepsItsType(s, value)

		s.And("the value sits in a []any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return []any{randomSetVarValue(t), value.Super(t)}
			})

			thenTheValueKeepsItsType(s, value)
		})

		s.And("the value sits in a map[string]any", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return map[string]any{
					"plain":      randomSetVarValue(t),
					"registered": value.Super(t),
				}
			})

			thenTheValueKeepsItsType(s, value)
		})

		// A slice or a map of the workflow interface the value implements keeps its type too,
		// like a []workflow.Definition, instead of coming back as a []any or a map[string]any.
		s.And("the value sits in a slice of the workflow interface it implements", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return inSliceOfItsInterface(t, value.Super(t))
			})

			thenTheValueKeepsItsType(s, value)
		})

		s.And("the value sits in a map of the workflow interface it implements", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return inMapOfItsInterface(t, value.Super(t))
			})

			thenTheValueKeepsItsType(s, value)
		})
	})

	s.Test("Definition reached only via the interface round-trips", func(t *testcase.T) {
		defs := []workflow.Definition{
			workflow.Sequence{MakeDefinition(t), MakeDefinition(t)},
			workflow.If{
				Cond: MakeCondition(t),
				Then: MakeDefinition(t),
				Else: MakeDefinition(t),
			},
			workflow.Sleep{
				While: MakeCondition(t),
				Until: MakeCondition(t),
			},
			workflow.Break{},
			workflow.For{
				Init: MakeLeafDefinition(t),
				Cond: MakeCondition(t),
				Post: MakeLeafDefinition(t),
				Do:   MakeLeafDefinition(t),
			},
			workflow.SetVar{
				Name:  workflow.VarName(t.Random.String()),
				Value: randomSetVarValue(t),
			},
			workflow.DeclareVar{
				Name:   workflow.VarName(t.Random.String()),
				Global: t.Random.Bool(),
			},
			workflow.DeleteVar{
				Name: workflow.VarName(t.Random.String()),
			},
			workflow.Increment{
				Name: workflow.VarName(t.Random.String()),
			},
			workflow.Execute{
				Participant: workflow.ParticipantID(t.Random.String()),
				Input:       randomVarNames(t),
				Output:      randomVarNames(t),
			},
			workflow.Defer{Definition: MakeDefinition(t)},
		}
		def := defs[t.Random.IntN(len(defs))]
		assertRoundTripDefinition(t, codec, def)
	})

	// commonSpec applies the standard set of polymorphic-context tests to
	// every concrete type registered for (de)serialisation. If you add a
	// new type to wfjson.NewCodec (or any other codec), wire it up here
	// too — that's how the contract stays in sync with the codec.
	specTypeCodec(s, codec, func(t *testcase.T) workflow.Sequence {
		// Leaf-only elements avoid exercising the codec's nested
		// polymorphic-slice behaviour, which is verified separately.
		// 1..N elements: the codec's empty-slice round-trip is broken.
		return random.Slice[workflow.Definition](t.Random.IntBetween(1, 7), func() workflow.Definition {
			return MakeLeafDefinition(t)
		})
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.If {
		var v workflow.If
		v.Cond = random.Pick(t.Random,
			func() workflow.Condition { return nil },
			func() workflow.Condition { return randomTemplateCondition(t) },
			func() workflow.Condition { return MakeCondition(t) },
		)()
		v.Then = random.Pick(t.Random,
			func() workflow.Definition { return nil },
			func() workflow.Definition { return MakeDefinition(t) },
		)()
		v.Else = random.Pick(t.Random,
			func() workflow.Definition { return nil },
			func() workflow.Definition { return MakeDefinition(t) },
		)()
		return v
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Sleep {
		return workflow.Sleep{
			While: random.Pick(t.Random,
				func() workflow.Condition { return nil },
				func() workflow.Condition { return MakeCondition(t) },
			)(),
			Until: random.Pick(t.Random,
				func() workflow.Condition { return nil },
				func() workflow.Condition { return MakeCondition(t) },
			)(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.For {
		// Use leaf-only parts here. A nested polymorphic Definition through
		// the same codec path is exercised by the top-level "For round-trips"
		// test above, which deliberately uses MakeDefinition.
		var leafOrNil = func() workflow.Definition {
			return random.Pick(t.Random,
				func() workflow.Definition { return nil },
				func() workflow.Definition { return MakeLeafDefinition(t) },
			)()
		}
		return workflow.For{
			Init: leafOrNil(),
			Cond: random.Pick(t.Random,
				func() workflow.Condition { return nil },
				func() workflow.Condition { return randomTemplateCondition(t) },
				func() workflow.Condition { return MakeCondition(t) },
			)(),
			Post: leafOrNil(),
			Do:   leafOrNil(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Break {
		return workflow.Break{}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.ForEach {
		// Use leaf-only `Do` here. A nested polymorphic Definition through
		// the same codec path is exercised by the top-level "ForEach
		// round-trips" test above, which deliberately uses MakeDefinition.
		return workflow.ForEach{
			Over: workflow.VarName(t.Random.String()),
			K:    workflow.VarName(t.Random.String()),
			V:    workflow.VarName(t.Random.String()),
			Do: random.Pick(t.Random,
				func() workflow.Definition { return nil },
				func() workflow.Definition { return MakeLeafDefinition(t) },
			)(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.SetVar {
		return workflow.SetVar{
			Name:  workflow.VarName(t.Random.String()),
			Value: randomSetVarValue(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.DeclareVar {
		return workflow.DeclareVar{
			Name:   workflow.VarName(t.Random.String()),
			Global: t.Random.Bool(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.DeleteVar {
		return workflow.DeleteVar{
			Name: workflow.VarName(t.Random.String()),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Increment {
		return workflow.Increment{
			Name: workflow.VarName(t.Random.String()),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Execute {
		if t.Random.Bool() {
			return workflow.Execute{
				Participant: workflow.ParticipantID(t.Random.String()),
				Input:       randomVarNames(t),
				Output:      randomVarNames(t),
			}
		}
		return workflow.Execute{
			Condition: workflow.ConditionID(t.Random.String()),
			Input:     randomVarNames(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Spawn {
		return workflow.Spawn{
			Name: workflow.SpawnName(t.Random.String()),
			Definition: random.Pick(t.Random,
				func() workflow.Definition { return nil },
				func() workflow.Definition { return MakeDefinition(t) },
			)(),
			Vars: makeVarMapping(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Defer {
		return workflow.Defer{
			Definition: random.Pick(t.Random,
				func() workflow.Definition { return nil },
				func() workflow.Definition { return MakeDefinition(t) },
			)(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.Join {
		var v workflow.Join
		v.Spawn = random.Pick(t.Random,
			func() workflow.SpawnName { return workflow.SpawnName(t.Random.String()) },
			func() workflow.SpawnName { return "" },
		)()
		// Collect could be anything between nil, empty or random values
		v.Collect = random.Pick(t.Random,
			func() workflow.VarMapping { return nil },
			func() workflow.VarMapping { return workflow.VarMapping{} },
			func() workflow.VarMapping {
				return random.Map(t.Random.IntBetween(3, 7), func() (workflow.VarName, workflow.VarName) {
					return workflow.VarName(t.Random.String()), workflow.VarName(t.Random.String())
				})
			},
		)()
		return v
	})

	specTypeCodec(s, codec, func(t *testcase.T) wftemplate.Condition {
		return randomTemplateCondition(t)
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventCompleted {
		return workflow.EventCompleted{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventDeclareVar {
		return workflow.EventDeclareVar{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
			Name:      workflow.VarName(t.Random.String()),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventSetVar {
		return workflow.EventSetVar{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
			Name:      workflow.VarName(t.Random.String()),
			Value: random.Pick(t.Random,
				func() any { return nil },
				func() any { return randomSetVarValue(t) },
			)(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventDeleteVar {
		return workflow.EventDeleteVar{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
			Name:      workflow.VarName(t.Random.String()),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventParticipant {
		return workflow.EventParticipant{
			EventID:       randomEventID(t),
			ProcessID:     randomEventProcessID(t),
			Timestamp:     randomEventTimestamp(t),
			ParticipantID: workflow.ParticipantID(t.Random.String()),
			Path:          randomPath(t),
			Input: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return randomSetVarValue(t)
			}),
			Output: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return randomSetVarValue(t)
			}),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventCondition {
		return workflow.EventCondition{
			EventID:     randomEventID(t),
			ProcessID:   randomEventProcessID(t),
			Timestamp:   randomEventTimestamp(t),
			ConditionID: workflow.ConditionID(t.Random.String()),
			Path:        randomPath(t),
			Input: random.Slice(t.Random.IntBetween(0, 3), func() any {
				return randomSetVarValue(t)
			}),
			Answer: t.Random.Bool(),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventUseDefinition {
		return workflow.EventUseDefinition{
			EventID:    randomEventID(t),
			ProcessID:  randomEventProcessID(t),
			Timestamp:  randomEventTimestamp(t),
			Definition: MakeDefinition(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventSpawn {
		childID, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return workflow.EventSpawn{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			ChildID:   childID,
			Name:      workflow.SpawnName(t.Random.String()),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventSleepCompleted {
		return workflow.EventSleepCompleted{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventDefer {
		return workflow.EventDefer{
			EventID:    randomEventID(t),
			ProcessID:  randomEventProcessID(t),
			Timestamp:  randomEventTimestamp(t),
			Path:       randomPath(t),
			Definition: MakeDefinition(t),
		}
	})

	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventForEachIteration {
		return workflow.EventForEachIteration{
			EventID:   randomEventID(t),
			ProcessID: randomEventProcessID(t),
			Timestamp: randomEventTimestamp(t),
			Path:      randomPath(t),
			Over:      workflow.VarName(t.Random.String()),
			K:         random.Pick(t.Random, "", workflow.VarName(t.Random.String())),
			V:         random.Pick(t.Random, "", workflow.VarName(t.Random.String())),
		}
	})

	// A failed participant call is recorded as an EventError, which is on the log just like the rest.
	specTypeCodec(s, codec, func(t *testcase.T) workflow.EventError {
		return workflow.EventError{
			EventID:       randomEventID(t),
			ProcessID:     randomEventProcessID(t),
			Timestamp:     randomEventTimestamp(t),
			Error:         t.Random.Error().Error(),
			Path:          randomPath(t),
			ParticipantID: workflow.ParticipantID(t.Random.String()),
		}
	})

	return s.AsSuite("Codec")
}

// assertRoundTripEvent is the Event counterpart of assertRoundTripDefinition.
func assertRoundTripEvent(tc *testcase.T, codec workflow.Codec, event workflow.Event) {
	data, err := codec.Marshal(&event)
	assert.NoError(tc, err, assert.MessageF("expected the %T to round-trip through the codec, but marshalling failed", event))
	assert.NotEmpty(tc, data)

	var got workflow.Event
	assert.NoError(tc, codec.Unmarshal(data, &got),
		assert.MessageF("expected the %T to round-trip through the codec, but unmarshalling failed\nJSON: %s", event, string(data)))
	assert.Equal(tc, event, got,
		assert.MessageF("expected the %T to round-trip through the codec\nJSON: %s", event, string(data)))
	assertPathsRoundTrip(tc, event, got, data)
}

// assertPathsRoundTrip checks the workflow.Path fields of an event Scope by Scope,
// the Var flags included.
//
// A workflow.Scope is identified by its name, so assert.Equal, which honours its Equal method,
// doesn't notice a codec that drops whether a Scope is a variable scope.
func assertPathsRoundTrip(tc *testcase.T, exp, got workflow.Event, data []byte) {
	tc.Helper()
	var (
		expV = reflectkit.BaseValueOf(exp)
		gotV = reflectkit.BaseValueOf(got)
	)
	if expV.Kind() != reflect.Struct || expV.Type() != gotV.Type() {
		return
	}
	for i := range expV.NumField() {
		if expV.Type().Field(i).Type != reflect.TypeFor[workflow.Path]() {
			continue
		}
		var (
			name    = expV.Type().Field(i).Name
			expPath = expV.Field(i).Interface().(workflow.Path)
			gotPath = gotV.Field(i).Interface().(workflow.Path)
		)
		assert.Equal(tc, len(expPath), len(gotPath))
		for j := range expPath {
			assert.True(tc, expPath[j] == gotPath[j],
				assert.MessageF("expected %T#%s to round-trip Scope by Scope\nexpected: %#v\ngot: %#v\nJSON: %s",
					exp, name, expPath, gotPath, string(data)))
		}
	}
}

// randomEventTimestamp builds a non-zero, varied timestamp for event fixtures
// so the round-trip asserts are not degenerate.
func randomEventTimestamp(tc *testcase.T) time.Time {
	return clock.Now().UTC().Add(-tc.Random.DurationBetween(time.Hour, 7*24*time.Hour))
}

func specTypeCodec[T any](s *testcase.Spec, c workflow.Codec, init testcase.VarInit[T]) {
	s.Context(fmt.Sprintf("%s", reflect.TypeFor[T]().String()), func(s *testcase.Spec) {
		var value = let.Var(s, func(t *testcase.T) T {
			var v T = init(t)
			testcase.OnFail(t, func() {
				t.Log("value:")
				t.LogPretty(v)
			})
			return v
		})

		s.Test("direct", func(t *testcase.T) {
			var exp = value.Get(t)

			// Marshaled as itself, the value is read back into its own type, which the caller knows.
			data, err := c.Marshal(exp)
			assert.NoError(t, err)

			testcase.OnFail(t, func() {
				t.Log("marshalled format:")
				t.Log(string(data))
			})

			var got T
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.Equal(t, exp, got)
		})

		if reflectkit.TypeImplements[T, workflow.Definition]() {
			definition := let.As[workflow.Definition](value)

			s.Test("can be unmarshaled as workflow.Definition", func(t *testcase.T) {
				exp := definition.Get(t)

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				testcase.OnFail(t, func() {
					t.Log("marshalled format:")
					t.Log(string(data))
				})

				var got workflow.Definition
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Definition](t, exp, got)
			})

			s.Test("can be composited in a workflow sequence", func(t *testcase.T) {
				var exp workflow.Definition = workflow.Sequence{
					definition.Get(t),
				}

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				var got workflow.Definition
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Definition](t, exp, got)
			})

			s.Test("can be composited in a workflow if then/else", func(t *testcase.T) {
				var def = workflow.If{
					Cond: randomTemplateCondition(t),
				}
				if t.Random.Bool() {
					def.Then = definition.Get(t)
				} else {
					def.Else = definition.Get(t)
				}
				var exp workflow.Definition = def

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				var got workflow.Definition
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Definition](t, exp, got)
			})
		}

		if reflectkit.TypeImplements[T, workflow.Condition]() {
			condition := let.As[workflow.Condition](value)

			s.Test("can be unmarshaled as workflow.Condition", func(t *testcase.T) {
				exp := condition.Get(t)

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				var got workflow.Condition
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Condition](t, exp, got)
			})

			s.Test("can be composited in a workflow definition's condition part", func(t *testcase.T) {
				var exp workflow.Definition = workflow.If{
					Cond: condition.Get(t),
				}

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				var got workflow.Definition
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Definition](t, exp, got)
			})
		}

		if reflectkit.TypeImplements[T, workflow.Event]() {
			event := let.As[workflow.Event](value)

			s.Test("can be unmarshaled as workflow.Event", func(t *testcase.T) {
				exp := event.Get(t)

				data, err := c.Marshal(&exp)
				assert.NoError(t, err)

				var got workflow.Event
				assert.NoError(t, c.Unmarshal(data, &got))

				assert.Equal[workflow.Event](t, exp, got)
				assertPathsRoundTrip(t, exp, got, data)
			})
		}
	})
}
