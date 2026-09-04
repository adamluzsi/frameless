//go:build wfjsongen
// +build wfjsongen

// Run with: go test -tags wfjsongen -run TestGenerateV1Snapshots ./pkg/workflow/wfjson/
//
// This file produces a fresh dump of what wfjson.NewCodec() marshals for each
// concrete type, written to v1_snapshots.txt in the same directory. It is the
// regeneration tool for the snapshot constants in compat_test.go.
//
// Usage after an intentional, ratified wire-format change:
//  1. Run this generator.
//  2. Open v1_snapshots.txt; copy each "name:\n<json>\n\n" block into the
//     corresponding assertMatchesV1Snapshot call in compat_test.go.
//  3. Commit compat_test.go + this file together, with a note in the commit
//     message that explains the format change and the version bump.
//
// The build tag keeps the file out of normal `go test` runs so it cannot
// silently mutate the test data.
package wfjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/uuid"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
)

func TestGenerateV1Snapshots(t *testing.T) {
	pidUUID, _ := uuid.Parse("00000000-0000-4000-8000-000000000001")
	evUUID, _ := uuid.Parse("00000000-0000-4000-8000-000000000002")
	childUUID, _ := uuid.Parse("00000000-0000-4000-8000-000000000003")
	pid := workflow.ProcessID(pidUUID)
	evID := workflow.EventID(evUUID)
	childPID := workflow.ProcessID(childUUID)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	c := wfjson.NewCodec()

	type entry struct {
		name string
		val  any
	}

	// Definitions
	entries := []entry{
		{"Sequence(empty)", workflow.Sequence{}},
		{"Sequence(1 elem)", workflow.Sequence{workflow.SetVar{Name: "n", Value: "v"}}},
		{"If", workflow.If{
			Cond: workflow.ExecuteCondition{ID: "is-x", Input: []workflow.VarName{"a"}},
			Then: workflow.SetVar{Name: "t", Value: "1"},
			Else: workflow.SetVar{Name: "e", Value: "2"},
		}},
		{"Sleep", workflow.Sleep{
			While: wftemplate.Condition(".x == .y"),
			Until: workflow.ExecuteCondition{ID: "ok", Input: []workflow.VarName{"a"}},
		}},
		{"For", workflow.For{
			Init: workflow.SetVar{Name: "i", Value: "0"},
			Cond: wftemplate.Condition(".i < 10"),
			Post: workflow.SetVar{Name: "i", Value: "1"},
			Do:   workflow.SetVar{Name: "x", Value: "1"},
		}},
		{"ForEach", workflow.ForEach{
			Over: workflow.VarName("items"),
			Do:   workflow.SetVar{Name: "x", Value: "1"},
			K:    workflow.VarName("k"),
			V:    workflow.VarName("v"),
		}},
		{"DeclareVar", workflow.DeclareVar{Name: "x", Global: true}},
		{"SetVar", workflow.SetVar{Name: "x", Value: "hello"}},
		{"DeleteVar", workflow.DeleteVar{Name: "x"}},
		{"Increment", workflow.Increment{Name: "x"}},
		{"Spawn", workflow.Spawn{
			Name:       "child1",
			Definition: workflow.SetVar{Name: "x", Value: "1"},
			Vars:       workflow.VarMapping{"a": "b"},
		}},
		{"ExecuteParticipant", workflow.ExecuteParticipant{
			ID:     "p1",
			Input:  []workflow.VarName{"a", "b"},
			Output: []workflow.VarName{"c"},
		}},
		{"ExecuteCondition", workflow.ExecuteCondition{
			ID:    "c1",
			Input: []workflow.VarName{"a"},
		}},
		{"Join", workflow.Join{SpawnName: "child1", Collect: workflow.VarMapping{"x": "y"}}},
		// Conditions
		{"TemplateCondition", wftemplate.Condition(".x == .y")},
		// Events
		{"EventCompleted", workflow.EventCompleted{EventID: evID, ProcessID: pid, Timestamp: ts}},
		{"EventTerminated", workflow.EventTerminated{EventID: evID, ProcessID: pid, Timestamp: ts}},
		{"EventDeclareVar", workflow.EventDeclareVar{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			Path:  workflow.Path{"sequence", "[0]", "participant"},
			Name:  "x",
			Scope: workflow.VarScope{"[0]"},
		}},
		{"EventSetVar", workflow.EventSetVar{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			Path: workflow.Path{"sequence", "[0]", "participant"},
			Name: "x", Value: "v",
		}},
		{"EventDeleteVar", workflow.EventDeleteVar{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			Path: workflow.Path{"sequence", "[0]", "participant"},
			Name: "x",
		}},
		{"EventParticipant", workflow.EventParticipant{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			ParticipantID: "p1",
			Path:          workflow.Path{"sequence", "[0]", "participant"},
			Input:         []any{"a"},
			Output:        []any{"b"},
		}},
		{"EventCondition", workflow.EventCondition{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			ConditionID: "c1",
			Path:        workflow.Path{"sequence", "[0]", "participant"},
			Input:       []any{"a"},
			Answer:      true,
		}},
		{"EventUseDefinition", workflow.EventUseDefinition{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			Definition: workflow.SetVar{Name: "x", Value: "1"},
		}},
		{"EventSpawn", workflow.EventSpawn{
			EventID:   evID,
			ProcessID: pid,
			ChildID:   childPID,
			Name:      "child1",
			Timestamp: ts,
		}},
		{"EventJoin", workflow.EventJoin{
			EventID: evID, ProcessID: pid, Timestamp: ts,
			Children: []workflow.ProcessID{childPID},
			Path:     workflow.Path{"sequence", "[0]"},
		}},
		// Schedule-side
		{"ProcessExecution", workflow.ProcessExecution{
			ProcessID: pid, StartTime: ts, CreatedAt: ts,
			// FailureCount zero so the omitempty path is exercised
		}},
		{"ProcessSchedule", workflow.ProcessSchedule{ProcessID: pid}},
		{"ProcessCancel", workflow.ProcessCancel{ProcessID: pid}},
	}

	var sb strings.Builder
	for _, e := range entries {
		data, err := c.Marshal(e.val)
		if err != nil {
			t.Fatalf("marshalling %s: %v", e.name, err)
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, data); err != nil {
			t.Fatalf("compacting %s: %v", e.name, err)
		}
		fmt.Fprintf(&sb, "%s:\n%s\n\n", e.name, buf.String())
	}

	if err := os.WriteFile("v1_snapshots.txt", []byte(sb.String()), 0644); err != nil {
		t.Fatalf("writing v1_snapshots.txt: %v", err)
	}
	t.Logf("wrote %d snapshots to v1_snapshots.txt", len(entries))
}
