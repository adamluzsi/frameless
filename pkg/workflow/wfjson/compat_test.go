// Package wfjson_test contains the v1 wire-format compatibility contract for
// the wfjson codec.
//
// The codecs that ship with this package define a wire format that is
// persisted to a database. Event logs and stored definitions outlive any one
// version of the code: an `EventUseDefinition` written today must still decode
// correctly after the next breaking change is shipped, and ideally far longer.
//
// TestCompat_v1WireFormat pins the v1 of that wire format. For every concrete
// type the codec knows about, it builds a representative value with stable IDs,
// marshals it, and asserts:
//
//  1. The marshalled bytes match the snapshot that the codec produced on
//     2026-01-02 — the v1 baseline. Any rename of a JSON field, reordering of
//     struct fields, or change to an `@type` discriminator that would shift
//     the bytes trips this assertion.
//
//  2. Unmarshalling the snapshot, then re-marshalling the result, produces the
//     same bytes. The format is canonical: there is one and only one wire
//     representation for any given value, so a stored value decodes to the
//     same shape it was written from.
//
//  3. The marshalled bytes are a JSON object that carries an `@type`
//     discriminator. The polymorphic codec relies on that discriminator to
//     dispatch the value on Unmarshal; a regression that emits a bare value
//     without an envelope would still pass the byte-equality check above
//     for some shapes, but would silently break the decoder.
//
// To regenerate the v1 baseline after an intentional, ratified format change:
//
//	go test -tags wfjsongen -run TestGenerateV1Snapshots ./pkg/workflow/wfjson/
//
// The generator writes v1_snapshots.txt next to this file; paste the relevant
// lines into the snapshot constants below and commit the change as part of the
// version bump.
package wfjson_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/pkg/uuid"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
)

// Stable IDs are pinned so the snapshot bytes are deterministic across runs.
// Random UUIDs / timestamps would shift the marshalled output and break the
// byte-equality assertion that is the point of this test.
const (
	v1PIDString     = "00000000-0000-4000-8000-000000000001"
	v1EventIDString = "00000000-0000-4000-8000-000000000002"
	v1ChildIDString = "00000000-0000-4000-8000-000000000003"

	// v1Timestamp is the UTC instant used as the Timestamp field on every
	// event in the snapshot fixtures. Pinned so the wire format renders the
	// same RFC 3339 string regardless of when the test runs.
	v1Timestamp = "2026-01-02T03:04:05Z"
)

// v1StableIDs holds the parsed UUIDs that every snapshot fixture reuses. It is
// a let.Var so each subtest gets a fresh value but the underlying parsed UUID
// is computed once per process.
func v1StableIDs(t *testcase.T) (pid, childID workflow.ProcessID, evID workflow.EventID) {
	t.Helper()
	pidUUID, err := uuid.Parse(v1PIDString)
	assert.NoError(t, err)
	childUUID, err := uuid.Parse(v1ChildIDString)
	assert.NoError(t, err)
	evUUID, err := uuid.Parse(v1EventIDString)
	assert.NoError(t, err)
	return workflow.ProcessID(pidUUID), workflow.ProcessID(childUUID), workflow.EventID(evUUID)
}

func v1FixedTime(t *testcase.T) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, v1Timestamp)
	assert.NoError(t, err)
	return ts
}

// assertMatchesV1Snapshot marshals v, asserts the marshalled bytes equal the
// expected v1 baseline, then verifies the unmarshal + re-marshal round-trip
// reproduces the same bytes (so the format is canonical), then asserts the
// bytes carry an @type envelope so the polymorphic decoder can dispatch them.
func assertMatchesV1Snapshot(t *testcase.T, c workflow.Codec, v any, expected string) {
	t.Helper()

	actual, err := c.Marshal(v)
	assert.NoError(t, err)
	assert.Equal(t, expected, string(actual),
		assert.MessageF(
			"v1 wire format drifted.\n"+
				"expected (v1): %s\n"+
				"actual:        %s\n"+
				"if this change is intentional, regenerate v1_snapshots.txt with `go test -tags wfjsongen` "+
				"and bump the package version.",
			expected, string(actual)))

	// Canonical-form check: the same value, round-tripped through Unmarshal,
	// must re-emit the same bytes. A codec that produces JSON which decodes
	// but doesn't re-marshal identically is the silent-corruption shape of
	// bug — stored data looks fine but no longer matches what new code writes.
	got, reEmitted := roundTripViaInterface(t, c, actual, v)
	assert.Equal(t, expected, string(reEmitted),
		assert.MessageF("v1 wire format is not canonical for %T: round-tripped bytes differ", v))

	// Stability check: a third round-trip (marshal got, marshal again) must
	// produce the same bytes as the first re-emission. This catches a codec
	// whose Unmarshal produces a value that re-marshals differently from
	// the original input — the silent-corruption shape of regression.
	secondReEmit, err := c.Marshal(got)
	assert.NoError(t, err)
	assert.Equal(t, string(reEmitted), string(secondReEmit),
		assert.MessageF("v1 wire format is not idempotent for %T: round-trip drifts", v))

	assertHasTypeEnvelope(t, expected, string(reEmitted))
}

