package promise_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
)

var record = flag.Bool("record", false,
	"record the fixture of every scenario that has none yet; an existing fixture is never overwritten")

// scenario is a workflow written with the current API,
// whose fixture pins down what an earlier version stored for it.
//
// Its Definition runs until it suspends, mostly on compat::await-upgrade, which stands for the upgrade:
// everything before it was recorded by the version that wrote the fixture,
// and everything after it runs on the current version.
// The children the Process spawns run along with it, and suspend on compat::await-upgrade the same way.
type scenario struct {
	// Name identifies the scenario and names its fixture, so it never changes.
	Name string
	// Definition is the scenario's workflow, written with the current API.
	Definition workflow.Definition
	// FailsBeforeUpgrade tells that the Process fails before the upgrade, rather than suspending,
	// like a Process whose fix is what the upgrade ships.
	FailsBeforeUpgrade bool
	// ResumedCalls lists the participants and conditions called after the upgrade, in order,
	// the ones of the spawned children included.
	// Whatever was recorded before the upgrade is replayed from the history instead.
	ResumedCalls []string
	// Vars are the process variables once the resumed Process has completed.
	// They are the variables of the root scope;
	// the ones of a nested scope, like a loop body, are checked where they are read, with compat::see.
	Vars map[workflow.VarName]any
	// ChildVars are the variables of each spawned child once the resumed Process has completed,
	// listed by spawn name, in the order the children of a name were spawned.
	ChildVars map[workflow.SpawnName][]map[workflow.VarName]any
}

