package wfjson_test

import (
	"encoding/json"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

// This file documents the known broken branches of the polymorphic codec
// (jsonkit) that wfjson.NewCodec inherits. Each test pins down the
// expected behaviour that the codec SHOULD honour; they currently FAIL on
// the broken branches and will pass once the codec is fixed.
//
// They live outside the wfcontract.Codec suite so the rest of the codec
// contract keeps running cleanly. Run them with `-run TestWfjsonCodecBug`
// to focus on just these scenarios.
//
// All three bugs share the same shape: a typed envelope ("@type":"…") is
// emitted on Marshal, but the unmarshalling side either drops the envelope
// on the floor (Sequence-in-Sequence), refuses to interpret the envelope
// for primitive kinds (wftemplate.Condition), or fails to disambiguate
// the slice element type when the outer slice is itself a @type envelope
// (Sequence as concrete).

// TestWfjsonCodecBug_NestedSequenceRoundTrip pins Bug #1.
//
// A workflow.Sequence nested inside another workflow.Sequence marshals as
// {"@type":"workflow::sequence","@value":[{…},{[{…}]]}} — the inner
// Sequence is encoded as a bare JSON array rather than wrapped in its own
// @type envelope. The unmarshalling side then sees a `[]interface{}` for
// the inner element, can't convert it to workflow.Definition, and the
// round-trip fails.
//
// Once the codec wraps the inner Sequence in {"@type":"…","@value":[…]}
// (or otherwise surfaces it as a polymorphic workflow.Definition), this
// test will pass.
func TestWfjsonCodecBug_NestedSequenceRoundTrip(t *testing.T) {
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

	// Sanity-check the marshalled output: the inner Sequence must carry
	// its own @type envelope so the polymorphic decoder can dispatch it.
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

// TestWfjsonCodecBug_EmptySequenceDirectUnmarshal pins Bug #2.
//
// Marshalling an empty workflow.Sequence produces
// {"@type":"workflow::sequence","@value":[]}. Unmarshalling that back into
// the concrete type fails:
//
//	json: cannot unmarshal object into Go value of type []json.RawMessage
//
// because the slice unmarshaller does not honour the typed envelope for
// the outer slice itself.
//
// Unmarshalling through workflow.Definition works (the test would pass if
// this bug is fixed too), so this assertion exercises both paths: the
// concrete one (the failure mode) and the interface one (which currently
// masks the bug in practice).
func TestWfjsonCodecBug_EmptySequenceDirectUnmarshal(t *testing.T) {
	c := wfjson.NewCodec()

	def := workflow.Sequence{}
	data, err := c.Marshal(def)
	assert.NoError(t, err)

	// The direct, concrete-type unmarshal is the failing case.
	var got workflow.Sequence
	assert.NoError(t, c.Unmarshal(data, &got),
		assert.MessageF("empty workflow.Sequence should round-trip into its concrete type"))
	assert.Equal(t, def, got)

	// The interface unmarshal is the workaround that currently passes.
	var gotDef workflow.Definition
	assert.NoError(t, c.Unmarshal(data, &gotDef))
	assert.Equal[workflow.Definition](t, def, gotDef)
}

// TestWfjsonCodecBug_PrimitiveConditionDirectUnmarshal pins Bug #3.
//
// wftemplate.Condition is `type Condition string` — a Go primitive kind
// with a custom name. The codec wraps it in a typed envelope on Marshal
// ({"@type":"workflow::template::condition","@value":"…"}), but the
// unmarshalling side does not unwrap the envelope when the target is a
// named primitive type. The result is the canonical "cannot unmarshal
// object into Go value of type wftemplate.Condition" error.
//
// This is the same shape as Bug #2 but for primitive kinds: the typed
// envelope is emitted on the way out, the codec forgets how to undo it
// on the way in.
func TestWfjsonCodecBug_PrimitiveConditionDirectUnmarshal(t *testing.T) {
	c := wfjson.NewCodec()

	cond := wftemplate.Condition(".X == .Y")
	data, err := c.Marshal(cond)
	assert.NoError(t, err)

	// The direct, concrete-type unmarshal is the failing case.
	var got wftemplate.Condition
	assert.NoError(t, c.Unmarshal(data, &got),
		assert.MessageF("wftemplate.Condition should round-trip into its concrete type"))
	assert.Equal(t, cond, got)

	// The interface unmarshal is the workaround that currently passes.
	var gotCond workflow.Condition
	assert.NoError(t, c.Unmarshal(data, &gotCond))
	assert.Equal[workflow.Condition](t, cond, gotCond)
}

// TestWfjsonCodecBug_FuzzNestedSequence exercises Bug #1 across a spread
// of random nested Sequence shapes. This guards against the bug being
// fixed for the textbook case but regressing for, say, a Sequence whose
// only element is another Sequence.
//
// Each sub-test marshals the random shape and expects the marshalled JSON
// to carry a @type envelope on every nested Sequence element.
func TestWfjsonCodecBug_FuzzNestedSequence(t *testing.T) {
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
		// @type discriminator. This will catch the bare-array leakage
		// of nested Sequences.
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
			// envelope — that's what the codec dispatches on. If
			// the element is a bare JSON array instead, the nested
			// Sequence bug has struck.
			if _, isArray := child.([]any); isArray {
				assert.True(t, false,
					assert.MessageF("polymorphic slice element is a bare array, not a typed envelope: %#v", child))
			}
			walkFuzz(t, child)
		}
	}
}