// roundTripViaInterface decodes actual back into v's interface type and
// re-marshals it. The interface dispatch (Definition / Condition / Event /
// Process*) is what production callers do, so the round-trip is checked
// through the same path the rest of the codebase uses.
func roundTripViaInterface(t *testcase.T, c workflow.Codec, actual []byte, v any) (any, []byte) {
	t.Helper()
	switch v.(type) {
	case workflow.Definition:
		var got workflow.Definition
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	case workflow.Condition:
		var got workflow.Condition
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	case workflow.Event:
		var got workflow.Event
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	case workflow.ProcessExecution:
		var got workflow.ProcessExecution
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	case workflow.ProcessSchedule:
		var got workflow.ProcessSchedule
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	case workflow.ProcessCancel:
		var got workflow.ProcessCancel
		assert.NoError(t, c.Unmarshal(actual, &got))
		reEmitted, err := c.Marshal(got)
		assert.NoError(t, err)
		return got, reEmitted
	default:
		t.Fatalf("roundTripViaInterface: unsupported fixture type %T", v)
		return nil, nil
	}
}

// assertHasTypeEnvelope asserts that the marshalled JSON for a polymorphic
// type carries an `@type` discriminator. A regression that emitted a bare
// value would lose the polymorphic decoder's ability to dispatch it on the
// other end. The Definition / Condition / Event / Process* interface paths all
// rely on the envelope; the only types that legitimately marshal without it
// are the concrete typed slices that the contract helper already covers.
func assertHasTypeEnvelope(t *testcase.T, expected, reEmitted string) {
	t.Helper()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(reEmitted), &probe); err != nil {
		return // non-object shape; nothing to check
	}
	if _, ok := probe["@type"]; !ok {
		assert.True(t, false,
			assert.MessageF(
				"v1 wire format lost its @type envelope.\nexpected: %s\nactual:   %s",
				expected, reEmitted))
	}
}

