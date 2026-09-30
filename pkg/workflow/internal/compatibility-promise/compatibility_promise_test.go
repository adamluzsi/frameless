package compatibilitypromise_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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
// Its Definition runs until compat::await-upgrade suspends it, which stands for the upgrade:
// everything before it was recorded by the version that wrote the fixture,
// and everything after it runs on the current version.
type scenario struct {
	// Name identifies the scenario and names its fixture, so it never changes.
	Name string
	// Definition is the scenario's workflow, written with the current API.
	Definition workflow.Definition
	// ResumedCalls lists the participants and conditions called after the upgrade, in order.
	// Whatever was recorded before the upgrade is replayed from the history instead.
	ResumedCalls []string
	// Vars are the process variables once the resumed Process has completed.
	Vars map[workflow.VarName]any
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
		Name: "defer-registered-before-the-upgrade",
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
			// A float, because that is how a stored number reads back, and a template only compares numbers of the same kind.
			workflow.SetVar{Name: "i", Value: 0.0},
			workflow.For{
				Cond: wftemplate.Condition(`lt .i 2.0`),
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
	// awaitUpgrade suspends the Process until the upgrade.
	awaitUpgrade workflow.ParticipantID = "compat::await-upgrade"
	// beforeUpgrade answers whether the Process still runs on the version that recorded it,
	// so an answer recorded before the upgrade differs from a fresh one.
	beforeUpgrade workflow.ConditionID = "compat::before-upgrade"
)

// vocabulary implements the scenarios' participants and conditions.
type vocabulary struct {
	// upgraded tells whether the Process runs on the current version.
	upgraded bool
	// calls lists the participants and conditions called so far, in order.
	// compat::await-upgrade only stands for the upgrade, so it is not listed.
	calls []string
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
		awaitUpgrade: func(context.Context) error {
			if !v.upgraded {
				return workflow.Suspend{}
			}
			return nil
		},
	}
}

func (v *vocabulary) conditions() workflow.Conditions {
	return workflow.Conditions{
		beforeUpgrade: func(context.Context) (bool, error) {
			v.calls = append(v.calls, string(beforeUpgrade))
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

// fixture is what an earlier version stored for a scenario.
type fixture struct {
	// Definition is the scenario's Definition, as the recording version encoded it.
	Definition json.RawMessage `json:"definition"`
	// ProcessID identifies the recorded Process.
	ProcessID workflow.ProcessID `json:"process_id"`
	// History is the event history of the Process, as the recording version encoded it,
	// up to where it suspended before the upgrade.
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
			t.Fatalf("scenario %q has no fixture yet, record it with:\n\n\tgo test ./pkg/workflow/internal/compatibility-promise/ -record\n\nthen commit %s",
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

// recordFixture runs the scenario on the current version until it suspends before the upgrade,
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
	if err := rt.Execute(ctx, pid); !errors.Is(err, workflow.Suspend{}) {
		t.Fatalf("scenario %q has to suspend on %s before the upgrade, but its execution returned: %v",
			sc.Name, awaitUpgrade, err)
	}
	var f = fixture{ProcessID: pid}
	f.Definition, err = codec.Marshal(sc.Definition)
	assert.Must(t).NoError(err)
	for event, err := range events.FindByProcessID(ctx, pid) {
		assert.Must(t).NoError(err)
		data, err := codec.Marshal(event)
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

// canonical returns the current encoding of def, indented, to compare two definitions by what they persist as.
func canonical(t *testcase.T, codec workflow.Codec, def workflow.Definition) string {
	t.Helper()
	data, err := codec.Marshal(def)
	assert.Must(t).NoError(err)
	pretty, err := json.MarshalIndent(json.RawMessage(data), "", "\t")
	assert.Must(t).NoError(err)
	return string(pretty)
}

// asJSON returns v the way it reads back from JSON,
// so values recorded in a history and values set on the current version compare alike.
func asJSON(t *testcase.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	assert.Must(t).NoError(err)
	var out any
	assert.Must(t).NoError(json.Unmarshal(data, &out))
	return out
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

				s.Then("it still decodes to the scenario's definition", func(t *testcase.T) {
					got, err := act(t)
					assert.NoError(t, err)
					assert.Equal(t, canonical(t, codec.Get(t), sc.Definition), canonical(t, codec.Get(t), got))
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
					return newRuntime(events.Get(t), vocab.Get(t)).Execute(t.Context(), stored.Get(t).ProcessID)
				}

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
					assert.Equal(t, asJSON(t, expected), asJSON(t, vars))
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