// scenarios is append-only: a recorded scenario is a promise.
var scenarios = []scenario{
	{
		// The steps recorded before the upgrade are not executed again.
		Name: "sequence",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: stepA},
			workflow.Execute{Participant: stepB},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: stepC},
		},
		ResumedCalls: []string{string(stepC)},
	},
	{
		// The condition answered true before the upgrade, but it would answer false after it,
		// so resuming in the Else branch means the recorded answer got lost.
		Name: "if-with-a-condition-answered-before-the-upgrade",
		Definition: workflow.If{
			Cond: workflow.Execute{Condition: beforeUpgrade},
			Then: workflow.Sequence{
				workflow.Execute{Participant: awaitUpgrade},
				workflow.Execute{Participant: stepA},
			},
			Else: workflow.Execute{Participant: stepB},
		},
		ResumedCalls: []string{string(stepA)},
	},
	{
		// The template answered true before the upgrade, then the variable it reads changed,
		// so resuming in the Else branch means the recorded answer got lost.
		Name: "if-with-a-template-condition-answered-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "stage", Value: "before"},
			workflow.If{
				Cond: wftemplate.Condition(`eq .stage "before"`),
				Then: workflow.Sequence{
					workflow.SetVar{Name: "stage", Value: "after"},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: stepA},
				},
				Else: workflow.Execute{Participant: stepB},
			},
		},
		ResumedCalls: []string{string(stepA)},
		Vars:         map[workflow.VarName]any{"stage": "after"},
	},
	{
		// The Output recorded before the upgrade is the Input of a call after it.
		Name: "input-and-output-variables",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: greet, Input: []workflow.VarName{"greeting"}, Output: []workflow.VarName{"reply"}},
		},
		ResumedCalls: []string{string(greet)},
		Vars: map[workflow.VarName]any{
			"name":     "world",
			"greeting": "hello world",
			"reply":    "hello hello world",
		},
	},
	{
		// The cleanup deferred before the upgrade runs when the Process completes after it.
		Name: "defer-hook",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: stepA},
			workflow.Defer{Definition: workflow.Execute{Participant: stepB}},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: stepC},
		},
		ResumedCalls: []string{string(stepC), string(stepB)},
	},
	{
		// The first round was recorded before the upgrade, so only the second one calls its body.
		Name: "for-loop-resumed-mid-round",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "i", Value: 0},
			workflow.For{
				Cond: wftemplate.Condition(`lt .i 2`),
				Post: workflow.Increment{Name: "i"},
				Do: workflow.Sequence{
					workflow.Execute{Participant: stepA},
					workflow.Execute{Participant: awaitUpgrade},
				},
			},
		},
		ResumedCalls: []string{string(stepA)},
		Vars:         map[workflow.VarName]any{"i": 2},
	},
	{
		// The first element was handled before the upgrade, so only the second one calls the body.
		Name: "foreach-resumed-mid-collection",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "names", Value: []string{"alice", "bob"}},
			workflow.ForEach{
				Over: "names",
				V:    "name",
				Do: workflow.Sequence{
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
				},
			},
		},
		ResumedCalls: []string{string(greet)},
		Vars:         map[workflow.VarName]any{"names": []string{"alice", "bob"}},
	},
	{
		// The child was spawned, and it completed, before the upgrade,
		// so the resumed parent neither spawns it again nor waits on it.
		Name: "spawn-recorded-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Spawn{Name: "child", Definition: workflow.Execute{Participant: stepB}},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Join{},
			workflow.Execute{Participant: stepA},
		},
		ResumedCalls: []string{string(stepA)},
		ChildVars:    map[workflow.SpawnName][]map[workflow.VarName]any{"child": {nil}},
	},
	{
		// The parent waits on the child across the upgrade,
		// and the child reads the variable forwarded to it before the upgrade after it.
		Name: "join-waiting-on-a-child-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.Spawn{
				Name: "greeter",
				Definition: workflow.Sequence{
					workflow.Execute{Participant: stepB},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
				},
				Vars: workflow.VarMapping{"name": "name"},
			},
			workflow.Join{Spawn: "greeter"},
			workflow.Execute{Participant: stepA},
		},
		ResumedCalls: []string{string(greet), string(stepA)},
		Vars:         map[workflow.VarName]any{"name": "world"},
		ChildVars: map[workflow.SpawnName][]map[workflow.VarName]any{
			"greeter": {{"name": "world", "greeting": "hello world"}},
		},
	},
	{
		// The body of the first round declared a variable before the upgrade, and the same round reads it after.
		Name: "for-loop-body-variable-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.SetVar{Name: "i", Value: 0},
			workflow.For{
				Cond: wftemplate.Condition(`lt .i 2`),
				Post: workflow.Increment{Name: "i"},
				Do: workflow.Sequence{
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
				},
			},
		},
		ResumedCalls: []string{seen("hello world"), string(greet), seen("hello world")},
		Vars:         map[workflow.VarName]any{"name": "world", "i": 2},
	},
	{
		// The first iteration declared a variable before the upgrade, and the same iteration reads it after.
		Name: "foreach-body-variable-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "names", Value: []string{"alice", "bob"}},
			workflow.ForEach{
				Over: "names",
				V:    "name",
				Do: workflow.Sequence{
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
				},
			},
		},
		ResumedCalls: []string{seen("hello alice"), string(greet), seen("hello bob")},
		Vars:         map[workflow.VarName]any{"names": []string{"alice", "bob"}},
	},
	{
		// The deferred definition declared a variable in its own scope before the upgrade, and reads it after.
		Name: "deferred-variable-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.Defer{Definition: workflow.Sequence{
				workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
				workflow.Execute{Participant: awaitUpgrade},
				workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
			}},
		},
		ResumedCalls: []string{seen("hello world")},
		Vars:         map[workflow.VarName]any{"name": "world"},
	},
	{
		// The parent waits on the child across the upgrade,
		// then collects the variable the child set before the upgrade.
		Name: "join-collecting-a-child-variable-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.Spawn{
				Name: "greeter",
				Definition: workflow.Sequence{
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
				},
				Vars: workflow.VarMapping{"name": "name"},
			},
			workflow.Join{Spawn: "greeter", Collect: workflow.VarMapping{"greeting": "reply"}},
			workflow.Execute{Participant: see, Input: []workflow.VarName{"reply"}},
		},
		ResumedCalls: []string{seen("hello world")},
		Vars:         map[workflow.VarName]any{"name": "world", "reply": "hello world"},
		ChildVars: map[workflow.SpawnName][]map[workflow.VarName]any{
			"greeter": {{"name": "world", "greeting": "hello world"}},
		},
	},
	{
		// The child was joined, and its variable collected, before the upgrade,
		// then the parent reassigned the collected variable,
		// so collecting it again on the resume would undo the reassignment.
		Name: "join-collected-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Spawn{Name: "greeter", Definition: workflow.SetVar{Name: "greeting", Value: "hello"}},
			workflow.Join{Spawn: "greeter", Collect: workflow.VarMapping{"greeting": "reply"}},
			workflow.Execute{Participant: greet, Input: []workflow.VarName{"reply"}, Output: []workflow.VarName{"reply"}},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: see, Input: []workflow.VarName{"reply"}},
		},
		ResumedCalls: []string{seen("hello hello")},
		Vars:         map[workflow.VarName]any{"reply": "hello hello"},
		ChildVars: map[workflow.SpawnName][]map[workflow.VarName]any{
			"greeter": {{"greeting": "hello"}},
		},
	},
	{
		// The branch declared a variable in its own scope before the upgrade, and reads it after.
		Name: "if-branch-variable-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.If{
				Cond: wftemplate.Condition(`eq .name "world"`),
				Then: workflow.Sequence{
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
				},
			},
		},
		ResumedCalls: []string{seen("hello world")},
		Vars:         map[workflow.VarName]any{"name": "world"},
	},
	{
		// Every round spawns a greeter of its own, and collects the greeting of the child it spawned.
		// The first round was done before the upgrade,
		// so the resumed Process replays it, and spawns the child of the second round only.
		Name: "foreach-joining-a-child-per-round-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "names", Value: []string{"alice", "bob"}},
			workflow.ForEach{
				Over: "names",
				V:    "name",
				Do: workflow.Sequence{
					workflow.Spawn{
						Name:       "greeter",
						Definition: workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
						Vars:       workflow.VarMapping{"name": "name"},
					},
					workflow.Join{Spawn: "greeter", Collect: workflow.VarMapping{"greeting": "greeting"}},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
				},
			},
		},
		ResumedCalls: []string{string(greet), seen("hello bob")},
		Vars:         map[workflow.VarName]any{"names": []string{"alice", "bob"}},
		ChildVars: map[workflow.SpawnName][]map[workflow.VarName]any{
			"greeter": {
				{"name": "alice", "greeting": "hello alice"},
				{"name": "bob", "greeting": "hello bob"},
			},
		},
	},
	{
		// The template answered false before the upgrade, then the variable it reads changed,
		// so resuming in the Then branch means the recorded answer got lost.
		Name: "if-else-branch-taken-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "stage", Value: "before"},
			workflow.If{
				Cond: wftemplate.Condition(`eq .stage "after"`),
				Then: workflow.Execute{Participant: stepB},
				Else: workflow.Sequence{
					workflow.SetVar{Name: "stage", Value: "after"},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: stepA},
				},
			},
		},
		ResumedCalls: []string{string(stepA)},
		Vars:         map[workflow.VarName]any{"stage": "after"},
	},
	{
		// The Sleep woke up before the upgrade, on an answer it would no longer get after it,
		// so sleeping again means the recorded wake-up got lost.
		Name: "sleep-woken-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Sleep{Until: workflow.Execute{Condition: beforeUpgrade}},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: stepA},
		},
		ResumedCalls: []string{string(stepA)},
	},
	{
		// The Sleep was kept asleep before the upgrade, and it asks again after it, in a later second,
		// so replaying the answer that kept it asleep would keep it asleep for good.
		Name: "sleep-asleep-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: stepB},
			workflow.Sleep{While: workflow.Execute{Condition: beforeUpgrade}},
			workflow.Execute{Participant: stepA},
		},
		ResumedCalls: []string{string(beforeUpgrade), string(stepA)},
	},
	{
		// Init, and the first round, which counted i to 1, were recorded before the upgrade,
		// so running Init again after it would reset i, and the loop would take an extra round.
		Name: "for-loop-init-recorded-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.For{
				Init: workflow.SetVar{Name: "i", Value: 0},
				Cond: wftemplate.Condition(`lt .i 2`),
				Post: workflow.Increment{Name: "i"},
				Do: workflow.Sequence{
					workflow.Execute{Participant: stepA},
					workflow.If{
						Cond: wftemplate.Condition(`eq .i 1`),
						Then: workflow.Execute{Participant: awaitUpgrade},
					},
				},
			},
			workflow.Execute{Participant: stepB},
		},
		ResumedCalls: []string{string(stepB)},
	},
	{
		// The loop has no Cond, so it only ends on the Break it reaches after the upgrade.
		Name: "for-loop-broken-out-of-after-the-upgrade",
		Definition: workflow.Sequence{
			workflow.For{
				Do: workflow.Sequence{
					workflow.Execute{Participant: stepA},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Break{},
				},
			},
			workflow.Execute{Participant: stepB},
		},
		ResumedCalls: []string{string(stepB)},
	},
	{
		// The iteration over the entry of a map was recorded before the upgrade,
		// and its key and value are read after it, in the body, and in the hook deferred from the body.
		// The map has a single entry, as Go iterates a map in no particular order.
		Name: "foreach-over-a-map-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "roles", Value: map[string]string{"alice": "admin"}},
			workflow.ForEach{
				Over: "roles",
				K:    "name",
				V:    "role",
				Do: workflow.Sequence{
					workflow.Defer{Definition: workflow.Execute{Participant: see, Input: []workflow.VarName{"role"}}},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
				},
			},
		},
		ResumedCalls: []string{string(greet), seen("hello alice"), seen("admin")},
		Vars:         map[workflow.VarName]any{"roles": map[string]string{"alice": "admin"}},
	},
	{
		// The branch declared a variable that shadows an outer one, and a global one, before the upgrade,
		// and they are read after it: the shadowing one in the branch, and the global one after the If.
		Name: "declared-variables-read-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "name", Value: "world"},
			workflow.If{
				Cond: wftemplate.Condition(`eq .name "world"`),
				Then: workflow.Sequence{
					workflow.DeclareVar{Name: "name"},
					workflow.SetVar{Name: "name", Value: "branch"},
					workflow.DeclareVar{Name: "greeting", Global: true},
					workflow.Execute{Participant: greet, Input: []workflow.VarName{"name"}, Output: []workflow.VarName{"greeting"}},
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: see, Input: []workflow.VarName{"name"}},
				},
			},
			workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
		},
		ResumedCalls: []string{seen("branch"), seen("hello branch")},
		Vars:         map[workflow.VarName]any{"name": "world", "greeting": "hello branch"},
	},
	{
		// The variable deleted before the upgrade stays deleted after it,
		// while the call that read it before it was deleted is still replayed.
		Name: "variable-deleted-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "token", Value: "secret"},
			workflow.Execute{Participant: greet, Input: []workflow.VarName{"token"}, Output: []workflow.VarName{"greeting"}},
			workflow.DeleteVar{Name: "token"},
			workflow.Execute{Participant: awaitUpgrade},
			workflow.Execute{Participant: see, Input: []workflow.VarName{"greeting"}},
		},
		ResumedCalls: []string{seen("hello secret")},
		Vars:         map[workflow.VarName]any{"greeting": "hello secret"},
	},
	{
		// Every kind of value set before the upgrade reads back after it as the same value, of the same type.
		Name: "variable-values-of-every-kind-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "int", Value: 42},
			workflow.SetVar{Name: "negative_int", Value: -7},
			workflow.SetVar{Name: "big_int", Value: 1<<62 + 1},
			workflow.SetVar{Name: "whole_float", Value: 2.0},
			workflow.SetVar{Name: "float", Value: 3.14},
			workflow.SetVar{Name: "big_float", Value: 1e21},
			workflow.SetVar{Name: "bool", Value: true},
			workflow.SetVar{Name: "string", Value: "hello"},
			workflow.SetVar{Name: "nil", Value: nil},
			workflow.SetVar{Name: "slice", Value: []int{1, 2}},
			workflow.SetVar{Name: "map", Value: map[string]int{"one": 1}},
			workflow.Execute{Participant: awaitUpgrade},
		},
		Vars: map[workflow.VarName]any{
			"int":          42,
			"negative_int": -7,
			"big_int":      1<<62 + 1,
			"whole_float":  2.0,
			"float":        3.14,
			"big_float":    1e21,
			"bool":         true,
			"string":       "hello",
			"nil":          nil,
			"slice":        []int{1, 2},
			"map":          map[string]int{"one": 1},
		},
	},
	{
		// The condition answered before the upgrade, given a number,
		// so its answer is replayed after it only as long as the number reads back the same.
		Name: "if-with-a-condition-given-a-number-answered-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.SetVar{Name: "n", Value: 42},
			workflow.If{
				Cond: workflow.Execute{Condition: beforeUpgradeGiven, Input: []workflow.VarName{"n"}},
				Then: workflow.Sequence{
					workflow.Execute{Participant: awaitUpgrade},
					workflow.Execute{Participant: stepA},
				},
				Else: workflow.Execute{Participant: stepB},
			},
		},
		ResumedCalls: []string{string(stepA)},
		Vars:         map[workflow.VarName]any{"n": 42},
	},
	{
		// The follow-up definition compat::plan returned before the upgrade is resumed after it,
		// without asking compat::plan for it again.
		Name: "follow-up-definition-resumed-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: stepA},
			workflow.Execute{Participant: plan},
		},
		ResumedCalls: []string{string(stepC)},
	},
	{
		// compat::replace replaced the definition of the Process before the upgrade,
		// so the Process resumes in the replacement, rather than in the definition it was bound to.
		Name: "replaced-definition-resumed-across-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: replace},
			workflow.Execute{Participant: stepB},
		},
		ResumedCalls: []string{string(stepA)},
	},
	{
		// The Process failed before the upgrade, which ships the fix,
		// so it carries on after it from the step that failed, with the failure on record.
		Name: "failed-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Execute{Participant: stepA},
			workflow.Execute{Participant: failBeforeUpgrade},
			workflow.Execute{Participant: stepB},
		},
		FailsBeforeUpgrade: true,
		ResumedCalls:       []string{string(failBeforeUpgrade), string(stepB)},
	},
	{
		// One child was terminated before the upgrade, so it stays terminated after it:
		// neither its remaining steps, nor its deferred hook, run again.
		// The parent waits on the other child across the upgrade, so the children run after it too.
		Name: "child-terminated-before-the-upgrade",
		Definition: workflow.Sequence{
			workflow.Spawn{Name: "terminated", Definition: workflow.Sequence{
				workflow.Defer{Definition: workflow.Execute{Participant: stepC}},
				workflow.Execute{Participant: terminate},
				workflow.Execute{Participant: stepB},
			}},
			workflow.Spawn{Name: "waiting", Definition: workflow.Execute{Participant: awaitUpgrade}},
			workflow.Join{Spawn: "waiting"},
			workflow.Execute{Participant: stepA},
		},
		ResumedCalls: []string{string(stepA)},
		ChildVars:    map[workflow.SpawnName][]map[workflow.VarName]any{"terminated": {nil}, "waiting": {nil}},
	},
}