// TestCompat_v1WireFormat pins the wire format produced by wfjson.NewCodec()
// as of the v1 baseline (the initial registered type-IDs and DTO field names).
//
// A failure here means a future change has shifted the bytes the codec
// produces, which would silently invalidate any event log or stored
// definition written by a previous version. Treat as a backwards-compatibility
// regression unless the change is intentionally widening the wire format as
// part of a versioned migration.
func TestCompat_v1WireFormat(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	pid := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		p, _, _ := v1StableIDs(t)
		return p
	})
	childID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		_, c, _ := v1StableIDs(t)
		return c
	})
	evID := let.Var(s, func(t *testcase.T) workflow.EventID {
		_, _, e := v1StableIDs(t)
		return e
	})
	ts := let.Var(s, v1FixedTime)
	path := let.Var(s, func(t *testcase.T) workflow.Path {
		return workflow.Path{"sequence", "[0]", "participant"}
	})
	scope := let.Var(s, func(t *testcase.T) workflow.VarScope {
		return workflow.VarScope{"[0]"}
	})

	// ---- Definitions ----

	s.Test("Sequence (empty)", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Sequence{},
			`{"@type":"workflow::sequence","@value":[]}`)
	})

	s.Test("Sequence with elements", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Sequence{workflow.SetVar{Name: "n", Value: "v"}},
			`{"@type":"workflow::sequence","@value":[{"@type":"workflow::var::set","name":"n","value":"v"}]}`)
	})

	s.Test("If", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.If{
				Cond: workflow.ExecuteCondition{ID: "is-x", Input: []workflow.VarName{"a"}},
				Then: workflow.SetVar{Name: "t", Value: "1"},
				Else: workflow.SetVar{Name: "e", Value: "2"},
			},
			`{"@type":"workflow::if","cond":{"@type":"workflow::condition","id":"is-x","input":["a"]},"then":{"@type":"workflow::var::set","name":"t","value":"1"},"else":{"@type":"workflow::var::set","name":"e","value":"2"}}`)
	})

	s.Test("Sleep", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Sleep{
				While: wftemplate.Condition(".x == .y"),
				Until: workflow.ExecuteCondition{ID: "ok", Input: []workflow.VarName{"a"}},
			},
			`{"@type":"workflow::sleep","while":{"@type":"workflow::template::condition","@value":".x == .y"},"until":{"@type":"workflow::condition","id":"ok","input":["a"]}}`)
	})

	s.Test("For", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.For{
				Init: workflow.SetVar{Name: "i", Value: "0"},
				Cond: wftemplate.Condition(".i < 10"),
				Post: workflow.SetVar{Name: "i", Value: "1"},
				Do:   workflow.SetVar{Name: "x", Value: "1"},
			},
			// jsonkit's HTML-safe encoder escapes `<` to `\u003c`. Pin the
			// bytes as the codec actually produces them; if a future change
			// stops the escape, that change must be ratified.
			`{"@type":"workflow::for","init":{"@type":"workflow::var::set","name":"i","value":"0"},"cond":{"@type":"workflow::template::condition","@value":".i \u003c 10"},"post":{"@type":"workflow::var::set","name":"i","value":"1"},"do":{"@type":"workflow::var::set","name":"x","value":"1"}}`)
	})

	s.Test("ForEach", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ForEach{
				Over: workflow.VarName("items"),
				Do:   workflow.SetVar{Name: "x", Value: "1"},
				K:    workflow.VarName("k"),
				V:    workflow.VarName("v"),
			},
			`{"@type":"workflow::foreach","over":"items","do":{"@type":"workflow::var::set","name":"x","value":"1"},"key":"k","value":"v"}`)
	})

	s.Test("DeclareVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.DeclareVar{Name: "x", Global: true},
			`{"@type":"workflow::var::declare","name":"x","global":true}`)
	})

	s.Test("SetVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.SetVar{Name: "x", Value: "hello"},
			`{"@type":"workflow::var::set","name":"x","value":"hello"}`)
	})

	s.Test("DeleteVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.DeleteVar{Name: "x"},
			`{"@type":"workflow::var::delete","name":"x"}`)
	})

	s.Test("Increment", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Increment{Name: "x"},
			`{"@type":"workflow::op::increment","name":"x"}`)
	})

	s.Test("Spawn", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Spawn{
				Name:       "child1",
				Definition: workflow.SetVar{Name: "x", Value: "1"},
				Vars:       workflow.VarMapping{"a": "b"},
			},
			`{"@type":"workflow::spawn","name":"child1","def":{"@type":"workflow::var::set","name":"x","value":"1"},"vars":{"a":"b"}}`)
	})

	s.Test("ExecuteParticipant", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ExecuteParticipant{
				ID:     "p1",
				Input:  []workflow.VarName{"a", "b"},
				Output: []workflow.VarName{"c"},
			},
			`{"@type":"workflow::participant","id":"p1","input":["a","b"],"output":["c"]}`)
	})

	s.Test("ExecuteCondition", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ExecuteCondition{
				ID:    "c1",
				Input: []workflow.VarName{"a"},
			},
			`{"@type":"workflow::condition","id":"c1","input":["a"]}`)
	})

	s.Test("Join", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.Join{SpawnName: "child1", Collect: workflow.VarMapping{"x": "y"}},
			`{"@type":"workflow::join","spawn_name":"child1","collect":{"x":"y"}}`)
	})

	// ---- Conditions ----

	s.Test("TemplateCondition", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			wftemplate.Condition(".x == .y"),
			`{"@type":"workflow::template::condition","@value":".x == .y"}`)
	})

	// ---- Events ----

	s.Test("EventCompleted", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventCompleted{EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t)},
			`{"@type":"workflow::event::completed","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z"}`)
	})

	s.Test("EventTerminated", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventTerminated{EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t)},
			`{"@type":"workflow::event::terminated","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z"}`)
	})

	s.Test("EventDeclareVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventDeclareVar{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				Path: path.Get(t), Name: "x", Scope: scope.Get(t),
			},
			`{"@type":"workflow::event::var::declare","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","path":["sequence","[0]","participant"],"name":"x","scope":["[0]"]}`)
	})

	s.Test("EventSetVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventSetVar{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				Path: path.Get(t), Name: "x", Value: "v",
			},
			`{"@type":"workflow::event::var::set","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","path":["sequence","[0]","participant"],"name":"x","value":"v"}`)
	})

	s.Test("EventDeleteVar", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventDeleteVar{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				Path: path.Get(t), Name: "x",
			},
			`{"@type":"workflow::event::var::delete","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","path":["sequence","[0]","participant"],"name":"x"}`)
	})

	s.Test("EventParticipant", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventParticipant{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				ParticipantID: "p1",
				Path:          path.Get(t),
				Input:         []any{"a"},
				Output:        []any{"b"},
			},
			`{"@type":"workflow::event::participant","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","participant_id":"p1","path":["sequence","[0]","participant"],"input":["a"],"output":["b"]}`)
	})

	s.Test("EventCondition", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventCondition{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				ConditionID: "c1",
				Path:        path.Get(t),
				Input:       []any{"a"},
				Answer:      true,
			},
			`{"@type":"workflow::event::condition","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","condition_id":"c1","path":["sequence","[0]","participant"],"input":["a"],"answer":true,"timestamp":"2026-01-02T03:04:05Z"}`)
	})

	s.Test("EventUseDefinition", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventUseDefinition{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				Definition: workflow.SetVar{Name: "x", Value: "1"},
			},
			`{"@type":"workflow::event::use-definition","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","definition":{"@type":"workflow::var::set","name":"x","value":"1"}}`)
	})

	s.Test("EventSpawn", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventSpawn{
				EventID: evID.Get(t), ProcessID: pid.Get(t), ChildID: childID.Get(t),
				Name: "child1", Timestamp: ts.Get(t),
			},
			`{"@type":"workflow::event::spawn","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","child_id":"00000000-0000-4000-8000-000000000003","name":"child1","timestamp":"2026-01-02T03:04:05Z"}`)
	})

	s.Test("EventJoin", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.EventJoin{
				EventID: evID.Get(t), ProcessID: pid.Get(t), Timestamp: ts.Get(t),
				Children: []workflow.ProcessID{childID.Get(t)},
				Path:     workflow.Path{"sequence", "[0]"},
			},
			`{"@type":"workflow::event::join","event_id":"00000000-0000-4000-8000-000000000002","process_id":"00000000-0000-4000-8000-000000000001","timestamp":"2026-01-02T03:04:05Z","children":["00000000-0000-4000-8000-000000000003"],"path":["sequence","[0]"]}`)
	})

	// ---- Schedule-side ----

	s.Test("ProcessExecution", func(t *testcase.T) {
		// FailureCount is omitempty in the DTO, so the zero value drops from
		// the wire. That is the only field shape quirk the test has to know
		// about — pin the marshalled bytes accordingly.
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ProcessExecution{
				ProcessID: pid.Get(t),
				StartTime: ts.Get(t),
				CreatedAt: ts.Get(t),
				// FailureCount intentionally zero so the omitempty path is exercised.
			},
			`{"@type":"workflow::execution","process_id":"00000000-0000-4000-8000-000000000001","start":"2026-01-02T03:04:05Z","created_at":"2026-01-02T03:04:05Z"}`)
	})

	s.Test("ProcessSchedule", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ProcessSchedule{ProcessID: pid.Get(t)},
			`{"@type":"workflow::schedule","process_id":"00000000-0000-4000-8000-000000000001"}`)
	})

	s.Test("ProcessCancel", func(t *testcase.T) {
		assertMatchesV1Snapshot(t, codec.Get(t),
			workflow.ProcessCancel{ProcessID: pid.Get(t)},
			`{"@type":"workflow::cancel","process_id":"00000000-0000-4000-8000-000000000001"}`)
	})
}
