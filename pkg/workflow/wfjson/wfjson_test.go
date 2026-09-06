package wfjson_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

// TestPathDTO covers how a workflow.Path is written to and read back from a
// persisted event log.
//
// A path is a chain of scope names, so its wire form is a plain JSON array of
// strings. Event logs outlive the code that wrote them, which is why this
// stable, long-lived shape is pinned here.
func TestPathDTO(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			// path is the JSON value of the "path" field, as found in the event log.
			path = let.Var(s, func(t *testcase.T) string {
				return `["sequence","[0]","participant"]`
			})
			data = let.Var(s, func(t *testcase.T) []byte {
				return fmt.Appendf(nil, `{"@type":"workflow::event::var::set",`+
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

		s.Then("the path is restored as the chain of scope names", func(t *testcase.T) {
			got, ok := act(t).(workflow.EventSetVar)
			assert.True(t, ok, "expected an EventSetVar")

			assert.Equal(t, workflow.Path{"sequence", "[0]", "participant"}, got.Path)
		})

		s.When("the path field is absent", func(s *testcase.Spec) {
			data.Let(s, func(t *testcase.T) []byte {
				return []byte(`{"@type":"workflow::event::var::set",` +
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
					Path:      workflow.Path{"sequence", "[0]", "participant"},
					Name:      "n",
					Scope:     workflow.VarScope{"[0]"},
				}
			})
		)

		act := let.Act(func(t *testcase.T) workflow.Event {
			data, err := subject.Get(t).Marshal(event.Get(t))
			assert.NoError(t, err)

			var got workflow.Event
			assert.NoError(t, subject.Get(t).Unmarshal(data, &got))
			return got
		})

		s.Then("the path and the variable scope survive the round-trip", func(t *testcase.T) {
			got, ok := act(t).(workflow.EventDeclareVar)
			assert.True(t, ok, "expected an EventDeclareVar")

			assert.Equal(t, event.Get(t).Path, got.Path)
			assert.Equal(t, event.Get(t).Scope, got.Scope)
			assert.Equal(t, event.Get(t).Name, got.Name)
		})
	})
}

// TestWfjsonCodec_EmptySequenceRoundTrip pins the contract that an empty
// workflow.Sequence{} round-trips through the codec both as the polymorphic
// workflow.Definition (the path consumers normally use) AND directly into
// the concrete workflow.Sequence type (the path the jsonkit slice
// unmarshaller historically fumbled — the typed envelope was emitted on
// Marshal but not unwrapped when the target was a concrete slice type).
//
// Without this pin, a future regression that emits the slice as a bare
// `[]` (no @type envelope) would still pass the interface-based contract
// test in TestWfjsonCodecContract, because that helper routes through
// workflow.Definition. The concrete path is the silent failure mode.
func TestWfjsonCodec_EmptySequenceRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()

	def := workflow.Sequence{}

	data, err := c.Marshal(def)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Sanity-check the wire format: it must carry the typed envelope,
	// not a bare array, otherwise the decoder has no discriminator to
	// pick the right concrete type on the way back in.
	assert.Contains(t, string(data), `"@type":"workflow::sequence"`)
	assert.Contains(t, string(data), `"@value":[]`)

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

	// Re-marshal the unmarshalled value and confirm the wire is stable.
	data2, err := c.Marshal(gotConcrete)
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

	data, err := c.Marshal(cond)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Sanity-check the wire format: it must carry the typed envelope,
	// not a bare string, otherwise the decoder has no discriminator.
	assert.Contains(t, string(data), `"@type":"workflow::template::condition"`)
	assert.Contains(t, string(data), `"@value":".X == .Y"`)

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

	// Re-marshal the unmarshalled value and confirm the wire is stable.
	data2, err := c.Marshal(gotConcrete)
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
	// implementations. The codec must dispatch on the @type envelope so
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
			data, err := c.Marshal(tc.ev)
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

	// Note: the inner SetVar's Value is a string, not an int. JSON round-trips
	// `any`-typed numbers as float64, so an `int` literal would fail the deep
	// equality check below even when the codec itself is correct.
	outer := workflow.Sequence{
		workflow.SetVar{Name: "marker", Value: "before"},
		workflow.Sequence{
			workflow.SetVar{Name: "inner", Value: "marker-value"},
		},
		workflow.SetVar{Name: "after", Value: "after"},
	}

	data, err := c.Marshal(outer)
	assert.NoError(t, err)

	t.Logf("marshalled JSON: %s", string(data))

	// Sanity-check the marshalled output: the inner Sequence carries its
	// own @type envelope so the polymorphic decoder can dispatch it.
	var probe map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(data, &probe))

	var values []json.RawMessage
	assert.NoError(t, json.Unmarshal(probe["@value"], &values))
	assert.Equal(t, 3, len(values), "expected three sequence elements")
	inner := values[1]
	assert.True(t,
		len(inner) > 0 && inner[0] == '{',
		assert.MessageF("nested workflow.Sequence must round-trip with a @type envelope, got bare array: %s", string(inner)))

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
	//
	// Note: SetVar#Value is typed as any, and JSON has only one number kind.
	// An int literal like 0 round-trips as float64, which fails deep equality
	// even when the codec itself is correct. Strings are stable across the
	// round trip, so the reproducer uses string-typed values throughout.
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

		// Use a Sequence with a few stable-typed leaf values so the
		// round-trip is unaffected by JSON's int→float64 quirk.
		def := workflow.Sequence{
			workflow.SetVar{Name: "v", Value: "ok"},
			workflow.ExecuteParticipant{ID: "echo"},
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
		// own typed envelope (`workflow::sequence`) and embedded under
		// the `definition` key. Without this the round-trip below would
		// still pass on a happy-path, but a regression in the envelope
		// shape would silently break any external consumer that reads
		// the JSON directly.
		assert.Contains(t, string(data), `"definition":`,
			assert.MessageF("a non-nil Definition must appear on the wire\nJSON: %s", string(data)))
		assert.Contains(t, string(data), `"@type":"workflow::sequence"`,
			assert.MessageF("a nested Sequence Definition must carry its @type envelope\nJSON: %s", string(data)))

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
// {"@type":"workflow::sequence","@value":[]} which fails to unmarshal back
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

	def := workflow.Sequence{}
	data, err := c.Marshal(def)
	assert.NoError(t, err)

	// Direct, concrete-type unmarshal.
	var got workflow.Sequence
	assert.NoError(t, c.Unmarshal(data, &got),
		assert.MessageF("empty workflow.Sequence should round-trip into its concrete type"))
	assert.Equal(t, def, got)

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
	data, err := c.Marshal(cond)
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

	s.Test("nested Sequence always carries a @type envelope", func(t *testcase.T) {
		// Build a sequence whose elements are a mix of leaves and
		// (possibly nested) Sequences.
		def := workflow.Sequence{
			MakeNestedFuzzLeaf(t),
			MakeNestedFuzzSequence(t, t.Random.IntBetween(0, 2)),
			MakeNestedFuzzLeaf(t),
		}

		data, err := c.Marshal(def)
		assert.NoError(t, err)

		t.Logf("marshalled JSON: %s", string(data))

		// Walk the marshalled JSON and assert that every JSON object
		// that contains a `Definition`-shaped envelope also carries a
		// @type discriminator. Catches the bare-array leakage shape of
		// nested Sequences.
		AssertAllObjectsHaveTypeEnvelope(t, data)
	})

	s.Test("nested Sequence round-trips through the polymorphic interface", func(t *testcase.T) {
		def := workflow.Sequence{
			MakeNestedFuzzLeaf(t),
			MakeNestedFuzzSequence(t, t.Random.IntBetween(0, 2)),
			MakeNestedFuzzLeaf(t),
		}

		data, err := c.Marshal(def)
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
			return workflow.ExecuteParticipant{ID: workflow.ParticipantID(t.Random.String())}
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
// "@type" discriminator. This is the structural assertion that the bug
// breaks today: nested Sequence elements are emitted as bare JSON arrays
// instead of {"@type":"workflow::sequence","@value":[…]} envelopes.
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
		// Walk each child. Typed envelopes (carrying "@type") are
		// allowed; their "@value" payload is descended into so the
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