// The vocabulary of the scenarios.
//
// Recorded histories refer to these IDs, so an ID keeps its meaning and signature for good.
// Add a new one rather than changing an existing one.
const (
	stepA workflow.ParticipantID = "compat::a"
	stepB workflow.ParticipantID = "compat::b"
	stepC workflow.ParticipantID = "compat::c"
	// greet takes a name and returns "hello <name>".
	greet workflow.ParticipantID = "compat::greet"
	// see takes a value, and is listed among the calls together with it (see seen),
	// so a scenario can tell what a variable held where the step read it.
	see workflow.ParticipantID = "compat::see"
	// awaitUpgrade suspends the Process until the upgrade.
	awaitUpgrade workflow.ParticipantID = "compat::await-upgrade"
	// beforeUpgrade answers whether the Process still runs on the version that recorded it,
	// so an answer recorded before the upgrade differs from a fresh one.
	beforeUpgrade workflow.ConditionID = "compat::before-upgrade"
	// failBeforeUpgrade fails until the upgrade, like a participant whose fix the upgrade ships.
	failBeforeUpgrade workflow.ParticipantID = "compat::fail-before-upgrade"
	// terminate calls off the Process it runs in.
	terminate workflow.ParticipantID = "compat::terminate"
	// plan returns the follow-up definition Sequence{compat::b, compat::await-upgrade, compat::c}.
	plan workflow.ParticipantID = "compat::plan"
	// replace replaces the definition of the Process with Sequence{compat::c, compat::await-upgrade, compat::a}.
	replace workflow.ParticipantID = "compat::replace"
	// beforeUpgradeGiven takes a number, and answers the way beforeUpgrade does.
	// It is listed among the calls together with the number it was given.
	beforeUpgradeGiven workflow.ConditionID = "compat::before-upgrade-given"
)

