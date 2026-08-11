package wfjson_test

import (
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/testcase/assert"
)

func TestWfjsonCodecContract(t *testing.T) {
	wfcontract.Codec(wfjson.NewCodec()).Test(t)
}

func TestWfjsonCodec_ProcessExecutionRoundTrip(t *testing.T) {
	c := wfjson.NewCodec()
	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	exp := workflow.ProcessExecution{
		ProcessID:    pid,
		StartTime:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		CreatedAt:    time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
		FailureCount: 2,
	}

	data, err := c.Marshal(exp)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var got workflow.ProcessExecution
	assert.NoError(t, c.Unmarshal(data, &got))
	assert.Equal[workflow.ProcessExecution](t, exp, got)
}

func TestWfjsonCodec_ProcessChangeEventDispatch(t *testing.T) {
	// ProcessChangeEvent is an interface with multiple concrete
	// implementations. The codec must dispatch on the @type envelope so
	// each concrete type round-trips back to its own Go type (rather than
	// to a single shared envelope struct).
	c := wfjson.NewCodec()
	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	cases := []struct {
		name string
		ev   workflow.ProcessChangeEvent
	}{
		{"start", workflow.ProcessStart{ProcessID: pid}},
		{"stop", workflow.ProcessStop{ProcessID: pid}},
		{"sleep", workflow.ProcessSleep{ProcessID: pid}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := c.Marshal(tc.ev)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var got workflow.ProcessChangeEvent
			assert.NoError(t, c.Unmarshal(data, &got))
			assert.NotNil(t, got)

			if got.GetProcessID() != tc.ev.GetProcessID() {
				t.Fatalf("ProcessID mismatch: got %s, want %s", got.GetProcessID(), tc.ev.GetProcessID())
			}
			if got.ChangeType() != tc.ev.ChangeType() {
				t.Fatalf("ChangeType mismatch: got %s, want %s", got.ChangeType(), tc.ev.ChangeType())
			}
		})
	}
}
