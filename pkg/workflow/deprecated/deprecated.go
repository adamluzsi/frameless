// Package deprecated keeps the workflow definitions that were replaced,
// so definitions recorded with their wire tags can still be decoded and executed.
//
// Don't use them in new definitions.
package deprecated

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"
)

// ExecuteParticipant executes the participant registered under ID.
//
// It records and replays its calls the same way as workflow.Execute with ID as its ParticipantID,
// so a call recorded by one of them is replayed by the other.
//
// Deprecated: use workflow.Execute with ParticipantID instead.
type ExecuteParticipant struct {
	ID     workflow.ParticipantID
	Input  []workflow.VarName
	Output []workflow.VarName
}

var _ workflow.Definition = ExecuteParticipant{}

func (d ExecuteParticipant) Name() string  { return "workflow::participant" }
func (d ExecuteParticipant) Error() string { return d.Name() }

func (d ExecuteParticipant) Execute(ctx context.Context, pid workflow.ProcessID) error {
	return d.execute().Execute(ctx, pid)
}

// Deprecated: use workflow.Execute#ExecuteWith with ID as its participant ID argument instead.
func (d ExecuteParticipant) ExecuteWith(ctx context.Context, pid workflow.ProcessID, executeFn func(ctx context.Context, processID workflow.ProcessID) error) error {
	return workflow.Execute{Input: d.Input, Output: d.Output}.ExecuteWith(ctx, pid, d.ID, executeFn)
}

func (d ExecuteParticipant) execute() workflow.Execute {
	return workflow.Execute{ParticipantID: d.ID, Input: d.Input, Output: d.Output}
}

// ExecuteCondition asks the condition registered under ID.
//
// It records and replays its answers the same way as workflow.Execute with ID as its ConditionID,
// so an answer recorded by one of them is replayed by the other.
//
// Deprecated: use workflow.Execute with ConditionID instead.
type ExecuteCondition struct {
	ID    workflow.ConditionID
	Input []workflow.VarName
}

var (
	_ workflow.Condition  = ExecuteCondition{}
	_ workflow.Definition = ExecuteCondition{}
)

func (d ExecuteCondition) Name() string  { return "workflow::condition" }
func (d ExecuteCondition) Error() string { return d.Name() }

func (d ExecuteCondition) Evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) {
	return d.execute().Evaluate(ctx, pid)
}

// Deprecated: use workflow.Execute#EvaluateWith with ID as its condition ID argument instead.
func (d ExecuteCondition) EvaluateWith(ctx context.Context, pid workflow.ProcessID, evaluateFn func(ctx context.Context, processID workflow.ProcessID) (bool, error)) (bool, error) {
	return workflow.Execute{Input: d.Input}.EvaluateWith(ctx, pid, d.ID, evaluateFn)
}

// Execute asks the condition as a step, under a workflow::condition path segment, and discards its answer.
//
// workflow.Execute doesn't support a condition as a step,
// since an answer that is neither stored nor branched on has no effect on the process.
func (d ExecuteCondition) Execute(ctx context.Context, pid workflow.ProcessID) error {
	_, err := d.execute().Evaluate(workflow.WithName(ctx, d.Name()), pid)
	return err
}

func (d ExecuteCondition) execute() workflow.Execute {
	return workflow.Execute{ConditionID: d.ID, Input: d.Input}
}