// errBeforeUpgrade is how compat::fail-before-upgrade fails.
var errBeforeUpgrade = errors.New("compat: failing until the upgrade")

// vocabulary implements the scenarios' participants and conditions.
type vocabulary struct {
	// upgraded tells whether the Process runs on the current version.
	upgraded bool
	// calls lists the participants and conditions called so far, in order.
	// compat::await-upgrade only stands for the upgrade, so it is not listed.
	calls []string
}

// seen is how a call to compat::see is listed, with the value it was given.
func seen(value string) string {
	return fmt.Sprintf("%s(%s)", see, value)
}

func (v *vocabulary) participants() workflow.Participants {
	step := func(id workflow.ParticipantID) func(context.Context) error {
		return func(context.Context) error {
			v.calls = append(v.calls, string(id))
			return nil
		}
	}
	return workflow.Participants{
		stepA: step(stepA),
		stepB: step(stepB),
		stepC: step(stepC),
		greet: func(_ context.Context, name string) (string, error) {
			v.calls = append(v.calls, string(greet))
			return "hello " + name, nil
		},
		see: func(_ context.Context, value string) error {
			v.calls = append(v.calls, seen(value))
			return nil
		},
		awaitUpgrade: func(context.Context) error {
			if !v.upgraded {
				return workflow.Suspend{}
			}
			return nil
		},
		failBeforeUpgrade: func(context.Context) error {
			v.calls = append(v.calls, string(failBeforeUpgrade))
			if !v.upgraded {
				return errBeforeUpgrade
			}
			return nil
		},
		terminate: func(context.Context) error {
			v.calls = append(v.calls, string(terminate))
			return workflow.Terminate{}
		},
		plan: func(context.Context) error {
			v.calls = append(v.calls, string(plan))
			return workflow.Sequence{
				workflow.Execute{Participant: stepB},
				workflow.Execute{Participant: awaitUpgrade},
				workflow.Execute{Participant: stepC},
			}
		},
		replace: func(context.Context) error {
			v.calls = append(v.calls, string(replace))
			return workflow.Replace{Definition: workflow.Sequence{
				workflow.Execute{Participant: stepC},
				workflow.Execute{Participant: awaitUpgrade},
				workflow.Execute{Participant: stepA},
			}}
		},
	}
}

func (v *vocabulary) conditions() workflow.Conditions {
	return workflow.Conditions{
		beforeUpgrade: func(context.Context) (bool, error) {
			v.calls = append(v.calls, string(beforeUpgrade))
			return !v.upgraded, nil
		},
		beforeUpgradeGiven: func(_ context.Context, n int) (bool, error) {
			v.calls = append(v.calls, fmt.Sprintf("%s(%d)", beforeUpgradeGiven, n))
			return !v.upgraded, nil
		},
	}
}

// newRuntime makes a Runtime which executes on request only,
// so a suspended Process stays suspended until the test resumes it.
func newRuntime(events workflow.EventRepository, v *vocabulary) workflow.Runtime {
	return workflow.Runtime{
		Events:        events,
		Participants:  v.participants(),
		Conditions:    v.conditions(),
		Locks:         &memory.WorkflowProcessLocks{},
		Queue:         &memory.WorkflowQueue{},
		Notifications: &memory.WorkflowNotificationBroadcast{},
		Codec:         wfjson.NewCodec(),
		RetryStrategy: noRetry{},
	}
}

// noRetry makes a failure show at once, rather than after the retries.
type noRetry struct{}

func (noRetry) ShouldTry(_ context.Context, attempt resilience.RetryAttempt) bool {
	return attempt.FailureCount == 0
}

// run executes the Process, together with the children it spawned, as far as they can get.
//
// Nothing runs the scheduled children in the background, so they run here:
// whenever the Process suspends, maybe to Join them, its unfinished children run,
// and once any of them completes, the Process is executed again.
// It returns what the last execution of the Process returned.
//
// A scenario needs only a few rounds, so running out of them means
// a replay keeps making new progress, for example by spawning a new child on every execution.
func run(ctx context.Context, rt workflow.Runtime, pid workflow.ProcessID) error {
	const maxRounds = 16
	for range maxRounds {
		err := rt.Execute(ctx, pid)
		if !errors.Is(err, workflow.Suspend{}) {
			return err
		}
		progressed, cerr := runChildren(ctx, rt, pid)
		if cerr != nil {
			return cerr
		}
		if !progressed {
			return err
		}
	}
	return fmt.Errorf("process %s did not settle after %d rounds of execution", pid, maxRounds)
}

// runChildren runs the children of the Process which have not completed yet,
// and reports whether any of them completed meanwhile.
func runChildren(ctx context.Context, rt workflow.Runtime, pid workflow.ProcessID) (bool, error) {
	children, err := childrenOf(ctx, rt.Events, pid)
	if err != nil {
		return false, err
	}
	var progressed bool
	for _, child := range children {
		if done, err := workflow.IsCompleted(ctx, rt.Events, child.ChildID); err != nil || done {
			if err != nil {
				return false, err
			}
			continue
		}
		if err := run(ctx, rt, child.ChildID); err != nil && !errors.Is(err, workflow.Suspend{}) {
			return false, err
		}
		done, err := workflow.IsCompleted(ctx, rt.Events, child.ChildID)
		if err != nil {
			return false, err
		}
		progressed = progressed || done
	}
	return progressed, nil
}

// childrenOf lists the spawns of the Process, in the order they were made.
func childrenOf(ctx context.Context, events workflow.EventRepository, pid workflow.ProcessID) ([]workflow.EventSpawn, error) {
	var spawns []workflow.EventSpawn
	for event, err := range events.FindByProcessID(ctx, pid) {
		if err != nil {
			return nil, err
		}
		if spawn, ok := event.(workflow.EventSpawn); ok {
			spawns = append(spawns, spawn)
		}
	}
	return spawns, nil
}

// historyOf lists the events of the Process, followed by the ones of the children it spawned, recursively.
func historyOf(ctx context.Context, events workflow.EventRepository, pid workflow.ProcessID) ([]workflow.Event, error) {
	var history []workflow.Event
	for event, err := range events.FindByProcessID(ctx, pid) {
		if err != nil {
			return nil, err
		}
		history = append(history, event)
	}
	children, err := childrenOf(ctx, events, pid)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		childHistory, err := historyOf(ctx, events, child.ChildID)
		if err != nil {
			return nil, err
		}
		history = append(history, childHistory...)
	}
	return history, nil
}

// fixture is what an earlier version stored for a scenario.
type fixture struct {
	// Definition is the scenario's Definition, as the recording version encoded it.
	Definition json.RawMessage `json:"definition"`
	// ProcessID identifies the recorded Process.
	ProcessID workflow.ProcessID `json:"process_id"`
	// History is the event history of the Process, followed by the ones of the children it spawned,
	// as the recording version encoded them, up to where they suspended before the upgrade.
	History []json.RawMessage `json:"history"`
}

const fixtureDir = "testdata"

func fixturePath(name string) string {
	return filepath.Join(fixtureDir, name+".json")
}

func loadFixture(t *testcase.T, sc scenario) fixture {
	t.Helper()
	data, err := os.ReadFile(fixturePath(sc.Name))
	if errors.Is(err, fs.ErrNotExist) {
		if !*record {
			t.Fatalf("scenario %q has no fixture yet, record it with:\n\n\tgo test ./pkg/workflow/internal/promise/ -record\n\nthen commit %s",
				sc.Name, fixturePath(sc.Name))
		}
		recordFixture(t, sc)
		data, err = os.ReadFile(fixturePath(sc.Name))
	}
	assert.Must(t).NoError(err)
	var f fixture
	assert.Must(t).NoError(json.Unmarshal(data, &f))
	return f
}

// recordFixture runs the scenario on the current version until it suspends, or fails, before the upgrade,
// and writes down what got stored meanwhile.
func recordFixture(t *testcase.T, sc scenario) {
	t.Helper()
	var (
		ctx    = t.Context()
		codec  = wfjson.NewCodec()
		events = &memory.WorkflowEventRepository{}
		rt     = newRuntime(events, &vocabulary{})
	)
	pid, err := workflow.MakeProcessID()
	assert.Must(t).NoError(err)
	assert.Must(t).NoError(rt.Bind(ctx, pid, sc.Definition))
	switch err := run(ctx, rt, pid); {
	case sc.FailsBeforeUpgrade:
		if err == nil || errors.Is(err, workflow.Suspend{}) {
			t.Fatalf("scenario %q has to fail before the upgrade, but its execution returned: %v", sc.Name, err)
		}
	case !errors.Is(err, workflow.Suspend{}):
		t.Fatalf("scenario %q has to suspend before the upgrade, mostly on %s, but its execution returned: %v",
			sc.Name, awaitUpgrade, err)
	}
	var f = fixture{ProcessID: pid}
	// Like the stores, which read them back into a workflow.Definition and a workflow.Event,
	// they are written as these interfaces hold them, with their $type.
	f.Definition, err = codec.Marshal(&sc.Definition)
	assert.Must(t).NoError(err)
	history, err := historyOf(ctx, events, pid)
	assert.Must(t).NoError(err)
	for _, event := range history {
		data, err := codec.Marshal(&event)
		assert.Must(t).NoError(err)
		f.History = append(f.History, data)
	}
	data, err := json.MarshalIndent(f, "", "\t")
	assert.Must(t).NoError(err)
	assert.Must(t).NoError(os.MkdirAll(fixtureDir, 0o755))
	// O_EXCL keeps a committed fixture from ever being overwritten.
	file, err := os.OpenFile(fixturePath(sc.Name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	assert.Must(t).NoError(err)
	_, err = file.Write(append(data, '\n'))
	assert.Must(t).NoError(errors.Join(err, file.Close()))
	t.Logf("recorded %s", fixturePath(sc.Name))
}

func TestCompatibilityPromise(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) workflow.Codec {
		return wfjson.NewCodec()
	})

	for _, sc := range scenarios {
		s.Context(sc.Name, func(s *testcase.Spec) {
			stored := let.Var(s, func(t *testcase.T) fixture {
				return loadFixture(t, sc)
			})

			s.Describe("stored definition", func(s *testcase.Spec) {
				act := func(t *testcase.T) (workflow.Definition, error) {
					var def workflow.Definition
					err := codec.Get(t).Unmarshal(stored.Get(t).Definition, &def)
					return def, err
				}

				// It is the same Go value, so a value in it, like the one of a SetVar, has kept its type too.
				s.Then("it still decodes to the scenario's definition", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, sc.Definition, got)
				})
			})

			s.Describe("stored in-flight history", func(s *testcase.Spec) {
				var (
					vocab = let.Var(s, func(t *testcase.T) *vocabulary {
						return &vocabulary{upgraded: true}
					})
					events = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
						var events = &memory.WorkflowEventRepository{}
						for _, data := range stored.Get(t).History {
							var event workflow.Event
							assert.Must(t).NoError(codec.Get(t).Unmarshal(data, &event))
							assert.Must(t).NoError(events.Create(t.Context(), &event))
						}
						return events
					})
				)
				act := func(t *testcase.T) error {
					return run(t.Context(), newRuntime(events.Get(t), vocab.Get(t)), stored.Get(t).ProcessID)
				}

				s.Before(func(t *testcase.T) {
					stored.Get(t)
					// The upgrade takes a while, so the Process resumes in a later second than its history was recorded in,
					// even when it was recorded just now, which a Sleep asks its condition under.
					timecop.Travel(t, time.Second)
				})

				s.Then("the Process resumes on the current version and completes", func(t *testcase.T) {
					assert.NoError(t, act(t))

					completed, err := workflow.IsCompleted(t.Context(), events.Get(t), stored.Get(t).ProcessID)
					assert.NoError(t, err)
					assert.True(t, completed)
				})

				s.Then("the work recorded before the upgrade is replayed rather than repeated", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, sc.ResumedCalls, vocab.Get(t).calls)
				})

				s.Then("the Process ends with the expected variables", func(t *testcase.T) {
					assert.NoError(t, act(t))

					vars, err := workflow.Vars{
						ProcessID:        stored.Get(t).ProcessID,
						EventsRepository: events.Get(t),
					}.ToMap(t.Context())
					assert.NoError(t, err)

					var expected = map[workflow.VarName]any{}
					maps.Copy(expected, sc.Vars)
					assert.Equal(t, expected, vars)
				})

				s.Then("the spawned children end with the expected variables", func(t *testcase.T) {
					assert.NoError(t, act(t))

					children, err := childrenOf(t.Context(), events.Get(t), stored.Get(t).ProcessID)
					assert.NoError(t, err)
					var got = map[workflow.SpawnName][]map[workflow.VarName]any{}
					for _, child := range children {
						vars, err := workflow.Vars{
							ProcessID:        child.ChildID,
							EventsRepository: events.Get(t),
						}.ToMap(t.Context())
						assert.NoError(t, err)
						got[child.Name] = append(got[child.Name], vars)
					}

					var expected = map[workflow.SpawnName][]map[workflow.VarName]any{}
					for name, children := range sc.ChildVars {
						for _, vars := range children {
							var childVars = map[workflow.VarName]any{}
							maps.Copy(childVars, vars)
							expected[name] = append(expected[name], childVars)
						}
					}
					assert.Equal(t, expected, got)
				})
			})
		})
	}

	s.Describe("fixtures", func(s *testcase.Spec) {
		act := func(t *testcase.T) []string {
			paths, err := filepath.Glob(filepath.Join(fixtureDir, "*.json"))
			assert.Must(t).NoError(err)
			var names []string
			for _, path := range paths {
				names = append(names, strings.TrimSuffix(filepath.Base(path), ".json"))
			}
			return names
		}

		s.Then("every fixture belongs to a scenario, as a promise can't be withdrawn", func(t *testcase.T) {
			var known = map[string]struct{}{}
			for _, sc := range scenarios {
				known[sc.Name] = struct{}{}
			}
			for _, name := range act(t) {
				_, ok := known[name]
				assert.True(t, ok, assert.MessageF("%s has no scenario; restore the scenario named %q",
					fixturePath(name), name))
			}
		})
	})
}
